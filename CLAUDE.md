# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build   # Build binary to bin/routatic-proxy (CGO disabled by default)
make run     # Run without building
make test    # Run tests with race detector
make lint    # go vet + test
make clean   # Remove build artifacts
make install # Build and install to $GOPATH/bin
make dist    # Cross-compile for all platforms

# Start proxy with dashboard (recommended)
./bin/routatic-proxy start

# Start proxy only (headless)
./bin/routatic-proxy serve
```

### Architecture

**routatic-proxy start** runs both the proxy server and GUI dashboard:
- Proxy listens on `127.0.0.1:3456` (configurable)
- Dashboard at `http://127.0.0.1:3445`
- Usage data persists to SQLite (`~/.local/share/routatic-proxy/data.db`) regardless of dashboard state
- Press Ctrl+C to stop both servers

**routatic-proxy serve** runs headless (no dashboard).

## Architecture

**Purpose:** routatic-proxy is a proxy server that sits between Claude Code and OpenCode Go. It intercepts Anthropic API requests, transforms them to OpenAI Chat Completions format, forwards them to OpenCode Go, and transforms responses back to Anthropic SSE.

**Model routing is config-driven, not code-driven.** All models are defined in `~/.config/routatic-proxy/config.json` — adding a new model requires no code changes. Go provider models are transformed to OpenAI Chat Completions format automatically. Zen models use endpoint classification via `ClassifyEndpoint()`. The router in `internal/router/` selects models by matching request content against scenario patterns defined in `scenarios.go`.

If a model's upstream doesn't support Anthropic tool format (`type: "custom"` server-tool shorthands), set `"anthropic_tools_disabled": true` in the model config to force it through the Chat Completions transform path instead of the raw Anthropic endpoint.

**Two API endpoints:**

- OpenAI endpoint (`/v1/chat/completions`) — used by most models (GLM, Kimi, MiMo, Qwen)
- Anthropic endpoint (`/v1/messages`) — used only by MiniMax models

**Available models:**

| Model | Provider | Type | Best For |
|-------|----------|------|----------|
| GLM-5.2 | Go | Premium | Complex reasoning, architecture decisions (new) |
| GLM-5.1 | Go | Standard | Complex patterns, tool operations |
| GLM-5 | Go | Standard | Reasoning tasks (deprecated May 14, 2026) |
| Kimi K2.7 Code | Go | Code specialist | Code generation, 32K output context (new) |
| Kimi K2.6 | Go | Standard | General purpose, default fallback |
| Qwen3.7 Plus | Go | Fast | Streaming, low-latency (new) |
| Qwen3.7 Max | Go | Fast | Background tasks (new) |
| Qwen3.6 Plus | Go | Fast | Streaming fallback |
| Qwen3.5 Plus | Go | Fast | Simple read-only ops |
| MiniMax | Zen | Long context | 1M context window |
| MiMo | Go | Reasoning | Step-by-step reasoning |

`internal/client/opencode.go` routes Go provider models to Chat Completions; Zen models are classified by `models.ClassifyEndpoint()` in `internal/models/classifier.go`. If a model's upstream doesn't support Anthropic tool format, set `anthropic_tools_disabled: true` in config.

**Scenario detection priority** (`internal/router/scenarios.go`):

1. Long Context (>80K tokens, configurable) → MiniMax (1M context)
2. Complex (architectural patterns, tool operations) → GLM-5.2
3. Think (reasoning keywords in system prompt) → GLM-5.1
4. Background (simple read-only ops, no tools) → Qwen3.7 Max
5. Default → Kimi K2.6

**Cost-based routing:** when `cost_routing.enabled` is set, `Selector` in `internal/router/selector.go` replaces the static primary model with automatic cheapest-model selection from the catalog. It applies `max_context_window` (hard cap on context window), `prefer_providers` (global provider filter, intersected with per-scenario preferences), and `penalty_per_provider` (per-provider cost penalty added during sort). Enabled via `cost_routing.enabled` or the legacy `enable_cost_based_routing` flag.

