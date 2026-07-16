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
      "model_id": "kimi-k2.6",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "background": {
      "provider": "opencode-go",
      "model_id": "qwen3.5-plus",
      "temperature": 0.5,
      "max_tokens": 2048
    },
    "think": {
      "provider": "opencode-go",
      "model_id": "glm-5.1",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "complex": {
      "provider": "opencode-go",
      "model_id": "glm-5.1",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "long_context": {
      "provider": "opencode-go",
      "model_id": "minimax-m2.7",
      "temperature": 0.7,
      "max_tokens": 16384,
      "context_threshold": 80000
    },
    "fast": {
      "provider": "opencode-go",
      "model_id": "qwen3.6-plus",
      "temperature": 0.7,
      "max_tokens": 4096
    }
  },

  "fallbacks": {
    "default": [
      { "provider": "opencode-go", "model_id": "glm-5" },
      { "provider": "opencode-go", "model_id": "qwen3.6-plus" }
    ],
    "think": [{ "provider": "opencode-go", "model_id": "glm-5" }],
    "complex": [{ "provider": "opencode-go", "model_id": "glm-5" }],
    "long_context": [{ "provider": "opencode-go", "model_id": "minimax-m2.5" }],
    "fast": [{ "provider": "opencode-go", "model_id": "qwen3.5-plus" }]
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

| 场景 | 触发条件 | 模型 | 原因 |
|------|----------|------|------|
| **长上下文** | >80K tokens（可配置） | MiniMax M2.7 | 1M 上下文窗口 vs 其他 128-256K |
| **复杂** | 系统提示包含 "architect"、"refactor"、"complex" | GLM-5.1 | 最佳推理和架构理解 |
| **思考** | 系统提示包含 "think"、"plan"、"reason" | GLM-5 | 良好的推理，比 GLM-5.1 便宜 |
| **后台** | "read file"、"grep"、"list directory" | Qwen3.5 Plus | 最便宜（~10K 请求/5小时），适合简单操作 |
| **默认** | 其他所有 | Kimi K2.6 | 质量与成本的最佳平衡（~1.8K 请求/5小时） |

**详细模型能力、成本和路由建议请参见 [MODELS.md](MODELS.md)。**

DeepSeek V4 用户可以将任何场景模型设置为 `deepseek-v4-pro` 或 `deepseek-v4-flash`。对于确定性最大思考，在该场景的模型配置和降级条目中添加 `reasoning_effort: "max"` 和 `thinking: {"type":"enabled"}`。

### 路由详情

| 场景 | 触发条件 | 配置键 | 默认模型 |
|------|----------|--------|----------|
| **默认** | 标准聊天 | `models.default` | `kimi-k2.6` |
| **思考** | 系统提示包含 "think"、"plan"、"reason"；或思考内容块 | `models.think` | `glm-5.1` |
| **长上下文** | Token 数超过 `context_threshold` | `models.long_context` | `minimax-m2.7` |
| **后台** | 文件读取、目录列表、grep 模式 | `models.background` | `qwen3.5-plus` |

路由优先级：**长上下文** > **思考** > **后台** > **默认**

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
