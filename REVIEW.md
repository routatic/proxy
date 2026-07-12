# Code Review Guide

This file defines the review standards for routatic-proxy. It is designed for LLM-based review tools, CI/CD pipelines, and human reviewers. Each check is tagged with its layer and severity.

**Severity:**
- **BLOCKER** — must fix before merge
- **REQUIRED** — should fix, merge only with justification
- **ADVISORY** — best practice, consider

**Layers:**
- `[T]` — Technical (Go idioms, safety, concurrency)
- `[L]` — Logical (correctness, edge cases, data flow)
- `[B]` — Business/Architecture (domain invariants, config, release)

---

## 1. Technical Layer

### 1.1 Go Idioms

| # | Check | Severity | Source |
|---|-------|----------|--------|
| T1 | Exported symbols have doc comments (`// Package`, `// TypeName`, `// FuncName`) | REQUIRED | CONTRIBUTING.md |
| T2 | Constructors use `New<TypeName>` pattern, return pointer, apply defaults for nil/zero params | REQUIRED | `rules/auto-detected/CONSTRUCTOR_PATTERN.md` |
| T3 | `context.Context` is the first parameter in functions that accept it | REQUIRED | `rules/auto-detected/CONTEXT_PROPAGATION.md` |
| T4 | Per-attempt contexts use `context.WithTimeout`; `cancel()` is always called (defer or explicit) | REQUIRED | `rules/auto-detected/CONTEXT_PROPAGATION.md` |
| T5 | Loop boundaries check `ctx.Err()` to respect cancellation | REQUIRED | `rules/auto-detected/CONTEXT_PROPAGATION.md` |
| T6 | Errors use `fmt.Errorf("context: %w", err)` wrapping, never bare `return err` | REQUIRED | `rules/auto-detected/ERROR_HANDLING.md` |
| T7 | Sentinel errors declared as `var ErrX = errors.New("...")` at package level | ADVISORY | `rules/auto-detected/ERROR_HANDLING.md` |
| T8 | Error classification via `func IsXError(err error) bool` helpers, not `strings.Contains` | REQUIRED | `rules/auto-detected/ERROR_HANDLING.md` |
| T9 | Polymorphic Anthropic fields (`system`, `content`) use `json.RawMessage` with accessor methods | REQUIRED | `rules/auto-detected/JSON_RAWMESSAGE.md` |
| T10 | Accessors try simplest format first (string before array), fallback to raw bytes | REQUIRED | `rules/auto-detected/JSON_RAWMESSAGE.md` |
| T11 | Logging uses `log/slog` with key-value pairs, never `log.Printf` or `fmt.Println` for diagnostics | REQUIRED | `rules/auto-detected/SLOG_LOGGING.md` |
| T12 | Log levels: Debug=routine, Info=significant events, Warn=recoverable issues, Error=failures | REQUIRED | `rules/auto-detected/SLOG_LOGGING.md` |
| T13 | Nil logger defaults to `slog.Default()` | ADVISORY | `rules/auto-detected/SLOG_LOGGING.md` |
| T14 | `sync.Mutex` embedded in structs, `mu.Lock()` / `defer mu.Unlock()` pattern | REQUIRED | `rules/auto-detected/SYNC_MUTEX.md` |
| T15 | Separate mutexes for independent state (not one big lock) | ADVISORY | `rules/auto-detected/SYNC_MUTEX.md` |
| T16 | Use `sync.RWMutex` when reads dominate writes | ADVISORY | `rules/auto-detected/SYNC_MUTEX.md` |
| T17 | Ensure `gofmt` compliance (run `make lint`) | BLOCKER | CONTRIBUTING.md |
| T18 | All files compile with `CGO_ENABLED=0 go build` (default) | BLOCKER | Makefile |

### 1.2 Memory & Resource Safety

