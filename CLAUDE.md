# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build   # Build binary to bin/routatic-proxy (CGO disabled by default)
make run     # Run without building
make test    # Run tests with race detector
make lint    # gofmt check + go vet (does NOT run tests)
make lint-strict # golangci-lint run with .golangci.yml (requires golangci-lint 2.x)
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

**Four endpoint types** (`EndpointType`, `internal/models/classifier.go`):

- `EndpointChatCompletions` — OpenAI-compatible `/v1/chat/completions`. The default, and what most models use.
- `EndpointAnthropic` — Anthropic `/v1/messages`.
- `EndpointResponses` — OpenAI native `/v1/responses`. Used by the GPT-5.x families (`IsResponsesModel`).
- `EndpointGemini` — Google `/v1/models/{id}`. Used by `gemini-3.5-flash`, `gemini-3.1-pro`, `gemini-3-flash` (`IsGeminiModel`).

Which models take the Anthropic endpoint depends on the provider:

- **Go provider** — `IsAnthropicModel` (classifier.go) returns true for `minimax-m2.5`, `minimax-m2.7`, `minimax-m3` **and** `qwen3.5-plus`, `qwen3.6-plus`, `qwen3.7-plus`, `qwen3.7-max`. Everything else goes through the Chat Completions transform.
- **Zen provider** — `ClassifyEndpoint` is Zen-specific. `IsZenAnthropicModel` routes any `claude-*` or `qwen*` model to Anthropic; MiniMax on Zen uses Chat Completions (unlike MiniMax on the Go provider).

**Wire format overrides.** A model config's `wire_format` field overrides the built-in classification on the **Go provider only** — `"openai"` (aliases `chat`, `chat_completions`), `"anthropic"` (alias `messages`), or `"responses"`. This is how a Go model reaches the OpenAI Responses endpoint (`opencode_go.responses_base_url`), since Go classification never selects Responses on its own. `"gemini"`, `"auto"`, empty, and unrecognised values all fall back to classification — the Go provider has no Gemini path. Zen and Bedrock ignore the per-model override and classify by model ID.

`core.ParseWireFormat` is the only place `wire_format` strings are interpreted, and `Provider.WireFormat(config.ModelConfig)` is the only place a model's format is resolved. `Execute`, `Stream`, and the streaming handler in `internal/handlers/messages.go` all dispatch on that one method so the endpoint a request is sent to and the SSE parser used to read the reply cannot disagree. Do not re-derive the format at a call site.

**Available models:** the built-in capability registry is `modelMetadata` in `internal/config/model_registry.go`. It supplies context window, max output tokens, and vision support whenever the runtime config omits them (`ResolveModelConfig`). Every entry has `SupportsTools: true`.

| Model ID | Typical provider | Context | Max output | Vision | Best For |
|----------|------------------|---------|-----------|--------|----------|
| `deepseek-v4-pro` | Go | 1M | 8192 | no | Default + complex scenarios in the shipped config |
| `deepseek-v4-flash` | Go | 1M | 4096 | no | Background / fast scenarios |
| `deepseek-v4-flash-free` | Zen | 1M | 4096 | no | Free-tier fallback |
| `glm-5.2` | Go | 200K | 8192 | no | Think scenario, architecture decisions |
| `glm-5.1` | Go | 200K | 8192 | no | Complex patterns, tool operations |
| `glm-5` | Go | 200K | 8192 | no | Reasoning tasks (deprecated May 14, 2026) |
| `kimi-k3` | Go | 1M | 131072 | yes | Flagship Kimi, huge output budget, multimodal |
| `kimi-k2.7-code` | Go | 256K | 32768 | yes | Large code generation |
| `kimi-k2.6` | Go | 256K | 8192 | yes | General purpose, common fallback |
| `kimi-k2.5` | Go | 256K | 8192 | yes | Previous-generation Kimi fallback |
| `minimax-m3` | Go | 1M | 128000 | no | Long-context scenario in the shipped config |
| `minimax-m2.7` | Go | 200K | 8192 | no | Previous MiniMax generation |
| `minimax-m2.5` | Go | 200K | 4096 | no | Older MiniMax generation |
| `mimo-v2.5-pro` | Go | 1M | 16384 | no | Step-by-step reasoning, larger output |
| `mimo-v2.5` | Go | 1M | 8192 | no | Step-by-step reasoning |
| `mimo-v2.5-free` | Zen | 1M | 8192 | no | Free-tier fallback |
| `mimo-v2-omni` | Go | 1M | 8192 | yes | Multimodal MiMo |
| `qwen3.7-max` | Go | 1M | 8192 | yes | Complex coding, Qwen's best quality |
| `qwen3.7-plus` | Go | 1M | 8192 | yes | Streaming, low-latency |
| `qwen3.6-plus` | Go | 1M | 8192 | yes | Streaming fallback |
| `qwen3.5-plus` | Go | 1M | 8192 | yes | Simple read-only ops |

