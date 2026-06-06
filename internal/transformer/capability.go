// Package transformer handles request and response format conversion
// between Anthropic Messages API and OpenAI Chat Completions API.
package transformer

import (
	"fmt"
	"strings"
)

// ModelCapability describes what a model can and cannot do.
type ModelCapability struct {
	Description       string `json:"description"`        // Human-readable model name
	Multimodal        bool   `json:"multimodal"`         // Supports image/video input
	ContextWindow     string `json:"context_window"`     // e.g. "256K", "1M"
	Reasoning         bool   `json:"reasoning"`          // Supports thinking/reasoning
	ReasoningNote     string `json:"reasoning_note"`     // Optional note about reasoning
	MaxTokens         int    `json:"max_tokens"`         // Default max output tokens
	SupportsTools     bool   `json:"supports_tools"`     // Supports tool/function calling
}

// CapabilityInjectionConfig controls how model capabilities are injected.
type CapabilityInjectionConfig struct {
	Enabled    bool              `json:"enabled"`
	Prefix     string            `json:"prefix"`     // Text before the capability block
	Suffix     string            `json:"suffix"`     // Text after the capability block
	Overrides  map[string]string `json:"overrides"`  // Custom capability text per model ID (exact match)
}

// GetCapabilityText returns the injected capability text for a given model ID.
// Returns empty string if no match is found or injection is disabled.
func (cfg CapabilityInjectionConfig) GetCapabilityText(modelID string) string {
	if !cfg.Enabled {
		return ""
	}

	// Check for exact model ID override first
	if text, ok := cfg.Overrides[modelID]; ok {
		return text
	}

	cap, ok := matchModelCapability(modelID)
	if !ok {
		return ""
	}

	return cfg.formatCapability(modelID, cap)
}

// formatCapability builds the injection text from a matched capability.
func (cfg CapabilityInjectionConfig) formatCapability(modelID string, cap ModelCapability) string {
	var b strings.Builder

	if cfg.Prefix != "" {
		b.WriteString(cfg.Prefix)
		b.WriteString("\n")
	}

	b.WriteString(fmt.Sprintf("## Current Model: %s (%s)\n", cap.Description, modelID))
	b.WriteString(fmt.Sprintf("- Multimodal (image/video input): %s\n", boolToStr(cap.Multimodal)))
	b.WriteString(fmt.Sprintf("- Context window: %s\n", cap.ContextWindow))

	if cap.ReasoningNote != "" {
		b.WriteString(fmt.Sprintf("- Reasoning/thinking: %s (%s)\n", boolToStr(cap.Reasoning), cap.ReasoningNote))
	} else {
		b.WriteString(fmt.Sprintf("- Reasoning/thinking: %s\n", boolToStr(cap.Reasoning)))
	}

	b.WriteString(fmt.Sprintf("- Tool/function calling: %s\n", boolToStr(cap.SupportsTools)))

	if cfg.Suffix != "" {
		b.WriteString(cfg.Suffix)
		b.WriteString("\n")
	}

	return b.String()
}

// InjectSystemPrompt prepends the capability text to the system prompt.
// If systemText is empty, the capability text becomes the system prompt.
func (cfg CapabilityInjectionConfig) InjectSystemPrompt(systemText string, modelID string) string {
	capText := cfg.GetCapabilityText(modelID)
	if capText == "" {
		return systemText
	}

	if systemText == "" {
		return capText
	}

	return capText + "\n\n---\n\n" + systemText
}