| # | Check | Severity | Rationale |
|---|-------|----------|-----------|
| T19 | `defer` is not used inside `for`/`for-range` loops (runs on function return, leaks resources) | BLOCKER | TOCTOU + leak pattern |
| T20 | HTTP response bodies are closed explicitly (not deferred in loops) | BLOCKER | Resource leak |
| T21 | Test-bind-then-close (TOCTOU) patterns are absent — listeners are bound once and kept | BLOCKER | Race condition |
| T22 | `defer cancel()` or explicit `cancel()` present for every `context.WithTimeout`/`WithCancel` | REQUIRED | Context leak |
| T23 | No goroutine leaks: goroutines have a shutdown signal (ctx.Done(), channel close, WaitGroup) | REQUIRED | Production reliability |
| T24 | No `panic()` in library code; recover only at top-level HTTP handler boundaries | REQUIRED | Crash safety |

### 1.3 Frontend (HTML/CSS/JS)

| # | Check | Severity | Rationale |
|---|-------|----------|-----------|
| T25 | No external CDN scripts or stylesheets — all assets bundled via `//go:embed` | REQUIRED | Offline capability, CSP, supply-chain |
| T26 | CSP header restricts `default-src 'self'`; only `'unsafe-inline'` for scripts and styles | REQUIRED | XSS mitigation |
| T27 | No inline `onclick`/`onchange` handlers referencing undefined functions (check against app.js) | REQUIRED | Silent failures |
| T28 | `data-i18n` keys exist in `TRANSLATIONS.en` (and `TRANSLATIONS.zh` if Chinese) | REQUIRED | i18n completeness |
| T29 | Translations use `t(key)` function, not direct string references | REQUIRED | I18n correctness |
| T30 | No `confirm()`/`prompt()`/`alert()` in production code — use modal or `<select>` instead | ADVISORY | UX quality |
| T31 | Loading states shown during data fetches (spinner or skeleton) | ADVISORY | UX quality |
| T32 | `/api/*` endpoints accessed by the frontend correspond to real backend handlers | REQUIRED | 404 errors |

### 1.4 Configuration & Secrets

| # | Check | Severity | Rationale |
|---|-------|----------|-----------|
| T33 | API keys loaded from env vars or config file, never hardcoded | BLOCKER | Security |
| T34 | Config values support `${VAR}` env interpolation | REQUIRED | `internal/config/loader.go` |
| T35 | Provider-specific keys (`*_API_KEY`) take precedence over global; documented precedence chain | REQUIRED | CLAUDE.md |
| T36 | Port numbers are configurable via env var or CLI flag, not hardcoded | ADVISORY | Deploy flexibility |
| T37 | Config file writes are atomic (write temp, rename) | REQUIRED | Crash safety |

### 1.5 Documentation Completeness

| # | Check | Severity | Rationale |
|---|-------|----------|-----------|
| T38 | Every exported symbol has a `// <Name>` doc comment — packages, types, funcs, consts, vars | REQUIRED | CONTRIBUTING.md, godoc |
| T39 | Package-level doc comments describe the package's responsibility, not its file contents | REQUIRED | Go convention |
| T40 | Non-obvious logic has inline comments explaining *why*, not *what* | REQUIRED | Maintainability |
| T41 | Config changes (new fields, changed defaults, removed keys) update the example config and CLAUDE.md | REQUIRED | First-run UX, LLM accuracy |
| T42 | API endpoint changes update any user-facing docs (README, ARCHITECTURE, CLAUDE.md) | REQUIRED | Alignment — code vs docs |
| T43 | CHANGELOG or release notes updated for user-facing changes (new features, breaking changes, deprecations) | REQUIRED | Release readiness |
| T44 | `docs/` directory or inline `.md` files in the affected package are updated to reflect the change | ADVISORY | Discoverability |
| T45 | Deprecated symbols use `// Deprecated:` doc comment with migration path | ADVISORY | API hygiene |

### 1.6 Duplication & Reuse

