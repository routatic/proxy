package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/catalog"
	"github.com/routatic/proxy/internal/storage"
	"github.com/spf13/cobra"
)

// catalogFixture is a minimal catalog with three providers and one model each.
const catalogFixture = `{
  "providers": {
    "opencode-go": {"name": "opencode-go", "base_url": "https://opencode.ai/zen/go/v1/chat/completions", "enabled": true},
    "opencode-zen": {"name": "opencode-zen", "base_url": "https://opencode.ai/zen/v1/chat/completions", "enabled": true},
    "openrouter": {"name": "openrouter", "base_url": "https://openrouter.ai/api/v1", "enabled": true}
  },
  "models": {
    "opencode-go/model-go": {"id": "opencode-go/model-go", "name": "Model Go"},
    "opencode-zen/model-zen": {"id": "opencode-zen/model-zen", "name": "Model Zen"},
    "openrouter/model-router": {"id": "openrouter/model-router", "name": "Model Router"}
  }
}`

func writeTestConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func writeTestConfigWithDB(t *testing.T, dir, dbPath string) string {
	t.Helper()
	config := `{"api_key": "test-global-key", "storage": {"database_path": "` + dbPath + `"}}`
	return writeTestConfig(t, dir, config)
}

func writeTestCatalog(t *testing.T, dir, content string) {
	t.Helper()
	catalogDir := filepath.Join(dir, "catalog")
	if err := os.MkdirAll(catalogDir, 0755); err != nil {
		t.Fatalf("mkdir catalog: %v", err)
	}
	jsonPath := filepath.Join(catalogDir, "catalog.json")
	if err := os.WriteFile(jsonPath, []byte(content), 0644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
}

func migrateTestCatalogToSQLite(t *testing.T, dir string) {
	t.Helper()

	jsonPath := filepath.Join(dir, "catalog", "catalog.json")
	dbPath := filepath.Join(dir, "data.db")

	storageCfg := storage.DefaultConfig
	storageCfg.DatabasePath = dbPath

	db, err := storage.Open(storageCfg)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := catalog.MigrateFromJSON(ctx, db, jsonPath); err != nil {
		t.Fatalf("migrate catalog: %v", err)
	}
}

func newCaptureCommand(t *testing.T) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	cmd := &cobra.Command{Use: "routatic-proxy"}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	return cmd, buf
}

func TestRunModelsList_ProviderFilter(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "data.db")
	writeTestCatalog(t, tmp, catalogFixture)
	migrateTestCatalogToSQLite(t, tmp)
	configPath := writeTestConfigWithDB(t, tmp, dbPath)

	cmd, buf := newCaptureCommand(t)
	t.Setenv("ROUTATIC_PROXY_CONFIG", configPath)

	if err := runModelsList(cmd, configPath, "opencode-zen"); err != nil {
		t.Fatalf("runModelsList error: %v", err)
	}

	out := buf.String()
	want := "opencode-zen/model-zen"
	if !strings.Contains(out, want) {
		t.Fatalf("output missing %q:\n%s", want, out)
	}
	for _, unexpected := range []string{"opencode-go/model-go", "openrouter/model-router"} {
		if strings.Contains(out, unexpected) {
			t.Fatalf("output should not contain %q:\n%s", unexpected, out)
		}
	}
	if !strings.Contains(out, "Use these model IDs") {
		t.Fatalf("output missing usage footer:\n%s", out)
	}
}

func TestRunModelsList_EnabledProviders(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "data.db")
	writeTestCatalog(t, tmp, catalogFixture)
	migrateTestCatalogToSQLite(t, tmp)
	configPath := writeTestConfigWithDB(t, tmp, dbPath)

	cmd, buf := newCaptureCommand(t)
	t.Setenv("ROUTATIC_PROXY_CONFIG", configPath)

	if err := runModelsList(cmd, configPath, ""); err != nil {
		t.Fatalf("runModelsList error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"opencode-go/model-go", "opencode-zen/model-zen", "openrouter/model-router"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunModelsList_UnknownProvider(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "data.db")
	writeTestCatalog(t, tmp, catalogFixture)
	migrateTestCatalogToSQLite(t, tmp)
	configPath := writeTestConfigWithDB(t, tmp, dbPath)

	cmd, buf := newCaptureCommand(t)
	t.Setenv("ROUTATIC_PROXY_CONFIG", configPath)

	if err := runModelsList(cmd, configPath, "unknown"); err != nil {
		t.Fatalf("runModelsList error: %v", err)
	}

	out := buf.String()
	want := `No models found for provider "unknown".`
	if !strings.Contains(out, want) {
		t.Fatalf("output missing %q:\n%s", want, out)
	}
	if strings.Contains(out, "Use these model IDs") {
		t.Fatalf("usage footer should not appear when no models are found:\n%s", out)
	}
}

func TestRunModelsList_MissingCatalog(t *testing.T) {
	tmp := t.TempDir()
	configPath := writeTestConfig(t, tmp, `{"api_key": "test-global-key"}`)
	// Intentionally do not write catalog.json.

	cmd, _ := newCaptureCommand(t)
	t.Setenv("ROUTATIC_PROXY_CONFIG", configPath)

	err := runModelsList(cmd, configPath, "")
	if err == nil {
		t.Fatal("expected error for missing catalog, got nil")
	}
	want := "catalog not found; run 'routatic-proxy catalog sync' first"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error containing %q, got: %v", want, err)
	}
}