**Catalog schema:** Models are keyed as `provider/model-name` (e.g., `opencode-go/glm-5.2`). The catalog (`~/.config/routatic-proxy/catalog/catalog.json`) contains:
- `providers` — Provider definitions with `name`, `base_url`, `enabled`
- `models` — Model definitions keyed by full key with fields:
  - `id` — Full key (matches the map key)
  - `name` — Display name
  - `limit.context` — Context window size
  - `rates.input`/`rates.output` — Cost per million tokens
  - `tool_call` — Whether tools are supported
  - `modalities.input`/`output` — Input/output types (`["text"]` or `["text", "image"]` for vision)
  - `reasoning` — Whether reasoning mode is supported

Resolution functions in `internal/catalog/resolve.go` extract the provider from the key prefix. `ResolvedModel.ModelID` is the model name only (without provider prefix); `ResolvedModel.CanonicalName` is the full key.

For streaming, the router downgrades to fast models (Qwen3.7 Plus) for better TTFT.

**Deprecated models:**
- GLM-5 — deprecated May 14, 2026; use GLM-5.1 or GLM-5.2

**Polymorphic field handling:** Anthropic's `system` and `content` fields accept both strings and arrays. `pkg/types/` uses `json.RawMessage` with accessor methods (`SystemText()`, `ContentBlocks()`) to handle both formats.

**Long-running stream policy:** The proxy never kills a stream that is actively producing bytes. The server-level `WriteTimeout` is set to 0; instead each upstream read uses a per-`Read` deadline via `http.ResponseController.SetReadDeadline` that is renewed on every successful byte. If the gap between bytes exceeds `OpenCodeGo.stream_timeout_ms` (or `OpenCodeZen.stream_timeout_ms`), the connection is treated as stuck and the request is routed to the next fallback model. Defaults to `timeout_ms` when unset. Client disconnects during a stream are logged at `Debug` level — this is normal during Claude Code tool execution and is not a failure signal.

**Provider-specific API keys:** Each provider (OpenCode Go, OpenCode Zen, AWS Bedrock) can have its own `api_key` or `api_keys` array. Provider-specific keys take precedence over global keys. This enables per-provider fallback strategies and key rotation.

Environment variable overrides (single key):
- `ROUTATIC_PROXY_OPENCODE_GO_API_KEY`
- `ROUTATIC_PROXY_OPENCODE_ZEN_API_KEY`
- `ROUTATIC_PROXY_AWS_BEDROCK_API_KEY`

Environment variable overrides (comma-separated keys for round-robin):
- `ROUTATIC_PROXY_OPENCODE_GO_API_KEYS=key-1,key-2,key-3`
- `ROUTATIC_PROXY_OPENCODE_ZEN_API_KEYS=key-1,key-2`
- `ROUTATIC_PROXY_AWS_BEDROCK_API_KEYS=key-1,key-2`

Precedence: `*_API_KEYS` → `*_API_KEY` → global `API_KEYS` → global `API_KEY`.

## Key Files

- `cmd/routatic-proxy/main.go` — CLI entry point (cobra). Default config template is generated here.
- `internal/config/` — Config types and JSON loader with `${VAR}` env interpolation.
- `internal/transformer/` — Request/response format conversion (Anthropic ↔ OpenAI).
- `internal/router/fallback.go` — Circuit breaker per model (3 failures = 30s skip).
- `configs/config.example.json` — Reference config with all options documented.
- `internal/gui/` — Embedded HTTP server for the dashboard (serves static assets + API endpoints).
- `internal/gui/assets/` — HTML/CSS/JS for the dashboard (Overview, History, Analytics, Settings tabs).
- `internal/history/` — In-memory ring buffer (1000 entries, O(1) insert, thread-safe).
- `internal/metrics/` — In-process request counters (received, streamed, success, failed, model distribution).
- `internal/storage/` — SQLite persistence layer for request history, latency samples, and analytics.

### GUI Config Editing

The Settings tab exposes all config fields as editable form inputs. On save, only changed fields are sent to the backend as a JSON patch. The backend reads the current config from disk, merges the patch, writes back, and reloads atomically — the running proxy picks up changes immediately without restart.