The "typical provider" column reflects how the shipped config wires each model; the registry itself is provider-agnostic, so any model can be pointed at any provider in `config.json`. Zen additionally exposes many models that are not in the registry (Claude, Gemini, GPT-5.x, other free-tier models) — those get their capabilities from the catalog rather than `modelMetadata`.

`internal/client/opencode.go` routes Go provider models to Chat Completions; Zen models are classified by `models.ClassifyEndpoint()` in `internal/models/classifier.go`. If a model's upstream doesn't support Anthropic tool format, set `anthropic_tools_disabled: true` in config.

**Scenario detection priority** (`DetectScenario`, `internal/router/scenarios.go`). Models below are the built-in defaults from `cmd/routatic-proxy/templates/default_config.json`, which is what `routatic-proxy init` writes:

1. **Long context** — token count > threshold (`getLongContextThreshold`, default **100K**, configurable via the `long_context` model's `context_threshold`) → `minimax-m3`. If the latest user message also carries an image, the scenario is `vision_long_context` instead.
2. **Vision** — the latest user message contains an image. Splits by intent: `vision_complex` when the text also shows complex intent, otherwise `vision`.
3. **Complex** — architectural patterns or tool-heavy operations → `deepseek-v4-pro`.
4. **Think** — reasoning keywords → `glm-5.2`.
5. **Background** — simple read-only ops with no tools → `deepseek-v4-flash`.
6. **Default** → `deepseek-v4-pro`.

The three vision scenarios are `ScenarioVision`, `ScenarioVisionComplex`, and `ScenarioVisionLongContext` (scenarios.go). The shipped default config has no `vision*` model entries, so vision requests fall through to the ordinary scenario models unless you add them.

The `Reason` strings in `scenarios.go` describe only *why* a scenario matched and name no model. The resolved model is appended by `ModelRouter.Route` / `RouteForStreaming` (`describeRouting`), so the routing log line always reports the model that actually came from config — e.g. `scenario=complex (complex or tool-based operation keywords in latest user message) -> resolved model glm-5.2`. A test asserts detector reasons never name a model, so they cannot drift again.

**Model overrides:** two config blocks bypass scenario routing based on the requested model. `model_overrides` matches the `model` string **exactly** (best with CC-Switch, which sends a custom model string). `model_family_overrides` maps a Claude family keyword (`opus`, `sonnet`, `haiku`) via **case-insensitive substring** match, so the versioned IDs Claude Code sends natively (`claude-opus-4-20250514`) route without CC-Switch. Precedence: exact `model_overrides` → `model_family_overrides` (longest key first) → `respect_requested_model` → scenario routing. Both are wired through `ModelRouter.RouteWithOverride` / `RouteWithFamilyOverride` (`internal/router/model_router.go`) and merged with a deduplicated scenario safety-net chain in `buildModelChain` (`internal/handlers/messages.go`). Override entries accept any provider `models` and `fallbacks` accept — `opencode-go`, `opencode-zen`, `aws-bedrock`, `openrouter` (underscore spellings normalized) — validated against `config.KnownProviders` in `internal/config/provider.go`, which is the single source for provider names and `NormalizeProvider`; `client.Provider*` are aliases of those constants.

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

For streaming, `RouteForStreaming` downgrades complex/think requests to the `fast` scenario for better TTFT (`deepseek-v4-flash` in the shipped default config).

**Deprecated models:**
- GLM-5 — deprecated May 14, 2026; use GLM-5.1 or GLM-5.2

**Polymorphic field handling:** Anthropic's `system` and `content` fields accept both strings and arrays. `pkg/types/` uses `json.RawMessage` with accessor methods (`SystemText()`, `ContentBlocks()`) to handle both formats.

**Long-running stream policy:** The proxy never kills a stream that is actively producing bytes. The server-level `WriteTimeout` is set to 0; instead each upstream read uses a per-`Read` deadline via `http.ResponseController.SetReadDeadline` that is renewed on every successful byte. If the gap between bytes exceeds `OpenCodeGo.stream_timeout_ms` (or `OpenCodeZen.stream_timeout_ms`), the connection is treated as stuck and the request is routed to the next fallback model. Defaults to `timeout_ms` when unset. Client disconnects during a stream are logged at `Debug` level — this is normal during Claude Code tool execution and is not a failure signal.

**Provider-specific API keys:** Each provider (OpenCode Go, OpenCode Zen, AWS Bedrock, OpenRouter) can have its own `api_key` or `api_keys` array. Provider-specific keys take precedence over global keys. This enables per-provider fallback strategies and key rotation.

Environment variable overrides (single key):
- `ROUTATIC_PROXY_OPENCODE_GO_API_KEY`
- `ROUTATIC_PROXY_OPENCODE_ZEN_API_KEY`
- `ROUTATIC_PROXY_AWS_BEDROCK_API_KEY`
- `ROUTATIC_PROXY_OPENROUTER_API_KEY`

Environment variable overrides (comma-separated keys for round-robin):
- `ROUTATIC_PROXY_OPENCODE_GO_API_KEYS=key-1,key-2,key-3`
- `ROUTATIC_PROXY_OPENCODE_ZEN_API_KEYS=key-1,key-2`
- `ROUTATIC_PROXY_AWS_BEDROCK_API_KEYS=key-1,key-2`
- `ROUTATIC_PROXY_OPENROUTER_API_KEYS=key-1,key-2`

Precedence: `*_API_KEYS` → `*_API_KEY` → global `API_KEYS` → global `API_KEY`.

## Key Files

- `cmd/routatic-proxy/main.go` — CLI entry point (cobra). Default config template is generated here.
- `internal/config/` — Config types and JSON loader with `${VAR}` env interpolation.
- `internal/transformer/` — Request/response format conversion (Anthropic ↔ OpenAI).
- `internal/router/fallback.go` — Circuit breaker per model (3 failures = 30s skip).
- `internal/handlers/models.go` — `GET /v1/models` (OpenAI-style listing). Used by provider-switching tools like CC-Switch's "Fetch Models" button; sources IDs from `ModelRouter.ListModels` (config aliases + `model_overrides` keys + catalog canonical names).
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
- **Version format:** `v{UPCOMING}-beta.{N}` (e.g., `v0.6.4-beta.1`), where `{N}` is a sequential counter
- **GitHub release:** Marked as `prerelease: true`
- **Docker tags:** `v{UPCOMING}-beta.{N}`, `beta-{PROD}` (the latest *stable* version, e.g. `beta-v0.6.3`), and `beta` (rolling pointer to newest beta)

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
1. Fetch tags from the `origin/releases` branch to get current production version (e.g., `v0.6.3`)
2. Increment the **patch** to the next version (e.g., `v0.6.4`) - **beta is based on the upcoming patch release**
3. Generate beta version by appending `-beta.{N}`, where `{N}` is `max(existing beta counters for this upcoming version) + 1` - **the counter resets to 1 once the upcoming version ships as stable**
4. Output both versions as JSON for CI consumption


**Version Format Explanation:**
- `v0.6.4` = The upcoming production version (patch incremented from latest production)
- `beta.1` = Sequential prerelease counter for that upcoming version
- Full example: stable `v0.6.3` → `v0.6.4-beta.1`, then `v0.6.4-beta.2`, ... until `v0.6.4` ships → `v0.6.5-beta.1`

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
2. **rpm** — Build and verify the Fedora RPMs on ubuntu-latest, then pass them to `release` as the `rpm-packages` artifact (`.github/scripts/build-rpms.sh` and `verify-rpm.sh`)
3. **release** — Build cross-platform binaries and macOS DMG on macos-latest, and publish every asset — binaries, DMG, RPMs, checksums — through one atomic `gh release create`
4. **docker** — Publish multi-arch Docker images on ubuntu-latest

Production adds:
5. **homebrew** — Update the homebrew-tap formula
6. **scoop** — Update the scoop-bucket manifest

The RPMs are packaged in their own Linux job rather than in `release` for two reasons: `rpm`/`rpm2cpio` are unavailable on the macOS runner, so verification has to happen on Linux; and this repo publishes **immutable releases**, so assets cannot be added after `gh release create` — everything must be present for that single call.

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
