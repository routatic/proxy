# Cache Hit Rate and Request Latency Implementation Plan

Status: Proposed

Created: 2026-08-25

Branch: `perf/cache-hit-and-request-latency-plan`

Base: `main` at `d74b160`

## Objective

Improve repeated-request efficiency and reduce proxy-added latency without
changing routing decisions, provider-visible request semantics, or response
correctness.

The primary success measure is lower p95 time to first token for repeated
streaming conversations. Proxy-added latency and Provider Prompt Cache read
tokens are supporting measures. Request correctness is a hard requirement.
Non-streaming requests use total response time and are never included in TTFT
results.

Use a controlled repeated-conversation benchmark as the release gate. The
proxy has no reliable conversation identifier, so production metrics must not
guess that separate requests belong to one conversation. Production reports
show TTFT and Token Count Cache reuse as separate measurements.

The work covers five areas:

1. Preserve Provider Prompt Cache directives and cache-usage telemetry.
2. Cache repeated token-count results.
3. Remove SQLite telemetry writes from the response critical path.
4. Correct and extend performance measurements.
5. Make catalog refresh and cost-based routing cheaper.

General LLM response caching and in-flight request coalescing are explicitly
out of scope. Responses can be non-deterministic, streamed, tool-bearing, and
authorization-sensitive. This program does not store or share model responses.

## Delivery Principles

- Measure before and after every optimization.
- Change one bottleneck per performance commit.
- Keep caches bounded by an explicit capacity suited to the data they retain.
- Publish immutable data to request handlers.
- Treat analytics persistence as best-effort telemetry, not part of response
  correctness.
- Preserve stale-but-valid catalog data when a refresh fails.
- Keep provider-specific cache behavior behind explicit capability decisions.
- Include benchmark evidence in performance commit messages.

## Success Criteria

The implementation is complete when:

- p95 time to first token improves for repeated streaming conversations.
- Anthropic request, tool, system-block, and message-block cache directives
  survive normalization and reach supported upstream wire formats unchanged.
- Unsupported wire formats still omit cache directives intentionally.
- Cache read and cache creation token counts are visible in in-memory metrics,
  request history, and SQLite history when the upstream supplies them.
- Repeated conversation history produces Token Count Cache hits.
- The Token Count Cache has bounded memory, exposes hits, misses, and evictions,
  and remains race-free.
- A slow or locked SQLite database cannot delay a successful HTTP response.
- Storage queue overflow and shutdown-drain failures are observable.
- p95 and p99 calculations operate on sorted samples and are covered by tests.
- Catalog cache hits require no mutex acquisition.
- Catalog refresh does not make concurrent requests wait for disk or SQLite.
- Cost selection uses the provider index and a one-pass minimum rather than
  scanning the entire catalog once per provider and sorting all candidates.
- Each optimized path shows a statistically significant improvement under its
  targeted benchmark.
- Cold and unrelated paths do not regress by more than 5%.
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
  - current snapshot while a background refresh is active
- `BenchmarkAsyncStorageWriter`
  - enqueue only
  - batch sizes
  - queue saturation
  - graceful drain
- request-path integration benchmark using a fake provider with a fixed response
  and no external network.
- streaming repeated-conversation integration benchmark that appends one new
  turn per request and reports p95 TTFT

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
- upstream time to first non-empty model content
- upstream total duration
- response transformation
- storage enqueue
- total proxy duration

Measure TTFT from the moment the proxy begins reading the request until it
writes the first non-empty text, thinking, or tool content to the client.
Response headers, empty SSE events, and metadata-only events do not complete the
TTFT measurement. Record TTFT only for streaming requests. Record total response
time for non-streaming requests.

Use monotonic `time.Time` values already carried by Go timestamps. Avoid logging
every stage per request at info level; aggregate measurements in
`internal/metrics`.

## Workstream 1: Preserve Provider Prompt Cache Directives

### Current problem

Anthropic requests may carry cache directives at the top request level and on
tools, system blocks, and message content blocks. The current request types
only model system-block directives. `core.NormalizeRequest` then flattens the
system field and message content, so the provider registry path cannot preserve
the complete client request.

