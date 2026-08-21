# Supported Models

Complete model reference for routatic-proxy including OpenCode Go, Zen, OpenRouter, and deprecated models.

---

## OpenCode Go Models

Context and max-output values come from the built-in registry (`modelMetadata` in `internal/config/model_registry.go`).

| Model                     | Context      | Max output | Vision | Best For                                        |
| ------------------------- | ------------ | ---------- | ------ | ----------------------------------------------- |
| **DeepSeek V4 Pro**       | ~1M tokens   | 8192       | no     | Default + complex scenarios in the shipped config |
| **DeepSeek V4 Flash**     | ~1M tokens   | 4096       | no     | Background / fast scenarios, cheap and quick    |
| **GLM-5.2**               | ~200K tokens | 8192       | no     | Critical architecture, production code review   |
| **GLM-5.1**               | ~200K tokens | 8192       | no     | Complex patterns, tool-heavy operations         |
| **Kimi K3**               | ~1M tokens   | 131072     | yes    | Latest Kimi, code + agentic, huge max output    |
| **Kimi K2.7 Code**        | ~256K tokens | 32768      | yes    | Large code generation                           |
| **Kimi K2.6**             | ~256K tokens | 8192       | yes    | General purpose, common fallback                |
| **Kimi K2.5**             | ~256K tokens | 8192       | yes    | Previous-generation Kimi fallback               |
| **MiniMax M3**            | ~1M tokens   | 128000     | no     | Long-context scenario, very large output budget |
| **MiniMax M2.7**          | ~200K tokens | 8192       | no     | Previous MiniMax generation                     |
| **MiniMax M2.5**          | ~200K tokens | 4096       | no     | Older MiniMax generation                        |
| **MiMo V2.5 Pro**         | ~1M tokens   | 16384      | no     | Step-by-step reasoning, larger output           |
| **MiMo V2.5**             | ~1M tokens   | 8192       | no     | Step-by-step reasoning                          |
| **MiMo V2 Omni**          | ~1M tokens   | 8192       | yes    | Multimodal MiMo                                 |
| **Qwen3.7 Max**           | ~1M tokens   | 8192       | yes    | Complex coding, Qwen's best quality             |
| **Qwen3.7 Plus**          | ~1M tokens   | 8192       | yes    | General coding, better quality than Qwen3.6     |
| **Qwen3.6 Plus**          | ~1M tokens   | 8192       | yes    | Streaming fallback                              |
| **Qwen3.5 Plus**          | ~1M tokens   | 8192       | yes    | Simple read-only ops                            |

All registry models support tool use. The registry is provider-agnostic — the free variants (`deepseek-v4-flash-free`, `mimo-v2.5-free`) are wired to Zen in the shipped config.

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

Model IDs, context windows, and pricing change often, so this page does not keep its own copy. See [openrouter.md](./openrouter.md) ("Model Examples") for the model-key list plus complete OpenRouter setup and configuration, and [openrouter.ai/models](https://openrouter.ai/models) for authoritative live pricing and context limits.

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