**Partial update flow:**
1. Frontend builds a patch object with only fields the user changed (compared to the last loaded config)
2. Backend reads current config from disk via `config.LoadFromPath()`
3. Backend merges patch fields onto current config via JSON marshal/unmarshal
4. Backend validates essential fields (host, port)
5. Backend writes merged config to disk and calls `atomicCfg.Reload()`

**Nil safety:** The `/api/metrics` and `/api/history` handlers handle nil dependencies gracefully — they return zero values instead of panicking if the history or metrics instance is unavailable.

## Dual Release Channel System

This project uses a dual release channel system for separating beta and production releases:

### Beta Channel (Automatic)
- **Trigger:** Every push to `main` branch (see `.github/workflows/beta-release.yml`)
- **Version format:** `v{UPCOMING}-beta.{N}` (e.g., `v0.5.3-beta.1`), where `{N}` is a sequential counter
- **GitHub release:** Marked as `prerelease: true`
- **Docker tags:** `v{UPCOMING}-beta.{N}`, `beta-{UPCOMING}`, and `beta` (rolling pointer to newest beta)

Beta releases are fully automated and include:
- Test suite validation
- Cross-platform binary builds (darwin-amd64/arm64, linux-amd64/arm64, windows-amd64/arm64)
- macOS DMG with CGO-enabled binary
- AI-generated changelog from commits
- Docker images for linux/amd64 and linux/arm64

### Production Channel (Manual)
- **Trigger:** Manual `workflow_dispatch` on `releases` branch (see `.github/workflows/release.yml`)
- **Version format:** `vX.Y.Z` (semantic versioning)
- **GitHub release:** Marked as `prerelease: false` (stable)
- **Docker tags:** `vX.Y.Z`, `vX.Y`, `vX`, `latest`

Production releases include all beta features plus:
- Homebrew tap update (requires `HOMEBREW_PAT` secret)
- Scoop bucket update (requires `SCOOP_PAT` secret)

### Version Detection Script

`.github/scripts/get-versions.sh` is used by the beta workflow to:
1. Fetch tags from the `origin/releases` branch to get current production version (e.g., `v0.5.2`)
2. Increment the **patch** to the next version (e.g., `v0.5.3`) - **beta is based on the upcoming patch release**
3. Generate beta version by appending `-beta.{N}`, where `{N}` is `max(existing beta counters for this upcoming version) + 1` - **the counter resets to 1 once the upcoming version ships as stable**
4. Output both versions as JSON for CI consumption


**Version Format Explanation:**
- `v0.5.3` = The upcoming production version (patch incremented from latest production)
- `beta.1` = Sequential prerelease counter for that upcoming version
- Full example: stable `v0.5.2` → `v0.5.3-beta.1`, then `v0.5.3-beta.2`, ... until `v0.5.3` ships → `v0.5.4-beta.1`

### Creating a Production Release

1. Merge all changes to `main` and verify via beta
2. Ensure `releases` branch exists and is up-to-date
3. Go to GitHub Actions → Release workflow
4. Click "Run workflow"
5. Enter version (must follow `vX.Y.Z` format)
6. Workflow validates, builds, and releases

### Release Workflow Stages

Both workflows share the same stages:

1. **validate** — Run `go vet`, `go test -race`, and build sanity check on ubuntu-latest
2. **release** — Build cross-platform binaries and macOS DMG on macos-latest
3. **docker** — Publish multi-arch Docker images on ubuntu-latest

Production adds:
4. **homebrew** — Update the homebrew-tap formula
5. **scoop** — Update the scoop-bucket manifest

## Skill routing

When the user's request matches an available skill, invoke it via the Skill tool. When in doubt, invoke the skill.

Key routing rules:
- Product ideas/brainstorming → invoke /office-hours
- Strategy/scope → invoke /plan-ceo-review
- Architecture → invoke /plan-eng-review
- Design system/plan review → invoke /design-consultation or /plan-design-review
- Full review pipeline → invoke /autoplan
- Bugs/errors → invoke /investigate
- QA/testing site behavior → invoke /qa or /qa-only
- Code review/diff check → invoke /review
- Visual polish → invoke /design-review
- Ship/deploy/PR → invoke /ship or /land-and-deploy
- Save progress → invoke /context-save
- Resume context → invoke /context-restore
- Author a backlog-ready spec/issue → invoke /spec
