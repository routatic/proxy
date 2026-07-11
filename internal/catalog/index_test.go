package catalog

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func ptr(b bool) *bool { return &b }

func TestIndex_BuildProviderIndex_Valid(t *testing.T) {
	cat := Catalog{
		Providers: map[string]Provider{
			"openai":    {Name: "openai", Enabled: nil},
			"anthropic": {Name: "anthropic", Enabled: ptr(true)},
			"disabled":  {Name: "disabled", Enabled: ptr(false)},
		},
		Models: map[string]Model{
			"openai/gpt-4": {
				ID:   "openai/gpt-4",
				Name: "gpt-4",
			},
			"anthropic/claude-3": {
				ID:   "anthropic/claude-3",
				Name: "claude-3",
			},
			"openai/gpt-3.5": {
				ID:   "openai/gpt-3.5",
				Name: "gpt-3.5",
			},
		},
	}

	idx, err := BuildProviderIndex(cat)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string][]string{
		"openai":    {"openai/gpt-3.5", "openai/gpt-4"},
		"anthropic": {"anthropic/claude-3"},
	}

	if !reflect.DeepEqual(idx.ProviderModels, want) {
		t.Fatalf("index mismatch: got %+v, want %+v", idx.ProviderModels, want)
	}

	if _, ok := idx.ProviderModels["disabled"]; ok {
		t.Fatalf("expected disabled provider to be omitted")
	}
}

func TestIndex_NoEnabledProviders(t *testing.T) {
	cat := Catalog{
		Providers: map[string]Provider{
			"disabled": {Name: "disabled", Enabled: ptr(false)},
		},
		Models: map[string]Model{
			"disabled/gpt-4": {ID: "disabled/gpt-4", Name: "gpt-4"},
		},
	}

	_, err := BuildProviderIndex(cat)
	if err == nil {
		t.Fatalf("expected error for no enabled providers, got nil")
	}
}

func TestIndex_EmptyModels(t *testing.T) {
	cat := Catalog{
		Providers: map[string]Provider{
			"openai": {Name: "openai"},
		},
		Models: map[string]Model{},
	}

	_, err := BuildProviderIndex(cat)
	if err == nil {
		t.Fatalf("expected error for empty models, got nil")
	}
}

func TestIndex_ModelsNoMatchEnabledProviders(t *testing.T) {
	cat := Catalog{
		Providers: map[string]Provider{
			"openai":   {Name: "openai"},
			"disabled": {Name: "disabled", Enabled: ptr(false)},
		},
		Models: map[string]Model{
			"disabled/only-model": {ID: "disabled/only-model", Name: "only-model"},
		},
	}

	_, err := BuildProviderIndex(cat)
	if err == nil {
		t.Fatalf("expected error for no models matching enabled providers, got nil")
	}
}

func TestIndex_WriteReadRoundTrip(t *testing.T) {
	dir := t.TempDir()

	cat := Catalog{
		Providers: map[string]Provider{
			"openai": {Name: "openai"},
		},
		Models: map[string]Model{
			"openai/gpt-4": {ID: "openai/gpt-4", Name: "gpt-4"},
		},
	}

	idx, err := BuildProviderIndex(cat)
	if err != nil {
		t.Fatalf("BuildProviderIndex: %v", err)
	}

	if err := idx.Write(dir); err != nil {
		t.Fatalf("Write: %v", err)
	}

	readIdx, err := ReadProviderIndex(dir)
	if err != nil {
		t.Fatalf("ReadProviderIndex: %v", err)
	}

	if !reflect.DeepEqual(idx.ProviderModels, readIdx.ProviderModels) {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", readIdx.ProviderModels, idx.ProviderModels)
	}
}

func TestIndex_ReadMissingFile(t *testing.T) {
	dir := t.TempDir()

	_, err := ReadProviderIndex(dir)
	if err == nil {
		t.Fatal("expected error for missing index file")
	}
}

func TestIndex_WriteToMissingDir(t *testing.T) {
	idx := &ProviderModelIndex{ProviderModels: map[string][]string{"p": {"m"}}}
	err := idx.Write("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func TestIndex_WriteNilIndex(t *testing.T) {
	dir := t.TempDir()
	var idx *ProviderModelIndex
	err := idx.Write(dir)
	if err == nil {
		t.Fatal("expected error for nil index")
	}
}

func TestReadProviderIndex_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	badPath := filepath.Join(dir, "provider_model_index.json")
	if err := os.WriteFile(badPath, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := ReadProviderIndex(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
