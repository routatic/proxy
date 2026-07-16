# Supported Models

Complete model reference for routatic-proxy including OpenCode Go, Zen, OpenRouter, and deprecated models.

---

## OpenCode Go Models

| Model              | Context      | Best For                                      |
| ------------------ | ------------ | --------------------------------------------- |
| **GLM-5.2**        | ~200K tokens | Critical architecture, production code review |
| **Kimi K2.7 Code** | ~256K tokens | Large code generation, 32K max output         |
| **Qwen3.7 Plus**   | ~128K tokens | General coding, better quality than Qwen3.6   |
| **Qwen3.7 Max**    | ~128K tokens | Complex coding, Qwen's best quality           |

See [MODELS.md](../MODELS.md) for the complete model list including costs and routing recommendations.

---

## OpenCode Zen Models

Zen provides pay-as-you-go access to additional models:

- **Claude Models**: Claude Fable 5, Claude Opus 4.8/4.6/4.5/4.1, Claude Sonnet 4
- **Gemini Models**: Gemini 3.5 Flash, Gemini 3.1 Pro, Gemini 3 Flash
- **GPT Models**: GPT 5.5, GPT 5.4, GPT 5.3 Codex, and more
- **Free Tier**: Nemotron 3 Ultra Free, MiMo V2.5 Free, DeepSeek V4 Flash Free, and others

See [MODELS.md](../MODELS.md#opencodes-zen) for the full Zen model list.

---

## OpenRouter Models

OpenRouter provides unified access to 100+ models from multiple providers through a single API endpoint.

### Popular Models

| Model | Provider | Context Window | Input Cost ($/M) | Output Cost ($/M) | Best For |
|-------|----------|----------------|------------------|-------------------|----------|
| **Claude 3.5 Sonnet** | Anthropic | 200K | $3.00 | $15.00 | Complex reasoning, coding, analysis |
| **Claude 3 Opus** | Anthropic | 200K | $15.00 | $75.00 | Maximum quality, difficult tasks |
| **GPT-4o** | OpenAI | 128K | $2.50 | $10.00 | General purpose, vision tasks |
| **GPT-4o Mini** | OpenAI | 128K | $0.15 | $0.60 | Cost-effective, high volume |
| **Gemini 2.5 Pro** | Google | 1M | $1.25 | $10.00 | Long context, coding, reasoning |
| **Gemini 2.0 Flash** | Google | 1M | $0.10 | $0.40 | Fast responses, cost efficiency |
| **Llama 3.3 70B** | Meta | 128K | $0.12 | $0.30 | Open source, customizable |
| **Mistral Large** | Mistral | 128K | $2.00 | $6.00 | European provider, GDPR compliant |
| **DeepSeek V3** | DeepSeek | 64K | $0.07 | $1.10 | Cost efficiency, coding |

See [docs/openrouter.md](./openrouter.md) for complete OpenRouter setup and configuration.

---

## Deprecated Models

The following models are deprecated and will be removed:

| Model | Deprecation Date | Replacement |
|-------|------------------|-------------|
| GPT 5.2/5.1/5 Codex variants | July 23, 2026 | GPT 5.3 Codex |
| Claude Sonnet 4 | June 15, 2026 | Claude Sonnet 4.5/4.6 |
| GLM 5 | May 14, 2026 | GLM 5.1/5.2 |
| MiniMax M2.1 | March 15, 2026 | MiniMax M2.5/M2.7/M3 |
| Gemini 3 Pro | March 9, 2026 | Gemini 3.1 Pro |
| Kimi K2/K2 Thinking | March 6, 2026 | Kimi K2.5/K2.6/K2.7 Code |

See [MODELS.md](../MODELS.md#deprecated-zen-models) for the complete deprecation schedule.
