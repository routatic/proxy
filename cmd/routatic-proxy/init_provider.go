package main

import (
	"fmt"
	"sort"
	"strings"
)

// ProviderPreset contains provider-specific configuration defaults
// and a generator for the config template.
type ProviderPreset struct {
	Name        string
	EnvVarName  string // Environment variable for API key
	Description string
	BaseURL     string
	Generator   func() string // Returns the JSON config template for this provider
}

// Supported provider presets.
var providerPresets = map[string]ProviderPreset{
	"opencode-go": {
		Name:        "OpenCode Go",
		EnvVarName:  "ROUTATIC_PROXY_OPENCODE_GO_API_KEY",
		Description: "OpenCode Go subscription - $5/month with powerful coding models",
		BaseURL:     "https://opencode.ai/zen/go/v1/chat/completions",
		Generator:   getOpenCodeGoConfig,
	},
	"opencode-zen": {
		Name:        "OpenCode Zen",
		EnvVarName:  "ROUTATIC_PROXY_OPENCODE_ZEN_API_KEY",
		Description: "OpenCode Zen - pay-as-you-go access to Claude, GPT, Gemini, and more",
		BaseURL:     "https://opencode.ai/zen/v1/chat/completions",
		Generator:   getOpenCodeZenConfig,
	},
	"aws-bedrock": {
		Name:        "AWS Bedrock",
		EnvVarName:  "ROUTATIC_PROXY_AWS_BEDROCK_API_KEY",
		Description: "AWS Bedrock Mantle - run models on your own AWS infrastructure",
		BaseURL:     "https://bedrock-mantle.us-east-1.api.aws/v1/chat/completions",
		Generator:   getAWSBedrockConfig,
	},
	"openrouter": {
		Name:        "OpenRouter",
		EnvVarName:  "ROUTATIC_PROXY_OPENROUTER_API_KEY",
		Description: "OpenRouter - unified API for 100+ models from multiple providers",
		BaseURL:     "https://openrouter.ai/api/v1/chat/completions",
		Generator:   getOpenRouterConfig,
	},
}

// getProviderConfig returns a config template optimized for a specific provider.
// Supported providers are derived from providerPresets so there is only one
// registry to maintain.
func getProviderConfig(provider string) (string, error) {
	preset, ok := providerPresets[provider]
	if !ok {
		supported := make([]string, 0, len(providerPresets))
		for p := range providerPresets {
			supported = append(supported, p)
		}
		sort.Strings(supported)
		return "", fmt.Errorf("unknown provider %q; supported: %s", provider, strings.Join(supported, ", "))
	}
	return preset.Generator(), nil
}

// getOpenRouterConfig returns a config optimized for OpenRouter.
func getOpenRouterConfig() string {
	return `{
  "api_key": "${ROUTATIC_PROXY_API_KEY}",
  "host": "127.0.0.1",
  "port": 3456,
  "hot_reload": false,
  "enable_streaming_scenario_routing": false,
  "respect_requested_model": false,

  "models": {
    "default": {
      "provider": "openrouter",
      "model_id": "anthropic/claude-sonnet-4",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "background": {
      "provider": "openrouter",
      "model_id": "openai/gpt-4o-mini",
      "temperature": 0.5,
      "max_tokens": 2048
    },
    "think": {
      "provider": "openrouter",
      "model_id": "anthropic/claude-sonnet-4",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "complex": {
      "provider": "openrouter",
      "model_id": "anthropic/claude-opus-4",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "long_context": {
      "provider": "openrouter",
      "model_id": "google/gemini-2.5-pro",
      "temperature": 0.7,
      "max_tokens": 16384,
      "context_threshold": 80000
    },
    "fast": {
      "provider": "openrouter",
      "model_id": "openai/gpt-4o-mini",
      "temperature": 0.7,
      "max_tokens": 4096
    }
  },

  "fallbacks": {
    "default": [
      { "provider": "openrouter", "model_id": "anthropic/claude-3.5-sonnet" },
      { "provider": "openrouter", "model_id": "openai/gpt-4o" }
    ],
    "background": [
      { "provider": "openrouter", "model_id": "meta-llama/llama-3.3-70b-instruct" }
    ],
    "think": [
      { "provider": "openrouter", "model_id": "anthropic/claude-opus-4" }
    ],
    "complex": [
      { "provider": "openrouter", "model_id": "anthropic/claude-sonnet-4" }
    ],
    "long_context": [
      { "provider": "openrouter", "model_id": "anthropic/claude-sonnet-4" }
    ],
    "fast": [
      { "provider": "openrouter", "model_id": "openai/gpt-4o-mini" }
    ]
  },

  "openrouter": {
    "base_url": "https://openrouter.ai/api/v1/chat/completions",
    "api_key": "${ROUTATIC_PROXY_OPENROUTER_API_KEY}",
    "api_keys": [],
    "timeout_ms": 300000,
    "stream_timeout_ms": 60000,
    "streaming_timeout_ms": 600000
  },

  "logging": {
    "level": "info",
    "requests": true
  }
}
`
}

