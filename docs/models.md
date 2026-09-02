# Supported Models

The full model reference lives in **[MODELS.md](../MODELS.md)** at the repository
root: every model with its context window, max output tokens, vision and tool
support, endpoint, pricing, and the scenario each one is suited to.

This file used to carry a second, shorter copy of those tables. Two copies meant
two places to drift, and they did — a correctness pass once fixed the capability
numbers here while leaving the same errors in `MODELS.md`. There is now one
source of truth for prose, and one for data:

| Looking for | Go to |
|-------------|-------|
| Model capabilities, pricing, endpoints, recommendations | [MODELS.md](../MODELS.md) |
| Chinese translation | [docs/zh/MODELS.md](zh/MODELS.md) |
| The values the proxy actually enforces at runtime | `modelMetadata` in `internal/config/model_registry.go` |
| Which endpoint a model is routed to | `internal/models/classifier.go` |
| Adding a new model | [howto-add-model.md](howto-add-model.md) |
| OpenRouter specifics | [openrouter.md](openrouter.md) |

`modelMetadata` is authoritative: when a doc and the registry disagree, the
registry is right and the doc is a bug.
