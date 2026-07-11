package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/storage"
)

const testCatalogFixture = `{
  "providers": {
    "opencode-go": {"name": "opencode-go", "base_url": "https://opencode.ai/zen/go/v1/chat/completions", "enabled": true},
    "opencode-zen": {"name": "opencode-zen", "base_url": "https://opencode.ai/zen/v1/chat/completions", "enabled": true}
  },
  "models": {
    "opencode-go/model-go": {"id": "opencode-go/model-go", "name": "Model Go"},
    "opencode-zen/model-zen": {"id": "opencode-zen/model-zen", "name": "Model Zen"}
  }
}`

func TestMigrateFromJSON(t *testing.T) {
	tmp := t.TempDir()

	catalogDir := filepath.Join(tmp, "catalog")
	if err := os.MkdirAll(catalogDir, 0755); err != nil {
		t.Fatalf("mkdir catalog: %v", err)
	}
	jsonPath := filepath.Join(catalogDir, "catalog.json")
	if err := os.WriteFile(jsonPath, []byte(testCatalogFixture), 0644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}

	dbPath := filepath.Join(tmp, "data.db")
	storageCfg := storage.DefaultConfig
	storageCfg.DatabasePath = dbPath

	db, err := storage.Open(storageCfg)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	start := time.Now()
	migrated, err := MigrateFromJSON(ctx, db, jsonPath)
	elapsed := time.Since(start)

	t.Logf("MigrateFromJSON took: %v", elapsed)

	if err != nil {
		t.Fatalf("MigrateFromJSON: %v", err)
	}
	if !migrated {
		t.Fatal("expected migrated=true, got false")
	}

	idx, err := LoadFromSQLite(ctx, db)
	if err != nil {
		t.Fatalf("LoadFromSQLite: %v", err)
	}

	if len(idx.Providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(idx.Providers))
	}
	if len(idx.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(idx.Models))
	}
}
