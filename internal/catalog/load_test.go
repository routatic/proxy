package catalog

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeTempCatalog(t *testing.T, catalog Catalog) string {
	t.Helper()
	data, err := json.Marshal(catalog)
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write temp catalog: %v", err)
	}
	return path
}

func TestLoad_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write placeholder file: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove temp file: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestLoad_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write malformed json: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for malformed json")
	}
}

func TestLoad_EmptyProviders(t *testing.T) {
	path := writeTempCatalog(t, Catalog{
		Providers: map[string]Provider{},
		Models: map[string]Model{
			"p1/m1": {ID: "p1/m1", Name: "m1"},
		},
	})

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for empty providers")
	}
	if !containsSubstring(err.Error(), "providers") {
		t.Fatalf("expected error to mention providers, got %v", err)
	}
}

func TestLoad_EmptyModels(t *testing.T) {
	path := writeTempCatalog(t, Catalog{
		Providers: map[string]Provider{
			"p1": {Name: "p1"},
		},
		Models: map[string]Model{},
	})

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for empty models")
	}
	if !containsSubstring(err.Error(), "models") {
		t.Fatalf("expected error to mention models, got %v", err)
	}
}

func TestLoad_UnknownProvider(t *testing.T) {
	path := writeTempCatalog(t, Catalog{
		Providers: map[string]Provider{
			"known": {Name: "known"},
		},
		Models: map[string]Model{
			"unknown/m1": {ID: "unknown/m1", Name: "m1"},
		},
	})

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestLoad_ModelKeyNoProviderPrefix(t *testing.T) {
	path := writeTempCatalog(t, Catalog{
		Providers: map[string]Provider{
			"p1": {Name: "p1"},
		},
		Models: map[string]Model{
			"no-prefix-model": {ID: "no-prefix-model", Name: "no-prefix-model"},
		},
	})

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for model key without provider prefix")
	}
}

func TestLoad_ValidCatalog(t *testing.T) {
	path := writeTempCatalog(t, Catalog{
		Providers: map[string]Provider{
			"opencode-go": {Name: "opencode-go"},
			"other":       {Name: "other"},
		},
		Models: map[string]Model{
			"opencode-go/model-a": {
				ID:   "opencode-go/model-a",
				Name: "model-a",
			},
			"other/model-b": {
				ID:   "other/model-b",
				Name: "model-b",
			},
		},
	})

	idx, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := len(idx.Models), 2; got != want {
		t.Errorf("len(Models) = %d, want %d", got, want)
	}
	if got, want := len(idx.Providers), 2; got != want {
		t.Errorf("len(Providers) = %d, want %d", got, want)
	}

	openCodeModels := idx.ModelsForProvider("opencode-go")
	if got, want := len(openCodeModels), 1; got != want {
		t.Errorf("len(ModelsForProvider(\"opencode-go\")) = %d, want %d", got, want)
	}

	otherModels := idx.ModelsForProvider("other")
	if got, want := len(otherModels), 1; got != want {
		t.Errorf("len(ModelsForProvider(\"other\")) = %d, want %d", got, want)
	}

	missingModels := idx.ModelsForProvider("does-not-exist")
	if missingModels != nil {
		t.Errorf("ModelsForProvider(\"does-not-exist\") = %v, want nil", missingModels)
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
