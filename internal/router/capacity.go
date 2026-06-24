package router

import (
	"fmt"

	"github.com/routatic/proxy/internal/config"
)

const minimumOutputTokens = 256

type SkippedModel struct {
	ModelID string `json:"model_id"`
	Reason  string `json:"reason"`
}

type CapacityDecision struct {
	Models             []config.ModelConfig
	Skipped            []SkippedModel
	InputTokens        int
	RequestedMaxTokens int
	SelectedMaxTokens  int
	ContextWindow      int
	ContextMargin      int
	NeedsVision        bool
	NeedsTools         bool
}

func FilterByCapacity(chain []config.ModelConfig, inputTokens int, requestedMaxTokens int, needsVision bool, needsTools bool) (CapacityDecision, error) {
	decision := CapacityDecision{
		InputTokens:        inputTokens,
		RequestedMaxTokens: requestedMaxTokens,
		NeedsVision:        needsVision,
		NeedsTools:         needsTools,
	}

	for _, raw := range chain {
		model := config.ResolveModelConfig(raw)
		if needsVision && !model.Vision {
			decision.Skipped = append(decision.Skipped, SkippedModel{ModelID: model.ModelID, Reason: "vision_not_supported"})
			continue
		}
		if needsTools && !config.SupportsTools(model) {
			decision.Skipped = append(decision.Skipped, SkippedModel{ModelID: model.ModelID, Reason: "tools_not_supported"})
			continue
		}

		// A model is capacity-ineligible only when its own context window is
		// exhausted below the output floor. A client that requests a small
		// max_tokens (e.g. the safety classifier asks for 64 tokens to render
		// a yes/no verdict) must NOT trigger a skip — the model still has
		// room, it just needs to produce fewer tokens. Skipping on the
		// clamped value here caused every sub-256-token request to fail with
		// "no eligible model for request capacity", which the harness surfaces
		// as "model temporarily unavailable, auto mode cannot determine
		// safety".
		if model.ContextWindow > 0 {
			remaining := model.ContextWindow - inputTokens - model.ContextMargin
			if remaining < minimumOutputTokens {
				decision.Skipped = append(decision.Skipped, SkippedModel{ModelID: model.ModelID, Reason: "context_window_exceeded"})
				continue
			}
		}

		sentMax := clampOutputTokens(model, inputTokens, requestedMaxTokens)
		model.MaxTokens = sentMax
		if len(decision.Models) == 0 {
			decision.SelectedMaxTokens = sentMax
			decision.ContextWindow = model.ContextWindow
			decision.ContextMargin = model.ContextMargin
		}
		decision.Models = append(decision.Models, model)
	}

	if len(decision.Models) == 0 {
		return decision, fmt.Errorf("no eligible model for request capacity")
	}
	return decision, nil
}

func clampOutputTokens(model config.ModelConfig, inputTokens int, requestedMaxTokens int) int {
	if inputTokens < 0 {
		inputTokens = 0
	}
	limit := model.MaxTokens
	if requestedMaxTokens > 0 && (limit == 0 || requestedMaxTokens < limit) {
		limit = requestedMaxTokens
	}
	if model.MaxOutputTokens > 0 && (limit == 0 || model.MaxOutputTokens < limit) {
		limit = model.MaxOutputTokens
	}
	if model.ContextWindow <= 0 {
		return limit
	}
	remaining := model.ContextWindow - inputTokens - model.ContextMargin
	if limit == 0 || remaining < limit {
		if remaining < 0 {
			return 0
		}
		limit = remaining
	}
	return limit
}
