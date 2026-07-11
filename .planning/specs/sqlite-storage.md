# Feature Spec: SQLite-Based Storage for routatic-proxy

**Status:** draft  
**Created:** 2026-07-11  
**Priority:** medium

---

## Problem Statement

Current storage is in-memory ring buffers:
- `history.History` - Request records (last 1000)
- `metrics.Metrics` - Latency samples (last 1000)
- `gui.LogBuffer` - Log entries (last 1000)

**Limitations:**
1. Data lost on restart
2. Cannot query by time range (no timestamps stored)
3. Cannot aggregate stats over arbitrary periods
4. Limited to 1000 records (cannot tune per data type)
5. Performance heatmap cannot filter by `range=1h|24h|7d`

---

## Proposed Solution

Replace in-memory ring buffers with SQLite storage using `modernc.org/sqlite` (pure Go, no CGO).

**Benefits:**
- Persist across restarts
- Time-range queries for performance metrics
- Configurable retention (prune old records)
- Export aggregated data for external analysis
- Foundation for future analytics features

---

## Scope

### In Scope
- SQLite database at `~/.local/share/routatic-proxy/data.db`
- Tables: `requests`, `latency_samples`, `logs`
- Migrate existing `history.History` to SQLite
- Migrate `metrics.Metrics` latency tracking to SQLite
- Migrate `gui.LogBuffer` to SQLite (optional)
- Automatic cleanup of old records (configurable retention)
- Update `/api/perf/models` to support `?range=` parameter

### Out of Scope
- WebUI for querying historical data (beyond existing heatmap)
- Schema migrations (v1 schema only for now)
- Multiple database backends (PostgreSQL, etc.)
- Replication or clustering

---

## Requirements

### Functional

1. **Request Storage**
   - Store all `RequestRecord` fields with timestamps
   - Query last N requests (existing `Last(n)` API)
   - Query by time range for analytics
   - Auto-prune records older than retention period

2. **Latency Samples**
   - Store latency with timestamp per model
   - Calculate P50/P90/P99 for arbitrary time ranges
   - Support `?range=1h|24h|7d|all` in `/api/perf/models`

3. **Logs (Optional)**
   - Store log entries with timestamps
   - Query with level filter
   - Streaming still works (pub/sub in-memory, persist to SQLite async)

4. **Database Management**
   - Create database if not exists
   - Vacuum on startup to reclaim space (optional)
   - Backup/restore via config export (include SQLite file)

### Non-Functional

- **Performance**: Insert latency < 1ms (batch writes acceptable)
- **Memory**: Don't load entire database into memory
- **Disk**: Default retention = 7 days, configurable
- **Concurrency**: Thread-safe writes (use connection pool or mutex)
- **No CGO**: Must work with `CGO_ENABLED=0`

---

## User Stories

1. As an operator, I want to see performance metrics for the last 24 hours, not just since last restart
2. As a developer, I want to debug routing decisions from requests made hours ago
3. As an operator, I want logs to persist across restarts for post-mortem analysis
4. As a developer, I want to export request data for analysis in external tools

---

## Architecture

### Package Structure

```
internal/storage/
├── database.go      # Connection pool, initialization
├── requests.go      # Request record CRUD
├── latency.go       # Latency sample CRUD + percentiles
├── logs.go          # Log entry CRUD (optional)
└── retention.go     # Background cleanup job
```

### Database Schema

```sql
-- Request records (migrates from history.RequestRecord)
CREATE TABLE IF NOT EXISTS requests (
    id TEXT PRIMARY KEY,
    model TEXT NOT NULL,
    provider TEXT,
    scenario TEXT,
    start_time TIMESTAMP NOT NULL,
    duration_ms INTEGER,
    input_tokens INTEGER,
    output_tokens INTEGER,
    streaming INTEGER,  -- SQLite doesn't have BOOLEAN
    success INTEGER,
    error_msg TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_requests_start_time ON requests(start_time);
CREATE INDEX idx_requests_model ON requests(model);

-- Latency samples per model
CREATE TABLE IF NOT EXISTS latency_samples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    model TEXT NOT NULL,
    latency_ms INTEGER NOT NULL,
    recorded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_latency_model_time ON latency_samples(model, recorded_at);

-- Log entries
CREATE TABLE IF NOT EXISTS logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    level TEXT NOT NULL,
    message TEXT,
    field TEXT,
    value TEXT,
    recorded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_logs_time ON logs(recorded_at);

-- Configuration placeholder (schema version)
CREATE TABLE IF NOT EXISTS schema_info (
    key TEXT PRIMARY KEY,
    value TEXT
);

INSERT OR IGNORE INTO schema_info (key, value) VALUES ('version', '1');
```

