package transformer

import (
	"testing"

	"oc-go-cc/internal/config"
)

func TestMatchModelCapability(t *testing.T) {
	tests := []struct {
		modelID     string
		wantMatch   bool
		wantDesc    string
		wantMulti   bool
		wantContext string
		wantReason  bool
		wantTools   bool
	}{
		// DeepSeek family
		{"deepseek-v4-pro", true, "DeepSeek V4 Pro", false, "1M tokens", true, true},
		{"deepseek-v4-flash", true, "DeepSeek V4 Flash", false, "1M tokens", true, true},
		{"deepseek-v4", true, "DeepSeek V4", false, "1M tokens", true, true},
		{"deepseek-r1", true, "DeepSeek R1", false, "128K tokens", true, true},
		{"deepseek-v3", true, "DeepSeek V3", false, "128K tokens", false, true},
		// GLM family
		{"glm-5.1", true, "GLM-5.1", false, "200K tokens", true, true},
		{"glm-5", true, "GLM-5", false, "200K tokens", true, true},
		{"glm-4", true, "GLM-4", false, "128K tokens", false, true},
		// Kimi family
		{"kimi-k2.6", true, "Kimi K2.6", false, "256K tokens", true, true},
		{"kimi-k2.5", true, "Kimi K2.5", false, "256K tokens", true, true},
		{"kimi-k2", true, "Kimi K2", false, "256K tokens", true, true},
		// Qwen family
		{"qwen3.6-plus", true, "Qwen3.6 Plus", false, "1M tokens", false, true},
		{"qwen3.5-plus", true, "Qwen3.5 Plus", false, "1M tokens", false, true},
		// MiniMax family (multimodal)
		{"minimax-m2.7", true, "MiniMax M2.7", true, "1M tokens", false, true},
		{"minimax-m2.5", true, "MiniMax M2.5", true, "1M tokens", false, true},
		{"minimax-m1", true, "MiniMax M1", true, "1M tokens", false, true},
		// MiMo family
		{"mimo-v2-pro", true, "MiMo V2 Pro", false, "128K tokens", false, true},
		// Claude family (via Zen)
		{"claude-sonnet-4.5", true, "Claude Sonnet 4", true, "1M tokens", true, true},
		{"claude-opus-4", true, "Claude Opus 4", true, "1M tokens", true, true},
		{"claude-haiku-4.5", true, "Claude Haiku 4", false, "200K tokens", false, true},
		// Unknown model
		{"unknown-model", false, "", false, "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			cap, ok := matchModelCapability(tt.modelID)
			if ok != tt.wantMatch {
				t.Errorf("matchModelCapability(%q) ok = %v, want %v", tt.modelID, ok, tt.wantMatch)
				return
			}
			if !tt.wantMatch {
				return
			}
			if cap.Description != tt.wantDesc {
				t.Errorf("Description = %q, want %q", cap.Description, tt.wantDesc)
			}
			if cap.Multimodal != tt.wantMulti {
				t.Errorf("Multimodal = %v, want %v", cap.Multimodal, tt.wantMulti)
			}
			if cap.ContextWindow != tt.wantContext {
				t.Errorf("ContextWindow = %q, want %q", cap.ContextWindow, tt.wantContext)
			}
			if cap.Reasoning != tt.wantReason {
				t.Errorf("Reasoning = %v, want %v", cap.Reasoning, tt.wantReason)
			}
			if cap.SupportsTools != tt.wantTools {
				t.Errorf("SupportsTools = %v, want %v", cap.SupportsTools, tt.wantTools)
			}
		})
	}
}

func TestMatchModelCapabilityPrefixPriority(t *testing.T) {
	// More specific prefixes should match before less specific ones.
	// "deepseek-v4-pro" should match "deepseek-v4-pro" not "deepseek-v4"
	cap, ok := matchModelCapability("deepseek-v4-pro")
	if !ok {
		t.Fatal("expected match for deepseek-v4-pro")
	}
	if cap.Description != "DeepSeek V4 Pro" {
		t.Errorf("got %q, want DeepSeek V4 Pro", cap.Description)
	}

	// "deepseek-v4" (bare) should match the generic prefix
	cap, ok = matchModelCapability("deepseek-v4")
	if !ok {
		t.Fatal("expected match for deepseek-v4")
	}
	if cap.Description != "DeepSeek V4" {
		t.Errorf("got %q, want DeepSeek V4", cap.Description)
	}
}

