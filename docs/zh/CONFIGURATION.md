# 配置指南

[English](../../CONFIGURATION.md) | **中文**

## 配置文件

位置：`~/.config/routatic-proxy/config.json`

可通过 `ROUTATIC_PROXY_CONFIG` 环境变量覆盖。

为了迁移兼容，当新配置文件不存在时会加载 `~/.config/oc-go-cc/config.json`，所有 `OC_GO_CC_*` 环境变量仍然作为其 `ROUTATIC_PROXY_*` 替换项的备选。

## 完整配置参考

```json
{
  "api_key": "${ROUTATIC_PROXY_API_KEY}",
  "host": "127.0.0.1",
  "port": 3456,
  "hot_reload": false,
  "anthropic_first": {
    "enabled": false,
    "base_url": "https://api.anthropic.com"
  },

  "enable_cost_based_routing": false,
  "cost_routing": {
    "enabled": true,
    "prefer_providers": ["opencode-go", "aws-bedrock"],
    "max_context_window": 1000000,
    "penalty_per_provider": {
      "openrouter": 0.05
    }
  },

  "models": {
    "default": {
      "provider": "opencode-go",
      "model_id": "deepseek-v4-pro",
      "temperature": 0.7,
      "max_tokens": 8192,
      "reasoning_effort": "max",
      "thinking": { "type": "enabled" }
    },
    "background": {
      "provider": "opencode-go",
      "model_id": "deepseek-v4-flash",
      "temperature": 0.5,
      "max_tokens": 2048
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
    "long_context": {
      "provider": "opencode-go",
      "model_id": "minimax-m3",
      "temperature": 0.7,
      "max_tokens": 16384,
      "context_threshold": 80000
    },
    "fast": {
      "provider": "opencode-go",
      "model_id": "deepseek-v4-flash",
      "temperature": 0.7,
      "max_tokens": 4096
    }
  },

  "fallbacks": {
    "default": [
      { "provider": "opencode-go", "model_id": "qwen3.7-plus" },
      { "provider": "opencode-go", "model_id": "qwen3.7-max" },
      { "provider": "opencode-zen", "model_id": "nemotron-3-ultra-free" },
      { "provider": "opencode-zen", "model_id": "mimo-v2.5-free" },
      { "provider": "opencode-zen", "model_id": "deepseek-v4-flash-free" }
    ],
    "think": [{ "provider": "opencode-go", "model_id": "qwen3.7-plus" }],
    "complex": [{ "provider": "opencode-go", "model_id": "qwen3.7-plus" }],
    "long_context": [{ "provider": "opencode-go", "model_id": "qwen3.7-plus" }],
    "fast": [{ "provider": "opencode-go", "model_id": "qwen3.7-plus" }]
  },

  "model_overrides": {
    "claude-sonnet-4.5": {
      "provider": "opencode-zen",
      "model_id": "claude-sonnet-4.5",
      "temperature": 0.7,
      "max_tokens": 8192,
      "vision": true
    },
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
    }
  },

  "opencode_go": {
    "base_url": "https://opencode.ai/zen/go/v1/chat/completions",
    "anthropic_base_url": "https://opencode.ai/zen/go/v1/messages",
    "timeout_ms": 300000
  },

  "opencode_zen": {
    "base_url": "https://opencode.ai/zen/v1/chat/completions",
    "anthropic_base_url": "https://opencode.ai/zen/v1/messages",
    "responses_base_url": "https://opencode.ai/zen/v1/responses",
    "gemini_base_url": "https://opencode.ai/zen/v1/models",
    "timeout_ms": 300000
  },

  "logging": {
    "level": "info",
    "requests": true
  }
}
```

## Anthropic 优先故障切换

启用此模式以保持 Anthropic 作为 Claude Code 的主要 API，仅在 Anthropic 不可用时使用配置的 OpenCode 模型链：

```json
{
  "anthropic_first": {
    "enabled": true,
    "base_url": "https://api.anthropic.com"
  }
}
```