### Design

Use one ordered normalized content-block model for system and message content:

```go
type NormalizedCacheControl struct {
    Type string
}

type NormalizedContentBlock struct {
    Type         NormalizedContentType
    Text         string
    Image        *NormalizedImage
    ToolCall     *NormalizedToolCall
    ToolResult   *NormalizedToolResult
    Thinking     string
    CacheControl *NormalizedCacheControl
    Raw          json.RawMessage
}
```

`NormalizedRequest` stores ordered system blocks. `NormalizedMessage` stores
ordered message blocks. Replace the current separate text, image, thinking,
tool-call, and tool-result fields rather than retaining two representations
that can diverge.

Add read-only helpers such as `SystemText()` and `MessageText()` for routing and
token counting. Provider transformers iterate the ordered blocks directly so
they preserve the client's content order.

Extend the provider interface with a pure request-compatibility check. Each
provider adapter validates the normalized request against the selected wire
format before any network call. Return a typed compatibility error containing
the unsupported block or feature. The router does not contain wire-format
rules.

Normalization rules:

- A top-level cache directive remains attached to the normalized request.
- A string system prompt becomes one cacheless normalized block.
- An array remains an ordered set of blocks.
- `cache_control.type` is copied without provider interpretation.
- Tool cache directives remain attached to their tool definitions.
- Message cache directives remain attached to the ordered content blocks where
  the client placed them.
- An unknown content-block type remains in order as raw JSON.

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
- Anthropic-format providers may round-trip an unknown raw content block.
- Other formats mark the block as unsupported before making an upstream call.
  The fallback handler then tries only models whose provider format can
  preserve the block.
- If no compatible fallback exists, return a clear client error naming the
  unsupported block type.
- Compatibility failures do not count as provider failures and do not affect
  circuit breakers.
- When a selected provider format cannot use a cache directive, remove the
  directive and continue the request. Increment a bounded-cardinality metric
  by provider and wire format, and emit a sampled debug log. Never fail a
  request only because its cache directive is unsupported.

The provider or wire-format boundary owns the support decision. Core
normalization only preserves information.

Cache boundaries remain client-owned. The proxy must not add, move, or infer a
cache directive. It only preserves directives already present in the incoming
request and forwards them when the selected provider format supports them.

### Cache-usage telemetry

Extend `history.RequestRecord` with:

- `CacheReadTokens`
- `CacheCreationTokens`
- `CacheUsageReported`

Add nullable cache-token columns and a `cache_usage_reported` column through an
idempotent SQLite migration. When `cache_usage_reported` is false, persist the
token columns as null. When it is true, zero is a real provider-reported value.
Populate the fields from both streaming and non-streaming responses.

Extend the Responses usage type only after verifying the actual upstream
payload shape with provider fixtures. Do not infer a cached-token JSON field.

Expose raw cache counters per provider and model. Avoid a universal "hit rate"
formula until the provider-specific token accounting denominator is defined.
Do not include records with unreported cache usage in provider cache totals.

### Tests

- Normalize string system prompts.
- Normalize multiple system blocks while retaining order.
- Round-trip mixed text, image, thinking, tool-call, and tool-result blocks
  without reordering them.
- Round-trip unknown content blocks as raw JSON through Anthropic formats.
- Round-trip top-level, tool, system-block, and message-block cache directives.
- Verify the Zen Anthropic request body retains cache directives.
- Verify the DeepSeek Chat request retains supported directives.
- Verify unsupported Chat, Responses, and Gemini bodies omit them.
- Verify unsupported directives increment the omission metric without changing
  response behavior.
- Verify an unsupported content block skips incompatible models, uses a
  compatible fallback, and does not affect circuit breakers.
- Verify no compatible fallback returns a clear client error containing the
  block type.
- Verify provider compatibility validation performs no network or mutable
  provider-state work.
- Verify stream and non-stream usage populate history and storage.
- Verify reported zero remains distinct from unreported cache usage.
- Verify migrations work on both new and existing databases.
- Add golden provider-body fixtures where practical.

