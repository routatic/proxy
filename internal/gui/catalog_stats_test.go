package gui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/routatic/proxy/internal/storage"
)

func TestHandleCatalogStats_NoStorage(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/api/catalog/stats", nil)
	rec := httptest.NewRecorder()

	s.handleCatalogStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if available, ok := resp["available"].(bool); !ok || available {
		t.Error("expected available=false")
	}
}

func TestHandleCatalogStats_EmptyCatalog(t *testing.T) {
	tmp := t.TempDir()
	dbPath := tmp + "/test.db"

	storageCfg := storage.DefaultConfig
	storageCfg.DatabasePath = dbPath

	db, err := storage.Open(storageCfg)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer func() { _ = db.Close() }()

	s := &Server{storage: db}

	req := httptest.NewRequest(http.MethodGet, "/api/catalog/stats", nil)
	rec := httptest.NewRecorder()

	s.handleCatalogStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if available, ok := resp["available"].(bool); !ok || available {
		t.Error("expected available=false for empty catalog")
	}
}

func TestHandleCatalogStats_WithCatalog(t *testing.T) {
	tmp := t.TempDir()
	dbPath := tmp + "/test.db"

	storageCfg := storage.DefaultConfig
	storageCfg.DatabasePath = dbPath

	db, err := storage.Open(storageCfg)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := storage.NewCatalogRepo(db)
	ctx := context.Background()

	enabled := true
	providers := []storage.ProviderRecord{
		{Name: "provider-a", Enabled: &enabled},
		{Name: "provider-b", Enabled: &enabled},
	}
	models := []storage.ModelRecord{
		{ID: "provider-a/model-1", Name: "Model 1", ToolCall: true, Vision: true},
		{ID: "provider-a/model-2", Name: "Model 2", ToolCall: true, Reasoning: true},
		{ID: "provider-b/model-3", Name: "Model 3", ToolCall: true},
	}

	if err := repo.UpsertBatch(ctx, providers, models); err != nil {
		t.Fatalf("failed to seed catalog: %v", err)
	}

	s := &Server{storage: db}

	req := httptest.NewRequest(http.MethodGet, "/api/catalog/stats", nil)
	rec := httptest.NewRecorder()

	s.handleCatalogStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if available, ok := resp["available"].(bool); !ok || !available {
		t.Error("expected available=true")
	}

	if totalProviders, ok := resp["total_providers"].(float64); !ok || int(totalProviders) != 2 {
		t.Errorf("expected total_providers=2, got %v", resp["total_providers"])
	}

	if totalModels, ok := resp["total_models"].(float64); !ok || int(totalModels) != 3 {
		t.Errorf("expected total_models=3, got %v", resp["total_models"])
	}

	if modelsWithTools, ok := resp["models_with_tools"].(float64); !ok || int(modelsWithTools) != 3 {
		t.Errorf("expected models_with_tools=3, got %v", resp["models_with_tools"])
	}

	if modelsWithVision, ok := resp["models_with_vision"].(float64); !ok || int(modelsWithVision) != 1 {
		t.Errorf("expected models_with_vision=1, got %v", resp["models_with_vision"])
	}
}

func TestHandleCatalogStats_MethodNotAllowed(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodPost, "/api/catalog/stats", nil)
	rec := httptest.NewRecorder()

	s.handleCatalogStats(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}