仅使用代理地址配置 Claude Code：

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:3456
unset ANTHROPIC_AUTH_TOKEN ANTHROPIC_API_KEY
```

保持凭证变量未设置可保留已保存的 Claude Pro、Max、Team 或 Enterprise 登录信息。代理将原始请求、OAuth 凭证、`anthropic-version` 和完整的 `anthropic-beta` 能力头转发给 Anthropic。

故障切换在响应开始前对 HTTP 408、429、5xx 和传输失败触发。HTTP 400、401、403、404 和其他请求错误原样返回。失败后，代理遵循 `Retry-After`；否则使用从 30 秒到 15 分钟的指数退避。一个真实的用户请求会探测恢复，同时并发请求继续通过 OpenCode。不会发送合成的健康检查请求。

一旦响应字节开始传输，失败的流无法在其他模型上重新启动而不重复内容。`/v1/messages/count_tokens` 保持本地处理，不影响可用性状态。

当 OpenCode Go 返回 `GoUsageLimitError` 时，该请求跳过剩余的 Go 模型，链前进到 Zen。默认链使用 Qwen3.7 Plus、Qwen3.7 Max，然后是当前可用的 Zen 免费 Nemotron 3 Ultra、MiMo V2.5 和 DeepSeek V4 Flash 模型。免费的 Zen 端点有时间限制，可能根据 [OpenCode 的文档隐私条款](https://opencode.ai/docs/zen/#privacy) 保留数据。

## 提供商

routatic-proxy 支持三个提供商进行上游 API 调用：

### OpenCode Go (`opencode-go`)

- 大多数模型的默认提供商
- 使用 OpenAI Chat Completions 和 Anthropic Messages 端点
- 定价：$5/月订阅 + 按使用量计费

### OpenCode Zen (`opencode-zen`)

- 精选的、经过测试的模型，按使用量付费
- 支持额外的端点格式：
  - **Chat Completions** (`/v1/chat/completions`) — OpenAI 兼容模型
  - **Anthropic Messages** (`/v1/messages`) — Claude、Qwen 模型
  - **OpenAI Responses** (`/v1/responses`) — GPT 模型
  - **Google Gemini** (`/v1/models/{id}`) — Gemini 模型
- 在模型配置中设置 `"provider": "opencode-zen"` 使用 Zen

### AWS Bedrock (`aws-bedrock`)

- 在 AWS Bedrock Mantle 上托管的模型
- 支持两种传输格式：
  - **OpenAI Chat Completions** (`/v1/chat/completions`) — 默认，适用于大多数模型
  - **Anthropic Messages** (`/v1/messages`) — 用于 Claude 和其他 Anthropic 原生模型
- 支持 `OpenAI-Project` 头进行基于项目的路由
- Bedrock 专用 API key 未设置时回退到全局密钥池
- 在模型配置中设置 `"provider": "aws-bedrock"` 使用 Bedrock

```json
{
  "aws_bedrock": {
    "base_url": "https://bedrock-mantle.us-east-1.api.aws/v1/chat/completions",
    "anthropic_base_url": "https://bedrock-mantle.us-east-1.api.aws/v1/messages",
    "api_key": "${BEDROCK_API_KEY}",
    "project_id": "proj_xxx",
    "wire_format": "openai",
    "timeout_ms": 300000,
    "stream_timeout_ms": 60000,
    "streaming_timeout_ms": 600000
  }
}
```

对于需要原始 Anthropic Messages 格式的模型（如 Bedrock 上的 Claude），设置 `wire_format: "anthropic"`。需要配置 `anthropic_base_url`。

### OpenRouter (`openrouter`)

- 统一 API，可访问来自多个提供商（OpenAI、Anthropic、Google、Meta、Mistral 等）的 200+ 模型
- 使用 OpenAI Chat Completions API 格式
- 按使用量付费，费率有竞争力
- 在模型配置中设置 `"provider": "openrouter"` 使用 OpenRouter

#### 配置结构

```json
{
  "openrouter": {
    "name": "openrouter",
    "base_url": "https://openrouter.ai/api/v1",
    "api_key": "${OPENROUTER_API_KEY}",
    "api_keys": ["${OPENROUTER_KEY_1}", "${OPENROUTER_KEY_2}"],
    "enabled": true,
    "timeout_ms": 300000,
    "stream_timeout_ms": 60000
  }
}
```

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `name` | `string` | 否 | 提供商显示名称（默认为 "openrouter"） |
| `base_url` | `string` | 否 | API 端点基础 URL。默认值：`https://openrouter.ai/api/v1` |
| `api_key` | `string` | 是* | 用于认证的单个 API key。若未设置 `api_keys` 则必需 |
| `api_keys` | `string[]` | 是* | 用于轮询轮换的多个 API key。若未设置 `api_key` 则必需 |
| `enabled` | `bool` | 否 | 此提供商是否启用。默认值：`true` |
| `timeout_ms` | `int` | 否 | 请求超时（毫秒）。默认值：`300000`（5 分钟） |
| `stream_timeout_ms` | `int` | 否 | 流式传输期间的分块超时。默认值：`60000`（1 分钟） |

