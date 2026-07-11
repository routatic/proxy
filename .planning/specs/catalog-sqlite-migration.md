# Feature Spec: Migrate Catalog from JSON to SQLite

**Status:** draft  
**Created:** 2026-07-11  
**Priority:** medium

---

## Problem Statement

The catalog (`catalog.json`) stores model and provider metadata in a JSON file that:
1. Is loaded entirely into memory on every process start
2. Lacks query indexes for efficient lookups
3. Cannot be queried by capability or cost without scanning all entries
4. Requires full file rewrite on sync (risk of corruption)
5. Is separate from the SQLite database already used for requests/latency/logs

---

## Proposed Solution

Migrate catalog storage to SQLite, unified with the existing `data.db`:

**Benefits:**
- Indexed queries (model lookups go from O(n) to O(log n))
- Partial updates (no full file rewrite on sync)
- Query by capability, cost, provider
- Unified storage (one file for all data)
- ACID transactions (safe sync/crash recovery)

---

## Scope

### In Scope
- Add catalog tables to existing SQLite database
- Migrate `catalog.LoadCatalog()` to SQLite
- Migrate `catalog.Sync()` to SQLite (incremental updates)
- Update all catalog consumers to use SQLite
- Backwards compatibility: support reading old `catalog.json` for migration
- Export catalog to JSON on demand (for backup/debugging)

### Out of Scope
- Complex query API (just basic lookups for now)
- Full-text search on model names (can add later)
- Cloud sync (local file only)

---

## Database Schema

```sql
-- Providers (e.g., opencode-go, opencode-zen)
CREATE TABLE IF NOT EXISTS providers (
    name TEXT PRIMARY KEY,
    base_url TEXT,
    api_key TEXT,
    enabled INTEGER DEFAULT 1,
    anthropic_tools_disabled INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_providers_enabled ON providers(enabled);

-- Models (e.g., opencode-go/glm-5.2)
CREATE TABLE IF NOT EXISTS models (
    id TEXT PRIMARY KEY,              -- 'opencode-go/glm-5.2'
    provider TEXT NOT NULL,           -- 'opencode-go'
    name TEXT NOT NULL,               -- 'GLM-5.2'
    display_name TEXT,                -- 'GLM-5.2 (reasoning)'
    context_window INTEGER,
    cost_input_per_m REAL,
    cost_output_per_m REAL,
    supports_tools INTEGER DEFAULT 1,
    supports_vision INTEGER DEFAULT 0,
    supports_reasoning INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (provider) REFERENCES providers(name)
);

CREATE INDEX IF NOT EXISTS idx_models_provider ON models(provider);
CREATE INDEX IF NOT EXISTS idx_models_name ON models(name);
CREATE INDEX IF NOT EXISTS idx_models_enabled ON models(provider) 
    WHERE provider IN (SELECT name FROM providers WHERE enabled = 1);
```

---

## API Changes

### Package `internal/catalog`

**Before:**
```go
func LoadCatalog(path string) (*Catalog, error)
func Sync(sourceURL, destDir string) (*LockFile, error)
```

**After:**
```go
func LoadCatalog(db *storage.Database) (*Catalog, error)
func Sync(db *storage.Database, sourceURL string) error
func SaveLegacyJSON(db *storage.Database, path string) error  // Export for backup
func MigrateFromJSON(db *storage.Database, jsonPath string) error  // One-time migration
```

### IndexedCatalog

The `IndexedCatalog` struct stays the same, but is built from SQLite queries instead of JSON unmarshaling.

---

## Migration Path

### Phase 1: Add Schema

```go
// internal/storage/database.go - add to initSchema()
CREATE TABLE IF NOT EXISTS providers (...)
CREATE TABLE IF NOT EXISTS models (...)
```

### Phase 2: Add Catalog Repository

```go
// internal/catalog/sqlite.go
type CatalogRepo struct {
    db *storage.Database
}

func (r *CatalogRepo) Load() (*Catalog, error)
func (r *CatalogRepo) Sync(sourceURL string) error
func (r *CatalogRepo) GetModel(id string) (*Model, error)
func (r *CatalogRepo) ListModelsByProvider(provider string) ([]Model, error)
```

### Phase 3: Migrate Consumers

Update all callers:
- `cmd/routatic-proxy/catalog.go` - sync command
- `internal/router/model_router.go` - model selection
- `internal/config/config.go` - cost routing
- GUI endpoints that query catalog

### Phase 4: Deprecation

- Keep `catalog.json` parsing for backwards compatibility during migration
- Add `routatic-proxy catalog migrate` command
- Remove JSON parsing in next major version

---

## Performance Comparison

| Operation | JSON (current) | SQLite (proposed) |
|-----------|---------------|-------------------|
| Load catalog | 50-100ms, full file into memory | 1-2ms, indexed query |
| Model lookup by name | O(n) scan | O(log n) index |
| Query models by cost | O(n) scan all | O(log n) index |
| Sync (update) | Rewrite entire file | Incremental INSERT/UPDATE |
| Memory footprint | ~100KB+ (entire catalog) | ~1KB per query |

---

## File Locations

| Before | After |
|--------|-------|
| `~/.local/share/routatic-proxy/catalog/catalog.json` | Unified into `data.db` |
| `~/.local/share/routatic-proxy/catalog/catalog.lock` | Removed (use DB transactions) |

---

## Backwards Compatibility

1. **First run**: Check if `catalog.json` exists
2. **If yes**: Migrate to SQLite, mark as migrated
3. **If no**: Use SQLite directly
4. **Export command**: `routatic-proxy catalog export` writes JSON for debugging

---

## User Stories

1. As an operator, I want model lookups to be fast even with 500+ models
2. As a developer, I want to query "all models under $1/M input cost"
3. As an operator, I want catalog sync to be safe from corruption
4. As a developer, I want one storage file for all proxy data

---

## Estimated Effort

- **Phase 1-2**: 1 day (schema + repository)
- **Phase 3**: 1 day (migrate consumers)
- **Phase 4**: 0.5 day (deprecation + export)
- **Total**: 2-3 days
