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
| GLM-5, GLM-5.1, GLM-5.2, Kimi K2.5, Kimi K2.6, Kimi K2.7 Code, MiMo-V2.5, MiMo-V2.5-Pro, DeepSeek V4 Pro, DeepSeek V4 Flash | `https://opencode.ai/zen/go/v1/chat/completions` | OpenAI 兼容 |
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
