# Cache Hit Rate and Request Latency Implementation Plan

Status: Proposed

Created: 2026-08-25

Branch: `perf/cache-hit-and-request-latency-plan`

Base: `main` at `d74b160`

## Objective

Improve repeated-request efficiency and reduce proxy-added latency without
changing routing decisions, provider-visible request semantics, or response
correctness.

The work covers five areas:

1. Preserve upstream prompt-cache directives and cache-usage telemetry.
2. Cache repeated token-count results.
3. Remove SQLite telemetry writes from the response critical path.
4. Correct and extend performance measurements.
5. Make catalog refresh and cost-based routing cheaper.

General LLM response caching is explicitly out of scope. Responses can be
non-deterministic, streamed, tool-bearing, and authorization-sensitive. Exact
in-flight request coalescing can be considered later as a separate feature.

## Delivery Principles

- Measure before and after every optimization.
- Change one bottleneck per performance commit.
- Keep caches bounded by both entry count and retained bytes.
- Publish immutable data to request handlers.
- Treat analytics persistence as best-effort telemetry, not part of response
  correctness.
- Preserve stale-but-valid catalog data when a refresh fails.
- Keep provider-specific cache behavior behind explicit capability decisions.
- Include benchmark evidence in performance commit messages.

## Success Criteria

The implementation is complete when:

- Anthropic system cache directives survive normalization and reach supported
  upstream wire formats unchanged.
- Unsupported wire formats still omit cache directives intentionally.
- Cache read and cache creation token counts are visible in in-memory metrics,
  request history, and SQLite history when the upstream supplies them.
- Repeated conversation history produces token-count cache hits.
- The token cache has bounded memory, exposes hits, misses, evictions, and
  retained bytes, and remains race-free.
- A slow or locked SQLite database cannot delay a successful HTTP response.
- Storage queue overflow and shutdown-drain failures are observable.
- p95 and p99 calculations operate on sorted samples and are covered by tests.
- Catalog cache hits require no mutex acquisition.
- Catalog refresh does not make concurrent requests wait for disk or SQLite.
- Cost selection uses the provider index and a one-pass minimum rather than
  scanning the entire catalog once per provider and sorting all candidates.
- Each optimized path shows a statistically significant improvement under its
  targeted benchmark, with no statistically significant regression in its
  cold or fallback path.
- `go test ./... -race`, `make lint`, and `make lint-strict` pass.

## Measurement Foundation

This phase is delivered first because the current latency percentile methods
index samples in arrival order rather than sorted order.

### Benchmarks

Add focused benchmarks using Go 1.25 `b.Loop()`:

- `BenchmarkCountMessages`
  - cold cache
  - warm cache
  - 10, 50, and 200 message histories
  - growing conversation where one message is appended per turn
  - repeated system prompt
  - parallel callers
- `BenchmarkMetricsRecordSuccess`
  - empty buffer
  - full buffer
  - parallel writers
  - concurrent snapshot readers
- `BenchmarkSelectorSelectCheapest`
  - small, medium, and large catalogs
  - one and several eligible providers
  - restrictive and permissive constraints
- `BenchmarkModelRouterCatalogHit`
  - fresh snapshot
  - expired snapshot while another refresh is active
- `BenchmarkAsyncStorageWriter`
  - enqueue only
  - batch sizes
  - queue saturation
  - graceful drain
- request-path integration benchmark using a fake provider with a fixed response
  and no external network.

Run benchmarks serially and retain reports outside the repository:

```bash
GOCACHE=/tmp/routatic-perf-go-cache \
  go test -run '^$' -bench=. -benchmem -count=10 ./internal/token ./internal/metrics ./internal/router ./internal/handlers \
  | tee /tmp/routatic-perf-before.txt

GOCACHE=/tmp/routatic-perf-go-cache \
  go test -run '^$' -bench=. -benchmem -count=10 ./internal/token ./internal/metrics ./internal/router ./internal/handlers \
  | tee /tmp/routatic-perf-after.txt

benchstat /tmp/routatic-perf-before.txt /tmp/routatic-perf-after.txt
```

Do not claim an improvement from a single run or from statistically
insignificant output.

### Request-stage timing

Record separate durations for:

- body read and JSON parsing
- message extraction
- token counting
- request-fact analysis and routing
- provider request transformation
- upstream time to first byte or first SSE payload
- upstream total duration
- response transformation
- storage enqueue
- total proxy duration

Use monotonic `time.Time` values already carried by Go timestamps. Avoid logging
every stage per request at info level; aggregate measurements in
`internal/metrics`.

