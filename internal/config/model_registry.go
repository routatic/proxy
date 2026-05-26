package config

const DefaultContextMargin = 8192

type ModelMetadata struct {
	ContextWindow   int
	MaxOutputTokens int
	SupportsVision  bool
	SupportsTools   bool
}

var modelMetadata = map[string]ModelMetadata{
	"deepseek-v4-pro":   {ContextWindow: 1000000, MaxOutputTokens: 8192, SupportsVision: false, SupportsTools: true},
	"deepseek-v4-flash": {ContextWindow: 1000000, MaxOutputTokens: 4096, SupportsVision: false, SupportsTools: true},
	"kimi-k2.6":         {ContextWindow: 256000, MaxOutputTokens: 8192, SupportsVision: true, SupportsTools: true},
	"kimi-k2.5":         {ContextWindow: 256000, MaxOutputTokens: 8192, SupportsVision: true, SupportsTools: true},
	"mimo-v2.5-pro":     {ContextWindow: 1000000, MaxOutputTokens: 16384, SupportsVision: false, SupportsTools: true},
	"mimo-v2.5":         {ContextWindow: 1000000, MaxOutputTokens: 8192, SupportsVision: false, SupportsTools: true},
	"minimax-m2.7":      {ContextWindow: 200000, MaxOutputTokens: 8192, SupportsVision: false, SupportsTools: true},
	"minimax-m2.5":      {ContextWindow: 200000, MaxOutputTokens: 4096, SupportsVision: false, SupportsTools: true},
	"qwen3.6-plus":      {ContextWindow: 1000000, MaxOutputTokens: 8192, SupportsVision: true, SupportsTools: true},
	"qwen3.5-plus":      {ContextWindow: 1000000, MaxOutputTokens: 8192, SupportsVision: true, SupportsTools: true},
	"glm-5.1":           {ContextWindow: 200000, MaxOutputTokens: 8192, SupportsVision: false, SupportsTools: true},
	"glm-5":             {ContextWindow: 200000, MaxOutputTokens: 8192, SupportsVision: false, SupportsTools: true},
}

func ResolveModelConfig(model ModelConfig) ModelConfig {
	if meta, ok := modelMetadata[model.ModelID]; ok {
		if model.ContextWindow == 0 {
			model.ContextWindow = meta.ContextWindow
		}
		if model.MaxOutputTokens == 0 {
			model.MaxOutputTokens = meta.MaxOutputTokens
		}
		if !model.SupportsVision {
			model.SupportsVision = meta.SupportsVision
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

func SupportsTools(model ModelConfig) bool {
	model = ResolveModelConfig(model)
	return model.SupportsTools == nil || *model.SupportsTools
}
