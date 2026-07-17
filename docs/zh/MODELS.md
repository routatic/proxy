# OpenCode 模型指南

[English](../../MODELS.md) | **中文**

OpenCode Go 和 Zen 模型的综合指南，包括能力、成本和路由建议。

**来源：** [OpenCode Go 文档](https://opencode.ai/docs/go/) | [OpenCode Zen 文档](https://opencode.ai/docs/zen/)

## 快速成本对比

> 💰 **注重成本的路由很重要！** Qwen3.5 Plus 让你用 $12 获得 10,200 次请求，而 GLM-5.1 只有 880 次 —— 同样的预算少了 **11.6 倍** 的请求。

| 模型 | 提供商 | 每 $12 请求数 (5小时) | 成本效率 | 质量 |
|------|--------|------------------------|----------|------|
| **Qwen3.5 Plus** | Go | **10,200** | ★★★★★ | ★★☆☆☆ |
| **MiniMax M2.5** | Go | **6,300** | ★★★★★ | ★★☆☆☆ |
| **Qwen3.7 Plus** | Go | **4,300** | ★★★★★ | ★★★☆☆ |
| **MiniMax M2.7** | Go | **3,400** | ★★★★☆ | ★★★☆☆ |
| **MiniMax M3** | Go | **3,200** | ★★★★☆ | ★★★☆☆ |
| **Qwen3.6 Plus** | Go | **3,300** | ★★★★☆ | ★★★☆☆ |
| **MiMo-V2.5** | Go | **2,150** | ★★★☆☆ | ★★★☆☆ |
| **MiMo-V2.5-Pro** | Go | **1,290** | ★★☆☆☆ | ★★★★☆ |
| **Kimi K2.5** | Go | **1,850** | ★★☆☆☆ | ★★★★☆ |
| **Kimi K2.6** | Go | **~1,150** | ★☆☆☆☆ | ★★★★★ |
| **Kimi K2.7 Code** | Go | **1,350** | ★☆☆☆☆ | ★★★★★ |
| **Kimi K3** | Go | **$3/$15 每 1M** | ☆☆☆☆☆ | ★★★★★ |
| **GLM-5** | Go | **1,150** | ★☆☆☆☆ | ★★★★☆ |
| **GLM-5.1** | Go | **880** | ☆☆☆☆☆ | ★★★★★ |
| **GLM-5.2** | Go | **880** | ☆☆☆☆☆ | ★★★★★ |
| **Qwen3.7 Max** | Go | **950** | ☆☆☆☆☆ | ★★★★☆ |

## 提供商

### OpenCode Go (`opencode-go`)

- 订阅制（$5/月，之后 $10/月）
- OpenAI Chat Completions 和 Anthropic Messages 端点
- 最适合：大多数用例，性价比高的模型

### OpenCode Zen (`opencode-zen`)

- 按使用量付费
- 额外端点格式：Responses (GPT)、Gemini
- 最适合：GPT 模型、Gemini 模型、高级 Anthropic 模型

### AWS Bedrock (`aws-bedrock`)

- 在 AWS Bedrock Mantle 上托管的模型
- 支持 OpenAI Chat Completions（默认）和 Anthropic Messages 格式
- 为 Claude 和其他 Anthropic 原生模型设置 `wire_format: "anthropic"`
- 最适合：部署在自己 AWS 基础设施上的模型

## 重要：API 端点

⚠️ **关键：** 不是所有模型都使用相同的 API 端点！routatic-proxy 自动处理这个问题，但你应该了解：

### OpenCode Go 端点

| 模型 | 端点 | 格式 |
|------|------|------|
| GLM-5, GLM-5.1, GLM-5.2, Kimi K2.5, Kimi K2.6, Kimi K2.7 Code, Kimi K3, MiMo-V2.5, MiMo-V2.5-Pro, DeepSeek V4 Pro, DeepSeek V4 Flash | `https://opencode.ai/zen/go/v1/chat/completions` | OpenAI 兼容 |
| **MiniMax M2.5, MiniMax M2.7, MiniMax M3, Qwen3.5 Plus, Qwen3.6 Plus, Qwen3.7 Plus, Qwen3.7 Max** | `https://opencode.ai/zen/go/v1/messages` | **Anthropic 兼容** |

### OpenCode Zen 端点

| 模型 | 端点 | 格式 |
|------|------|------|
| MiniMax, GLM, Kimi, DeepSeek, 免费层模型 | `https://opencode.ai/zen/v1/chat/completions` | OpenAI 兼容 |
| **Claude 模型**, **Qwen 模型** | `https://opencode.ai/zen/v1/messages` | **Anthropic 兼容** |
| **GPT 模型** | `https://opencode.ai/zen/v1/responses` | **OpenAI Responses** |
| **Gemini 模型** | `https://opencode.ai/zen/v1/models/{id}` | **Google Gemini** |

**为什么这很重要：** 在 Go 提供商上，MiniMax 和 Qwen 模型原生使用 Anthropic 格式。在 Zen 上，只有 Claude 和 Qwen 使用 Anthropic 端点 —— MiniMax 使用 chat completions。routatic-proxy 自动处理所有路由。

## 使用 OpenCode Zen

要使用 Zen 模型，在模型配置中设置 `"provider": "opencode-zen"`：

```json
{
  "models": {
    "default": {
      "provider": "opencode-zen",
      "model_id": "kimi-k2.6",
      "temperature": 0.7,
      "max_tokens": 4096
    }
  }
}
```

### Zen 专用模型（共 50+ 个）

所有 OpenCode Go 模型也可在 Zen 上使用。Zen 还额外提供：

- **Claude 模型（Anthropic 端点）：** claude-fable-5, claude-opus-4-8, claude-opus-4-7, claude-opus-4-6, claude-opus-4-5, claude-opus-4-1, claude-sonnet-4-6, claude-sonnet-4-5, claude-sonnet-4, claude-haiku-4-5, claude-3-5-haiku
- **GPT 模型（Responses 端点）：** gpt-5.5, gpt-5.5-pro, gpt-5.4, gpt-5.4-pro, gpt-5.4-mini, gpt-5.4-nano, gpt-5.3-codex 等
- **Gemini 模型（Gemini 端点）：** gemini-3.5-flash, gemini-3.1-pro, gemini-3-flash
- **免费层（chat completions）：** deepseek-v4-pro, deepseek-v4-flash-free, grok-build-0.1, big-pickle, mimo-v2.5-free, north-mini-code-free, nemotron-3-ultra-free

#### 已弃用的 Zen 模型

以下模型已弃用，将被移除：

| 模型 | 弃用日期 | 替代模型 |
|------|----------|----------|
| GPT 5.2 Codex | 2026年7月23日 | GPT 5.3 Codex |
| GPT 5.1 Codex | 2026年7月23日 | GPT 5.3 Codex |
| GPT 5.1 Codex Max | 2026年7月23日 | GPT 5.3 Codex |
| GPT 5.1 Codex Mini | 2026年7月23日 | GPT 5.3 Codex Spark |
| GPT 5 Codex | 2026年7月23日 | GPT 5.3 Codex |
| Claude Sonnet 4 | 2026年6月15日 | Claude Sonnet 4.5/4.6 |
| GLM 5 | 2026年5月14日 | GLM 5.1/5.2 |
| MiniMax M2.1 | 2026年3月15日 | MiniMax M2.5/M2.7/M3 |
| GLM 4.7 | 2026年3月15日 | GLM 5/5.1/5.2 |
| GLM 4.6 | 2026年3月15日 | GLM 5/5.1/5.2 |
| Gemini 3 Pro | 2026年3月9日 | Gemini 3.1 Pro |
| Kimi K2 Thinking | 2026年3月6日 | Kimi K2.5/K2.6/K2.7 Code |
| Kimi K2 | 2026年3月6日 | Kimi K2.5/K2.6/K2.7 Code |
| Claude Haiku 3.5 | 2026年2月16日 | Claude Haiku 4.5 |
| Qwen3 Coder 480B | 2026年2月6日 | Qwen3.7 Plus/Max |

DeepSeek V4 Pro 和 Flash 在 Go 和 Zen 提供商上都是 OpenAI 兼容的。DeepSeek V4 Flash Free 是免费的 Zen 变体。routatic-proxy 将 Claude Code 的 Anthropic 请求转换为 OpenAI Chat Completions 格式，包括工具、工具结果、思考历史、`reasoning_effort` 和 `thinking`。

对于 Claude Code 和 OpenCode 风格的代理工作流，DeepSeek V4 支持最大思考模式：

```json
{
  "model_id": "deepseek-v4-pro",
  "reasoning_effort": "max",
  "thinking": {
    "type": "enabled"
  }
}
```

使用 `deepseek-v4-pro` 用于默认、复杂、思考和长上下文路由。使用 `deepseek-v4-flash` 用于快速、后台或子代理风格的工作负载。

要通过 Zen（免费层）而不是 Go（付费）路由 DeepSeek V4 Pro，添加一个 `model_overrides` 条目：

```json
{
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
    }
  }
}
```

## 注重成本的路由策略

### 默认便宜，必要时升级

**大多数请求应该使用便宜的模型。** 只有在以下情况才升级到昂贵模型：

1. **任务复杂度要求**（多步推理、架构）
2. **你尝试过便宜模型但失败了**
3. **代码质量至关重要**（生产代码审查）

### 推荐路由

```json
{
  "models": {
    "background": {
      // 简单操作
      "model_id": "qwen3.5-plus",
      "max_tokens": 2048
    },
    "default": {
      // 更好质量，中等成本
      "model_id": "kimi-k2.6",
      "max_tokens": 4096
    },
    "long_context": {
      // 仅大文件
      "model_id": "minimax-m2.5",
      "context_threshold": 80000
    },
    "think": {
      // 推理任务
      "model_id": "glm-5",
      "max_tokens": 8192
    },
    "complex": {
      // 仅复杂架构
      "model_id": "glm-5.1",
      "max_tokens": 4096
    },
    "fast": {
      // 流式请求（优先 TTFT）
      "model_id": "qwen3.6-plus",
      "max_tokens": 4096
    }
  }
}
```

### 决策树

```
上下文是否 > 80K tokens？
├── 是 → 使用 MiniMax M2.5（1M 上下文，6,300 请求/$12）
│
是否是复杂任务（架构、重构、工具操作）？
├── 是 → 使用 GLM-5.1（880 请求/$12）
│
是否是推理/规划任务？
├── 是 → 使用 GLM-5（1,150 请求/$12）
│
是否是简单后台任务（读文件、grep、列目录、无工具）？
├── 是 → 使用 Qwen3.5 Plus（10,200 请求/$12）
│
默认 → 使用 Kimi K2.6（1,850 请求/$12，★★★★★）或 Qwen3.6 Plus（3,300 请求/$12）
```

## 详细模型简介

### 性价比之王 💰

#### Qwen3.5 Plus —— 工作马

- **模型 ID：** `qwen3.5-plus`
- **成本：** **每 $12 10,200 次请求**（最佳性价比！）
- **上下文：** ~128K tokens
- **质量：** ★★☆☆☆（适合简单任务）
- **最适合：**
  - 文件读取操作
  - 目录列表
  - Grep/搜索
  - 简单问题
  - 批量操作
  - 后台任务
- **何时使用：** 当你需要大量操作且成本低廉时

#### MiniMax M2.5 —— 预算长上下文

- **模型 ID：** `minimax-m2.5`
- **端点：** **Anthropic 兼容**（Go 上 `/v1/messages`），**OpenAI 兼容**（Zen 上 `/chat/completions`）
- **成本：** **每 $12 6,300 次请求**
- **上下文：** **~1M tokens**（100 万！）
- **质量：** ★★☆☆☆（可接受）
- **速度：** 快
- **最适合：**
  - 超大文件
  - 长对话
  - 多文件上下文
- **何时使用：** 当你需要 1M 上下文但想最小化成本时

#### MiniMax M3 —— 最新 MiniMax，1M 上下文

- **模型 ID：** `minimax-m3`
- **端点：** **Anthropic 兼容**（Go 上 `/v1/messages`），**OpenAI 兼容**（Zen 上 `/chat/completions`）
- **上下文：** **~1M tokens**
- **质量：** ★★★☆☆
- **最适合：**
  - 需要比 M2.5 更好质量的长上下文任务
  - 大型代码库分析
  - 文档处理
- **何时使用：** 当你需要 1M 上下文且想要比 M2.5 更好的质量时

### 平衡模型（质量 + 成本）

#### DeepSeek V4 Pro —— 代理编码 + 最大思考

- **模型 ID：** `deepseek-v4-pro`
- **端点：** **OpenAI 兼容**（`/chat/completions`）
- **上下文：** **~1M tokens**
- **质量：** ★★★★★
- **提供商：** Go（付费）或 Zen（免费层）
- **最适合：**
  - Claude Code 代理工作流
  - 复杂实现和调试
  - 架构和重构
  - 长上下文编码任务
  - 最大思考模式
- **推荐配置（Go）：**

  ```json
  {
    "provider": "opencode-go",
    "model_id": "deepseek-v4-pro",
    "temperature": 0.1,
    "max_tokens": 8192,
    "reasoning_effort": "max",
    "thinking": {
      "type": "enabled"
    }
  }
  ```

- **推荐配置（Zen 免费层）：**

  ```json
  {
    "provider": "opencode-zen",
    "model_id": "deepseek-v4-pro",
    "temperature": 0.1,
    "max_tokens": 8192,
    "reasoning_effort": "max",
    "thinking": {
      "type": "enabled"
    }
  }
  ```

#### DeepSeek V4 Flash —— 快速代理工作负载

- **模型 ID：** `deepseek-v4-flash`
- **端点：** **OpenAI 兼容**（`/chat/completions`）
- **上下文：** **~1M tokens**
- **质量：** ★★★★☆
- **最适合：**
  - 快速路由
  - 后台任务
  - 子代理风格工作
  - DeepSeek V4 Pro 的降级备选
- **推荐配置：**

  ```json
  {
    "provider": "opencode-go",
    "model_id": "deepseek-v4-flash",
    "temperature": 0.1,
    "max_tokens": 4096,
    "reasoning_effort": "max",
    "thinking": {
      "type": "enabled"
    }
  }
  ```

#### Qwen3.6 Plus —— 经济高效的通用编码 ⭐ 推荐默认

- **模型 ID：** `qwen3.6-plus`
- **端点：** **Anthropic 兼容**（`/v1/messages` —— Go），**Anthropic 兼容**（`/v1/messages` —— Zen）
- **成本：** **每 $12 3,300 次请求**（比 GLM-5.1 多 3.8 倍！）
- **上下文：** ~128K tokens
- **质量：** ★★★☆☆（对大多数任务足够好）
- **速度：** 快
- **最适合：**
  - 通用编码（默认选择）
  - 功能实现
  - Bug 修复
  - 重构
- **何时使用：** 注重成本用户的默认选择

#### Qwen3.7 Plus —— 升级版通用编码

- **模型 ID：** `qwen3.7-plus`
- **端点：** **Anthropic 兼容**（`/v1/messages`）
- **成本：** **每 $12 4,300 次请求**
- **上下文：** ~128K tokens
- **质量：** ★★★★☆
- **速度：** 快
- **最适合：**
  - 比 Qwen3.6 质量更好的通用编码
  - 功能实现
  - Bug 修复
- **何时使用：** 当你想要比 Qwen3.6 更好的质量且速度相近时

#### Qwen3.7 Max —— 最大质量 Qwen

- **模型 ID：** `qwen3.7-max`
- **端点：** **Anthropic 兼容**（`/v1/messages`）
- **成本：** **每 $12 950 次请求**
- **上下文：** ~128K tokens
- **质量：** ★★★★☆
- **最适合：**
  - 复杂编码任务
  - 当 Qwen3.7 Plus 不够时
- **何时使用：** 当你需要 Qwen 的最佳质量时

#### Kimi K2.5 —— 质量 + 合理成本（前代）

- **模型 ID：** `kimi-k2.5`
- **成本：** **每 $12 1,850 次请求**
- **上下文：** ~256K tokens（是大多数模型的两倍）
- **质量：** ★★★★☆（优秀）
- **速度：** 快
- **最适合：**
  - 复杂编码任务
  - 代码审查
  - 架构讨论
  - 当你需要比预算模型更好的质量时
- **何时使用：** 当质量比最大成本节省更重要时

#### Kimi K2.6 —— 平衡成本下的最佳质量

- **模型 ID：** `kimi-k2.6`
- **成本：** **每 $12 ~1,850 次请求**
- **上下文：** ~256K tokens
- **质量：** ★★★★★（优秀）
- **速度：** 快
- **最适合：**
  - 复杂编码任务
  - 代码审查
  - 架构讨论
  - 通用默认（最佳质量成本比）
- **何时使用：** 默认选择 —— 比 K2.5 更好的质量，成本相近

### 高级模型（谨慎使用！）

#### GLM-5 —— 推理专家

- **模型 ID：** `glm-5`
- **成本：** **每 $12 1,150 次请求**（比 Qwen3.5 Plus 贵 9 倍！）
- **上下文：** ~200K tokens
- **质量：** ★★★★☆（优秀）
- **最适合：**
  - 多步推理
  - 复杂规划
  - 算法设计
  - 困难调试
- **何时使用：** 当需要推理/规划且预算模型失败时

#### GLM-5.1 —— 最高质量

- **模型 ID：** `glm-5.1`
- **成本：** **每 $12 880 次请求**（比 Qwen3.5 Plus 贵 11.6 倍！）
- **上下文：** ~200K tokens
- **质量：** ★★★★★（最佳）
- **速度：** 中等
- **最适合：**
  - 关键架构决策
  - 复杂多文件重构
  - 生产代码审查
  - 当你需要绝对最佳质量时
- **何时使用：** 只有当便宜模型无法处理任务时

#### GLM-5.2 —— 最新高级模型

- **模型 ID：** `glm-5.2`
- **成本：** **每 $12 880 次请求**（与 GLM-5.1 相同）
- **上下文：** ~200K tokens
- **质量：** ★★★★★（最佳）
- **速度：** 中等
- **最适合：**
  - 相比 5.1 有改进的最新 GLM 模型
  - 关键架构决策
  - 复杂多文件重构
  - 生产代码审查
- **何时使用：** 使用此模型替代 GLM-5.1 以获得最新改进

#### Kimi K2.7 Code —— 代码专家

- **模型 ID：** `kimi-k2.7-code`
- **成本：** **每 $12 1,350 次请求**
- **上下文：** ~256K tokens
- **质量：** ★★★★★（代码任务优秀）
- **最大输出：** 32K tokens（最高可用！）
- **速度：** 快
- **最适合：**
  - 大型代码生成任务
  - 需要长输出的复杂重构
  - 详细反馈的代码审查
- **何时使用：** 当你需要高质量和超长输出（最多 32K）时

## 使用限制

OpenCode Go 限制：

- **5 小时限制：** $12 使用量
- **每周限制：** $30 使用量
- **每月限制：** $60 使用量

### 成本比较示例

**场景：** 你本月想发起 5,000 次请求。

| 模型 | 成本 | 能做到吗？ |
|------|------|------------|
| Qwen3.5 Plus | ~$6 | ✅ 可以，轻松 |
| MiniMax M2.5 | ~$10 | ✅ 可以 |
| Qwen3.6 Plus | ~$18 | ✅ 可以 |
| Kimi K2.5 | ~$32 | ❌ 超过 $30 每周 |
| GLM-5 | ~$52 | ❌ 超过限制 |
| GLM-5.1 | ~$68 | ❌ 超过限制 |

### 优化你的使用

**策略 1：分层方法**

```
1. 从 Qwen3.6 Plus 开始（便宜，质量好）
2. 如果失败，尝试 Kimi K2.5（质量更好）
3. 如果仍然失败，使用 GLM-5（推理）
4. 仅用于关键任务：GLM-5.1（高级）
```

**策略 2：基于任务的选择**

```
后台操作（grep、ls、cat）→ Qwen3.5 Plus
通用编码 → Qwen3.6 Plus 或 Kimi K2.5
复杂功能 → Kimi K2.5
架构/规划 → GLM-5
关键审查 → GLM-5.1（很少）
```

## 成本效率降级链

```json
{
  "fallbacks": {
    "background": [
      { "model_id": "qwen3.6-plus" },
      { "model_id": "minimax-m2.5" }
    ],
    "long_context": [{ "model_id": "minimax-m2.7" }],
    "default": [{ "model_id": "mimo-v2.5-pro" }, { "model_id": "qwen3.6-plus" }],
    "think": [{ "model_id": "kimi-k2.6" }],
    "complex": [{ "model_id": "glm-5" }],
    "fast": [{ "model_id": "qwen3.5-plus" }, { "model_id": "minimax-m2.5" }]
  }
}
```

**经验法则：** 如果任务用便宜模型就能成功，就不需要昂贵的模型。仅在必要时降级到昂贵模型。

## 快速参考

| 任务类型 | 推荐 | 成本（请求/$12） | 降级 |
|----------|------|-------------------|------|
| 读文件、ls、grep | Qwen3.5 Plus | 10,200 | Qwen3.6 Plus |
| 通用编码 | Qwen3.7 Plus | 4,300 | Qwen3.6 Plus |
| 复杂功能 | Kimi K2.6 | 1,850 | MiMo-V2.5-Pro |
| 长上下文（>80K）| MiniMax M2.5 | 6,300 | MiniMax M2.7 |
| 推理/规划 | GLM-5 | 1,150 | Kimi K2.6 |
| 关键架构 | GLM-5.2 | 880 | GLM-5.1 |
| 代码专家 | Kimi K2.7 Code | 1,350 | Kimi K2.6 |
| 批量操作 | Qwen3.5 Plus | 10,200 | MiniMax M2.5 |

## 省钱技巧

1. **将 Qwen3.6 Plus 作为默认** — 3,300 请求/$12 对大多数任务足够
2. **仅在关键任务使用 GLM-5.1** — 880 请求/$12 快速消耗预算
3. **简单操作使用 Qwen3.5 Plus** — 10,200 请求/$12 无敌
4. **长上下文使用 MiniMax M2.5** — 6,300 请求/$12 加 1M 上下文性价比惊人
5. **非关键任务使用 Zen 免费层模型** — deepseek-v4-pro、grok-build-0.1、big-pickle 等 $0
6. **在 [OpenCode 控制台](https://opencode.ai/auth) 监控使用量**

## 另请参阅

- [OpenCode Go 文档](https://opencode.ai/docs/go/)
- [routatic-proxy 配置](../configs/config.example.json)
- [README.md](../README.md) 获取设置说明