func TestCapabilityInjectionDisabled(t *testing.T) {
	cfg := config.CapabilityInjectionConfig{
		Enabled: false,
	}

	text := cfg.GetCapabilityText("glm-5.1")
	if text != "" {
		t.Errorf("expected empty text when disabled, got %q", text)
	}

	result := cfg.InjectSystemPrompt("original prompt", "glm-5.1")
	if result != "original prompt" {
		t.Errorf("expected unchanged prompt when disabled, got %q", result)
	}
}

func TestCapabilityInjectionEnabled(t *testing.T) {
	cfg := config.CapabilityInjectionConfig{
		Enabled: true,
	}

	text := cfg.GetCapabilityText("glm-5.1")
	if text == "" {
		t.Fatal("expected non-empty capability text for glm-5.1")
	}

	// Verify it contains the key information
	checks := []string{
		"GLM-5.1",
		"glm-5.1",
		"Multimodal",
		"Context window",
		"200K tokens",
		"Reasoning/thinking",
		"Tool/function calling",
		"no", // multimodal = false -> "no"
	}
	for _, check := range checks {
		if !contains(text, check) {
			t.Errorf("capability text missing %q:\n%s", check, text)
		}
	}
}

func TestInjectSystemPrompt(t *testing.T) {
	cfg := config.CapabilityInjectionConfig{
		Enabled: true,
	}

	// With existing system prompt
	result := cfg.InjectSystemPrompt("Be helpful.", "kimi-k2.6")
	if !contains(result, "Be helpful.") {
		t.Error("original system prompt should be preserved")
	}
	if !contains(result, "Kimi K2.6") {
		t.Error("capability info should be prepended")
	}
	// Capability should come before the original prompt
	capIdx := indexOf(result, "Kimi K2.6")
	promptIdx := indexOf(result, "Be helpful.")
	if capIdx >= promptIdx {
		t.Error("capability should appear before the original system prompt")
	}

	// With empty system prompt
	result = cfg.InjectSystemPrompt("", "qwen3.6-plus")
	if result == "" {
		t.Error("should still inject capability even with empty system prompt")
	}
	if contains(result, "---") {
		t.Error("should not add separator when there is no original system prompt")
	}
}

func TestCapabilityInjectionWithPrefixSuffix(t *testing.T) {
	cfg := config.CapabilityInjectionConfig{
		Enabled: true,
		Prefix:  "[MODEL CAPABILITIES]",
		Suffix:  "[END CAPABILITIES]",
	}

	text := cfg.GetCapabilityText("deepseek-v4-pro")
	if !contains(text, "[MODEL CAPABILITIES]") {
		t.Error("should contain prefix")
	}
	if !contains(text, "[END CAPABILITIES]") {
		t.Error("should contain suffix")
	}
}

func TestCapabilityInjectionOverride(t *testing.T) {
	customText := "This model can do everything. Trust it."
	cfg := config.CapabilityInjectionConfig{
		Enabled: true,
		Overrides: map[string]string{
			"my-custom-model": customText,
		},
	}

	// Exact model ID match should use override
	text := cfg.GetCapabilityText("my-custom-model")
	if text != customText {
		t.Errorf("override text = %q, want %q", text, customText)
	}

	// Non-matching model should fall back to prefix matching
	text = cfg.GetCapabilityText("glm-5.1")
	if text == "" {
		t.Error("non-override model should still get capability text via prefix matching")
	}
}

func TestUnknownModelNoInjection(t *testing.T) {
	cfg := config.CapabilityInjectionConfig{
		Enabled: true,
	}

	text := cfg.GetCapabilityText("completely-unknown-model-xyz")
	if text != "" {
		t.Errorf("expected empty text for unknown model, got %q", text)
	}
}

func contains(s, substr string) bool {
	return indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
