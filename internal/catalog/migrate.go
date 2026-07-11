package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/routatic/proxy/internal/storage"
)

// Migration stats record
type MigrationStats struct {
	Providers int
	Models    int
}

// MigrateFromJSON reads existing catalog.json and imports to SQLite.
// Returns true if migration happened, false if JSON didn't exist or already migrated.
func MigrateFromJSON(ctx context.Context, db *storage.Database, jsonPath string) (bool, error) {
	repo := storage.NewCatalogRepo(db)

	lastSync, err := repo.LastSync(ctx)
	if err != nil {
		return false, fmt.Errorf("check last sync: %w", err)
	}
	if !lastSync.IsZero() {
		return false, nil
	}

	idx, err := Load(jsonPath)
	if err != nil {
		return false, fmt.Errorf("load catalog from JSON: %w", err)
	}

	providers := make([]storage.ProviderRecord, 0, len(idx.Providers))
	for name, p := range idx.Providers {
		providers = append(providers, storage.ProviderRecord{
			Name:                   name,
			BaseURL:                p.BaseURL,
			APIKey:                 p.APIKey,
			Enabled:                p.Enabled,
			AnthropicToolsDisabled: p.AnthropicToolsDisabled,
		})
	}

	models := make([]storage.ModelRecord, 0, len(idx.Models))
	for key, m := range idx.Models {
		models = append(models, storage.ModelRecord{
			ID:            key,
			Name:          m.Name,
			Reasoning:     m.Reasoning,
			ToolCall:      m.ToolCall,
			Vision:        m.SupportsVision(),
			ContextWindow: m.ContextWindow(),
			CostInput:     m.CostInputPerM(),
			CostOutput:    m.CostOutputPerM(),
		})
	}

	if err := repo.UpsertBatch(ctx, providers, models); err != nil {
		return false, fmt.Errorf("import catalog to SQLite: %w", err)
	}

	return true, nil
}

// ExportJSON exports SQLite catalog to JSON for backup/debugging.
func ExportJSON(ctx context.Context, db *storage.Database, jsonPath string) error {
	repo := storage.NewCatalogRepo(db)

	idx, err := repo.Load(ctx)
	if err != nil {
		return fmt.Errorf("load catalog from SQLite: %w", err)
	}

	catalog := &Catalog{
		Providers: make(map[string]Provider, len(idx.Providers)),
		Models:    make(map[string]Model, len(idx.Models)),
	}

	for name, p := range idx.Providers {
		catalog.Providers[name] = Provider{
			Name:                   p.Name,
			BaseURL:                p.BaseURL,
			APIKey:                 p.APIKey,
			Enabled:                p.Enabled,
			AnthropicToolsDisabled: p.AnthropicToolsDisabled,
		}
	}

	for key, m := range idx.Models {
		model := Model{
			ID:        ModelNameFromKey(key),
			Name:      m.Name,
			Reasoning: m.Reasoning,
			ToolCall:  m.ToolCall,
		}

		if m.Vision {
			model.Modalities.Input = []string{"text", "image"}
		} else {
			model.Modalities.Input = []string{"text"}
		}
		model.Modalities.Output = []string{"text"}

		if m.Limit != nil {
			model.Limit = &Limit{Context: m.Limit.Context}
		}
		if m.Rates != nil {
			model.Rates = &Rates{
				Input:  m.Rates.Input,
				Output: m.Rates.Output,
			}
		}

		catalog.Models[key] = model
	}

	return WriteFile(jsonPath, catalog)
}

// LoadFromSQLite loads the catalog from SQLite and returns an IndexedCatalog.
func LoadFromSQLite(ctx context.Context, db *storage.Database) (*IndexedCatalog, error) {
	repo := storage.NewCatalogRepo(db)

	storageIdx, err := repo.Load(ctx)
	if err != nil {
		return nil, err
	}

	cat := &Catalog{
		Providers: make(map[string]Provider, len(storageIdx.Providers)),
		Models:    make(map[string]Model, len(storageIdx.Models)),
	}

	for name, p := range storageIdx.Providers {
		cat.Providers[name] = Provider{
			Name:                   p.Name,
			BaseURL:                p.BaseURL,
			APIKey:                 p.APIKey,
			Enabled:                p.Enabled,
			AnthropicToolsDisabled: p.AnthropicToolsDisabled,
		}
	}

	for key, m := range storageIdx.Models {
		model := Model{
			ID:        ModelNameFromKey(key),
			Name:      m.Name,
			Reasoning: m.Reasoning,
			ToolCall:  m.ToolCall,
		}

		if m.Vision {
			model.Modalities.Input = []string{"text", "image"}
		} else {
			model.Modalities.Input = []string{"text"}
		}
		model.Modalities.Output = []string{"text"}

		if m.Limit != nil {
			model.Limit = &Limit{Context: m.Limit.Context}
		}
		if m.Rates != nil {
			model.Rates = &Rates{
				Input:  m.Rates.Input,
				Output: m.Rates.Output,
			}
		}

		cat.Models[key] = model
	}

	idx := &IndexedCatalog{
		Catalog:        *cat,
		ProviderModels: make(map[string][]Model, len(storageIdx.ProviderModels)),
	}

	for prov, models := range storageIdx.ProviderModels {
		converted := make([]Model, len(models))
		for i, m := range models {
			converted[i] = Model{
				ID:        m.ID,
				Name:      m.Name,
				Reasoning: m.Reasoning,
				ToolCall:  m.ToolCall,
			}
			if m.Vision {
				converted[i].Modalities.Input = []string{"text", "image"}
			} else {
				converted[i].Modalities.Input = []string{"text"}
			}
			converted[i].Modalities.Output = []string{"text"}
			if m.Limit != nil {
				converted[i].Limit = &Limit{Context: m.Limit.Context}
			}
			if m.Rates != nil {
				converted[i].Rates = &Rates{Input: m.Rates.Input, Output: m.Rates.Output}
			}
		}
		idx.ProviderModels[prov] = converted
	}

	return idx, nil
}