// getAWSBedrockConfig returns a config optimized for AWS Bedrock.
func getAWSBedrockConfig() string {
	return `{
  "api_key": "${ROUTATIC_PROXY_API_KEY}",
  "host": "127.0.0.1",
  "port": 3456,
  "hot_reload": false,
  "enable_streaming_scenario_routing": false,
  "respect_requested_model": false,

  "models": {
    "default": {
      "provider": "aws-bedrock",
      "model_id": "anthropic.claude-sonnet-4-20250514-v1:0",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "background": {
      "provider": "aws-bedrock",
      "model_id": "amazon.nova-lite-v1:0",
      "temperature": 0.5,
      "max_tokens": 2048
    },
    "think": {
      "provider": "aws-bedrock",
      "model_id": "anthropic.claude-sonnet-4-20250514-v1:0",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "complex": {
      "provider": "aws-bedrock",
      "model_id": "anthropic.claude-opus-4-20250514-v1:0",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "long_context": {
      "provider": "aws-bedrock",
      "model_id": "anthropic.claude-sonnet-4-20250514-v1:0",
      "temperature": 0.7,
      "max_tokens": 16384,
      "context_threshold": 80000
    },
    "fast": {
      "provider": "aws-bedrock",
      "model_id": "amazon.nova-lite-v1:0",
      "temperature": 0.7,
      "max_tokens": 4096
    }
  },

  "fallbacks": {
    "default": [
      { "provider": "aws-bedrock", "model_id": "anthropic.claude-3-5-sonnet-20241022-v2:0" },
      { "provider": "aws-bedrock", "model_id": "amazon.nova-pro-v1:0" }
    ],
    "background": [
      { "provider": "aws-bedrock", "model_id": "amazon.nova-micro-v1:0" }
    ],
    "think": [
      { "provider": "aws-bedrock", "model_id": "anthropic.claude-opus-4-20250514-v1:0" }
    ],
    "complex": [
      { "provider": "aws-bedrock", "model_id": "anthropic.claude-sonnet-4-20250514-v1:0" }
    ],
    "long_context": [
      { "provider": "aws-bedrock", "model_id": "anthropic.claude-sonnet-4-20250514-v1:0" }
    ],
    "fast": [
      { "provider": "aws-bedrock", "model_id": "amazon.nova-lite-v1:0" }
    ]
  },

  "aws_bedrock": {
    "base_url": "https://bedrock-mantle.us-east-1.api.aws/v1/chat/completions",
    "anthropic_base_url": "https://bedrock-mantle.us-east-1.api.aws/v1/messages",
    "api_key": "${ROUTATIC_PROXY_AWS_BEDROCK_API_KEY}",
    "api_keys": [],
    "project_id": "",
    "wire_format": "openai",
    "timeout_ms": 300000,
    "stream_timeout_ms": 60000,
    "streaming_timeout_ms": 600000
  },

  "logging": {
    "level": "info",
    "requests": true
  }
}
`
}

