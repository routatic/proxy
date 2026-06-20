// Package router defines HTTP route registration and middleware chaining,
// as well as model selection based on request scenarios.
package router

import (
	"fmt"

	"github.com/routatic/proxy/internal/config"
)

// ModelRouter handles model selection based on scenarios.
type ModelRouter struct {
	atomic *config.AtomicConfig
}

// NewModelRouter creates a new model router.
func NewModelRouter(atomic *config.AtomicConfig) *ModelRouter {
	return &ModelRouter{atomic: atomic}
}

// isRespectRequestedModel returns true when the client-specified model should be
// used as the primary routing target.  nil (unset in config) defaults to true;
// an explicit *false from the user config is honoured.
func isRespectRequestedModel(cfg *config.Config) bool {
	if cfg.RespectRequestedModel == nil {
		return true // default when not explicitly set
	}
	return *cfg.RespectRequestedModel
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
	if !isRespectRequestedModel(cfg) || requestedModel == "" {
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
	if needsVision && !primary.Vision {
		return RouteResult{}, false, fmt.Errorf("requested model %s does not support vision", primary.ModelID)
	}

	fallbacks := cfg.Fallbacks["default"]

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

	// Get fallbacks for scenario
	fallbacks := cfg.Fallbacks[string(result.Scenario)]
	if len(fallbacks) == 0 {
		if isVisionScenario(result.Scenario) {
			return RouteResult{}, fmt.Errorf("vision scenario %s has no configured vision fallbacks", result.Scenario)
		}
		// Fall back to default fallbacks
		fallbacks = cfg.Fallbacks["default"]
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

// RouteWithOverride checks if the requested model matches a model_overrides entry.
//
// When matched, the returned RouteResult uses the override ModelConfig as the
// primary. The fallback chain is fallbacks[<requestedModel>], falling back to
// fallbacks["default"] when the override key has no entry (matching the
// behavior of Route and RouteForStreaming). The caller (MessagesHandler) is
// expected to merge a scenario-derived safety-net chain on top.
//
// Returns the override RouteResult and true if matched, or a zero value and
// false if the requested model has no entry in model_overrides.
func (r *ModelRouter) RouteWithOverride(requestedModel string) (RouteResult, bool) {
	cfg := r.atomic.Get()
	if cfg.ModelOverrides == nil {
		return RouteResult{}, false
	}
	override, ok := cfg.ModelOverrides[requestedModel]
	if !ok {
		return RouteResult{}, false
	}
	fallbacks := cfg.Fallbacks[requestedModel]
	if len(fallbacks) == 0 {
		fallbacks = cfg.Fallbacks["default"]
	}
	return RouteResult{
		Primary:   override,
		Fallbacks: fallbacks,
		Scenario:  ScenarioOverride,
	}, true
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
func (r *ModelRouter) RouteForStreaming(messages []MessageContent, tokenCount int, requestedModel string) (RouteResult, error) {
	cfg := r.atomic.Get()

	if result, ok, err := r.resolveRequestedModel(cfg, requestedModel, false); err == nil && ok {
		return result, nil
	}

	// Otherwise, use scenario-based routing for streaming
	result := RouteForStreaming(messages, tokenCount, cfg)

	// Get primary model for scenario
	primary, ok := cfg.Models[string(result.Scenario)]
	if !ok {
		if isVisionScenario(result.Scenario) {
			return RouteResult{Scenario: result.Scenario}, fmt.Errorf("vision scenario %s is not configured", result.Scenario)
		}
		// Fall back to fast scenario if not configured
		primary, ok = cfg.Models["fast"]
		if !ok {
			// Fall back to default
			primary = cfg.Models["default"]
		}
	}
	if primary.ModelID == "" {
		return RouteResult{}, fmt.Errorf("no model configured for streaming; neither scenario %q, \"fast\", nor \"default\" exist in models map", result.Scenario)
	}

	// Get fallbacks for scenario
	fallbacks := cfg.Fallbacks[string(result.Scenario)]
	if len(fallbacks) == 0 {
		if isVisionScenario(result.Scenario) {
			fallbacks = nil
		} else {
			// Fall back to fast fallbacks
			fallbacks = cfg.Fallbacks["fast"]
		}
	}

	return RouteResult{
		Primary:   primary,
		Fallbacks: fallbacks,
		Scenario:  result.Scenario,
	}, nil
}

func isVisionScenario(s Scenario) bool {
	return s == ScenarioVision || s == ScenarioVisionComplex || s == ScenarioVisionLongContext
}
