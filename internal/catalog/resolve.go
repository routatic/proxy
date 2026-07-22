package catalog

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ParseModelRef parses a model reference string into a Selector.
// Supported forms:
//   - lab/model@provider -> {Provider: "provider", Model: "model", Alias: "lab/model"}
//   - model@provider     -> {Provider: "provider", Model: "model", Alias: "model"}
//   - lab/model          -> {Model: "model", Alias: "lab/model"}
//   - model              -> {Model: "model", Alias: "model"}
func ParseModelRef(ref string) (Selector, error) {
	if ref == "" {
		return Selector{}, errors.New("model reference is empty")
	}

	parts := strings.Split(ref, "@")
	if len(parts) > 2 {
		return Selector{}, fmt.Errorf("model reference %q contains multiple @ separators", ref)
	}

	modelPart := parts[0]
	if modelPart == "" {
		return Selector{}, fmt.Errorf("model id is empty in reference %q", ref)
	}

	var provider string
	if len(parts) == 2 {
		provider = parts[1]
	}

	if idx := strings.LastIndex(modelPart, "/"); idx >= 0 {
		model := modelPart[idx+1:]
		if model == "" {
			return Selector{}, fmt.Errorf("model id is empty in reference %q", ref)
		}
		return Selector{Provider: provider, Model: model, Alias: modelPart}, nil
	}

	return Selector{Provider: provider, Model: modelPart, Alias: modelPart}, nil
}

// Resolve resolves a canonical selector into a fully materialized model/provider pair.
// The selector must include a provider.
func (ic *IndexedCatalog) Resolve(sel Selector) (ResolvedModel, error) {
	if sel.Provider == "" {
		return ResolvedModel{}, errors.New("provider is required for canonical resolution")
	}

	provider, ok := ic.Providers[sel.Provider]
	if !ok {
		return ResolvedModel{}, fmt.Errorf("unknown provider %q", sel.Provider)
	}

	model, modelKey := ic.findModel(sel)
	if modelKey == "" {
		return ResolvedModel{}, fmt.Errorf("unknown model %q", sel.Model)
	}

	if ProviderFromModelKey(modelKey) != sel.Provider {
		return ResolvedModel{}, fmt.Errorf("model %q is not available on provider %q", modelKey, sel.Provider)
	}

	return resolvedModel(provider, modelKey, model), nil
}

// ResolveShort resolves a legacy short model id to a fully materialized model/provider pair.
// It first matches by model key, then by model Name, then by key suffix. All matches are
// collected before checking provider availability, so an enabled provider on a lower-priority
// match won't be shadowed by a disabled provider on a higher-priority match.
func (ic *IndexedCatalog) ResolveShort(short string) (ResolvedModel, error) {
	if model, ok := ic.Models[short]; ok {
		return ic.resolveWithFirstEnabledProvider(model, short)
	}

	var matches []string
	for key, model := range ic.Models {
		if model.Name == short {
			matches = append(matches, key)
		}
	}
	for key := range ic.Models {
		if modelNameFromKey(key) == short {
			matches = append(matches, key)
		}
	}

	if len(matches) > 0 {
		return ic.resolveFromMatches(short, matches)
	}

	return ResolvedModel{}, fmt.Errorf("unknown short model id: %q", short)
}