// getOpenCodeZenConfig returns a config optimized for OpenCode Zen.
func getOpenCodeZenConfig() string {
	return `{
  "api_key": "${ROUTATIC_PROXY_API_KEY}",
  "host": "127.0.0.1",
  "port": 3456,
  "hot_reload": false,
  "enable_streaming_scenario_routing": false,
  "respect_requested_model": false,

  "models": {
    "default": {
      "provider": "opencode-zen",
      "model_id": "claude-sonnet-4.5",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "background": {
      "provider": "opencode-zen",
      "model_id": "nemotron-3-ultra-free",
      "temperature": 0.5,
      "max_tokens": 2048
    },
    "think": {
      "provider": "opencode-zen",
      "model_id": "claude-opus-4-8",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "complex": {
      "provider": "opencode-zen",
      "model_id": "claude-opus-4-8",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "long_context": {
      "provider": "opencode-zen",
      "model_id": "gemini-3.1-pro",
      "temperature": 0.7,
      "max_tokens": 16384,
      "context_threshold": 80000
    },
    "fast": {
      "provider": "opencode-zen",
      "model_id": "gemini-3.5-flash",
      "temperature": 0.7,
      "max_tokens": 4096
    }
  },

  "fallbacks": {
    "default": [
      { "provider": "opencode-zen", "model_id": "claude-sonnet-4" },
      { "provider": "opencode-zen", "model_id": "gemini-3.1-pro" }
    ],
    "background": [
      { "provider": "opencode-zen", "model_id": "mimo-v2.5-free" },
      { "provider": "opencode-zen", "model_id": "deepseek-v4-flash-free" }
    ],
    "think": [
      { "provider": "opencode-zen", "model_id": "claude-opus-4-6" }
    ],
    "complex": [
      { "provider": "opencode-zen", "model_id": "claude-opus-4-5" }
    ],
    "long_context": [
      { "provider": "opencode-zen", "model_id": "claude-sonnet-4.5" }
    ],
    "fast": [
      { "provider": "opencode-zen", "model_id": "gemini-3-flash" }
    ]
  },

  "model_overrides": {
    "claude-fable-5": {
      "provider": "opencode-zen",
      "model_id": "claude-fable-5",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "claude-opus-4-8": {
      "provider": "opencode-zen",
      "model_id": "claude-opus-4-8",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "claude-sonnet-4.5": {
      "provider": "opencode-zen",
      "model_id": "claude-sonnet-4.5",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "gemini-3.5-flash": {
      "provider": "opencode-zen",
      "model_id": "gemini-3.5-flash",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "gemini-3.1-pro": {
      "provider": "opencode-zen",
      "model_id": "gemini-3.1-pro",
      "temperature": 0.7,
      "max_tokens": 8192
    }
  },

  "opencode_zen": {
    "base_url": "https://opencode.ai/zen/v1/chat/completions",
    "anthropic_base_url": "https://opencode.ai/zen/v1/messages",
    "responses_base_url": "https://opencode.ai/zen/v1/responses",
    "gemini_base_url": "https://opencode.ai/zen/v1/models",
    "api_key": "${ROUTATIC_PROXY_OPENCODE_ZEN_API_KEY}",
    "api_keys": [],
    "timeout_ms": 300000,
    "streaming_timeout_ms": 600000
  },

  "logging": {
    "level": "info",
    "requests": true
  }
}
`
}