*`api_key` 和 `api_keys` 中至少要配置一个。

#### 环境变量覆盖

| 变量 | 描述 | 优先级 |
|------|------|--------|
| `ROUTATIC_PROXY_OPENROUTER_API_KEY` | 单个 API key 覆盖 | 最高 |
| `ROUTATIC_PROXY_OPENROUTER_API_KEYS` | 用于轮询的逗号分隔密钥 | 最高 |
| `ROUTATIC_PROXY_OPENROUTER_BASE_URL` | 自定义 base URL 覆盖 | 最高 |

环境变量优先于配置文件值。配置值支持 `${VAR}` 插值。

优先级顺序：`*_API_KEYS` → `*_API_KEY` → 配置文件 `api_keys` → 配置文件 `api_key`

#### 配置示例

**单密钥设置：**

```json
{
  "openrouter": {
    "api_key": "sk-or-v1-xxxxxxxxxxxxxxxxxxxxxxxx"
  }
}
```

**多密钥轮询以实现负载均衡：**

```json
{
  "openrouter": {
    "api_keys": [
      "sk-or-v1-key-1",
      "sk-or-v1-key-2",
      "sk-or-v1-key-3"
    ]
  }
}
```

**自定义 base URL（用于企业/自托管）：**

```json
{
  "openrouter": {
    "base_url": "https://openrouter.mycompany.com/api/v1",
    "api_key": "${OPENROUTER_API_KEY}",
    "enabled": true
  }
}
```

#### 与基于成本的路由集成

OpenRouter 可与 `cost_routing` 无缝配合。使用 `penalty_per_provider` 调整有效成本：

```json
{
  "cost_routing": {
    "enabled": true,
    "prefer_providers": ["openrouter", "opencode-go"],
    "max_context_window": 1000000,
    "penalty_per_provider": {
      "openrouter": 0.02,
      "opencode-go": 0.0,
      "aws-bedrock": 0.05
    }
  }
}
```

惩罚值累加到原始模型成本上。例如：OpenRouter 上成本为 $0.10/1M tokens 的模型，加上 0.02 的惩罚后有效成本为 $0.12/1M tokens。用它在不完全排除提供商的情况下调整路由偏好。

#### 通过目录解析模型

模型使用 `provider/model-name` 模式引用。OpenRouter 模型使用 `openrouter/` 前缀：

```json
{
  "model_overrides": {
    "claude-opus-4": {
      "provider": "openrouter",
      "model_id": "anthropic/claude-opus-4",
      "temperature": 0.7,
      "max_tokens": 8192,
      "vision": true
    },
    "gpt-4o": {
      "provider": "openrouter",
      "model_id": "openai/gpt-4o",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "gemini-2.5-pro": {
      "provider": "openrouter",
      "model_id": "google/gemini-2.5-pro-preview-07-11",
      "temperature": 0.7,
      "max_tokens": 8192
    }
  }
}
```

**发现模型：**

