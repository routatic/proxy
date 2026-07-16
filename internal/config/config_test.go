package config

import (
	"encoding/json"
	"testing"
)

func TestCostBasedRoutingEnabled_DefaultFalse(t *testing.T) {
	cfg := &Config{}
	if cfg.CostBasedRoutingEnabled() {
		t.Error("CostBasedRoutingEnabled() = true, want false for zero value Config")
	}
}

func TestCostBasedRoutingEnabled_TopLevelTrue(t *testing.T) {
	cfg := &Config{EnableCostBasedRouting: true}
	if !cfg.CostBasedRoutingEnabled() {
		t.Error("CostBasedRoutingEnabled() = false, want true when EnableCostBasedRouting is true")
	}
}

func TestCostBasedRoutingEnabled_NestedTrue(t *testing.T) {
	cfg := &Config{
		CostRouting: &CostRoutingConfig{Enabled: true},
	}
	if !cfg.CostBasedRoutingEnabled() {
		t.Error("CostBasedRoutingEnabled() = false, want true when CostRouting.Enabled is true")
	}
}

func TestCostBasedRoutingEnabled_BothTrue(t *testing.T) {
	cfg := &Config{
		EnableCostBasedRouting: true,
		CostRouting:            &CostRoutingConfig{Enabled: true},
	}
	if !cfg.CostBasedRoutingEnabled() {
		t.Error("CostBasedRoutingEnabled() = false, want true when both flags are true")
	}
}

func TestCostBasedRoutingEnabled_NestedFalseTopLevelTrue(t *testing.T) {
	cfg := &Config{
		EnableCostBasedRouting: true,
		CostRouting:            &CostRoutingConfig{Enabled: false},
	}
	if !cfg.CostBasedRoutingEnabled() {
		t.Error("CostBasedRoutingEnabled() = false, want true when top-level flag is true even if nested is false")
	}
}

func TestCostRoutingConfig_Parsing(t *testing.T) {
	raw := `{
		"enabled": true,
		"prefer_providers": ["openrouter", "aws_bedrock"],
		"max_context_window": 128000,
		"penalty_per_provider": {
			"opencode-go": 0.1,
			"openrouter": 0.05
		}
	}`

	var crc CostRoutingConfig
	if err := json.Unmarshal([]byte(raw), &crc); err != nil {
		t.Fatalf("failed to unmarshal CostRoutingConfig: %v", err)
	}

	if !crc.Enabled {
		t.Error("Enabled = false, want true")
	}
	wantProviders := []string{"openrouter", "aws_bedrock"}
	if len(crc.PreferProviders) != len(wantProviders) {
		t.Fatalf("PreferProviders = %v, want %v", crc.PreferProviders, wantProviders)
	}
	for i, p := range wantProviders {
		if crc.PreferProviders[i] != p {
			t.Errorf("PreferProviders[%d] = %q, want %q", i, crc.PreferProviders[i], p)
		}
	}
	if crc.MaxContextWindow != 128000 {
		t.Errorf("MaxContextWindow = %d, want 128000", crc.MaxContextWindow)
	}
	if len(crc.PenaltyPerProvider) != 2 {
		t.Fatalf("PenaltyPerProvider = %v, want 2 entries", crc.PenaltyPerProvider)
	}
	if crc.PenaltyPerProvider["opencode-go"] != 0.1 {
		t.Errorf("PenaltyPerProvider[opencode-go] = %v, want 0.1", crc.PenaltyPerProvider["opencode-go"])
	}
	if crc.PenaltyPerProvider["openrouter"] != 0.05 {
		t.Errorf("PenaltyPerProvider[openrouter] = %v, want 0.05", crc.PenaltyPerProvider["openrouter"])
	}
}

func TestConfig_CostRoutingField_Parsing(t *testing.T) {
	raw := `{
		"api_key": "test-key",
		"enable_cost_based_routing": false,
		"cost_routing": {
			"enabled": true,
			"prefer_providers": ["openrouter"]
		}
	}`

	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("failed to unmarshal Config: %v", err)
	}

	if cfg.CostRouting == nil {
		t.Fatal("CostRouting = nil, want non-nil")
	}
	if !cfg.CostRouting.Enabled {
		t.Error("CostRouting.Enabled = false, want true")
	}
	if len(cfg.CostRouting.PreferProviders) != 1 || cfg.CostRouting.PreferProviders[0] != "openrouter" {
		t.Errorf("CostRouting.PreferProviders = %v, want [openrouter]", cfg.CostRouting.PreferProviders)
	}
	if !cfg.CostBasedRoutingEnabled() {
		t.Error("CostBasedRoutingEnabled() = false, want true")
	}
}

func TestConfig_CostRoutingOmitted(t *testing.T) {
	raw := `{"api_key": "test-key", "enable_cost_based_routing": true}`

	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("failed to unmarshal Config: %v", err)
	}

	if cfg.CostRouting != nil {
		t.Errorf("CostRouting = %v, want nil when omitted from JSON", cfg.CostRouting)
	}
	if !cfg.CostBasedRoutingEnabled() {
		t.Error("CostBasedRoutingEnabled() = false, want true via legacy flag")
	}
}

func TestConfig_CostRoutingDisabled(t *testing.T) {
	raw := `{"api_key": "test-key", "cost_routing": {"enabled": false}}`

	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("failed to unmarshal Config: %v", err)
	}

	if cfg.CostBasedRoutingEnabled() {
		t.Error("CostBasedRoutingEnabled() = true, want false when cost_routing.enabled is false")
	}
}
