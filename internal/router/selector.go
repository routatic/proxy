package router

import (
	"errors"
	"fmt"
	"sort"

	"github.com/routatic/proxy/internal/catalog"
	"github.com/routatic/proxy/internal/config"
)

// ScenarioConstraints filters candidate models by required capabilities.
type ScenarioConstraints struct {
	Tools     bool
	Vision    bool
	Context   int64
	Reasoning bool
}

// Selector selects models from a catalog according to scenario requirements
// and runtime constraints such as enabled providers and API keys.
type Selector struct {
	catalog          *catalog.IndexedCatalog
	enabledProviders map[string]bool
	cfg              *config.Config
}

// NewSelector creates a Selector from an indexed catalog and active config.
// Providers are enabled when they have an effective API key in the config
// (either a global key or a provider-specific key) and are not explicitly
// disabled in the catalog.
func NewSelector(cat *catalog.IndexedCatalog, cfg *config.Config) *Selector {
	if cfg == nil {
		cfg = &config.Config{}
	}
	enabled := enabledProviders(cfg)
	for name, p := range cat.Providers {
		if p.Enabled != nil && !*p.Enabled {
			delete(enabled, name)
		}
	}
	return &Selector{
		catalog:          cat,
		enabledProviders: enabled,
		cfg:              cfg,
	}
}

// SelectCheapest returns the cheapest resolved model for the named scenario
// that satisfies both the scenario requirements and the supplied constraints.
//
// Candidates are sorted by total cost per million tokens ascending. Ties are
// broken by larger context window, then by model ID.
func (s *Selector) SelectCheapest(scenario string, constraints ScenarioConstraints) (catalog.ResolvedModel, error) {
	scen, ok := s.catalog.Scenarios[scenario]
	if !ok {
		return catalog.ResolvedModel{}, fmt.Errorf("unknown scenario %q", scenario)
	}

	candidates := s.resolveCandidates(scen, constraints)
	if len(candidates) == 0 {
		return catalog.ResolvedModel{}, fmt.Errorf("%w: scenario %q", ErrNoCandidateModel, scenario)
	}

	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		costA := a.CostInputPerM + a.CostOutputPerM + s.effectivePenalty(a.Provider)
		costB := b.CostInputPerM + b.CostOutputPerM + s.effectivePenalty(b.Provider)
		if costA != costB {
			return costA < costB
		}
		if a.ContextWindow != b.ContextWindow {
			return a.ContextWindow > b.ContextWindow
		}
		return a.ModelID < b.ModelID
	})

	return candidates[0], nil
}

// resolveCandidates enumerates all enabled provider/model pairs for a scenario
// and returns the resolved models that match the scenario requirements and
// constraints.
func (s *Selector) resolveCandidates(scen catalog.Scenario, constraints ScenarioConstraints) []catalog.ResolvedModel {
	providers := s.providerSet(scen)
	minContext := max(scen.MinContextWindow, constraints.Context)

	maxContext := int64(0)
	if s.cfg != nil && s.cfg.CostRouting != nil && s.cfg.CostRouting.MaxContextWindow > 0 {
		maxContext = s.cfg.CostRouting.MaxContextWindow
	}

	var candidates []catalog.ResolvedModel
	for providerName := range providers {
		provider, ok := s.catalog.Providers[providerName]
		if !ok {
			continue
		}
		for modelKey, model := range s.catalog.Models {
			if !modelSupportsProvider(modelKey, providerName) {
				continue
			}
			if maxContext > 0 && model.ContextWindow() > maxContext {
				continue
			}
			if !modelMatches(model, scen, constraints, minContext) {
				continue
			}
			candidates = append(candidates, catalog.ResolvedModel{
				Provider:               provider.Name,
				ModelID:                catalog.ModelNameFromKey(modelKey),
				CanonicalName:          modelKey,
				DisplayName:            model.DisplayName(),
				BaseURL:                provider.BaseURL,
				APIKey:                 provider.APIKey,
				AnthropicToolsDisabled: provider.AnthropicToolsDisabled,
				ContextWindow:          model.ContextWindow(),
				CostInputPerM:          model.CostInputPerM(),
				CostOutputPerM:         model.CostOutputPerM(),
				Tools:                  model.SupportsTools(),
				Vision:                 model.SupportsVision(),
				Reasoning:              model.Reasoning,
			})
		}
	}
	return candidates
}

