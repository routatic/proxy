# Implementation Plan: Advanced GUI Features

**Spec:** `.planning/specs/gui-advanced-features.md`  
**Created:** 2026-07-11  
**Status:** draft  
**Stack:** Go backend + vanilla JS frontend

---

## Architecture Overview

All features follow the existing GUI pattern:
- Backend: New API endpoints in `internal/gui/server.go`
- Frontend: New tab/sections in `internal/gui/assets/`
- No external dependencies (pure Go + vanilla JS)

---

## Components Table

| Component | Type | Purpose |
|-----------|------|---------|
| LogStreamer | Go struct | SSE log streaming to connected clients |
| ConfigExporter | Go func | Export config JSON with optional anonymization |
| ConfigImporter | Go func | Validate and apply imported config |
| ModelPerfAggregator | Go struct | Calculate latency percentiles per model |
| FallbackChainEditor | JS module | Drag-drop UI for fallback ordering |
| ModelTester | JS module | Send test prompts to models |

---

## File Locations Table

| File | Location | Purpose |
|------|----------|---------|
| `internal/gui/logs.go` | New | Log streaming SSE implementation |
| `internal/gui/config_io.go` | New | Export/import config logic |
| `internal/gui/perf.go` | New | Model performance aggregation |
| `internal/gui/assets/app-logs.js` | New | Logs tab JS logic |
| `internal/gui/assets/app-perf.js` | New | Performance heatmap JS |
| `internal/gui/assets/app-fallback.js` | New | Fallback editor JS |
| `internal/gui/assets/app-test.js` | New | Model tester JS |
| `internal/gui/assets/index.html` | Edit | Add new tabs/modals |
| `internal/gui/assets/style.css` | Edit | Styles for new components |
| `internal/gui/server.go` | Edit | Add new API endpoints |
| `internal/gui/assets/app.js` | Edit | Integrate new modules |

---

## Files to Change

| File | What Changes | Why |
|------|-------------|-----|
| `internal/gui/server.go` | Add 10+ new endpoints | Expose logs, config IO, perf data, test API |
| `internal/gui/assets/index.html` | Add 4 new tabs + modals | UI for new features |
| `internal/gui/assets/style.css` | Add styles for heatmap, drag-drop, logs | Visual presentation |
| `internal/gui/assets/app.js` | Import new modules, integrations | Wire everything together |
| `internal/history/record.go` | Add `latency_ms` field if missing | Track latency per request |
| `internal/metrics/metrics.go` | Add per-model latency tracking | Support percentile queries |

---

## Phase Breakdown

### Phase 1: Core Infrastructure (Backend)

| # | Task | Files |
|---|------|-------|
| 1.1 | Add per-model latency tracking to metrics | `internal/metrics/metrics.go` |
| 1.2 | Add SSE log streamer struct | `internal/gui/logs.go` (new) |
| 1.3 | Add config export/import handlers | `internal/gui/config_io.go` (new) |
| 1.4 | Add model performance aggregator | `internal/gui/perf.go` (new) |
| 1.5 | Register all new endpoints in server.go | `internal/gui/server.go` |

### Phase 2: Log Viewer Tab (Frontend)

| # | Task | Files |
|---|------|-------|
| 2.1 | Add Logs tab HTML structure | `internal/gui/assets/index.html` |
| 2.2 | Add log viewer styles | `internal/gui/assets/style.css` |
| 2.3 | Implement SSE client + log display | `internal/gui/assets/app-logs.js` (new) |
| 2.4 | Add pause/resume/search/filter controls | `internal/gui/assets/app-logs.js` |
| 2.5 | Integrate log module into app.js | `internal/gui/assets/app.js` |

### Phase 3: Config Backup/Restore

| # | Task | Files |
|---|------|-------|
| 3.1 | Add backup/restore buttons in Settings | `internal/gui/assets/index.html` |
| 3.2 | Implement download config with anonymize option | `internal/gui/assets/app.js` |
| 3.3 | Implement upload + validation modal | `internal/gui/assets/app.js` |
| 3.4 | Add i18n strings for backup/restore | `internal/gui/assets/app.js` |

### Phase 4: Model Performance Heatmap

| # | Task | Files |
|---|------|-------|
| 4.1 | Add Performance tab HTML | `internal/gui/assets/index.html` |
| 4.2 | Add heatmap table styles | `internal/gui/assets/style.css` |
| 4.3 | Implement data fetching + rendering | `internal/gui/assets/app-perf.js` (new) |
| 4.4 | Add time range filter + sorting | `internal/gui/assets/app-perf.js` |
| 4.5 | Color-code cells based on latency | `internal/gui/assets/app-perf.js` |