### Live provider verification

Before releasing Provider Prompt Cache support for a provider format:

- run an opt-in test outside CI with user-supplied credentials
- send the same stable prompt prefix twice
- change only the latest turn
- confirm the second response reports provider cache reuse
- record provider, model, wire format, TTFT, cache-read tokens, and
  cache-creation tokens
- never persist the prompt text in the test report

Unit and golden-body tests prove request fidelity but do not prove that an
upstream provider actually reuses the prefix.

If a provider accepts the request but does not expose observable cache-read
data, mark that cache path as unverified or experimental. Do not advertise a
cache-hit improvement without provider-reported evidence.

### Acceptance

- No cache directive disappears before provider capability handling.
- Existing provider stripping tests continue to pass.
- Requests without cache directives keep their existing common wire shape.
- Every advertised provider cache path has a successful live verification
  result. Unverified paths are labeled experimental and make no cache-hit claim.

## Workstream 2: Cache Repeated Token Counts

### Current problem

Every request tokenizes the system prompt and every message again. In an
interactive session, most previous message text is identical to the prior turn.

### Design

Add a bounded cache owned by `token.Counter`.

Each cache entry represents one system block or one message. Do not cache a
whole conversation as one entry because adding a new turn would invalidate the
entire key and prevent reuse of the stable history.

Initial implementation:

- one concurrency-safe LRU protected by one lock
- fixed-size SHA-256 keys built from the tokenizer name and input text
- no retained copy of the raw prompt text
- maximum of 8,192 entries by default
- skip entries below a measured minimum string length
- no unbounded `sync.Map`
- no `sync.Pool` unless a profile identifies allocation churn it can solve

The count remains deterministic and encoding-specific. Including the tokenizer
name in the fingerprint prevents a future model-specific tokenizer from
reusing an incompatible count. A cryptographic hash collision is treated as
negligible; the cache must not keep raw text only to check collisions.

Add explicit configuration:

```json
{
  "performance": {
    "token_cache_enabled": true,
    "token_cache_max_entries": 8192
  }
}
```

Defaults must be safe for desktop use. A disabled cache must preserve current
behavior exactly. The cache is enabled by default only after its benchmark and
race-test gates pass. Keep the disable switch for troubleshooting.

When cache settings change during config reload, build a new empty cache and
publish it atomically. Do not resize or mutate the active cache in place. Old
entries are discarded.

Do not add miss coalescing initially. Add it only if a parallel benchmark shows
that concurrent identical misses are common enough to outweigh coordination
cost. Do not shard the LRU initially. Add shards only if the parallel benchmark
shows lock contention.

### Metrics

Record:

- hits
- misses
- evictions
- skipped-small-input counts
- current entries
- tokenization duration

### Tests

- deterministic hit after first count
- distinct strings and encodings produce distinct fingerprints
- entry limit evicts
- disabled cache bypasses storage
- small-entry policy works
- parallel race test
- large input does not cause unbounded retention or retain raw prompt text
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

Use `requests.duration_ms` as the single source for latency reports. Stop
writing new rows to `latency_samples`, and update latency queries to read from
`requests`. This removes the duplicate write before asynchronous batching is
introduced. Leave the old table and its existing rows untouched in this branch
for rollback safety. A later release may remove the table after the new reports
have been proven stable.

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
- writes request rows in one transaction
- preserves record order within each batch
- keeps the existing single SQLite writer connection
- emits counters for enqueued, persisted, dropped, failed, queue depth, batch
  size, and drain duration
- samples repeated error logs

Do not add application-level write retries. SQLite already applies its
configured busy timeout for temporary lock contention. If a batch still fails,
count and drop it, emit a sampled error log, and continue with the next batch.

Queue capacity, batch size, and flush interval are internal constants selected
by benchmarks. Do not add user-facing settings for them in this work. Expose
queue depth, drops, batch size, failures, and drain time so later production
evidence can justify configuration if needed.