| # | Check | Severity | Rationale |
|---|-------|----------|-----------|
| T46 | New code first searches for existing helpers, types, or packages that already serve the purpose — no reinventing | REQUIRED | DRY, consistency |
| T47 | Shared logic (field mapping, type conversion, JSON construction) is extracted to named helpers, not duplicated inline | REQUIRED | Single source of truth |
| T48 | Repeated field-map patterns between types use a helper function, not copy-paste blocks | REQUIRED | `internal/catalog/migrate.go` precedent |
| T49 | New scenarios, fallbacks, or model configs use existing mechanisms (scenario map, config struct, catalog), not inline conditionals | REQUIRED | Config-driven architecture |
| T50 | Common transformations (Anthropic↔OpenAI field renames, token math, cost lookups) use the existing `internal/transformer/` or `internal/catalog/` packages — no ad-hoc reimplementation | REQUIRED | Correctness, maintainability |
| T51 | New types reuse existing project types (e.g. `pkg/types.Message`), not inline structs | REQUIRED | API contract integrity |
| T52 | Existing constructor, error, logger, and mutex patterns are followed — not one-off alternatives | ADVISORY | `rules/auto-detected/` consistency |

---

## 2. Logical Layer

### 2.1 Correctness

| # | Check | Severity | Rationale |
|---|-------|----------|-----------|
| L1 | Map value mutations re-assign to map: `v.Field = x; m[key] = v` (value type semantics in Go) | BLOCKER | Silent data loss |
| L2 | All branches of conditional assignments are complete — no omitted fields | REQUIRED | Data inconsistency |
| L3 | Fallback chain iteration correctly identifies the primary model (index 0) vs fallbacks | REQUIRED | Routing correctness |
| L4 | Circuit breaker counts only retryable errors (5xx), not 4xx or client cancellation | REQUIRED | `internal/router/fallback.go` |
| L5 | Stream idle timeout is per-`Read`, not server-level `WriteTimeout` | REQUIRED | CLAUDE.md stream policy |
| L6 | Client disconnects during stream are logged at Debug, not Error | REQUIRED | CLAUDE.md stream policy |
| L7 | `hasToolUsage` checks only unambiguous tool-calling patterns, not everyday words like "bash" | REQUIRED | False positives |
| L8 | Model fallbacks carry correct config (provider, temperature, max_tokens) — not just model_id | REQUIRED | Config-driven routing |

### 2.2 Edge Cases

| # | Check | Severity | Rationale |
|---|-------|----------|-----------|
| L9 | Nil `history.History` or `metrics.Metrics` returns zero values, not panics | REQUIRED | CLAUDE.md nil safety |
| L10 | Empty model chain returns an informative error, not panic/empty response | REQUIRED | First-run UX |
| L11 | Zero token count, zero cost, empty trend data render as `—` not `$NaN` or `undefined` | REQUIRED | Analytics dashboard |
| L12 | Headless mode (`--headless`, `serve`) does not attempt GUI operations | REQUIRED | Cross-platform |
| L13 | Port-scan fallback (GUI port 3445→3454) notifies user, doesn't silently pick different port | ADVISORY | User awareness |
| L14 | JSON body is limited with `http.MaxBytesReader` before parsing | REQUIRED | DOS prevention |
| L15 | SSE stream transformers handle partial/incomplete JSON chunks without panic | REQUIRED | Streaming robustness |

### 2.3 Data Flow

| # | Check | Severity | Rationale |
|---|-------|----------|-----------|
| L16 | Analytics cost query uses `SUM(tokens * COALESCE(rate, 0))` not `SUM(tokens) * rate` | BLOCKER | Per-model pricing |
| L17 | JSON field names in Go struct tags match what the frontend reads | BLOCKER | KPI correctness |
| L18 | Frontend references backend struct fields by their JSON tag, not Go field name | BLOCKER | Serialization mismatch |
| L19 | Auto-detected scenario keys in FallbackModule match config model keys (no hardcoded subset) | REQUIRED | Feature detection |
| L20 | Fallback save patch sends `{fallbacks: {scenario1: [...], ...}}` matching actual config structure | REQUIRED | Config integrity |
| L21 | Catalog resolution silently degrades with a warning log when catalog is unavailable | REQUIRED | Debuggability |