## Workstream 1: Preserve Prompt-Cache Directives

### Current problem

`types.MessageRequest` can represent system content blocks with
`cache_control`, but `core.NormalizeRequest` flattens the system field into
plain text. `normalizedToMessageRequest` then reconstructs a JSON string, so
the provider registry path cannot forward the original directive.

### Design

Replace the normalized request's single `SystemPrompt string` as the canonical
representation with ordered system blocks:

```go
type NormalizedCacheControl struct {
    Type string
}

type NormalizedSystemBlock struct {
    Text         string
    CacheControl *NormalizedCacheControl
}
```

Add a `SystemText()` helper for transformers that only need concatenated text.
Do not retain both mutable `SystemPrompt` and `SystemBlocks` fields because two
sources of truth can diverge.

Normalization rules:

- A string system prompt becomes one cacheless normalized block.
- An array remains an ordered set of blocks.
- `cache_control.type` is copied without provider interpretation.
- Unknown system block fields are either explicitly modeled or rejected from
  the "lossless" contract; do not silently claim losslessness.

Denormalization rules:

- Emit a JSON string for one cacheless text block to preserve the common wire
  shape.
- Emit an ordered block array when any cache directive is present or multiple
  blocks must be retained.
- Anthropic-format providers receive the block array unchanged.
- OpenAI Chat transformations retain the existing DeepSeek support and existing
  stripping behavior for unsupported models.
- Responses and Gemini transformations omit the directive until their provider
  contracts explicitly support an equivalent.

The provider or wire-format boundary owns the support decision. Core
normalization only preserves information.

### Cache-usage telemetry

Extend `history.RequestRecord` with:

- `CacheReadTokens`
- `CacheCreationTokens`

Add matching nullable/default-zero SQLite columns through an idempotent
migration. Populate them from both streaming and non-streaming responses.

Extend the Responses usage type only after verifying the actual upstream
payload shape with provider fixtures. Do not infer a cached-token JSON field.

Expose raw cache counters per provider and model. Avoid a universal "hit rate"
formula until the provider-specific token accounting denominator is defined.

### Tests

- Normalize string system prompts.
- Normalize multiple system blocks while retaining order.
- Round-trip a system `cache_control` directive.
- Verify the Zen Anthropic request body retains cache directives.
- Verify the DeepSeek Chat request retains supported directives.
- Verify unsupported Chat, Responses, and Gemini bodies omit them.
- Verify stream and non-stream usage populate history and storage.
- Verify migrations work on both new and existing databases.
- Add golden provider-body fixtures where practical.

### Acceptance

- No cache directive disappears before provider capability handling.
- Existing provider stripping tests continue to pass.
- Requests without cache directives keep their existing common wire shape.

## Workstream 2: Cache Repeated Token Counts

### Current problem

Every request tokenizes the system prompt and every message again. In an
interactive session, most previous message text is identical to the prior turn.

### Design

Add a bounded cache owned by `token.Counter`.

Initial implementation:

- concurrency-safe LRU or similarly bounded policy
- exact text keys
- `strings.Clone` when retaining keys so a short key cannot retain a large
  request-body backing allocation
- maximum retained bytes and maximum entries
- skip entries below a measured minimum string length
- entry weight based on retained key bytes plus fixed overhead
- no unbounded `sync.Map`
- no `sync.Pool` unless a profile identifies allocation churn it can solve

The count remains deterministic and encoding-specific. Include the encoding
name in the key or cache namespace so a future model-specific tokenizer cannot
reuse an incompatible count.

Add configuration only if operational tuning is necessary:

```json
{
  "performance": {
    "token_cache_enabled": true,
    "token_cache_max_entries": 10000,
    "token_cache_max_bytes": 33554432
  }
}
```

Defaults must be safe for desktop use. A disabled cache must preserve current
behavior exactly.

Do not add miss coalescing initially. Add it only if a parallel benchmark shows
that concurrent identical misses are common enough to outweigh coordination
cost.

### Metrics

Record:

- hits
- misses
- evictions
- skipped-small-input counts
- current entries
- retained bytes
- tokenization duration

### Tests

- deterministic hit after first count
- distinct strings and encodings do not collide
- byte and entry limits evict
- disabled cache bypasses storage
- small-entry policy works
- parallel race test
- large input does not cause unbounded retention
- cached and uncached `CountMessages` return identical totals

### Acceptance