Because this data is analytics telemetry, queue saturation should not block a
user response. When the queue is full, drop the newest analytics record and
increment an explicit counter. Older already-accepted records remain ordered.
Expose the drop count through metrics and show a dashboard warning when it is
non-zero.

Keep the in-memory history update synchronous because it is O(1) and supplies
the live dashboard immediately.

Enable async storage by default after its test and benchmark gates pass. Keep a
temporary setting that switches back to the current synchronous writer for one
stable release. Remove the setting and synchronous path after that release if
no rollback is needed.

### Shutdown lifecycle

Change server shutdown ordering:

1. Stop accepting new HTTP requests and wait for active handlers.
2. Close the completion recorder to new entries.
3. Drain accepted records within the caller's existing shutdown deadline.
4. Stop retention work.
5. Close SQLite.

Use the same lifecycle for signal-based and programmatic shutdown. The current
paths must not close SQLite while request handlers can still enqueue work. If
the deadline expires before the queue drains, log and count the records that
were not persisted.

### Tests

- a blocking storage backend cannot delay an HTTP response
- latency reports use `requests.duration_ms`
- one completion produces one SQLite write
- existing `latency_samples` data and schema remain untouched
- batches flush by size and by interval
- queue saturation follows the documented policy
- persistence errors do not stop later batches
- failed batches are counted and are not retried
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

- Replace latency slice shifting with fixed ring buffers holding the latest
  1,000 samples for each global metric.
- Calculate percentiles from a sorted copy.
- Sort once when calculating multiple percentiles.
- Copy samples while holding the lock, then release the lock before sorting.
- Use the same ring-buffer implementation with 200 samples for each known
  `provider/model`.
- Report the sample count with every percentile.
- Report p50, p90, and p95 for per-model data. Do not report per-model p99 from
  only 200 samples.
- Add table tests for empty, one-element, ordered, reverse-ordered, and repeated
  samples.

### New measurements

Add counters and bounded timing samples for the request stages listed in
Measurement Foundation.

Keep detailed stage timings in bounded memory only. Do not add SQLite columns
for stage breakdowns. Persistent request records continue to store total
duration, input and output tokens, and Provider Prompt Cache usage.

Create per-model metric state only for models known by the active config or
catalog. Group arbitrary or unknown requested model names under `other` so
request input cannot grow metric maps without a bound.

Key per-model metrics by canonical `provider/model` identity. Do not combine
the same model name across providers because their latency and cache behavior
may differ.

For streaming requests:

- record time to first non-empty model content separately from total stream
  duration
- use `sync.Once` or equivalent so first-content timing is recorded exactly once
- distinguish headers and metadata events from real model content

Do not add DNS, TLS, or connection-level `httptrace` instrumentation in this
program. Add it later only if request-stage timing shows connection setup is a
meaningful bottleneck. Do not alter HTTP pool sizes or enable a protocol based
only on intuition.

### Exposure

- Keep `/health` compact.
- Add detailed performance data to the existing metrics/dashboard API.
- Include Token Count Cache and storage queue state.
- Include raw provider cache-token counters.
- Document whether every duration includes or excludes upstream time.

### Tests and benchmarks

- percentile correctness independent of insertion order
- ring-buffer eviction order
- concurrent record/snapshot race tests
- unknown model names remain grouped under `other`
- identical model names on different providers remain separate
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

- Every request returns the current snapshot with one atomic load.
- Requests never start refresh work, wait for refresh work, or check files or
  SQLite for freshness.
- A successful refresh atomically publishes a new immutable snapshot.
- A failed refresh records the error and retains the last valid snapshot with
  no maximum age.
- Startup performs one bounded synchronous load when a catalog source exists.
- One background loop refreshes every 30 seconds.
- Catalog update events signal the same loop to refresh early.

Do not mutate an `IndexedCatalog` after publishing it.
Expose snapshot age and the latest refresh error. Use legacy routing only when
the process has never loaded a valid catalog.

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

Do not cache the final selected model. Token count, tools, images, reasoning
needs, provider state, and configuration can differ per request. Reuse immutable
indexes, then run the one-pass comparison for each request.

Pass already-computed `RequestFacts` and constraints through routing rather than
re-running message scans and lowercasing.