// SyncToSQLite downloads the catalog from sourceURL and imports it to SQLite.
func SyncToSQLite(ctx context.Context, db *storage.Database, sourceURL string) error {
	if sourceURL == "" {
		return fmt.Errorf("source URL is required")
	}
	if db == nil {
		return fmt.Errorf("database is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch catalog: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status: %d", resp.StatusCode)
	}

	limited := http.MaxBytesReader(nil, resp.Body, maxCatalogBytes)
	body, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read catalog: %w", err)
	}

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("parse catalog: %w", err)
	}
	if env.Models == nil || env.Providers == nil {
		return fmt.Errorf("catalog must contain models and providers objects")
	}

	var catalog Catalog
	if err := json.Unmarshal(body, &catalog); err != nil {
		return fmt.Errorf("parse catalog contents: %w", err)
	}

	providers := make([]storage.ProviderRecord, 0, len(catalog.Providers))
	for name, p := range catalog.Providers {
		providers = append(providers, storage.ProviderRecord{
			Name:                   name,
			BaseURL:                p.BaseURL,
			APIKey:                 p.APIKey,
			Enabled:                p.Enabled,
			AnthropicToolsDisabled: p.AnthropicToolsDisabled,
		})
	}

	models := make([]storage.ModelRecord, 0, len(catalog.Models))
	for key, m := range catalog.Models {
		models = append(models, storage.ModelRecord{
			ID:            key,
			Name:          m.Name,
			Reasoning:     m.Reasoning,
			ToolCall:      m.ToolCall,
			Vision:        m.SupportsVision(),
			ContextWindow: m.ContextWindow(),
			CostInput:     m.CostInputPerM(),
			CostOutput:    m.CostOutputPerM(),
		})
	}

	repo := storage.NewCatalogRepo(db)
	if err := repo.UpsertBatch(ctx, providers, models); err != nil {
		return fmt.Errorf("upsert catalog: %w", err)
	}

	return nil
}

// SyncStats holds statistics from a catalog sync operation.
type SyncStats struct {
	Providers int
	Models    int
	Duration  time.Duration
}

// SyncToSQLiteWithStats downloads the catalog and returns sync statistics.
func SyncToSQLiteWithStats(ctx context.Context, db *storage.Database, sourceURL string) (*SyncStats, error) {
	start := time.Now()

	if sourceURL == "" {
		return nil, fmt.Errorf("source URL is required")
	}
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch catalog: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status: %d", resp.StatusCode)
	}

	limited := http.MaxBytesReader(nil, resp.Body, maxCatalogBytes)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	if env.Models == nil || env.Providers == nil {
		return nil, fmt.Errorf("catalog must contain models and providers objects")
	}

	var catalog Catalog
	if err := json.Unmarshal(body, &catalog); err != nil {
		return nil, fmt.Errorf("parse catalog contents: %w", err)
	}

	providers := make([]storage.ProviderRecord, 0, len(catalog.Providers))
	for name, p := range catalog.Providers {
		providers = append(providers, storage.ProviderRecord{
			Name:                   name,
			BaseURL:                p.BaseURL,
			APIKey:                 p.APIKey,
			Enabled:                p.Enabled,
			AnthropicToolsDisabled: p.AnthropicToolsDisabled,
		})
	}

	models := make([]storage.ModelRecord, 0, len(catalog.Models))
	for key, m := range catalog.Models {
		models = append(models, storage.ModelRecord{
			ID:            key,
			Name:          m.Name,
			Reasoning:     m.Reasoning,
			ToolCall:      m.ToolCall,
			Vision:        m.SupportsVision(),
			ContextWindow: m.ContextWindow(),
			CostInput:     m.CostInputPerM(),
			CostOutput:    m.CostOutputPerM(),
		})
	}

	repo := storage.NewCatalogRepo(db)
	if err := repo.UpsertBatch(ctx, providers, models); err != nil {
		return nil, fmt.Errorf("upsert catalog: %w", err)
	}

	return &SyncStats{
		Providers: len(providers),
		Models:    len(models),
		Duration:  time.Since(start),
	}, nil
}
