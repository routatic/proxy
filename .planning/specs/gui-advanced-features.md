# Feature Spec: Advanced GUI Features for routatic-proxy

**Status:** draft  
**Created:** 2026-07-11  
**Priority:** medium

---

## Problem Statement

The current GUI dashboard provides basic metrics and history viewing. Users need more advanced features for debugging, configuration management, model performance analysis, and testing.

---

## Proposed Features

### 1. Live Log Viewer Tab

**Purpose:** Real-time log streaming for debugging and monitoring.

**Requirements:**
- New "Logs" tab in the dashboard
- SSE-based streaming from proxy server
- Real-time log output display
- Pause/resume streaming capability
- Clear logs button
- Filter by log level (debug, info, warn, error)
- Search/filter by keyword
- Auto-scroll toggle

**User Stories:**
- As a developer, I want to watch live logs to debug routing decisions
- As an operator, I want to filter error logs to investigate failures

---

### 2. Config Backup/Restore

**Purpose:** Export and import proxy configuration for backup or migration.

**Requirements:**
- Export config as JSON file (download)
- Import config from JSON file (upload)
- Anonymize API keys on export (optional checkbox)
- Validate config on import before applying
- Show preview of imported config
- Reset to defaults button

**User Stories:**
- As an operator, I want to backup config before making changes
- As a developer, I want to share config with team (anonymized)

---

### 3. Model Performance Heatmap

**Purpose:** Visualize latency and success rate per model.

**Requirements:**
- Visual heatmap table showing:
  - Average latency per model
  - P50, P90, P99 latency percentiles
  - Success rate
  - Request count
- Color-coded cells (red=slow, green=fast)
- Sortable by any column
- Filter by time range (last hour, 24h, 7d, all)

**User Stories:**
- As an operator, I want to identify slow models
- As a developer, I want to compare model performance metrics

---

### 4. Fallback Chain Visual Editor

**Purpose:** Configure model fallback order with drag-and-drop.

**Requirements:**
- Drag-to-reorder fallback chain
- Add/remove models from chain
- Visual connection lines between models
- Enable/disable individual fallbacks
- Preview fallback order before saving
- Support multiple scenarios (default, streaming, long-context)

**User Stories:**
- As an operator, I want to adjust fallback priority without editing JSON
- As a developer, I want to visualize the fallback chain

---

### 5. Quick Model Test

**Purpose:** Test model responses directly in the dashboard.

**Requirements:**
- Text input for prompt
- Model selector (dropdown)
- Send test request button
- Real-time streaming response display
- Show latency, tokens, success status
- Copy response to clipboard
- Save test history (last 10 tests)

**User Stories:**
- As a developer, I want to quickly test if a model is responding
- As an operator, I want to validate config changes by sending a test prompt

---

## Non-Goals

- Log persistence to disk (logs stay in-memory, capped at last 1000)
- Multi-user authentication (single-user dashboard)
- Advanced analytics dashboards (use external tools like Grafana)

---

## Success Criteria

All 5 features implemented with:
- Responsive UI (works on 13" screens)
- i18n support (English + Chinese)
- Keyboard shortcuts for common actions
- No performance regression on history/metrics polling
- Tests for all new API endpoints

---

## Dependencies

- Internal: `internal/gui/server.go` (API endpoints)
- Internal: `internal/gui/assets/` (frontend)
- Internal: `internal/history/` (for model performance data)
- Internal: `internal/config/` (for backup/restore)

---

## Estimate

**Duration:** 3-5 days  
**Risk:** Low (modular features, existing patterns)
