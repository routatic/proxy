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

// TestFilterByCapacityHonorsSmallMaxTokens guards the auto-mode classifier
// regression: the harness's safety classifier sends a tiny non-streaming
// request (max_tokens=64) to render a yes/no verdict. The capacity filter
// must keep the model eligible — the model has ample context room, it just
// needs to produce few output tokens — rather than rejecting it with "no
// eligible model for request capacity", which the harness reports as
// "model temporarily unavailable, auto mode cannot determine safety".
func TestFilterByCapacityHonorsSmallMaxTokens(t *testing.T) {
	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "glm-5.2", MaxTokens: 8192},
	}

	decision, err := FilterByCapacity(chain, 500, 64, false, false)
	if err != nil {
		t.Fatalf("FilterByCapacity() error = %v, want nil (small max_tokens must not skip)", err)
	}
	if len(decision.Models) != 1 {
		t.Fatalf("eligible models = %+v, want exactly 1", decision.Models)
	}
	if got, want := decision.Models[0].MaxTokens, 64; got != want {
		t.Fatalf("max_tokens = %d, want %d (client's small request honored)", got, want)
	}
	if len(decision.Skipped) != 0 {
		t.Fatalf("skipped = %+v, want none for small max_tokens with room to spare", decision.Skipped)
	}
}