- Warm repeated-history benchmarks improve significantly.
- Cold-cache performance has no meaningful regression.
- Memory remains within configured bounds under adversarial unique input.

## Workstream 3: Move SQLite Writes Off the Response Path

### Current problem

Successful requests execute separate request and latency inserts synchronously.
SQLite is configured with one open connection, so concurrent request
completions serialize before non-streaming response bodies are written.

### Design

Replace the two-method handler-facing storage interface with one completion
operation:

```go
type CompletionRecorder interface {
    RecordCompletion(history.RequestRecord)
    Shutdown(context.Context) error
}
```

`RecordCompletion` enqueues without waiting for SQLite. A dedicated writer:

- owns one bounded channel
- batches by maximum count or short flush interval
- writes request and latency rows in one transaction
- preserves record order within each batch
- keeps the existing single SQLite writer connection
- emits counters for enqueued, persisted, dropped, failed, retried, queue depth,
  batch size, and drain duration
- samples repeated error logs

Because this data is analytics telemetry, queue saturation should not block a
user response. Use a documented drop policy, preferably drop-newest with an
explicit counter, so older already-accepted records remain ordered.

Keep the in-memory history update synchronous because it is O(1) and supplies
the live dashboard immediately.

### Shutdown lifecycle

Change server shutdown ordering:

1. Stop accepting new HTTP requests and wait for active handlers.
2. Close the completion recorder to new entries.
3. Drain the queue within the caller's shutdown deadline.
4. Stop retention work.
5. Close SQLite.

Use the same lifecycle for signal-based and programmatic shutdown. The current
paths must not close SQLite while request handlers can still enqueue work.

### Tests

- a blocking storage backend cannot delay an HTTP response
- request and latency rows commit atomically
- batches flush by size and by interval
- queue saturation follows the documented policy
- persistence errors do not stop later batches
- shutdown drains accepted records
- shutdown deadline returns a clear error
- enqueue after shutdown is safe and observable
- race tests cover enqueue, flush, and shutdown

### Acceptance

- Handler storage-enqueue time remains bounded and independent of SQLite delay.
- No successful request fails because telemetry persistence fails.
- Accepted records drain on normal shutdown.

## Workstream 4: Correct and Extend Performance Metrics

### Correctness fixes

- Replace latency slice shifting with a fixed ring buffer.
- Calculate percentiles from a sorted copy.
- Sort once when calculating multiple percentiles.
- Copy samples while holding the lock, then release the lock before sorting.
- Apply the same ring-buffer implementation to global and per-model samples.
- Add table tests for empty, one-element, ordered, reverse-ordered, and repeated
  samples.

### New measurements

Add counters and bounded timing samples for the request stages listed in
Measurement Foundation.

For streaming requests:

- record time to first SSE payload separately from total stream duration
- use `sync.Once` or equivalent so first-payload timing is recorded exactly once
- distinguish a connection that produced headers from one that produced an SSE
  data event

For upstream connections, add opt-in `httptrace` sampling rather than tracing
every request. Capture:

- connection reused
- connection idle duration
- DNS duration
- connect duration
- TLS duration
- first response byte

This data determines whether sharing or retuning provider transports is worth a
later change. Do not alter HTTP pool sizes or enable a protocol based only on
intuition.

### Exposure

- Keep `/health` compact.
- Add detailed performance data to the existing metrics/dashboard API.
- Include token cache and storage queue state.
- Include raw provider cache-token counters.
- Document whether every duration includes or excludes upstream time.

### Tests and benchmarks

- percentile correctness independent of insertion order
- ring-buffer eviction order
- concurrent record/snapshot race tests
- first-SSE timing recorded once
- `RecordSuccess` full-buffer benchmark
- metrics snapshot benchmark with several models

### Acceptance

- Reported percentiles match a reference implementation.
- Metrics collection does not become a top allocation or lock-contention source.
- Proxy overhead and upstream latency can be distinguished.

## Workstream 5: Speed Up Catalog and Cost-Based Routing

### Current problem

All catalog hits acquire one mutex. When the 30-second entry expires, the
request holding that mutex performs SQLite or file loading while concurrent
requests wait. Cost selection then scans the full model map for every eligible
provider and sorts every candidate to select one.

### Catalog snapshot design

Publish an immutable snapshot through `atomic.Pointer`:

```go
type catalogSnapshot struct {
    Catalog  *catalog.IndexedCatalog
    LoadedAt time.Time
    Err      error
}
```

Request behavior:

- A fresh snapshot is returned with one atomic load.
- An expired snapshot remains usable.
- One background refresh is started through an atomic refresh flag.
- Other requests continue with stale data.
- A successful refresh atomically publishes a new immutable snapshot.
- A failed refresh records the error and retains the last valid snapshot.
- Startup performs one bounded synchronous load when a catalog source exists.
- Config/catalog update events may explicitly invalidate or refresh the
  snapshot instead of waiting for TTL.

Do not mutate an `IndexedCatalog` after publishing it.

### Selector design

Use `IndexedCatalog.ListProviderModels(provider)` or an equivalent precomputed
provider-keyed resolved-model index. Remove the nested full-catalog scan.

Replace candidate collection and sorting with a one-pass best-candidate
comparison using the existing deterministic ordering:

1. lower effective cost
2. larger context window
3. lexicographically smaller model ID

Build enabled-provider state once per immutable configuration/catalog
generation rather than once per request. Register an `AtomicConfig.OnReload`
callback to publish a new selector state.

Pass already-computed `RequestFacts` and constraints through routing rather than
re-running message scans and lowercasing.

### Tests

- fresh catalog hit performs no refresh
- concurrent expired hits trigger one refresh
- callers continue using stale data during refresh
- failed refresh preserves the last valid catalog
- first startup failure falls back to legacy config
- indexed selector matches current selector results
- one-pass tie breaking exactly matches current sort order
- config reload rebuilds enabled-provider state
- routing facts are computed once without changing scenario results
- race tests cover refresh, config reload, and selection

### Acceptance

- Catalog-hit benchmark has no request-path mutex contention.
- Selection scales with models belonging to eligible providers, not the full
  catalog multiplied by provider count.
- Routing output is unchanged for the existing fixture suite.

## Recommended Commit Sequence

Keep the work reviewable and reversible:

1. `test(perf): add request-path performance baselines`
2. `fix(metrics): correct latency percentiles and ring buffers`
3. `feat(metrics): record request stages and cache usage`
4. `feat(core): preserve system cache directives`
5. `feat(storage): persist provider cache token usage`
6. `perf(token): cache repeated token counts`
7. `refactor(storage): combine completion persistence`
8. `perf(storage): batch telemetry writes asynchronously`
9. `perf(router): publish immutable catalog snapshots`
10. `perf(router): use indexed one-pass model selection`
11. `perf(router): reuse analyzed request facts`
12. `docs(perf): record benchmark and rollout results`

Run the affected unit and benchmark suites after every commit. Run the complete
verification suite before merging.

## Rollout Strategy

1. Ship metric correctness and stage timing first.
2. Observe a representative workload before enabling new optimizations by
   default.
3. Enable prompt-cache preservation because it is a fidelity fix, guarded by
   provider capability tests.
4. Enable the bounded token cache with conservative desktop defaults.
5. Enable async persistence with queue-depth and dropped-record visibility.
6. Enable the routing snapshot and indexed selector after result-equivalence
   tests pass.
7. Compare proxy overhead, TTFT, cache tokens, queue behavior, CPU, allocations,
   and memory before and after.

Temporary feature flags are appropriate for token caching and async storage.
They should be removed after one stable release if rollback is no longer
needed.

## Risks and Mitigations

| Risk | Mitigation |
| --- | --- |
| Cache metadata changes provider request shape | Golden provider-body tests and explicit capability handling |
| Token cache retains sensitive or large prompts | Process-local cache, strict byte limit, eviction, no persistence, optional disable |
| Token cache lock becomes contended | Parallel benchmark first; shard only with evidence |
| Async storage drops analytics | Bounded queue, dropped counter, dashboard warning, graceful drain |
| Shutdown loses accepted records | One lifecycle owner and deadline-aware drain tests |
| Stale catalog persists after refresh failure | Expose snapshot age and refresh error; retain correctness-preserving legacy fallback |
| Selector optimization changes tie breaking | Differential tests against the existing implementation |
| Metrics create their own hot path | Bounded storage, ring buffers, sampled tracing, allocation benchmarks |
| Benchmark noise produces false wins | Ten serial runs and `benchstat`; retain hardware and Go version context |

## Final Verification

```bash
GOCACHE=/tmp/routatic-perf-go-cache go test ./... -count=1
GOCACHE=/tmp/routatic-perf-go-cache go test ./... -count=1 -race
make lint
make lint-strict
git diff --check
```

Attach the final `benchstat` comparison and the request-path latency breakdown
to the pull request. Clearly distinguish local benchmark results from deployed
production observations.