1. 访问 [openrouter.ai/models](https://openrouter.ai/models) 查看完整模型列表
2. 使用 `routatic-proxy models` 命令查看已缓存的目录条目
3. 查阅 [OpenRouter API 文档](https://openrouter.ai/docs) 了解定价和上下文限制

配置中的 `model_id` 必须与 OpenRouter 的模型标识符完全一致（例如 `anthropic/claude-opus-4`、`openai/gpt-4o`、`google/gemini-2.5-pro-preview-07-11`）。

#### 使用场景

**访问特定模型：** 当你需要其他提供商上没有的模型时使用 OpenRouter：

```json
{
  "models": {
    "complex": {
      "provider": "openrouter",
      "model_id": "anthropic/claude-opus-4",
      "temperature": 0.7,
      "max_tokens": 8192,
      "reasoning_effort": "max"
    }
  }
}
```

**降级链：** 在主要提供商失败时把 OpenRouter 作为降级项：

```json
{
  "fallbacks": {
    "default": [
      { "provider": "opencode-go", "model_id": "deepseek-v4-pro" },
      { "provider": "openrouter", "model_id": "anthropic/claude-sonnet-4.8" },
      { "provider": "openrouter", "model_id": "openai/gpt-4.1" }
    ]
  }
}
```

**成本优化：** 结合 `cost_routing` 和提供商惩罚，自动选择可用的最便宜模型：

```json
{
  "cost_routing": {
    "enabled": true,
    "prefer_providers": ["openrouter"],
    "penalty_per_provider": {
      "openrouter": -0.01
    }
  }
}
```

**专用模型：** 为特定任务访问小众模型：

```json
{
  "models": {
    "think": {
      "provider": "openrouter",
      "model_id": "deepseek/deepseek-r1-free",
      "temperature": 0.6,
      "max_tokens": 8192
    },
    "long_context": {
      "provider": "openrouter",
      "model_id": "google/gemini-1.5-pro",
      "temperature": 0.7,
      "max_tokens": 16384,
      "context_threshold": 80000
    }
  }
}
```

## 环境变量

环境变量覆盖配置文件值。配置值也支持 `${VAR}` 插值。

| 变量 | 描述 | 默认值 |
|------|------|--------|
| `ROUTATIC_PROXY_API_KEY` | OpenCode Go API key（**必需**） | — |
| `ROUTATIC_PROXY_CONFIG` | 自定义配置文件路径 | `~/.config/routatic-proxy/config.json` |
| `ROUTATIC_PROXY_HOST` | 代理监听主机 | `127.0.0.1` |
| `ROUTATIC_PROXY_PORT` | 代理监听端口 | `3456` |
| `ROUTATIC_PROXY_OPENCODE_URL` | OpenCode Go API 端点 | `https://opencode.ai/zen/go/v1/chat/completions` |
| `ROUTATIC_PROXY_OPENCODE_ZEN_URL` | OpenCode Zen API 端点 | `https://opencode.ai/zen/v1/chat/completions` |
| `ROUTATIC_PROXY_OPENROUTER_API_KEY` | OpenRouter 单个 API key | — |
| `ROUTATIC_PROXY_OPENROUTER_API_KEYS` | OpenRouter 密钥池（逗号分隔） | — |
| `ROUTATIC_PROXY_LOG_LEVEL` | 日志级别：`debug`、`info`、`warn`、`error` | `info` |

旧版等效变量如 `OC_GO_CC_API_KEY`、`OC_GO_CC_CONFIG` 和 `OC_GO_CC_PORT` 继续工作。当两者都设置时，`ROUTATIC_PROXY_*` 值优先。

## 热重载

默认情况下，配置更改需要重启服务器。启用热重载以监视配置文件变化并自动应用：

```json
{
  "hot_reload": true
}
```

启用后，代理监视配置目录的变化（处理通过重命名/创建保存的编辑器）并自动重新加载配置。你也可以通过向进程发送 `SIGHUP` 来触发手动重载：

```bash
kill -HUP <PID>
```

## 模型路由

代理自动检测请求类型，并根据上下文大小和内容分析路由到适当的模型：

| 场景 | 触发条件 | 默认模型 | 原因 |
|------|----------|----------|------|
| **长上下文** | >100K tokens（可配置） | `minimax-m3` | 1M 上下文窗口 |
| **视觉** | 最新的用户消息包含图像 | （未预先配置） | 拆分为 `vision` / `vision_complex` |
| **复杂** | 系统提示包含 "architect"、"refactor"、"complex" | `deepseek-v4-pro` | 最佳推理和架构理解 |
| **思考** | 系统提示包含 "think"、"plan"、"reason" | `glm-5.2` | 以较低成本获得强推理能力 |
| **后台** | "read file"、"grep"、"list directory" | `deepseek-v4-flash` | 便宜，适合简单操作 |
| **默认** | 其他所有 | `deepseek-v4-pro` | 质量与成本的最佳平衡 |

**详细模型能力、成本和路由建议请参见 [MODELS.md](MODELS.md)。**

DeepSeek V4 用户可以将任何场景模型设置为 `deepseek-v4-pro` 或 `deepseek-v4-flash`。对于确定性最大思考，在该场景的模型配置和降级条目中添加 `reasoning_effort: "max"` 和 `thinking: {"type":"enabled"}`。

### 路由详情

| 场景 | 触发条件 | 配置键 | 默认模型 |
|------|----------|--------|----------|
| **默认** | 标准聊天 | `models.default` | `deepseek-v4-pro` |
| **复杂** | 架构相关关键词或工具密集型操作 | `models.complex` | `deepseek-v4-pro` |
| **思考** | 系统提示包含 "think"、"plan"、"reason"；或思考内容块 | `models.think` | `glm-5.2` |
| **长上下文** | Token 数超过 `context_threshold`（默认 100K） | `models.long_context` | `minimax-m3` |
| **视觉** | 最新的用户消息包含图像 | `models.vision` | （未预先配置） |
| **后台** | 文件读取、目录列表、grep 模式 | `models.background` | `deepseek-v4-flash` |

路由优先级：**长上下文** > **视觉** > **复杂** > **思考** > **后台** > **默认**

## 基于成本的路由

启用后，代理使用模型定价目录自动为每个场景选择最便宜的合格模型，而非始终使用静态配置的主模型。

```json
{
  "cost_routing": {
    "enabled": true,
    "prefer_providers": ["opencode-go", "aws-bedrock"],
    "max_context_window": 1000000,
    "penalty_per_provider": {
      "openrouter": 0.05
    }
  }
}
```

| 字段 | 类型 | 描述 |
|------|------|------|
| `enabled` | `bool` | 激活基于成本的模型选择。也可通过旧版顶层 `enable_cost_based_routing` 标志设置。 |
| `prefer_providers` | `string[]` | 全局限制候选提供商。设置后，仅考虑这些提供商上的模型。与每场景 `preferred_providers` 交集处理。 |
| `max_context_window` | `int64` | 候选模型上下文窗口的硬上限。超过此大小的模型将被排除。`0`（默认）表示无上限。 |
| `penalty_per_provider` | `map[string]float64` | 按提供商的成本惩罚，在选择时加到有效成本上。用于在不完全移除提供商的情况下使其吸引力降低。 |

启用后，`SelectCheapest` 会解析匹配场景下所有符合条件的提供商/模型组合，应用最大上下文窗口上限，按首选提供商集合过滤，并按有效成本（原始成本 + 惩罚）排序。最便宜的候选者胜出。这会取代静态的 `models.<scenario>` 主模型。

```json
{
  "cost_routing": {
    "penalty_per_provider": {
      "opencode-go": 0.1,
      "openrouter": 0.05
    }
  }
}
```

惩罚值累加到原始成本上。`opencode-go` 上基础成本为 2.0 的模型加上 0.1 的惩罚后，有效成本为 2.1。

## 降级链

当模型请求失败（网络错误、速率限制、服务器错误）时，代理尝试降级链中的下一个模型：

```
主模型 -> 降级 1 -> 降级 2 -> ... -> 错误（全部失败）
```

每个模型还有一个**熔断器**，跟踪连续失败次数。3 次失败后，熔断器打开，该模型被跳过 30 秒，然后再次测试（半开状态）。

## 模型覆盖（`model_overrides`）

`model_overrides` 让你将特定的客户端请求模型名称（`/v1/messages` 中 `model` 字段的值）映射到固定的 `ModelConfig`。当你想让客户端能够请求特定模型（如 `claude-sonnet-4.5`）而不让该模型经过场景路由器时，这很有用。

当请求到达时，代理**首先**检查 `model_overrides[<model>]`。如果请求的模型有条目，则使用覆盖作为主模型。降级链是 `fallbacks[<model>]`，如果没有覆盖特定条目则回退到 `fallbacks["default"]`。场景路由链然后作为**安全网降级**追加（按 `model_id` 去重）。

```json
{
  "model_overrides": {
    "claude-sonnet-4.5": {
      "provider": "opencode-zen",
      "model_id": "claude-sonnet-4.5",
      "temperature": 0.7,
      "max_tokens": 8192,
      "vision": true
    },
    "deepseek-v4-pro": {
      "provider": "opencode-zen",
      "model_id": "deepseek-v4-pro",
      "temperature": 0.7,
      "max_tokens": 8192,
      "reasoning_effort": "max",
      "thinking": {
        "type": "enabled"
      }
    }
  }
}
```

每个条目接受与 `ModelConfig` 相同的字段（`provider`、`model_id`、`temperature`、`max_tokens`、`reasoning_effort`、`thinking` 等）。`model_id` 是必需的；`provider` 必须是 `"opencode-go"`、`"opencode-zen"` 或 `"aws-bedrock"`（或省略以继承默认值）。

运行 `routatic-proxy models` 查看所有端点类型（Claude、GPT、Gemini 和免费层）的完整 Zen 模型列表。

### 路由优先级

当请求到达时，代理使用以下顺序选择模型链：

1. **`model_overrides[<model>]`** — 如果请求的 `model` 字段有条目，使用它作为主模型，并追加场景链作为安全网。
2. **`respect_requested_model`** — 如果启用且 `models[<model>]` 已配置，使用请求的模型和默认降级。
3. **场景路由** — 回退到场景链（`default`、`background`、`think`、`complex`、`long_context`、`fast`）。

> **信任模型：** 任何请求通过代理的客户端都可以从配置的 `model_overrides` 集合中选择，无需额外认证。如果你将代理作为共享服务运行，请将 `model_overrides` 视为特权白名单。

### 流式场景路由

`enable_streaming_scenario_routing` 控制流式请求是否经过完整场景路由器评估，或直接路由到 `fast` 场景。

> **Claude Code `/review-code`、`/ultracode` 和多代理工作流注意事项**
>
> 如果你使用 Claude Code 工作流，该工作流会派发多个子代理或产生多个并行工具调用，请启用流式场景路由：
>
> ```json
> {
>   "enable_streaming_scenario_routing": true
> }
> ```
>
> 如果没有此选项，流式请求会被路由到 `fast` 场景，即使请求实际上是工具密集型的。这可能导致复杂的 Claude Code 工作负载（如带有许多 `Agent` 工具调用的 `/review-code`）被路由到一个可能无法可靠处理并行工具调用编排的快速模型。
>
> 启用后，流式请求与非流式请求经过相同的场景路由器评估，允许大型或工具密集型工作负载使用 `complex` 或 `long_context` 模型，而不是总是使用 `fast` 模型。

Claude Code 审查工作流推荐配置：

```json
{
  "enable_streaming_scenario_routing": true,
  "models": {
    "fast": {
      "provider": "opencode-go",
      "model_id": "deepseek-v4-flash",
      "max_tokens": 4096
    },
    "complex": {
      "provider": "opencode-go",
      "model_id": "minimax-m3",
      "max_tokens": 8192
    },
    "long_context": {
      "provider": "opencode-go",
      "model_id": "minimax-m3",
      "max_tokens": 16384,
      "context_threshold": 80000
    }
  }
}
```

将 `fast` 场景用于短/简单请求。将 `complex` 或 `long_context` 用于代码审查、多代理派发、大型差异、许多工具或长上下文 Claude Code 会话。

## Claude Code 模型选择器

你可以通过两种方式从 Claude Code 的 `/model` 选择器中选择代理模型。

### 直接输入任意模型名称（始终可用）

Claude Code 的 `/model` 选择器也接受自由格式的模型名称。输入任何代理能理解的值——场景别名（`default`、`fast`、`complex` 等）、`model_overrides` 键，或像 `opencode-go/kimi-k2.6` 这样的目录规范名称——代理都会完成路由。无需额外配置；无论 Claude Code 版本如何，此方式均有效。

### 网关模型发现（可选启用，会向选择器添加条目）

较新版本的 Claude Code 可以通过查询代理的 [`GET /v1/models`](../reference-api.md#get-v1models) 端点自动填充选择器。启用后，发现的模型会与内置条目（Sonnet、Opus 等）一起出现在 `/model` 中，并标记为 **"From gateway"**。

在设置 `ANTHROPIC_BASE_URL` 的同时按如下方式启用：

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:3456
export ANTHROPIC_AUTH_TOKEN=unused
export CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1
```

只有在以下条件全部满足时才会运行发现：已设置 `ANTHROPIC_BASE_URL`、`CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1`、未设置任何 `CLAUDE_CODE_USE_*` 提供商变量、base URL 不是 `api.anthropic.com`，以及 Claude Code 版本支持该功能（≥ 2.1.129）。结果会缓存到 `~/.claude/cache/gateway-models.json`。

> **重要 —— Claude Code 会过滤发现到的模型 ID。** Claude Code 只显示 `id` 以 **`claude`** 或 **`anthropic`** 开头的发现模型。因此代理的场景别名（`default`、`fast` 等）和目录名称（`opencode-go/kimi-k2.6`）会**被选择器过滤掉**。要让某个代理模型通过发现出现，请给它一个 `claude-*` 名称——最自然的做法是使用 [`model_overrides`](#模型覆盖model_overrides) 键：
>
> ```json
> {
>   "model_overrides": {
>     "claude-glm-5.2": { "provider": "opencode-go", "model_id": "glm-5.2" }
>   }
> }
> ```
>
> 之后 `claude-glm-5.2` 就会出现在选择器中（标记为 "From gateway"），选中它会路由到 GLM-5.2。ID 不以 `claude`/`anthropic` 开头的模型仍然完全可用——只需直接在 `/model` 中输入即可。

## 配合 CC-Switch 使用

[CC-Switch](https://github.com/farion1231/cc-switch) 是一个用于管理和热切换 Claude Code 提供商的桌面应用。routatic-proxy 开箱即可与它配合——代理讲的正是 Claude Code（因而也是 CC-Switch）本来就期待的 Anthropic API，所以你可以像添加任何其他自定义提供商一样添加它。

### 将 routatic-proxy 添加为自定义提供商

1. 启动代理：`routatic-proxy serve`（默认监听地址 `http://127.0.0.1:3456`）。
2. 在 CC-Switch 中，点击 **Add Provider → Custom** 并填写：

   | CC-Switch 字段 | 值 |
   |----------------|-----|
   | **Name** | `routatic-proxy`（任意标签） |
   | **Endpoint URL** | `http://127.0.0.1:3456` |
   | **API Key** | 任意非空值（例如 `unused`）—— 见下方说明 |

   CC-Switch 会把这些写入 Claude Code 的配置：

   ```json
   {
     "env": {
       "ANTHROPIC_BASE_URL": "http://127.0.0.1:3456",
       "ANTHROPIC_AUTH_TOKEN": "unused"
     }
   }
   ```

   这正是代理所依赖的那两个环境变量——与 [README](../../README-zh.md) 中手动快速上手部分使用的相同。
3. **启用** 该提供商。Claude Code 会热重载它，因此无需重启。

> **关于 API Key 字段：** `ANTHROPIC_AUTH_TOKEN` 中的令牌是 Claude Code 发送给*代理*的内容，而不是代理向上游发送的内容。你真正的上游密钥存放在代理自己的配置（`opencode_go.api_key`、`openrouter.api_key` 等）或环境变量（`ROUTATIC_PROXY_*`）中。如果你在代理配置中设置了 `api_key` / `api_keys`，该值必须与 CC-Switch 发送的一致；如果你没有设置代理端认证，则任意非空令牌均可。

### 配置特定模型

你有两种方式控制经由 CC-Switch 选择的请求运行在哪个模型上：

- **让 Claude Code 选择并予以尊重** —— 当 `respect_requested_model: true`（默认值）时，代理会使用 Claude Code 发送的任何模型字符串，并对照你的 `models` 配置和目录进行解析。设为 `false` 可强制使用场景路由，忽略请求的模型。
- **固定一个模型别名** —— 使用 [`model_overrides`](#模型覆盖model_overrides) 把客户端可见的模型名称映射到固定的上游模型。例如，请求 `claude-sonnet-4.5` 可以路由到你选择的任意提供商/模型。

### CC-Switch 的 "Fetch Models" 按钮

CC-Switch 的自定义提供商表单有一个 **Fetch Models** 按钮，它调用 OpenAI 风格的 `GET /v1/models` 端点来填充模型下拉列表。代理实现了此端点：它返回你可以请求的每一个模型标识符——配置中的 `models` 别名、`model_overrides` 键，以及目录规范名称（`provider/model`）。参见 [docs/reference-api.md](../reference-api.md#get-v1models)。

如果下拉列表看起来很短，通常意味着模型目录尚未同步到本地存储；场景别名（`default`、`fast`、`complex` 等）和任何 `model_overrides` 键始终会出现。

### 故障排查

- **CC-Switch 报告提供商不可达** —— 确认代理正在运行（`routatic-proxy status`），且端点 URL/端口与代理配置中的 `host`/`port` 一致。
- **代理返回 401 / 认证错误** —— CC-Switch 发送的令牌必须满足代理的 `api_key` / `api_keys`（或这些项未设置）。这是代理端的认证，与你的上游提供商密钥无关。
- **运行了错误的模型** —— 检查路由优先级：`model_overrides` 优先，然后是 `respect_requested_model`，最后是场景路由。参见 [路由优先级](#路由优先级)。