func (ic *IndexedCatalog) resolveFromMatches(short string, matches []string) (ResolvedModel, error) {
	sort.Strings(matches)

	var enabled []string
	var missingProviders []string
	var disabledProviders []string

	for _, key := range matches {
		providerName := ProviderFromModelKey(key)
		provider, ok := ic.Providers[providerName]
		if !ok {
			missingProviders = append(missingProviders, providerName)
			continue
		}
		if provider.Enabled != nil && !*provider.Enabled {
			disabledProviders = append(disabledProviders, providerName)
			continue
		}
		enabled = append(enabled, key)
	}

	if len(enabled) == 0 {
		if len(missingProviders) > 0 && len(disabledProviders) == 0 {
			return ResolvedModel{}, fmt.Errorf("model %q exists but provider(s) %q not found in catalog", short, strings.Join(missingProviders, ", "))
		}
		if len(disabledProviders) > 0 && len(missingProviders) == 0 {
			return ResolvedModel{}, fmt.Errorf("model %q exists but all providers %q are disabled", short, strings.Join(disabledProviders, ", "))
		}
		return ResolvedModel{}, fmt.Errorf("model %q exists but providers %q not found and providers %q are disabled", short, strings.Join(missingProviders, ", "), strings.Join(disabledProviders, ", "))
	}

	if len(enabled) == 1 {
		key := enabled[0]
		model := ic.Models[key]
		return ic.resolveWithFirstEnabledProvider(model, key)
	}

	var providers []string
	for _, key := range enabled {
		providers = append(providers, ProviderFromModelKey(key))
	}
	sort.Strings(providers)
	return ResolvedModel{}, fmt.Errorf("ambiguous model %q: available on multiple providers [%s] - use provider/model-id format", short, strings.Join(providers, ", "))
}

// ListProviderModels returns a slice of ResolvedModel for every model that supports the
// named provider. The iteration order follows the underlying map and is non-deterministic.
// If the provider is unknown, nil is returned.
func (ic *IndexedCatalog) ListProviderModels(provider string) []ResolvedModel {
	providerCfg, ok := ic.Providers[provider]
	if !ok {
		return nil
	}

	// Prefer the pre-built ProviderModels index (populated during Load /
	// LoadFromSQLite). Fall back to iterating Models by key prefix for
	// backward compatibility with hand-built catalogs (e.g. tests).
	if len(ic.ProviderModels) > 0 {
		models := ic.ProviderModels[provider]
		result := make([]ResolvedModel, 0, len(models))
		for _, model := range models {
			result = append(result, resolvedModel(providerCfg, model.ID, model))
		}
		return result
	}

	prefix := provider + "/"
	var result []ResolvedModel
	for key, model := range ic.Models {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		result = append(result, resolvedModel(providerCfg, key, model))
	}
	return result
}

func (ic *IndexedCatalog) findModel(sel Selector) (Model, string) {
	if model, ok := ic.Models[sel.Model]; ok {
		return model, sel.Model
	}
	// Try alias: if user asked "xai/grok-4.5", look it up directly.
	if sel.Alias != "" {
		if model, ok := ic.Models[sel.Alias]; ok {
			return model, sel.Alias
		}
	}
	// Try full key "provider/model-name" built from model name.
	// If sel.Provider is set, prefer matching that provider.
	var fallbackKeys []string
	for key, model := range ic.Models {
		if modelNameFromKey(key) == sel.Model {
			if sel.Provider != "" && ProviderFromModelKey(key) == sel.Provider {
				return model, key
			}
			// Collect candidates for fallback (when no provider-specific match)
			fallbackKeys = append(fallbackKeys, key)
		}
	}
	// Fall back to any provider. Use deterministic order.
	if len(fallbackKeys) > 0 {
		sort.Strings(fallbackKeys)
		key := fallbackKeys[0]
		return ic.Models[key], key
	}
	return Model{}, ""
}

func (ic *IndexedCatalog) resolveWithFirstEnabledProvider(model Model, key string) (ResolvedModel, error) {
	providerName := ProviderFromModelKey(key)
	if providerName == "" {
		return ResolvedModel{}, fmt.Errorf("model key %q has no provider prefix", key)
	}
	provider, ok := ic.Providers[providerName]
	if !ok {
		return ResolvedModel{}, fmt.Errorf("provider %q from model key %q not found in catalog", providerName, key)
	}
	if provider.Enabled != nil && !*provider.Enabled {
		return ResolvedModel{}, fmt.Errorf("provider %q for model %q is disabled", providerName, key)
	}
	return resolvedModel(provider, key, model), nil
}

func resolvedModel(provider Provider, modelKey string, model Model) ResolvedModel {
	return ResolvedModel{
		Provider:               provider.Name,
		ModelID:                modelNameFromKey(modelKey),
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
	}
}
