# Implementation Plan: SQLite Storage

**Spec:** `.planning/specs/sqlite-storage.md`  
**Created:** 2026-07-11  
**Status:** draft

---

## Overview

Replace in-memory ring buffers with SQLite for persistent storage with time-range query support.

---

## Components Table

| Component | Type | Purpose |
|-----------|------|---------|
| `storage.Database` | Go struct | Connection pool, schema initialization |
| `storage.Requests` | Go struct | CRUD for request records |
| `storage.Latency` | Go struct | CRUD for latency samples + percentile queries |
| `storage.Logs` | Go struct | CRUD for log entries (optional) |
| `storage.Retention` | Go struct | Background cleanup of old records |

---

## File Locations Table

| File | Location | Purpose |
|------|----------|---------|
| `internal/storage/database.go` | New | Connection pool, schema migrations |
| `internal/storage/requests.go` | New | Request record storage |
| `internal/storage/latency.go` | New | Latency sample storage + percentiles |
| `internal/storage/logs.go` | New | Log entry storage |
| `internal/storage/retention.go` | New | Cleanup job |
| `internal/config/config.go` | Edit | Add storage config fields |
| `internal/metrics/metrics.go` | Edit | Use SQLite for latency tracking |
| `internal/gui/perf.go` | Edit | Honor `?range=` parameter |
| `cmd/routatic-proxy/main.go` | Edit | Initialize storage |

---

## Dependencies

```bash
go get modernc.org/sqlite
```

Pure Go SQLite driver - works with `CGO_ENABLED=0`.

---

## Phase Breakdown

### Phase 1: Core Storage Layer

| # | Task | Files |
|---|------|-------|
| 1.1 | Add `modernc.org/sqlite` dependency | `go.mod` |
| 1.2 | Create `storage.Database` with connection pool | `internal/storage/database.go` |
| 1.3 | Create schema (requests, latency_samples, logs, schema_info) | `internal/storage/database.go` |
| 1.4 | Implement `storage.Requests` with Insert/Last/Count | `internal/storage/requests.go` |
| 1.5 | Implement `storage.Latency` with Insert/GetStats | `internal/storage/latency.go` |
| 1.6 | Write unit tests | `internal/storage/*_test.go` |

### Phase 2: Migrate History

| # | Task | Files |
|---|------|-------|
| 2.1 | Create `SQLiteHistory` implementing History interface | `internal/history/sqlite.go` |
| 2.2 | Update main.go to use SQLite-backed history | `cmd/routatic-proxy/main.go` |
| 2.3 | Add storage config to config struct | `internal/config/config.go` |
| 2.4 | Integration test for request persistence | `internal/history/sqlite_test.go` |

### Phase 3: Migrate Metrics Latency

| # | Task | Files |
|---|------|-------|
| 3.1 | Add `InsertLatency()` method to storage | `internal/storage/latency.go` |
| 3.2 | Add `GetStats(since time.Time)` with time filter | `internal/storage/latency.go` |
| 3.3 | Update `metrics.Metrics` to use SQLite | `internal/metrics/metrics.go` |
| 3.4 | Update `/api/perf/models` to pass time range | `internal/gui/perf.go` |
| 3.5 | Update frontend to use `?range=` parameter | `internal/gui/assets/app.js` |

### Phase 4: Retention Policy

| # | Task | Files |
|---|------|-------|
| 4.1 | Add retention config fields | `internal/config/config.go` |
| 4.2 | Implement background cleanup goroutine | `internal/storage/retention.go` |
| 4.3 | Add retention settings to GUI | `internal/gui/assets/index.html`, `app.js` |
| 4.4 | Test retention deletes old records | `internal/storage/retention_test.go` |

### Phase 5: Logs (Optional)

| # | Task | Files |
|---|------|-------|
| 5.1 | Implement `storage.Logs` with Insert/Last | `internal/storage/logs.go` |
| 5.2 | Update `gui.LogBuffer` to persist to SQLite | `internal/gui/logs.go` |
| 5.3 | Keep in-memory pub/sub for SSE streaming | `internal/gui/logs.go` |

---

## API Details

### storage.Database

```go
type Database struct {
    db   *sql.DB
    path string
    mu   sync.RWMutex
}

func Open(path string) (*Database, error)
func (d *Database) Close() error
func (d *Database) BeginTx(ctx context.Context) (*sql.Tx, error)
```

### storage.Requests

```go
type Requests struct {
    db *Database
}

func (r *Requests) Insert(rec history.RequestRecord) error
func (r *Requests) Last(n int) ([]history.RequestRecord, error)
func (r *Requests) Since(since time.Time) ([]history.RequestRecord, error)
func (r *Requests) Count() (int64, error)
```

### storage.Latency

```go
type Latency struct {
    db *Database
}

func (l *Latency) Insert(model string, latency time.Duration) error
func (l *Latency) GetStats(since time.Time) ([]ModelLatencyStats, error)

type ModelLatencyStats struct {
    Model string
    Count int64
    Avg   time.Duration
    P50   time.Duration
    P90   time.Duration
    P99   time.Duration
    Min   time.Duration
    Max   time.Duration
}
```

### storage.Retention

```go
type Retention struct {
    db     *Database
    days   int
    stopCh chan struct{}
}

func NewRetention(db *Database, days int) *Retention
func (r *Retention) Start()  // Background goroutine
func (r *Retention) Stop()
func (r *Retention) Run() error  // Delete old records
```

---

## Testing Plan

### Unit Tests

| Test | File | What it tests |
|------|------|---------------|
| `TestDatabase_Open` | `database_test.go` | Open/create database |
| `TestRequests_InsertAndLast` | `requests_test.go` | Insert + retrieve |
| `TestLatency_InsertAndGetStats` | `latency_test.go` | Percentile calculations |
| `TestLatency_TimeRange` | `latency_test.go` | `Since()` filtering |
| `TestRetention_Cleanup` | `retention_test.go` | Old record deletion |

### Integration Tests

| Test | What it tests |
|------|---------------|
| `TestHistory_PersistsAcrossRestarts` | Close + open database, verify data |
| `TestMetrics_PerformanceHeatmap` | End-to-end: insert → API → verify |
| `TestConcurrency_MultipleWriters` | Concurrent inserts from multiple goroutines |

### Benchmarks

```bash
go test -bench=. -benchmem ./internal/storage/
```

- `BenchmarkLatency_Insert` - Target: < 1ms per insert
- `BenchmarkLatency_GetStats_1k` - Target: < 10ms for 1000 samples
- `BenchmarkRequests_Insert` - Target: < 1ms per insert

---

## Gate 2 Checklist

**Architecture:**
- [x] Pure Go, no CGO (`modernc.org/sqlite`)
- [x] Connection pooling for thread safety
- [x] WAL mode for crash recovery
- [x] Indexed tables for performance

**Task Breakdown:**
- [x] All files listed with locations
- [x] All changes mapped to phases
- [x] Each task is small (one file or one function)
- [x] Dependencies clear (Phase 1 → 2 → 3 → 4)

**Testing:**
- [x] Unit tests for CRUD
- [x] Integration tests for persistence
- [x] Benchmarks for performance
- [x] Concurrent write testing

---

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Performance regression | Medium | High | Benchmark early, batch writes |
| Disk space growth | Medium | Medium | Retention policy, vacuum |
| SQLite file corruption | Low | High | WAL mode, fsync, backups |
| Migration complexity | Low | Medium | Start fresh, no legacy migration |