### Phase 5: Fallback Chain Editor

| # | Task | Files |
|---|------|-------|
| 5.1 | Add Fallback tab HTML + modal | `internal/gui/assets/index.html` |
| 5.2 | Add drag-drop styles | `internal/gui/assets/style.css` |
| 5.3 | Implement drag-drop reorder logic | `internal/gui/assets/app-fallback.js` (new) |
| 5.4 | Add save preview + apply API | `internal/gui/assets/app-fallback.js` |
| 5.5 | Handle scenario-specific chains | `internal/gui/assets/app-fallback.js` |

### Phase 6: Quick Model Test

| # | Task | Files |
|---|------|-------|
| 6.1 | Add Test tab HTML + modal | `internal/gui/assets/index.html` |
| 6.2 | Add test UI styles | `internal/gui/assets/style.css` |
| 6.3 | Implement test request sender | `internal/gui/assets/app-test.js` (new) |
| 6.4 | Display streaming response | `internal/gui/assets/app-test.js` |
| 6.5 | Show metrics (latency, tokens) | `internal/gui/assets/app-test.js` |

---

## Parallel vs Sequential

| Parallel Group | Tasks | Why |
|---------------|-------|-----|
| Group A | 1.1, 1.2, 1.3, 1.4 | Independent backend modules |
| Group B | 2.1-2.5, 3.1-3.4, 4.1-4.5, 5.1-5.5, 6.1-6.5 | Frontend tabs are independent |

| Sequential | Depends On | Why |
|-----------|-----------|-----|
| Phase 2 | Phase 1 | Frontend needs backend endpoints |
| Phase 3 | Phase 1 | Export/import needs API |
| Phase 4 | Phase 1 | Heatmap needs perf data |
| Phase 5 | Phase 1 | Fallback editor needs config API |
| Phase 6 | Phase 1 | Test needs backend proxy |

---

## API Endpoints

### Logs
- `GET /api/logs/stream` — SSE endpoint for live logs
- `POST /api/logs/clear` — Clear log buffer (optional)

### Config Backup/Restore
- `GET /api/config/export?anonymize=true` — Download config
- `POST /api/config/import` — Upload and validate config
- `POST /api/config/apply` — Apply imported config

### Model Performance
- `GET /api/perf/models` — Get latency percentiles per model
- Query params: `?range=1h|24h|7d|all`

### Fallback Chain
- `GET /api/fallback/chains` — Get all scenario fallback chains
- `POST /api/fallback/update` — Update fallback order
- `POST /api/fallback/preview` — Preview chain before applying

### Model Test
- `POST /api/test/send` — Send test prompt to model
- Returns: streaming response + metrics

---

## Testing Plan

### Backend Tests
- Test per-model latency tracking in `metrics/metrics_test.go`
- Test SSE log streaming in `gui/logs_test.go`
- Test config anonymization in `gui/config_io_test.go`
- Test performance aggregation in `gui/perf_test.go`

### Frontend Tests
- Test log viewer SSE connection
- Test config import validation
- Test drag-drop fallback reorder
- Test model test request

### Integration Tests
- Test full log streaming flow
- Test config backup → restore cycle
- Test heatmap data accuracy against history
- Test fallback chain persistence

---

## Gate 2 Checklist

**Architecture:**
- [x] Follows existing GUI patterns (embedded assets, API endpoints)
- [x] Each module is independent (logs, config, perf, fallback, test)
- [x] No external JS dependencies (vanilla JS only)

**Task Breakdown:**
- [x] All files listed with locations
- [x] All changes mapped to phases
- [x] Each task is small (one endpoint or one feature)
- [x] Dependencies between phases are clear
- [x] Backend (Phase 1) must complete before frontend phases

**Testing:**
- [x] Backend unit tests planned
- [x] Frontend smoke tests planned
- [x] Integration tests planned
- [x] Edge cases covered (empty logs, invalid config, no history)

---

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| SSE connection drops | Auto-reconnect with exponential backoff |
| Config import breaks proxy | Validate before applying, show diff preview |
| Performance data grows unbounded | Cap history at 1000 records (already done) |
| Drag-drop conflicts with scrolling | Use mouseleave to detect scroll intent |
