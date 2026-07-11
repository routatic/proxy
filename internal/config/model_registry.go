package config

const DefaultContextMargin = 8192

// ModelMetadata describes a known model's capabilities (context window size,
// max output tokens, vision support, and tool support). This metadata is
// used by ResolveModelConfig to fill in defaults when the user's runtime
// config omits optional fields, ensuring consistent behavior across models
// without requiring every property to be specified in the JSON config.
type ModelMetadata struct {
	ContextWindow   int
	MaxOutputTokens int
	Vision          bool
	SupportsTools   bool
}

var modelMetadata = map[string]ModelMetadata{
	"deepseek-v4-pro":        {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: false, SupportsTools: true},
	"deepseek-v4-flash":      {ContextWindow: 1000000, MaxOutputTokens: 4096, Vision: false, SupportsTools: true},
	"deepseek-v4-flash-free": {ContextWindow: 1000000, MaxOutputTokens: 4096, Vision: false, SupportsTools: true},
	"glm-5.2":                {ContextWindow: 200000, MaxOutputTokens: 8192, Vision: false, SupportsTools: true},
	"glm-5.1":                {ContextWindow: 200000, MaxOutputTokens: 8192, Vision: false, SupportsTools: true},
	"glm-5":                  {ContextWindow: 200000, MaxOutputTokens: 8192, Vision: false, SupportsTools: true},
	"kimi-k2.7-code":         {ContextWindow: 256000, MaxOutputTokens: 32768, Vision: true, SupportsTools: true},
	"kimi-k2.6":              {ContextWindow: 256000, MaxOutputTokens: 8192, Vision: true, SupportsTools: true},
	"kimi-k2.5":              {ContextWindow: 256000, MaxOutputTokens: 8192, Vision: true, SupportsTools: true},
	"mimo-v2-omni":           {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: true, SupportsTools: true},
	"mimo-v2.5-pro":          {ContextWindow: 1000000, MaxOutputTokens: 16384, Vision: false, SupportsTools: true},
	"mimo-v2.5":              {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: false, SupportsTools: true},
	"minimax-m3":             {ContextWindow: 1000000, MaxOutputTokens: 128000, Vision: false, SupportsTools: true},
	"minimax-m2.7":           {ContextWindow: 200000, MaxOutputTokens: 8192, Vision: false, SupportsTools: true},
	"minimax-m2.5":           {ContextWindow: 200000, MaxOutputTokens: 4096, Vision: false, SupportsTools: true},
	"qwen3.7-max":            {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: true, SupportsTools: true},
	"qwen3.7-plus":           {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: true, SupportsTools: true},
	"qwen3.6-plus":           {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: true, SupportsTools: true},
	"mimo-v2.5-free":         {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: false, SupportsTools: true},
	"qwen3.5-plus":           {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: true, SupportsTools: true},
}

// ResolveModelConfig fills in default capability values (context window,
// max output tokens, vision, tool support) for a ModelConfig by consulting
// the built-in modelMetadata registry. If the model is unknown or a field
// is already set, the existing value is preserved. Call this before using
// a ModelConfig so capacity filtering and scenario routing see accurate
// per-model limits.
func ResolveModelConfig(model ModelConfig) ModelConfig {
	if meta, ok := modelMetadata[model.ModelID]; ok {
		if model.ContextWindow == 0 {
			model.ContextWindow = meta.ContextWindow
		}
		if model.MaxOutputTokens == 0 {
			model.MaxOutputTokens = meta.MaxOutputTokens
		}
		if !model.Vision {
			model.Vision = meta.Vision
		}
		if model.SupportsTools == nil {
			v := meta.SupportsTools
			model.SupportsTools = &v
		}
	}
	if model.ContextMargin == 0 {
		model.ContextMargin = DefaultContextMargin
	}
	if model.SupportsTools == nil {
		v := true
		model.SupportsTools = &v
	}
	return model
}

// SupportsTools reports whether a model is capable of handling tool-use
// requests (function calling). It resolves the model config through
// ResolveModelConfig, then checks the SupportsTools field. Models that
// lack tool support (e.g., lightweight streaming-only models) should be
// excluded from requests that include tool definitions.
func SupportsTools(model ModelConfig) bool {
	model = ResolveModelConfig(model)
	return model.SupportsTools == nil || *model.SupportsTools
}
