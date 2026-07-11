package catalog

import "strings"

// Catalog is the parsed contents of a models.dev catalog.
type Catalog struct {
	Providers map[string]Provider `json:"providers"`
	Models    map[string]Model    `json:"models"`
	Scenarios map[string]Scenario `json:"scenarios"`
}

// Provider describes a model hosting endpoint.
type Provider struct {
	Name                   string `json:"name"`
	BaseURL                string `json:"base_url"`
	APIKey                 string `json:"api_key"`
	Enabled                *bool  `json:"enabled,omitempty"`
	AnthropicToolsDisabled bool   `json:"anthropic_tools_disabled"`
}

// Modalities describes the input/output formats a model supports.
type Modalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

// Limit describes model usage limits.
type Limit struct {
	Context int64 `json:"context"`
	Output  int64 `json:"output"`
}

// Rates describes model pricing per million tokens.
type Rates struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

// Model describes a model available through one or more providers.
// The provider is encoded in the model key (e.g. "xai/grok-4.5").
type Model struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Reasoning  bool       `json:"reasoning"`
	ToolCall   bool       `json:"tool_call"`
	Modalities Modalities `json:"modalities"`
	Limit      *Limit     `json:"limit,omitempty"`
	Rates      *Rates     `json:"rates,omitempty"`
}

// DisplayName returns the model's display name.
func (m Model) DisplayName() string {
	return m.Name
}

// SupportsTools returns whether the model supports tool calls.
func (m Model) SupportsTools() bool {
	return m.ToolCall
}

// SupportsVision returns whether the model supports image inputs.
func (m Model) SupportsVision() bool {
	for _, mod := range m.Modalities.Input {
		if mod == "image" {
			return true
		}
	}
	return false
}

// ContextWindow returns the model's context window limit, or 0 if unknown.
func (m Model) ContextWindow() int64 {
	if m.Limit != nil {
		return m.Limit.Context
	}
	return 0
}

// CostInputPerM returns the input cost per million tokens, or 0 if unknown.
func (m Model) CostInputPerM() float64 {
	if m.Rates != nil {
		return m.Rates.Input
	}
	return 0
}

// CostOutputPerM returns the output cost per million tokens, or 0 if unknown.
func (m Model) CostOutputPerM() float64 {
	if m.Rates != nil {
		return m.Rates.Output
	}
	return 0
}

// ProviderFromModelKey extracts the provider name from a model key
// of the form "provider/model-name". Returns "" if no separator found.
func ProviderFromModelKey(key string) string {
	idx := strings.IndexByte(key, '/')
	if idx < 0 {
		return ""
	}
	return key[:idx]
}

// ModelNameFromKey extracts the model name portion from a model key
// of the form "provider/model-name". Returns the full key if no separator.
func ModelNameFromKey(key string) string {
	idx := strings.IndexByte(key, '/')
	if idx < 0 {
		return key
	}
	return key[idx+1:]
}

// modelNameFromKey is an alias for internal use.
func modelNameFromKey(key string) string {
	return ModelNameFromKey(key)
}

// Scenario describes a workload that selects a model by capability.
type Scenario struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	RequiresTools      *bool    `json:"requires_tools,omitempty"`
	RequiresVision     *bool    `json:"requires_vision,omitempty"`
	RequiresReasoning  *bool    `json:"requires_reasoning,omitempty"`
	MinContextWindow   int64    `json:"min_context_window"`
	PreferredProviders []string `json:"preferred_providers"`
}

// Selector is a parsed model reference such as model@provider,
// lab/model@provider, or a short id.
type Selector struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Alias    string `json:"alias"`
}

// ResolvedModel is a fully materialized provider/model pair ready for use.
type ResolvedModel struct {
	Provider               string  `json:"provider"`
	ModelID                string  `json:"model_id"`
	CanonicalName          string  `json:"canonical_name"`
	DisplayName            string  `json:"display_name"`
	BaseURL                string  `json:"base_url"`
	APIKey                 string  `json:"api_key"`
	AnthropicToolsDisabled bool    `json:"anthropic_tools_disabled"`
	ContextWindow          int64   `json:"context_window"`
	CostInputPerM          float64 `json:"cost_input_per_m"`
	CostOutputPerM         float64 `json:"cost_output_per_m"`
	Tools                  bool    `json:"tools"`
	Vision                 bool    `json:"vision"`
	Reasoning              bool    `json:"reasoning"`
}