// getOpenCodeGoConfig returns the default config optimized for OpenCode Go.
func getOpenCodeGoConfig() string {
	// This is the same as getDefaultConfig() but explicit for provider
	return `{
  "api_key": "${ROUTATIC_PROXY_API_KEY}",
  "host": "127.0.0.1",
  "port": 3456,
  "hot_reload": false,
  "enable_streaming_scenario_routing": false,
  "respect_requested_model": false,
  "anthropic_first": {
    "enabled": false,
    "base_url": "https://api.anthropic.com"
  },
  "models": {
    "background": {
      "provider": "opencode-go",
      "model_id": "deepseek-v4-flash",
      "temperature": 0.5,
      "max_tokens": 2048
    },
    "default": {
      "provider": "opencode-go",
      "model_id": "deepseek-v4-pro",
      "temperature": 0.7,
      "max_tokens": 8192,
      "reasoning_effort": "max",
      "thinking": { "type": "enabled" }
    },
    "long_context": {
      "provider": "opencode-go",
      "model_id": "minimax-m3",
      "temperature": 0.7,
      "max_tokens": 16384,
      "context_threshold": 80000
    },
    "think": {
      "provider": "opencode-go",
      "model_id": "glm-5.2",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "complex": {
      "provider": "opencode-go",
      "model_id": "deepseek-v4-pro",
      "temperature": 0.7,
      "max_tokens": 8192,
      "reasoning_effort": "max",
      "thinking": { "type": "enabled" }
    },
    "fast": {
      "provider": "opencode-go",
      "model_id": "deepseek-v4-flash",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "glm-5.2": {
      "provider": "opencode-go",
      "model_id": "glm-5.2",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "kimi-k2.7-code": {
      "provider": "opencode-go",
      "model_id": "kimi-k2.7-code",
      "temperature": 0.7,
      "max_tokens": 32768
    },
    "qwen3.7-plus": {
      "provider": "opencode-go",
      "model_id": "qwen3.7-plus",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "qwen3.7-max": {
      "provider": "opencode-go",
      "model_id": "qwen3.7-max",
      "temperature": 0.7,
      "max_tokens": 8192
    }
  },
  "fallbacks": {
    "background": [
      { "provider": "opencode-go", "model_id": "qwen3.7-plus" },
      { "provider": "opencode-go", "model_id": "qwen3.7-max" },
      { "provider": "opencode-zen", "model_id": "nemotron-3-ultra-free" },
      { "provider": "opencode-zen", "model_id": "mimo-v2.5-free" },
      { "provider": "opencode-zen", "model_id": "deepseek-v4-flash-free" }
    ],
    "default": [
      { "provider": "opencode-go", "model_id": "qwen3.7-plus" },
      { "provider": "opencode-go", "model_id": "qwen3.7-max" },
      { "provider": "opencode-zen", "model_id": "nemotron-3-ultra-free" },
      { "provider": "opencode-zen", "model_id": "mimo-v2.5-free" },
      { "provider": "opencode-zen", "model_id": "deepseek-v4-flash-free" }
    ],
    "long_context": [
      { "provider": "opencode-go", "model_id": "qwen3.7-plus" },
      { "provider": "opencode-go", "model_id": "qwen3.7-max" },
      { "provider": "opencode-zen", "model_id": "nemotron-3-ultra-free" },
      { "provider": "opencode-zen", "model_id": "mimo-v2.5-free" },
      { "provider": "opencode-zen", "model_id": "deepseek-v4-flash-free" }
    ],
    "think": [
      { "provider": "opencode-go", "model_id": "qwen3.7-plus" },
      { "provider": "opencode-go", "model_id": "qwen3.7-max" },
      { "provider": "opencode-zen", "model_id": "nemotron-3-ultra-free" },
      { "provider": "opencode-zen", "model_id": "mimo-v2.5-free" },
      { "provider": "opencode-zen", "model_id": "deepseek-v4-flash-free" }
    ],
    "complex": [
      { "provider": "opencode-go", "model_id": "qwen3.7-plus" },
      { "provider": "opencode-go", "model_id": "qwen3.7-max" },
      { "provider": "opencode-zen", "model_id": "nemotron-3-ultra-free" },
      { "provider": "opencode-zen", "model_id": "mimo-v2.5-free" },
      { "provider": "opencode-zen", "model_id": "deepseek-v4-flash-free" }
    ],
    "fast": [
      { "provider": "opencode-go", "model_id": "qwen3.7-plus" },
      { "provider": "opencode-go", "model_id": "qwen3.7-max" },
      { "provider": "opencode-zen", "model_id": "nemotron-3-ultra-free" },
      { "provider": "opencode-zen", "model_id": "mimo-v2.5-free" },
      { "provider": "opencode-zen", "model_id": "deepseek-v4-flash-free" }
    ],
    "glm-5.2": [
      { "provider": "opencode-go", "model_id": "glm-5.1" },
      { "provider": "opencode-go", "model_id": "kimi-k2.6" }
    ],
    "kimi-k2.7-code": [
      { "provider": "opencode-go", "model_id": "kimi-k2.6" },
      { "provider": "opencode-go", "model_id": "glm-5.1" }
    ],
    "qwen3.7-plus": [
      { "provider": "opencode-go", "model_id": "qwen3.6-plus" },
      { "provider": "opencode-go", "model_id": "kimi-k2.6" }
    ],
    "qwen3.7-max": [
      { "provider": "opencode-go", "model_id": "qwen3.7-plus" },
      { "provider": "opencode-go", "model_id": "kimi-k2.6" }
    ]
  },
  "model_overrides": {
    "deepseek-v4-pro": {
      "provider": "opencode-zen",
      "model_id": "deepseek-v4-pro",
      "temperature": 0.7,
      "max_tokens": 8192,
      "reasoning_effort": "max",
      "thinking": {
        "type": "enabled"
      }
    },
    "deepseek-v4-flash-free": {
      "provider": "opencode-zen",
      "model_id": "deepseek-v4-flash-free",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "grok-build-0.1": {
      "provider": "opencode-zen",
      "model_id": "grok-build-0.1",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "big-pickle": {
      "provider": "opencode-zen",
      "model_id": "big-pickle",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "mimo-v2.5-free": {
      "provider": "opencode-zen",
      "model_id": "mimo-v2.5-free",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "north-mini-code-free": {
      "provider": "opencode-zen",
      "model_id": "north-mini-code-free",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "nemotron-3-ultra-free": {
      "provider": "opencode-zen",
      "model_id": "nemotron-3-ultra-free",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "claude-fable-5": {
      "provider": "opencode-zen",
      "model_id": "claude-fable-5",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "claude-opus-4-8": {
      "provider": "opencode-zen",
      "model_id": "claude-opus-4-8",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "claude-opus-4-6": {
      "provider": "opencode-zen",
      "model_id": "claude-opus-4-6",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "claude-opus-4-5": {
      "provider": "opencode-zen",
      "model_id": "claude-opus-4-5",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "claude-opus-4-1": {
      "provider": "opencode-zen",
      "model_id": "claude-opus-4-1",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "claude-sonnet-4": {
      "provider": "opencode-zen",
      "model_id": "claude-sonnet-4",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "gemini-3.5-flash": {
      "provider": "opencode-zen",
      "model_id": "gemini-3.5-flash",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "gemini-3.1-pro": {
      "provider": "opencode-zen",
      "model_id": "gemini-3.1-pro",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "gemini-3-flash": {
      "provider": "opencode-zen",
      "model_id": "gemini-3-flash",
      "temperature": 0.7,
      "max_tokens": 8192
    }
  },
  "opencode_go": {
    "base_url": "https://opencode.ai/zen/go/v1/chat/completions",
    "anthropic_base_url": "https://opencode.ai/zen/go/v1/messages",
    "api_key": "",
    "api_keys": [],
    "timeout_ms": 300000
  },
  "opencode_zen": {
    "base_url": "https://opencode.ai/zen/v1/chat/completions",
    "anthropic_base_url": "https://opencode.ai/zen/v1/messages",
    "responses_base_url": "https://opencode.ai/zen/v1/responses",
    "gemini_base_url": "https://opencode.ai/zen/v1/models",
    "api_key": "",
    "api_keys": [],
    "timeout_ms": 300000
  },
  "logging": {
    "level": "info",
    "requests": true
  }
}
`
}