// providerSet returns the enabled providers that should be considered for a
// scenario. When the scenario lists preferred providers, only those that are
// enabled are returned; when the global cost_routing.prefer_providers is
// non-empty it is intersected with the scenario's preferred providers (or used
// alone when the scenario has none). Otherwise all enabled providers are returned.
func (s *Selector) providerSet(scen catalog.Scenario) map[string]bool {
	// Handle nil config by returning all enabled providers.
	if s.cfg == nil {
		set := make(map[string]bool, len(s.enabledProviders))
		for p := range s.enabledProviders {
			set[p] = true
		}
		return set
	}

	globalPref := s.globalPreferProviders()
	scenarioPref := scen.PreferredProviders

	// If neither global nor scenario has preferred providers, return all enabled.
	if len(globalPref) == 0 && len(scenarioPref) == 0 {
		set := make(map[string]bool, len(s.enabledProviders))
		for p := range s.enabledProviders {
			set[p] = true
		}
		return set
	}

	// Resolve which list to use. When both are set, intersect them.
	candidates := scenarioPref
	if len(globalPref) > 0 {
		if len(scenarioPref) == 0 {
			candidates = globalPref
		} else {
			// Intersect global and scenario preferred providers.
			globalSet := make(map[string]struct{}, len(globalPref))
			for _, p := range globalPref {
				globalSet[p] = struct{}{}
			}
			candidates = nil
			for _, p := range scenarioPref {
				if _, ok := globalSet[p]; ok {
					candidates = append(candidates, p)
				}
			}
		}
	}

	set := make(map[string]bool, len(candidates))
	for _, p := range candidates {
		if s.enabledProviders[p] {
			set[p] = true
		}
	}
	return set
}

// globalPreferProviders returns the global prefer_providers list from config
// or nil when unset.
func (s *Selector) globalPreferProviders() []string {
	if s.cfg == nil || s.cfg.CostRouting == nil {
		return nil
	}
	return s.cfg.CostRouting.PreferProviders
}

func modelSupportsProvider(modelKey string, provider string) bool {
	return catalog.ProviderFromModelKey(modelKey) == provider
}

func modelMatches(model catalog.Model, scen catalog.Scenario, constraints ScenarioConstraints, minContext int64) bool {
	if model.ContextWindow() < minContext {
		return false
	}
	if scen.RequiresTools != nil && *scen.RequiresTools && !model.SupportsTools() {
		return false
	}
	if scen.RequiresVision != nil && *scen.RequiresVision && !model.SupportsVision() {
		return false
	}
	if scen.RequiresReasoning != nil && *scen.RequiresReasoning && !model.Reasoning {
		return false
	}
	if constraints.Tools && !model.SupportsTools() {
		return false
	}
	if constraints.Vision && !model.SupportsVision() {
		return false
	}
	if constraints.Reasoning && !model.Reasoning {
		return false
	}
	return true
}

// enabledProviders returns the providers that have an effective API key in the
// active config. A non-empty global API key enables all known providers.
func enabledProviders(cfg *config.Config) map[string]bool {
	enabled := make(map[string]bool)
	globalKeys := cfg.EffectiveAPIKeys()
	providerKeys := map[string][]string{
		"opencode-go":  cfg.OpenCodeGo.EffectiveAPIKeys(),
		"opencode-zen": cfg.OpenCodeZen.EffectiveAPIKeys(),
		"aws-bedrock":  cfg.AWSBedrock.EffectiveAPIKeys(),
		"openrouter":   cfg.OpenRouter.EffectiveAPIKeys(),
	}
	for p, keys := range providerKeys {
		if len(keys) > 0 || len(globalKeys) > 0 {
			enabled[p] = true
		}
	}
	return enabled
}

// IsEnabledProvider reports whether the named provider is enabled in the
// selector's runtime configuration.
func (s *Selector) IsEnabledProvider(provider string) bool {
	return s.enabledProviders[provider]
}

// effectivePenalty returns the additional per-provider cost penalty from the
// config, or 0 when no penalty is configured for the named provider.
func (s *Selector) effectivePenalty(provider string) float64 {
	if s.cfg == nil || s.cfg.CostRouting == nil || s.cfg.CostRouting.PenaltyPerProvider == nil {
		return 0
	}
	return s.cfg.CostRouting.PenaltyPerProvider[provider]
}

// ErrNoCandidateModel is returned when SelectCheapest cannot find a model that
// matches the scenario and constraints.
var ErrNoCandidateModel = errors.New("no candidate model matches scenario and constraints")