### Tests

- fresh catalog hit performs no refresh
- requests do not trigger refresh or block during refresh
- background timer and catalog-update events trigger one refresh at a time
- callers continue using the current snapshot during refresh
- failed refresh preserves the last valid catalog
- an arbitrarily old valid catalog remains usable and reports its age
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
4. `refactor(core): preserve ordered normalized content blocks`
5. `feat(core): preserve all client cache directives`
6. `feat(storage): persist provider cache token usage`
7. `perf(token): cache repeated token counts`
8. `refactor(storage): remove duplicate latency writes`
9. `refactor(storage): add completion recorder`
10. `perf(storage): batch telemetry writes asynchronously`
11. `perf(router): publish immutable catalog snapshots`
12. `perf(router): use indexed one-pass model selection`
13. `perf(router): reuse analyzed request facts`
14. `docs(perf): record benchmark and rollout results`

Run the affected unit and benchmark suites after every commit. Run the complete
verification suite before merging.

## Pull Request Delivery

Ship the work as one pull request with five ordered phases:

1. **Metrics and benchmarks**
   - benchmark foundations
   - percentile correctness
   - bounded metric rings
   - TTFT and request-stage measurements
2. **Ordered content and Provider Prompt Cache fidelity**
   - ordered normalized content blocks
   - request, tool, system, and message cache directives
   - cache-usage storage and metrics
3. **Token Count Cache**
   - SHA-256 fingerprints
   - 8,192-entry LRU
   - cache metrics and configuration
4. **Async SQLite storage**
   - read latency from `requests.duration_ms`
   - stop writing duplicate latency samples
   - bounded completion queue, batching, and shutdown drain
5. **Catalog and routing speed**
   - background catalog refresh
   - atomic immutable snapshots
   - indexed one-pass selection
   - one request-fact analysis pass

Keep the commits small and in the recommended order. Each phase must pass its
focused tests and benchmarks before work moves to the next phase. The pull
request is ready to merge only when all five phases and the full verification
suite pass.

Performance gates are specific to each phase:

- the targeted benchmark must show a statistically significant improvement
- cold and unrelated paths must not regress by more than 5%
- correctness-only fixes may ship without claiming a speed improvement
- the complete pull request must improve repeated-conversation p95 TTFT

## Rollout Strategy

1. Build and verify metric correctness and stage timing first within the pull
   request.
2. Observe a representative workload before enabling new optimizations by
   default.
3. Enable Provider Prompt Cache preservation because it is a fidelity fix,
   guarded by provider capability tests.
4. Enable the bounded Token Count Cache by default with 8,192 entries after
   benchmark and race-test gates pass. Keep the disable switch.
5. Enable async persistence with queue-depth and dropped-record visibility.
6. Enable the routing snapshot and indexed selector after result-equivalence
   tests pass.
7. Compare proxy overhead, TTFT, cache tokens, queue behavior, CPU, allocations,
   and memory before and after.
8. Run the opt-in live Provider Prompt Cache checks before advertising provider
   support.

Keep the Token Count Cache disable switch for troubleshooting. Keep the async
storage rollback switch for one stable release, then remove it together with
the synchronous writer if no rollback is needed.

## Risks and Mitigations

| Risk | Mitigation |
| --- | --- |
| Cache metadata changes provider request shape | Golden provider-body tests and explicit capability handling |
| Token Count Cache retains prompt text | Store only fixed SHA-256 fingerprints, enforce the entry limit, and never persist entries |
| Token Count Cache lock becomes contended | Parallel benchmark first; shard only with evidence |
| Async storage drops analytics | Bounded queue, dropped counter, dashboard warning, graceful drain |
| Shutdown loses accepted records | One lifecycle owner and deadline-aware drain tests |
| Stale catalog persists after refresh failure | Expose snapshot age and refresh error; retain correctness-preserving legacy fallback |
| Selector optimization changes tie breaking | Differential tests against the existing implementation |
| Metrics create their own hot path | Bounded storage, ring buffers, and allocation benchmarks |
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
