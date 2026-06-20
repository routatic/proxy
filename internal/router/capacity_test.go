package router

import (
	"testing"

	"github.com/routatic/proxy/internal/config"
)

func TestFilterByCapacitySkipsPrimaryAndUsesEligibleFallback(t *testing.T) {
	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "glm-5.1", MaxTokens: 8192},
		{Provider: "opencode-go", ModelID: "deepseek-v4-pro", MaxTokens: 8192},
	}

	decision, err := FilterByCapacity(chain, 250000, 8192, false, false)
	if err != nil {
		t.Fatalf("FilterByCapacity() error = %v", err)
	}
	if got, want := decision.Models[0].ModelID, "deepseek-v4-pro"; got != want {
		t.Fatalf("selected model = %s, want %s", got, want)
	}
	if len(decision.Skipped) != 1 || decision.Skipped[0].Reason != "context_window_exceeded" {
		t.Fatalf("skipped = %+v, want context skip", decision.Skipped)
	}
}

func TestFilterByCapacityRejectsVisionFallbackToTextModel(t *testing.T) {
	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "deepseek-v4-pro", MaxTokens: 8192},
	}

	decision, err := FilterByCapacity(chain, 1000, 8192, true, false)
	if err == nil {
		t.Fatal("FilterByCapacity() error = nil, want error")
	}
	if len(decision.Models) != 0 {
		t.Fatalf("eligible models = %+v, want none", decision.Models)
	}
	if len(decision.Skipped) != 1 || decision.Skipped[0].Reason != "vision_not_supported" {
		t.Fatalf("skipped = %+v, want vision skip", decision.Skipped)
	}
}

func TestFilterByCapacityClampsMaxTokens(t *testing.T) {
	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "kimi-k2.6", MaxTokens: 16384},
	}

	decision, err := FilterByCapacity(chain, 240000, 16384, true, false)
	if err != nil {
		t.Fatalf("FilterByCapacity() error = %v", err)
	}
	if got, want := decision.Models[0].MaxTokens, 256000-240000-config.DefaultContextMargin; got != want {
		t.Fatalf("max_tokens = %d, want %d", got, want)
	}
}