---

## API Changes

### New Endpoints

None required (existing endpoints work with SQLite backend)

### Modified Endpoints

| Endpoint | Change |
|----------|--------|
| `GET /api/perf/models?range=1h` | Now honors `range` parameter |
| `GET /api/history?range=24h` | Optional: Add time range filter |

### Internal API Changes

| Current | New |
|---------|-----|
| `history.History.Add()` | `storage.Requests.Insert()` |
| `history.History.Last(n)` | `storage.Requests.Last(n)` |
| `metrics.Metrics.recordModelLatency()` | `storage.Latency.Insert()` |
| `metrics.Metrics.GetModelLatencyStats()` | `storage.Latency.GetStats(range)` |
| `gui.LogBuffer.Add()` | `storage.Logs.Insert()` (if implemented) |

---

## Implementation Plan

### Phase 1: Core Storage Layer

| Task | Files |
|------|-------|
| Add SQLite dependency | `go.mod` |
| Create database package | `internal/storage/database.go` |
| Implement requests table | `internal/storage/requests.go` |
| Implement latency table | `internal/storage/latency.go` |
| Unit tests | `internal/storage/*_test.go` |

### Phase 2: Migrate History

| Task | Files |
|------|-------|
| Create storage adapter for History interface | `internal/history/sqlite.go` |
| Update main to use SQLite-backed history | `cmd/routatic-proxy/main.go` |
| Integration tests | `internal/history/history_test.go` |

### Phase 3: Migrate Metrics

| Task | Files |
|------|-------|
| Update metrics to write to SQLite | `internal/metrics/metrics.go` |
| Update `GetModelLatencyStats()` to accept time range | `internal/metrics/metrics.go` |
| Update `/api/perf/models` to use time range | `internal/gui/perf.go` |

### Phase 4: Retention Policy

| Task | Files |
|------|-------|
| Add retention config field | `internal/config/config.go` |
| Implement background cleanup job | `internal/storage/retention.go` |
| Add retention settings to GUI | `internal/gui/assets/` |

### Phase 5: Logs (Optional)

| Task | Files |
|------|-------|
| Implement logs table | `internal/storage/logs.go` |
| Update `LogBuffer` to persist to SQLite | `internal/gui/logs.go` |

---

## Configuration

Add to `config.json`:

```json
{
  "storage": {
    "database_path": "~/.local/share/routatic-proxy/data.db",
    "retention_days": 7,
    "vacuum_on_startup": false
  }
}
```

---

## Testing Strategy

### Unit Tests
- CRUD operations for each table
- Percentile calculations
- Time range filtering
- Retention cleanup

### Integration Tests
- Insert → Query → Verify
- Concurrent writes
- Large dataset performance (10k+ records)

### Benchmark Tests
- Insert latency
- Query latency with indexes
- Percentile calculation over N samples

---

## Risks

| Risk | Mitigation |
|------|------------|
| SQLite file corruption | Write-ahead log (WAL mode), backups |
| Performance regression | Batch writes, connection pooling |
| Disk space growth | Auto-retention, vacuum |
| CGO dependency | Use `modernc.org/sqlite` (pure Go) |
| Migration complexity | Start fresh, no v0 data migration |

---

## Success Criteria

1. All request data persists across restart
2. `/api/perf/models?range=24h` returns accurate stats
3. Database file < 50MB after 1 week of normal usage
4. No performance regression on request throughput
5. Tests pass with `CGO_ENABLED=0`

---

## Estimated Effort

- **Phase 1-2**: 2-3 days (core migration)
- **Phase 3-4**: 1-2 days (metrics + retention)
- **Phase 5**: 1 day (logs, optional)
- **Total**: 4-5 days