// boolToStr returns "yes" or "no" for a boolean.
func boolToStr(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

// matchModelCapability finds the best capability match for a model ID.
// Uses prefix matching so "deepseek-v4-pro" and "deepseek-v4-flash" both match.
func matchModelCapability(modelID string) (ModelCapability, bool) {
	lower := strings.ToLower(modelID)
	for prefix, cap := range modelCapabilityMap {
		if strings.HasPrefix(lower, prefix) {
			return cap, true
		}
	}
	return ModelCapability{}, false
}

// modelCapabilityMap defines known model capabilities keyed by model ID prefix.
// Ordered roughly by specificity; first prefix match wins.
var modelCapabilityMap = map[string]ModelCapability{
	// DeepSeek V4 family
	"deepseek-v4-pro": {
		Description:   "DeepSeek V4 Pro",
		Multimodal:    false,
		ContextWindow: "1M tokens",
		Reasoning:     true,
		ReasoningNote: "supports thinking mode with reasoning_effort",
		SupportsTools: true,
	},
	"deepseek-v4-flash": {
		Description:   "DeepSeek V4 Flash",
		Multimodal:    false,
		ContextWindow: "1M tokens",
		Reasoning:     true,
		ReasoningNote: "supports thinking mode with reasoning_effort",
		SupportsTools: true,
	},
	"deepseek-v4": {
		Description:   "DeepSeek V4",
		Multimodal:    false,
		ContextWindow: "1M tokens",
		Reasoning:     true,
		ReasoningNote: "supports thinking mode",
		SupportsTools: true,
	},

	// DeepSeek V3 / R1 family
	"deepseek-r1": {
		Description:   "DeepSeek R1",
		Multimodal:    false,
		ContextWindow: "128K tokens",
		Reasoning:     true,
		ReasoningNote: "built-in chain-of-thought, no separate thinking toggle",
		SupportsTools: true,
	},
	"deepseek-v3": {
		Description:   "DeepSeek V3",
		Multimodal:    false,
		ContextWindow: "128K tokens",
		Reasoning:     false,
		SupportsTools: true,
	},
	"deepseek": {
		Description:   "DeepSeek",
		Multimodal:    false,
		ContextWindow: "128K tokens",
		Reasoning:     false,
		SupportsTools: true,
	},

	// Kimi K2 family
	"kimi-k2.6": {
		Description:   "Kimi K2.6",
		Multimodal:    false,
		ContextWindow: "256K tokens",
		Reasoning:     true,
		ReasoningNote: "supports thinking, reasoning_effort: low/medium/high",
		SupportsTools: true,
	},
	"kimi-k2.5": {
		Description:   "Kimi K2.5",
		Multimodal:    false,
		ContextWindow: "256K tokens",
		Reasoning:     true,
		ReasoningNote: "supports thinking",
		SupportsTools: true,
	},
	"kimi-k2": {
		Description:   "Kimi K2",
		Multimodal:    false,
		ContextWindow: "256K tokens",
		Reasoning:     true,
		ReasoningNote: "supports thinking",
		SupportsTools: true,
	},
	"kimi": {
		Description:   "Kimi",
		Multimodal:    false,
		ContextWindow: "128K tokens",
		Reasoning:     false,
		SupportsTools: true,
	},

	// GLM family
	"glm-5.1": {
		Description:   "GLM-5.1",
		Multimodal:    false,
		ContextWindow: "200K tokens",
		Reasoning:     true,
		ReasoningNote: "supports deep reasoning, no image/video input",
		SupportsTools: true,
	},
	"glm-5": {
		Description:   "GLM-5",
		Multimodal:    false,
		ContextWindow: "200K tokens",
		Reasoning:     true,
		ReasoningNote: "supports reasoning",
		SupportsTools: true,
	},
	"glm-4": {
		Description:   "GLM-4",
		Multimodal:    false,
		ContextWindow: "128K tokens",
		Reasoning:     false,
		SupportsTools: true,
	},
	"glm": {
		Description:   "GLM",
		Multimodal:    false,
		ContextWindow: "128K tokens",
		Reasoning:     false,
		SupportsTools: true,
	},

	// Qwen family
	"qwen3.6-plus": {
		Description:   "Qwen3.6 Plus",
		Multimodal:    false,
		ContextWindow: "1M tokens",
		Reasoning:     false,
		SupportsTools: true,
	},
	"qwen3.6": {
		Description:   "Qwen3.6",
		Multimodal:    false,
		ContextWindow: "1M tokens",
		Reasoning:     false,
		SupportsTools: true,
	},
	"qwen3.5-plus": {
		Description:   "Qwen3.5 Plus",
		Multimodal:    false,
		ContextWindow: "1M tokens",
		Reasoning:     false,
		SupportsTools: true,
	},
	"qwen3.5": {
		Description:   "Qwen3.5",
		Multimodal:    false,
		ContextWindow: "1M tokens",
		Reasoning:     false,
		SupportsTools: true,
	},
	"qwen3": {
		Description:   "Qwen3",
		Multimodal:    false,
		ContextWindow: "128K tokens",
		Reasoning:     false,
		SupportsTools: true,
	},
	"qwen": {
		Description:   "Qwen",
		Multimodal:    false,
		ContextWindow: "128K tokens",
		Reasoning:     false,
		SupportsTools: true,
	},

	// MiniMax family
	"minimax-m2.7": {
		Description:   "MiniMax M2.7",
		Multimodal:    true,
		ContextWindow: "1M tokens",
		Reasoning:     false,
		ReasoningNote: "supports image input via Anthropic-native endpoint",
		SupportsTools: true,
	},
	"minimax-m2.5": {
		Description:   "MiniMax M2.5",
		Multimodal:    true,
		ContextWindow: "1M tokens",
		Reasoning:     false,
		ReasoningNote: "supports image input via Anthropic-native endpoint",
		SupportsTools: true,
	},
	"minimax-m2": {
		Description:   "MiniMax M2",
		Multimodal:    true,
		ContextWindow: "1M tokens",
		Reasoning:     false,
		ReasoningNote: "supports image input via Anthropic-native endpoint",
		SupportsTools: true,
	},
	"minimax-m1": {
		Description:   "MiniMax M1",
		Multimodal:    true,
		ContextWindow: "1M tokens",
		Reasoning:     false,
		SupportsTools: true,
	},
	"minimax": {
		Description:   "MiniMax",
		Multimodal:    true,
		ContextWindow: "1M tokens",
		Reasoning:     false,
		SupportsTools: true,
	},

	// MiMo family
	"mimo-v2-pro": {
		Description:   "MiMo V2 Pro",
		Multimodal:    false,
		ContextWindow: "128K tokens",
		Reasoning:     false,
		SupportsTools: true,
	},
	"mimo-v2": {
		Description:   "MiMo V2",
		Multimodal:    false,
		ContextWindow: "128K tokens",
		Reasoning:     false,
		SupportsTools: true,
	},
	"mimo": {
		Description:   "MiMo",
		Multimodal:    false,
		ContextWindow: "128K tokens",
		Reasoning:     false,
		SupportsTools: true,
	},

	// Claude (via Zen)
	"claude-opus-4": {
		Description:   "Claude Opus 4",
		Multimodal:    true,
		ContextWindow: "1M tokens",
		Reasoning:     true,
		ReasoningNote: "supports extended thinking with budget_tokens",
		SupportsTools: true,
	},
	"claude-sonnet-4": {
		Description:   "Claude Sonnet 4",
		Multimodal:    true,
		ContextWindow: "1M tokens",
		Reasoning:     true,
		ReasoningNote: "supports extended thinking with budget_tokens",
		SupportsTools: true,
	},
	"claude-haiku-4": {
		Description:   "Claude Haiku 4",
		Multimodal:    false,
		ContextWindow: "200K tokens",
		Reasoning:     false,
		SupportsTools: true,
	},
	"claude": {
		Description:   "Claude (generic)",
		Multimodal:    true,
		ContextWindow: "200K tokens",
		Reasoning:     false,
		SupportsTools: true,
	},

	// GPT / Codex (via Zen)
	"gpt-5": {
		Description:   "GPT-5 / Codex",
		Multimodal:    true,
		ContextWindow: "128K tokens",
		Reasoning:     true,
		ReasoningNote: "supports reasoning_effort",
		SupportsTools: true,
	},
	"codex": {
		Description:   "Codex",
		Multimodal:    false,
		ContextWindow: "128K tokens",
		Reasoning:     false,
		SupportsTools: true,
	},

	// Gemini (via Zen)
	"gemini-2.5": {
		Description:   "Gemini 2.5",
		Multimodal:    true,
		ContextWindow: "1M tokens",
		Reasoning:     true,
		ReasoningNote: "native multimodal support, supports thinking",
		SupportsTools: true,
	},
	"gemini-2": {
		Description:   "Gemini 2",
		Multimodal:    true,
		ContextWindow: "1M tokens",
		Reasoning:     false,
		SupportsTools: true,
	},
	"gemini": {
		Description:   "Gemini",
		Multimodal:    true,
		ContextWindow: "1M tokens",
		Reasoning:     false,
		SupportsTools: true,
	},
}
