# Implementation Plan: Catalog SQLite Migration

**Spec:** `.planning/specs/catalog-sqlite-migration.md`  
**Created:** 2026-07-11  
**Status:** completed

---

## Components Table

| Component | Type | Purpose |
|-----------|------|---------|
| `storage.CatalogRepo` | Go struct | SQLite operations for catalog |
| `catalog.LoadCatalog` | Func refactor | Load from SQLite instead of JSON |
| `catalog.Sync` | Func refactor | Incremental sync to SQLite |
| `catalog.MigrateFromJSON` | Func new | One-time JSON to SQLite migration |

---

## File Locations Table

| File | Location | Purpose |
|------|----------|---------|
| `internal/storage/catalog.go` | New | Catalog SQLite repository |
| `internal/storage/database.go` | Edit | Add providers/models tables to schema |
| `internal/catalog/catalog.go` | Edit | Refactor to use SQLite |
| `internal/catalog/sqlite.go` | New | SQLite-specific catalog operations |
| `cmd/routatic-proxy/catalog.go` | Edit | Update catalog sync command |
| `internal/router/model_router.go` | Edit | Load catalog from SQLite |

---

## Phase Breakdown

### Phase 1: Add Database Schema

| # | Task | Files |
|---|------|-------|
| 1.1 | Add providers table to initSchema | `internal/storage/database.go` |
| 1.2 | Add models table with indexes | `internal/storage/database.go` |
| 1.3 | Write schema migration tests | `internal/storage/database_test.go` |

### Phase 2: Implement Catalog Repository

| # | Task | Files |
|---|------|-------|
| 2.1 | Create CatalogRepo struct | `internal/storage/catalog.go` |
| 2.2 | Implement LoadCatalog() | `internal/storage/catalog.go` |
| 2.3 | Implement InsertProvider/Model | `internal/storage/catalog.go` |
| 2.4 | Implement GetModel(id) | `internal/storage/catalog.go` |
| 2.5 | Implement ListModels(provider) | `internal/storage/catalog.go` |
| 2.6 | Write unit tests | `internal/storage/catalog_test.go` |

### Phase 3: Implement Sync

| # | Task | Files |
|---|------|-------|
| 3.1 | Fetch catalog from source URL | `internal/catalog/sync.go` |
| 3.2 | Parse into providers/models structs | `internal/catalog/sync.go` |
| 3.3 | Implement incremental sync (INSERT OR REPLACE) | `internal/catalog/sync.go` |
| 3.4 | Update sync command to use SQLite | `cmd/routatic-proxy/catalog.go` |
| 3.5 | Write sync tests | `internal/catalog/sync_test.go` |

### Phase 4: Migrate Consumers

| # | Task | Files |
|---|------|-------|
| 4.1 | Update model_router to use SQLite | `internal/router/model_router.go` |
| 4.2 | Update cost routing to use SQLite | `internal/router/selector.go` |
| 4.3 | Update GUI catalog endpoints | `internal/gui/server.go` |
| 4.4 | Update server initialization | `internal/server/server.go` |

### Phase 5: Backwards Compatibility

| # | Task | Files |
|---|------|-------|
| 5.1 | Implement MigrateFromJSON() | `internal/catalog/migrate.go` |
| 5.2 | Auto-migrate on startup if JSON exists | `cmd/routatic-proxy/main.go` |
| 5.3 | Implement ExportJSON() for backup | `internal/catalog/export.go` |
| 5.4 | Add `catalog export` command | `cmd/routatic-proxy/catalog.go` |

---

## API Details

### storage.CatalogRepo

```go
type CatalogRepo struct {
    db *Database
}

func NewCatalogRepo(db *Database) *CatalogRepo

func (r *CatalogRepo) Load() (*catalog.Catalog, error)

func (r *CatalogRepo) GetModel(id string) (*catalog.Model, error)

func (r *CatalogRepo) ListModelsByProvider(provider string) ([]catalog.Model, error)

func (r *CatalogRepo) ListEnabledModels() ([]catalog.Model, error)

func (r *CatalogRepo) UpsertProvider(p catalog.Provider) error

func (r *CatalogRepo) UpsertModel(m catalog.Model) error

func (r *CatalogRepo) UpsertBatch(providers []catalog.Provider, models []catalog.Model) error
```

### catalog.Catalog (unchanged)

The `Catalog` struct stays the same - it's populated from SQLite instead of JSON:

```go
type Catalog struct {
    Providers map[string]Provider
    Models    map[string]Model
}
```

---

## Sync Strategy

**Current (JSON):**
```go
// Download entire catalog, rewrite file
data := fetch(sourceURL)
os.WriteFile("catalog.json", data, 0644)
```

**New (SQLite):**
```go
// Download catalog, incremental upsert
remote := fetch(sourceURL)
local := loadFromSQLite()

// Delete models that no longer exist
for id := range local.Models {
    if _, ok := remote.Models[id]; !ok {
        delete from models where id = ?
    }
}

// Upsert changes
for id, model := range remote.Models {
    upsertModel(model)
}
```

---

## Testing Plan

### Unit Tests
- `TestCatalogRepo_Load` - Load catalog from SQLite
- `TestCatalogRepo_GetModel` - Get by ID
- `TestCatalogRepo_Upsert` - Insert/update
- `TestCatalogRepo_DeleteOrphans` - Remove deleted models

### Integration Tests
- `TestCatalogSync_JSONToSQLite` - Migrate from existing JSON
- `TestCatalogSync_Incremental` - Sync updates only
- `TestCatalogSync_Rollback` - Transaction rollback on error

### Benchmark Tests
```go
func BenchmarkLoadCatalog_JSON(b *testing.B)    // Current
func BenchmarkLoadCatalog_SQLite(b *testing.B)  // New
```

Target: SQLite should be 5-10x faster for single-model lookups.

---

## Gate 2 Checklist

**Architecture:**
- [x] Reuses existing Database connection
- [x] Same Catalog struct (consumers unchanged)
- [x] Incremental sync (no full file rewrite)
- [x] Indexed queries for performance

**Task Breakdown:**
- [x] All files listed with locations
- [x] All tasks mapped to phases
- [x] Each task is small (one function/file)
- [x] Dependencies clear (Phase 1 → 2 → 3 → 4 → 5)

**Testing:**
- [x] Unit tests for repository
- [x] Integration tests for sync
- [x] Benchmarks for performance
- [x] Backwards compatibility tests

---

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|-------------|
| Schema migration breaks existing DBs | Low | Add tables only (no ALTER) |
| Sync slower than current | Low | Batch upserts in transaction |
| JSON compat issues | Medium | Export command for debugging |
