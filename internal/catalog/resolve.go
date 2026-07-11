package catalog

import (
	"errors"
	"fmt"
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
// It first matches by model key, then by model Name.
func (ic *IndexedCatalog) ResolveShort(short string) (ResolvedModel, error) {
	if model, ok := ic.Models[short]; ok {
		return ic.resolveWithFirstEnabledProvider(model, short)
	}

	for key, model := range ic.Models {
		if model.Name == short {
			return ic.resolveWithFirstEnabledProvider(model, key)
		}
	}

	for key, model := range ic.Models {
		if modelNameFromKey(key) == short {
			return ic.resolveWithFirstEnabledProvider(model, key)
		}
	}

	return ResolvedModel{}, fmt.Errorf("unknown short model id: %q", short)
}

// ListProviderModels returns a slice of ResolvedModel for every model that supports the
// named provider. The iteration order follows the underlying map and is non-deterministic.
// If the provider is unknown, nil is returned.
func (ic *IndexedCatalog) ListProviderModels(provider string) []ResolvedModel {
	providerCfg, ok := ic.Providers[provider]
	if !ok {
		return nil
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
	for key, model := range ic.Models {
		if modelNameFromKey(key) == sel.Model {
			return model, key
		}
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
		CostInputPerM:          0,
		CostOutputPerM:         0,
		Tools:                  model.SupportsTools(),
		Vision:                 model.SupportsVision(),
		Reasoning:              model.Reasoning,
	}
}