---

## 3. Business / Architecture Layer

### 3.1 Routing Invariants

| # | Check | Severity | Rationale |
|---|-------|----------|-----------|
| B1 | Model routing is config-driven, not code-driven — adding a model requires zero Go changes | BLOCKER | CLAUDE.md architecture |
| B2 | Scenario detection priority is: Long Context > Complex > Think > Background > Default | REQUIRED | `internal/router/scenarios.go` |
| B3 | Streaming requests use fast models (Qwen3.7 Plus) for better TTFT | REQUIRED | CLAUDE.md |
| B4 | Vision requests route to vision-capable models; non-vision models reject image content | REQUIRED | Capability check |
| B5 | Cost-based routing filters by constraints (tools, vision, reasoning, context) before sorting by price | REQUIRED | `internal/router/selector.go` |
| B6 | `respect_requested_model` bypasses scenario routing; provider-qualified refs that fail catalog resolution return error | REQUIRED | `internal/router/model_router.go` |

### 3.2 Provider & Model Integrity

| # | Check | Severity | Rationale |
|---|-------|----------|-----------|
| B7 | Anthropic tool format disabled models (`anthropic_tools_disabled: true`) route through Chat Completions transform | REQUIRED | CLAUDE.md |
| B8 | Bedrock xAI models get `/openai` path appended; other Bedrock models use standard path | REQUIRED | `internal/provider/aws_bedrock.go` |
| B9 | Catalog schema: models keyed as `provider/model-name`, resolved via `Resolve`/`ResolveShort` | REQUIRED | `internal/catalog/resolve.go` |
| B10 | Catalog seed prices are idempotent — update only where `cost_input_per_m IS NULL OR 0` | REQUIRED | `internal/storage/database.go` |
| B11 | Free-tier models seed-price matching: specific entries first, `-free` generic catch-all as fallback | ADVISORY | `seed_prices.json` ordering |

### 3.3 Release & Build

| # | Check | Severity | Rationale |
|---|-------|----------|-----------|
| B12 | `make build` runs `build-css` before `go build` (Tailwind CSS generation) | BLOCKER | Frontend asset delivery |
| B13 | `make test` runs with `-race` detector | REQUIRED | CONTRIBUTING.md |
| B14 | `make lint` checks `gofmt` and `go vet` | REQUIRED | CONTRIBUTING.md |
| B15 | Beta releases auto-generated on push to `main`; production releases manual on `releases` branch | REQUIRED | CLAUDE.md |
| B16 | Version follows `vX.Y.Z` for production, `vX.Y.Z-beta.TIMESTAMP` for beta | REQUIRED | CLAUDE.md |
| B17 | Docker image builds with `CGO_ENABLED=0` for portability | REQUIRED | Cross-platform |

### 3.4 API Contract

| # | Check | Severity | Rationale |
|---|-------|----------|-----------|
| B18 | `POST /v1/messages` accepts and returns Anthropic Messages API format | BLOCKER | Claude Code compatibility |
| B19 | SSE events are transformed in-flight, not buffered | REQUIRED | CLAUDE.md |
| B20 | `/health` returns 200 with `{"status":"ok"}` | REQUIRED | Health checks |
| B21 | Dashboard `/api/*` endpoints are under `internal/gui/` and require no auth (local-only) | REQUIRED | Design decision |

---

## 4. Code Smell Baseline (Fowler, Refactoring ch.3)

These are always judgement calls. A documented repo standard overrides any smell check.

