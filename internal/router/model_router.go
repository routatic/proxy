// Package router defines HTTP route registration and middleware chaining,
// as well as model selection based on request scenarios.
package router

import (
	"fmt"

	"oc-go-cc/internal/config"
)

// ModelRouter handles model selection based on scenarios.
type ModelRouter struct {
	atomic *config.AtomicConfig
}

// NewModelRouter creates a new model router.
func NewModelRouter(atomic *config.AtomicConfig) *ModelRouter {
	return &ModelRouter{atomic: atomic}
}

// RouteResult contains the selected model and fallback chain.
type RouteResult struct {
	Primary   config.ModelConfig
	Fallbacks []config.ModelConfig
	Scenario  Scenario
}

// resolveRequestedModel checks if the user-specified model should override
// scenario-based routing. Returns the route result and true if it matched,
// or zero value and false if scenario routing should proceed normally.
func (r *ModelRouter) resolveRequestedModel(cfg *config.Config, requestedModel string, needsVision bool) (RouteResult, bool, error) {
	if !cfg.RespectRequestedModel || requestedModel == "" {
		return RouteResult{}, false, nil
	}

	// Look up the requested model in config to inherit its settings
	primary, ok := cfg.Models[requestedModel]
	if !ok {
		// Unknown model — create a bare config and inherit defaults
		primary = config.ModelConfig{
			Provider: "opencode-go",
			ModelID:  requestedModel,
		}
		if def, ok := cfg.Models["default"]; ok {
			primary.Temperature = def.Temperature
			primary.MaxTokens = def.MaxTokens
		}
	}
	primary = config.ResolveModelConfig(primary)
	if needsVision && !primary.SupportsVision {
		return RouteResult{}, false, fmt.Errorf("requested model %s does not support vision", primary.ModelID)
	}

	fallbacks := normalizeModels(cfg.Fallbacks["default"])

	return RouteResult{
		Primary:   primary,
		Fallbacks: fallbacks,
		Scenario:  ScenarioDefault,
	}, true, nil
}

// Route determines which model to use for a request.
// If respect_requested_model is enabled and requestedModel is provided, it overrides scenario-based routing.
func (r *ModelRouter) Route(messages []MessageContent, tokenCount int, requestedModel string) (RouteResult, error) {
	cfg := r.atomic.Get()
	facts := AnalyzeRequestFacts(messages)

	if result, ok, err := r.resolveRequestedModel(cfg, requestedModel, facts.NeedsVision); err != nil {
		return RouteResult{}, err
	} else if ok {
		return result, nil
	}

	// Otherwise, use scenario-based routing
	result := DetectScenario(messages, tokenCount, cfg)

	// Get primary model for scenario
	primary, ok := cfg.Models[string(result.Scenario)]
	if !ok {
		if isVisionScenario(result.Scenario) {
			return RouteResult{}, fmt.Errorf("vision scenario %s is not configured", result.Scenario)
		}
		// Fall back to default if scenario model not configured
		primary, ok = cfg.Models["default"]
		if !ok {
			return RouteResult{}, fmt.Errorf("no default model configured")
		}
	}
	primary = config.ResolveModelConfig(primary)
	if isVisionScenario(result.Scenario) && !primary.SupportsVision {
		return RouteResult{}, fmt.Errorf("vision scenario %s primary model %s does not support vision", result.Scenario, primary.ModelID)
	}

	// Get fallbacks for scenario
	fallbacks := normalizeModels(cfg.Fallbacks[string(result.Scenario)])
	if len(fallbacks) == 0 {
		if isVisionScenario(result.Scenario) {
			return RouteResult{}, fmt.Errorf("vision scenario %s has no configured vision fallbacks", result.Scenario)
		}
		// Fall back to default fallbacks
		fallbacks = normalizeModels(cfg.Fallbacks["default"])
	}
	if isVisionScenario(result.Scenario) {
		for _, fallback := range fallbacks {
			if !fallback.SupportsVision {
				return RouteResult{}, fmt.Errorf("vision scenario %s fallback model %s does not support vision", result.Scenario, fallback.ModelID)
			}
		}
	}

	return RouteResult{
		Primary:   primary,
		Fallbacks: fallbacks,
		Scenario:  result.Scenario,
	}, nil
}

// IsStreamingScenarioRoutingEnabled returns whether streaming requests should use
// scenario-based routing instead of always routing to the fast model.
func (r *ModelRouter) IsStreamingScenarioRoutingEnabled() bool {
	return r.atomic.Get().EnableStreamingScenarioRouting
}

// GetModelChain returns the full chain of models to try (primary + fallbacks).
func (rr *RouteResult) GetModelChain() []config.ModelConfig {
	chain := []config.ModelConfig{rr.Primary}
	chain = append(chain, rr.Fallbacks...)
	return chain
}

// RouteForStreaming determines which model to use for streaming requests.
// Prioritizes fast TTFT (time-to-first-token) over capability.
// If respect_requested_model is enabled and requestedModel is provided, it overrides scenario-based routing.
func (r *ModelRouter) RouteForStreaming(messages []MessageContent, tokenCount int, requestedModel string) RouteResult {
	cfg := r.atomic.Get()
	facts := AnalyzeRequestFacts(messages)

	if result, ok, err := r.resolveRequestedModel(cfg, requestedModel, facts.NeedsVision); err == nil && ok {
		return result
	}

	// Otherwise, use scenario-based routing for streaming
	result := RouteForStreaming(messages, tokenCount, cfg)

	// Get primary model for scenario
	primary, ok := cfg.Models[string(result.Scenario)]
	if !ok {
		if isVisionScenario(result.Scenario) {
			return RouteResult{Scenario: result.Scenario}
		}
		// Fall back to fast scenario if not configured
		primary, ok = cfg.Models["fast"]
		if !ok {
			// Fall back to default
			primary = cfg.Models["default"]
		}
	}
	primary = config.ResolveModelConfig(primary)

	// Get fallbacks for scenario
	fallbacks := normalizeModels(cfg.Fallbacks[string(result.Scenario)])
	if len(fallbacks) == 0 {
		if isVisionScenario(result.Scenario) {
			fallbacks = nil
		} else {
			// Fall back to fast fallbacks
			fallbacks = normalizeModels(cfg.Fallbacks["fast"])
		}
	}

	return RouteResult{
		Primary:   primary,
		Fallbacks: fallbacks,
		Scenario:  result.Scenario,
	}
}

func isVisionScenario(s Scenario) bool {
	return s == ScenarioVision || s == ScenarioVisionComplex || s == ScenarioVisionLongContext
}

func normalizeModels(models []config.ModelConfig) []config.ModelConfig {
	if len(models) == 0 {
		return nil
	}
	out := make([]config.ModelConfig, 0, len(models))
	for _, model := range models {
		out = append(out, config.ResolveModelConfig(model))
	}
	return out
}
