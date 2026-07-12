package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/catalog"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/storage"
)

var ErrUnknownProvider = errors.New("unknown provider")

type ModelRouter struct {
	atomic      *config.AtomicConfig
	db          *storage.Database
	catalogPath string
	catMu       sync.Mutex
	cat         *catalog.IndexedCatalog
	catErr      error
	catCache    time.Time
}

func NewModelRouter(atomic *config.AtomicConfig) *ModelRouter {
	return &ModelRouter{atomic: atomic}
}

func NewModelRouterWithDB(atomic *config.AtomicConfig, db *storage.Database) *ModelRouter {
	return &ModelRouter{atomic: atomic, db: db}
}

func NewModelRouterWithCatalog(atomic *config.AtomicConfig, catalogPath string) *ModelRouter {
	return &ModelRouter{atomic: atomic, catalogPath: catalogPath}
}

func (r *ModelRouter) catalog() (*catalog.IndexedCatalog, error) {
	if r.db == nil && r.catalogPath == "" {
		slog.Warn("catalog not available — model resolution falling back to legacy config")
		return nil, nil
	}

	r.catMu.Lock()
	defer r.catMu.Unlock()

	if r.cat != nil && time.Since(r.catCache) < 30*time.Second {
		return r.cat, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if r.db != nil {
		r.cat, r.catErr = catalog.LoadFromSQLite(ctx, r.db)
	} else if r.catalogPath != "" {
		r.cat, r.catErr = catalog.Load(r.catalogPath)
	}
	if r.catErr == nil {
		r.catCache = time.Now()
	}
	return r.cat, r.catErr
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
		// Not in legacy config — try the catalog before falling back to the
		// legacy unknown-model behavior. Provider-qualified references that
		// fail catalog resolution are rejected with a clear error instead of
		// silently falling back to a bogus provider.
		sel, parseErr := catalog.ParseModelRef(requestedModel)
		providerQualified := parseErr == nil && sel.Provider != ""

		cat, _ := r.catalog()
		if cat != nil {
			if catalogPrimary, catalogOk := r.resolveFromCatalog(cat, requestedModel, sel); catalogOk {
				primary = catalogPrimary
			} else if providerQualified {
				return RouteResult{}, false, fmt.Errorf("model reference %q uses unknown provider %q: %w", requestedModel, sel.Provider, ErrUnknownProvider)
			} else {
				primary = r.legacyUnknownModelConfig(cfg, requestedModel)
			}
		} else if providerQualified {
			return RouteResult{}, false, fmt.Errorf("model reference %q uses unknown provider %q: %w", requestedModel, sel.Provider, ErrUnknownProvider)
		} else {
			primary = r.legacyUnknownModelConfig(cfg, requestedModel)
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

// resolvedModelToConfig converts a catalog resolved model into a runtime
// ModelConfig used by the router.
func resolvedModelToConfig(resolved catalog.ResolvedModel) config.ModelConfig {
	supportsTools := resolved.Tools
	return config.ModelConfig{
		Provider:      resolved.Provider,
		ModelID:       resolved.ModelID,
		ModelRef:      resolved.CanonicalName,
		Vision:        resolved.Vision,
		ContextWindow: int(resolved.ContextWindow),
		SupportsTools: &supportsTools,
	}
}

// requestConstraints maps request-level requirements to scenario constraints
// used by the cost-based selector.
func requestConstraints(messages []MessageContent, tokenCount int) ScenarioConstraints {
	facts := AnalyzeRequestFacts(messages)
	constraints := ScenarioConstraints{
		Vision:  facts.NeedsVision,
		Context: int64(tokenCount),
	}
	latest := latestUserMessages(messages)
	if hasThinkingPattern(latest) {
		constraints.Reasoning = true
	}
	if hasToolUsage(messages) {
		constraints.Tools = true
	}
	return constraints
}

// hasToolUsage reports whether the request likely requires tool support based
// on message roles or tool-related keywords.
func hasToolUsage(messages []MessageContent) bool {
	toolKeywords := []string{
		"tool", "function", "execute", "run command",
	}
	for _, msg := range messages {
		if msg.Role == "tool" || msg.Role == "function" {
			return true
		}
		lower := strings.ToLower(msg.Content)
		for _, kw := range toolKeywords {
			if strings.Contains(lower, kw) {
				return true
			}
		}
	}
	return false
}

// resolveFromCatalog attempts to resolve a requested model string through the
// catalog. It returns the model config and true on success, otherwise false.
func (r *ModelRouter) resolveFromCatalog(cat *catalog.IndexedCatalog, requestedModel string, sel catalog.Selector) (config.ModelConfig, bool) {
	var resolved catalog.ResolvedModel
	var err error
	if sel.Provider != "" {
		resolved, err = cat.Resolve(sel)
	} else {
		resolved, err = cat.ResolveShort(requestedModel)
	}
	if err != nil {
		return config.ModelConfig{}, false
	}

	cfg := resolvedModelToConfig(resolved)
	cfg.ModelRef = requestedModel
	return cfg, true
}

// legacyUnknownModelConfig builds a bare config for an unknown model and
// inherits Temperature and MaxTokens from the default model when available.
func (r *ModelRouter) legacyUnknownModelConfig(cfg *config.Config, requestedModel string) config.ModelConfig {
	primary := config.ModelConfig{
		Provider: "opencode-go",
		ModelID:  requestedModel,
	}
	if def, ok := cfg.Models["default"]; ok {
		primary.Temperature = def.Temperature
		primary.MaxTokens = def.MaxTokens
	}
	return primary
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
	scenarioKey := string(result.Scenario)

	// Get primary model for scenario. When cost-based routing is enabled and
	// a non-empty catalog is available, prefer the cheapest matching catalog
	// model while preserving the legacy fallback chain.
	primary, ok := cfg.Models[scenarioKey]
	if cat, catErr := r.catalog(); cfg.CostBasedRoutingEnabled() && cat != nil && catErr == nil && len(cat.Models) > 0 {
		constraints := requestConstraints(messages, tokenCount)
		selector := NewSelector(cat, cfg)
		if resolved, err := selector.SelectCheapest(scenarioKey, constraints); err == nil {
			primary = resolvedModelToConfig(resolved)
			ok = true
		}
	}

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
	fallbacks := cfg.Fallbacks[scenarioKey]
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

	if result, ok, err := r.resolveRequestedModel(cfg, requestedModel, false); err != nil {
		return RouteResult{}, err
	} else if ok {
		return result, nil
	}

	// Otherwise, use scenario-based routing for streaming
	result := RouteForStreaming(messages, tokenCount, cfg)
	scenarioKey := string(result.Scenario)

	// Get primary model for scenario. When cost-based routing is enabled and
	// a non-empty catalog is available, prefer the cheapest matching catalog
	// model while preserving the legacy fallback chain.
	primary, ok := cfg.Models[scenarioKey]
	if cat, catErr := r.catalog(); cfg.CostBasedRoutingEnabled() && cat != nil && catErr == nil && len(cat.Models) > 0 {
		constraints := requestConstraints(messages, tokenCount)
		selector := NewSelector(cat, cfg)
		if resolved, err := selector.SelectCheapest(scenarioKey, constraints); err == nil {
			primary = resolvedModelToConfig(resolved)
			ok = true
		}
	}
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
	fallbacks := cfg.Fallbacks[scenarioKey]
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