| # | Smell | What to look for | Severity |
|---|-------|------------------|----------|
| S1 | **Mysterious Name** | Function/variable/type name doesn't reveal what it does or holds | ADVISORY |
| S2 | **Duplicated Code** | Same logic shape in more than one hunk or file → check T46–T48 | ADVISORY |
| S3 | **Feature Envy** | Method reaches into another object's data more than its own | ADVISORY |
| S4 | **Data Clumps** | Same fields/params travelling together ← extract to a type | ADVISORY |
| S5 | **Primitive Obsession** | String for a domain concept that deserves its own small type | ADVISORY |
| S6 | **Repeated Switches** | Same switch/if-cascade on same type recurs across the change | ADVISORY |
| S7 | **Shotgun Surgery** | One logical change forces scattered edits across many files | ADVISORY |
| S8 | **Divergent Change** | One file modified for several unrelated reasons | ADVISORY |
| S9 | **Speculative Generality** | Abstraction/params/hooks for needs the spec doesn't have | ADVISORY |
| S10 | **Message Chains** | Long `a.b().c().d()` navigation | ADVISORY |
| S11 | **Middle Man** | Class/function that mostly just delegates onward | ADVISORY |
| S12 | **Refused Bequest** | Subclass/implementer ignores most of what it inherits | ADVISORY |

---

## 5. Review Process

### 5.1 Pre-Review Checklist (for CI)

```bash
# Build + CSS generation
make build

# Lint (gofmt + go vet)
make lint

# Tests with race detector
make test
```

### 5.2 Review Command

```bash
# Full diff against base
git diff main...HEAD --stat

# Focus areas by package
git diff main...HEAD -- internal/router/   # Routing correctness
git diff main...HEAD -- internal/gui/      # Dashboard + analytics
git diff main...HEAD -- internal/storage/  # Persistence + queries
git diff main...HEAD -- internal/catalog/  # Model resolution
git diff main...HEAD -- internal/handlers/ # API contracts
git diff main...HEAD -- cmd/               # CLI behavior
```

### 5.3 Commit Message Convention

```
<type>: <imperative description>

feat:     New feature
fix:      Bug fix
refactor: Code restructuring
docs:     Documentation
chore:    Build/tooling
test:     Tests
```

### 5.4 Two-Axis Reporting

Each review should produce findings under two independent axes:

- **Standards** — does the code follow the documented rules (sections 1–3)? Distinguish hard violations (tagged BUILDER/REQUIRED) from judgement calls (ADVISORY/smells).
- **Spec** — does the code faithfully implement what was asked? Identify missing requirements, scope creep, and wrong implementations separately.

Report them side by side, never reranked into a single score — a change can pass one axis and fail the other.

---

## 6. Quick Reference: File-to-Rule Mapping

**Universal rules (all files):** T38–T45 (documentation), T46 (search before invent), T47 (extract shared logic), T48 (no copy-paste mapping), T50 (reuse transformer/catalog), T51 (reuse project types), T52 (follow established patterns), S2 (no duplicated code), S6 (no repeated switches), S7 (shotgun surgery).

| File / Package | Applicable Rules |
|----------------|-----------------|
| `internal/router/` | T3, T4, T5, T6, T14, T15, L3, L4, L8, B1–B6 |
| `internal/handlers/` | T9, T10, T11, T24, L15, B18, B19, B20 |
| `internal/transformer/` | T9, T10, L15, B18 |
| `internal/client/` | T4, T6, T22, B7 |
| `internal/provider/` | T3, T4, T6, B8 |
| `internal/gui/` | T25–T32, T36, L9, L13, L17–L20, B12, B21 |
| `internal/storage/` | T1, L16, L21, B10, B11 |
| `internal/catalog/` | L21, B9 |
| `internal/config/` | T34, T35, T37 |
| `internal/metrics/` | T14, T15, T23 |
| `internal/server/` | T19, T20, T21, T26 |
| `internal/daemon/` | T22, L12 |
| `cmd/routatic-proxy/` | T17, T18, T36, L12, B12–B17 |
| `internal/gui/assets/` | T25–T32 |
| `pkg/types/` | T9, T10 |
