# OpenCode Models Guide

Comprehensive guide to OpenCode Go and Zen models with capabilities, costs, and routing recommendations.

**Sources:** [OpenCode Go Documentation](https://opencode.ai/docs/go/) | [OpenCode Zen Documentation](https://opencode.ai/docs/zen/)

## Quick Cost Comparison

> 💰 **Cost-conscious routing matters!** Qwen3.5 Plus gives you 10,200 requests per $12, while GLM-5.1 gives you only 880 — that's **11.6x fewer requests** for the same budget.

| Model            | Provider      | Requests per $12 (5hr) | Cost Efficiency | Quality |
| ---------------- | ------------- | ---------------------- | --------------- | ------- |
| **Qwen3.5 Plus** | Go            | **10,200**             | ★★★★★           | ★★☆☆☆   |
| **MiniMax M2.5** | Go            | **6,300**              | ★★★★★           | ★★☆☆☆   |
| **MiniMax M2.7** | Go            | **3,400**              | ★★★★☆           | ★★★☆☆   |
| **Qwen3.6 Plus** | Go            | **3,300**              | ★★★★☆           | ★★★☆☆   |
| **MiMo-V2-Omni** | Go            | **2,150**              | ★★★☆☆           | ★★★☆☆   |
| **Kimi K2.5**    | Go            | **1,850**              | ★★☆☆☆           | ★★★★☆   |
| **MiMo-V2-Pro**  | Go            | **1,290**              | ★★☆☆☆           | ★★★★☆   |
| **Kimi K2.6**    | Go            | **~1,150**             | ★☆☆☆☆           | ★★★★★   |
| **GLM-5**        | Go            | **1,150**              | ★☆☆☆☆           | ★★★★☆   |
| **GLM-5.1**      | Go            | **880**                | ☆☆☆☆☆           | ★★★★★   |

## Providers

### OpenCode Go (`opencode-go`)

- Subscription-based ($5/month then $10/month)
- OpenAI Chat Completions and Anthropic Messages endpoints
- Best for: Most use cases, cost-effective models

### OpenCode Zen (`opencode-zen`)

- Pay-as-you-go pricing
- Additional endpoint formats: Responses (GPT), Gemini
- Best for: GPT models, Gemini models, premium Anthropic models

## Important: API Endpoints

⚠️ **Critical:** Not all models use the same API endpoint! oc-go-cc handles this automatically, but you should know:

### OpenCode Go Endpoints

| Models                                                                                                             | Endpoint                                         | Format                   |
| ------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------ | ------------------------ |
| GLM-5, GLM-5.1, Kimi K2.6, Kimi K2.5, MiMo-V2-Pro, MiMo-V2-Omni, Qwen3.5 Plus, Qwen3.6 Plus, DeepSeek V4 Pro/Flash | `https://opencode.ai/zen/go/v1/chat/completions` | OpenAI-compatible        |
| **MiniMax M2.5, MiniMax M2.7**                                                                                     | `https://opencode.ai/zen/go/v1/messages`         | **Anthropic-compatible** |

### OpenCode Zen Endpoints

| Models                                    | Endpoint                                     | Format                   |
| ----------------------------------------- | -------------------------------------------- | ------------------------ |
| MiniMax M2.5, MiniMax M2.7, GLM-5, GLM-5.1, Kimi K2.5, Kimi K2.6, Qwen3.5 Plus, Qwen3.6 Plus, DeepSeek V4 Flash | `https://opencode.ai/zen/v1/chat/completions` | OpenAI-compatible        |
| **Claude models, Qwen3.7 Max**            | `https://opencode.ai/zen/v1/messages`        | **Anthropic-compatible** |
| **GPT models** (gpt-5.5, gpt-5.4, etc.)  | `https://opencode.ai/zen/v1/responses`       | **OpenAI Responses**     |
| **Gemini models** (gemini-3.5-flash, etc.)| `https://opencode.ai/zen/v1/models/{id}`     | **Google Gemini**        |

**Why this matters:** MiniMax models expect Anthropic format natively. oc-go-cc detects MiniMax models and routes them to the correct endpoint automatically without transformation.

## Using OpenCode Zen

To use Zen models, set `"provider": "opencode-zen"` in your model config:

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

### Zen-Specific Models

Some models are only available through Zen:

- **GPT Models:** gpt-5.5, gpt-5.4, gpt-5.3-codex, gpt-5.2, gpt-5.1, gpt-5
- **Gemini Models:** gemini-3.5-flash, gemini-3.1-pro, gemini-3-flash
- **Claude Models:** claude-opus-4.8, claude-sonnet-4.6, etc.
- **Free Models:** big-pickle, deepseek-v4-flash-free, mimo-v2.5-free, nemotron-3-super-free

DeepSeek V4 Pro and Flash are OpenAI-compatible in OpenCode Go. oc-go-cc transforms Claude Code's Anthropic request into OpenAI Chat Completions format, including tools, tool results, thinking history, `reasoning_effort`, and `thinking`.

For Claude Code and OpenCode-style agent workflows, DeepSeek V4 supports max thinking mode with:

```json
{
  "model_id": "deepseek-v4-pro",
  "reasoning_effort": "max",
  "thinking": {
    "type": "enabled"
  }
}
```

Use `deepseek-v4-pro` for default, complex, thinking, and long-context routing. Use `deepseek-v4-flash` for fast, background, or subagent-style workloads.

## Cost-Conscious Routing Strategy

### Default to Cheap, Upgrade When Necessary

**Most requests should use cheap models.** Only upgrade to expensive models when:

1. **Task complexity demands it** (multi-step reasoning, architecture)
2. **You've tried cheaper models and they failed**
3. **Code quality is critical** (production code review)

### Recommended Routing

```json
{
  "models": {
    "background": {
      // Simple operations
      "model_id": "qwen3.5-plus",
      "max_tokens": 2048
    },
    "default": {
      // Better quality, moderate cost
      "model_id": "kimi-k2.6",
      "max_tokens": 4096
    },
    "long_context": {
      // Large files only
      "model_id": "minimax-m2.5",
      "context_threshold": 80000
    },
    "think": {
      // Reasoning tasks
      "model_id": "glm-5",
      "max_tokens": 8192
    },
    "complex": {
      // Complex architecture only
      "model_id": "glm-5.1",
      "max_tokens": 4096
    },
    "fast": {
      // Streaming requests (prioritize TTFT)
      "model_id": "qwen3.6-plus",
      "max_tokens": 4096
    }
  }
}
```

### Decision Tree

```
Is context > 80K tokens?
├── YES → Use MiniMax M2.5 (1M context, 6,300 req/$12)
│
Is it a complex task (architecture, refactoring, tool operations)?
├── YES → Use GLM-5.1 (880 req/$12)
│
Is it a reasoning/planning task?
├── YES → Use GLM-5 (1,150 req/$12)
│
Is it a simple background task (read file, grep, list dir, no tools)?
├── YES → Use Qwen3.5 Plus (10,200 req/$12)
│
Default → Use Kimi K2.6 (1,850 req/$12, ★★★★★) or Qwen3.6 Plus (3,300 req/$12)
```

## Detailed Model Profiles

### Budget Champions 💰

#### Qwen3.5 Plus — The Workhorse

- **Model ID:** `qwen3.5-plus`
- **Cost:** **10,200 requests per $12** (best value!)
- **Context:** ~128K tokens
- **Quality:** ★★☆☆☆ (adequate for simple tasks)
- **Best For:**
  - File reading operations
  - Directory listing
  - Grep/search
  - Simple questions
  - Bulk operations
  - Background tasks
- **When to Use:** When you need to do lots of operations cheaply

#### MiniMax M2.5 — Long Context on a Budget

- **Model ID:** `minimax-m2.5`
- **Endpoint:** **Anthropic-compatible** (`/v1/messages`)
- **Cost:** **6,300 requests per $12**
- **Context:** **~1M tokens** (1 million!)
- **Quality:** ★★☆☆☆ (acceptable)
- **Speed:** Fast
- **Best For:**
  - Very large files
  - Long conversations
  - Multi-file context
- **When to Use:** When you need 1M context but want to minimize cost
- **Note:** Uses Anthropic endpoint - oc-go-cc handles this automatically

### Balanced Models (Quality + Cost)

#### DeepSeek V4 Pro — Agentic Coding + Max Thinking

- **Model ID:** `deepseek-v4-pro`
- **Endpoint:** **OpenAI-compatible** (`/chat/completions`)
- **Context:** **~1M tokens**
- **Quality:** ★★★★★
- **Best For:**
  - Claude Code agent workflows
  - Complex implementation and debugging
  - Architecture and refactoring
  - Long-context coding tasks
  - Max thinking mode
- **Recommended Config:**

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

#### DeepSeek V4 Flash — Fast Agent Workloads

- **Model ID:** `deepseek-v4-flash`
- **Endpoint:** **OpenAI-compatible** (`/chat/completions`)
- **Context:** **~1M tokens**
- **Quality:** ★★★★☆
- **Best For:**
  - Fast routing
  - Background tasks
  - Subagent-style work
  - Fallback for DeepSeek V4 Pro
- **Recommended Config:**

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

#### Qwen3.6 Plus — Cost-Effective General Coding ⭐ RECOMMENDED DEFAULT

- **Model ID:** `qwen3.6-plus`
- **Cost:** **3,300 requests per $12** (3.8x more than GLM-5.1!)
- **Context:** ~128K tokens
- **Quality:** ★★★☆☆ (good enough for most tasks)
- **Speed:** Fast
- **Best For:**
  - General coding (default choice)
  - Feature implementation
  - Bug fixes
  - Refactoring
- **When to Use:** Default for cost-conscious users

#### Kimi K2.6 — Best Quality at Balanced Cost

- **Model ID:** `kimi-k2.6`
- **Cost:** **~1,850 requests per $12**
- **Context:** ~256K tokens (successor to K2.5 with improvements)
- **Quality:** ★★★★★ (excellent — successor improvements)
- **Speed:** Fast
- **Best For:**
  - Complex coding tasks
  - Code review
  - Architecture discussions
  - General-purpose default (best quality-to-cost ratio)
- **When to Use:** Default choice — better quality than K2.5 at similar cost

#### Kimi K2.5 — Quality + Reasonable Cost (Predecessor)

- **Model ID:** `kimi-k2.5`
- **Cost:** **1,850 requests per $12**
- **Context:** ~256K tokens (2x most others)
- **Quality:** ★★★★☆ (excellent)
- **Speed:** Fast
- **Best For:**
  - Complex coding tasks
  - Code review
  - Architecture discussions
  - When you need better quality than budget models
- **When to Use:** When quality matters more than maximum cost savings

### Premium Models (Use Sparingly!)

#### GLM-5 — Reasoning Specialist

- **Model ID:** `glm-5`
- **Cost:** **1,150 requests per $12** (9x more expensive than Qwen3.5 Plus!)
- **Context:** ~200K tokens
- **Quality:** ★★★★☆ (excellent)
- **Best For:**
  - Multi-step reasoning
  - Complex planning
  - Algorithm design
  - Difficult debugging
- **When to Use:** When reasoning/planning is required and budget models fail

#### GLM-5.1 — Maximum Quality

- **Model ID:** `glm-5.1`
- **Cost:** **880 requests per $12** (11.6x more expensive than Qwen3.5 Plus!)
- **Context:** ~200K tokens
- **Quality:** ★★★★★ (best available)
- **Speed:** Moderate
- **Best For:**
  - Critical architectural decisions
  - Complex multi-file refactoring
  - Production code review
  - When you need the absolute best quality
- **When to Use:** Only when cheaper models can't handle the task

## Usage Limits

OpenCode Go limits:

- **5-hour limit:** $12 of usage
- **Weekly limit:** $30 of usage
- **Monthly limit:** $60 of usage

### Cost Comparison Example

**Scenario:** You want to make 5,000 requests this month.

| Model        | Cost | Can you do it?        |
| ------------ | ---- | --------------------- |
| Qwen3.5 Plus | ~$6  | ✅ Yes, easily        |
| MiniMax M2.5 | ~$10 | ✅ Yes                |
| Qwen3.6 Plus | ~$18 | ✅ Yes                |
| Kimi K2.5    | ~$32 | ❌ Exceeds $30 weekly |
| GLM-5        | ~$52 | ❌ Exceeds limits     |
| GLM-5.1      | ~$68 | ❌ Exceeds limits     |

### Optimizing Your Usage

**Strategy 1: Tiered Approach**

```
1. Start with Qwen3.6 Plus (cheap, good quality)
2. If it fails, try Kimi K2.5 (better quality)
3. If still failing, use GLM-5 (reasoning)
4. Only for critical tasks: GLM-5.1 (premium)
```

**Strategy 2: Task-Based Selection**

```
Background ops (grep, ls, cat) → Qwen3.5 Plus
General coding → Qwen3.6 Plus or Kimi K2.5
Complex features → Kimi K2.5
Architecture/Planning → GLM-5
Critical review → GLM-5.1 (rarely)
```

## Fallback Chains for Cost Efficiency

```json
{
  "fallbacks": {
    "background": [
      { "model_id": "qwen3.6-plus" },
      { "model_id": "minimax-m2.5" }
    ],
    "long_context": [{ "model_id": "minimax-m2.5" }],
    "default": [{ "model_id": "mimo-v2-pro" }, { "model_id": "qwen3.6-plus" }],
    "think": [{ "model_id": "kimi-k2.6" }],
    "complex": [{ "model_id": "glm-5" }],
    "fast": [{ "model_id": "qwen3.5-plus" }, { "model_id": "minimax-m2.5" }]
  }
}
```

**Rule of thumb:** If a task succeeds with a cheap model, it doesn't need an expensive one. Only fall back to expensive models when necessary.

## Quick Reference

| Task Type             | Recommended  | Cost (req/$12) | Fallback     |
| --------------------- | ------------ | -------------- | ------------ |
| Read file, ls, grep   | Qwen3.5 Plus | 10,200         | Qwen3.6 Plus |
| General coding        | Qwen3.6 Plus | 3,300          | Kimi K2.5    |
| Complex features      | Kimi K2.6    | 1,850          | Kimi K2.5    |
| Long context (>80K)   | MiniMax M2.5 | 6,300          | MiniMax M2.7 |
| Reasoning/planning    | GLM-5        | 1,150          | Kimi K2.5    |
| Critical architecture | GLM-5.1      | 880            | GLM-5        |
| Bulk operations       | Qwen3.5 Plus | 10,200         | MiniMax M2.5 |

## Cost-Saving Tips

1. **Use Qwen3.6 Plus as default** — 3,300 req/$12 is plenty for most tasks
2. **Reserve GLM-5.1 for critical tasks only** — 880 req/$12 drains budget fast
3. **Use Qwen3.5 Plus for simple operations** — 10,200 req/$12 is unbeatable
4. **MiniMax M2.5 for long context** — 6,300 req/$12 with 1M context is amazing value
5. **Monitor your usage** in the [OpenCode console](https://opencode.ai/auth)

## See Also

- [OpenCode Go Documentation](https://opencode.ai/docs/go/)
- [oc-go-cc Configuration](../configs/config.example.json)
- [README.md](../README.md) for setup instructions
