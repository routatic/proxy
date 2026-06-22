// Package config provides configuration management for the routatic-proxy.
package config

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/routatic/proxy/internal/auth"
)

// setupTestDB creates an in-memory SQLite database for testing.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Create schema
	if _, err := db.Exec(schemaDDL); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

// seedTestData inserts test workspace, supermodels, and providers.
func seedTestData(t *testing.T, db *sql.DB, workspaceID string) {
	t.Helper()

	ctx := context.Background()

	// Insert workspace
	workspaceConfig := map[string]interface{}{
		"logging": map[string]interface{}{
			"level":             "info",
			"log_requests":      true,
			"log_responses":     false,
			"log_latency":       true,
			"log_rate_limits":   true,
			"pii_masking":       true,
			"sensitive_headers": []string{"authorization", "x-api-key"},
		},
		"enforcement": map[string]interface{}{
			"require_auth":            true,
			"enforce_model_allowlist": true,
			"enforce_budgets":         true,
			"enforce_rate_limits":     true,
		},
	}
	workspaceJSON, _ := json.Marshal(workspaceConfig)

	_, err := db.ExecContext(ctx, `
		INSERT INTO workspaces (id, config_json, version, updated_at)
		VALUES (?, ?, ?, ?)
	`, workspaceID, string(workspaceJSON), "v1", time.Now().Unix())
	if err != nil {
		t.Fatalf("failed to insert workspace: %v", err)
	}

	// Insert supermodels
	supermodels := []map[string]interface{}{
		{
			"name":        "default-chat",
			"description": "Default chat model",
			"default": map[string]interface{}{
				"provider":       "opencode-go",
				"model_id":       "gpt-4",
				"temperature":    0.7,
				"max_tokens":     4096,
				"supports_tools": true,
				"wire_format":    "openai",
			},
			"scenarios": map[string]interface{}{
				"long_context": map[string]interface{}{
					"provider":       "opencode-zen",
					"model_id":       "claude-3-opus",
					"temperature":    0.5,
					"max_tokens":     8192,
					"context_window": 200000,
				},
			},
		},
		{
			"name":        "code-assistant",
			"description": "Code assistant model",
			"default": map[string]interface{}{
				"provider":    "opencode-go",
				"model_id":    "code-davinci-002",
				"temperature": 0.2,
				"max_tokens":  8192,
				"wire_format": "openai",
			},
		},
	}

	for _, sm := range supermodels {
		configJSON, _ := json.Marshal(sm)
		_, err := db.ExecContext(ctx, `
			INSERT INTO supermodels (workspace_id, name, config_json)
			VALUES (?, ?, ?)
		`, workspaceID, sm["name"], string(configJSON))
		if err != nil {
			t.Fatalf("failed to insert supermodel %s: %v", sm["name"], err)
		}
	}

	// Insert providers
	providers := []map[string]interface{}{
		{
			"name":               "opencode-go",
			"type":               "opencode-go",
			"base_url":           "https://api.opencode-go.com/v1",
			"anthropic_base_url": "https://api.opencode-go.com/anthropic",
			"timeout_ms":         30000,
			"stream_timeout_ms":  60000,
		},
		{
			"name":               "opencode-zen",
			"type":               "opencode-zen",
			"base_url":           "https://api.opencode-zen.com/v1",
			"anthropic_base_url": "https://api.opencode-zen.com/anthropic",
			"timeout_ms":         45000,
			"stream_timeout_ms":  90000,
		},
	}

	for _, prov := range providers {
		configJSON, _ := json.Marshal(prov)
		_, err := db.ExecContext(ctx, `
			INSERT INTO providers (workspace_id, name, config_json)
			VALUES (?, ?, ?)
		`, workspaceID, prov["name"], string(configJSON))
		if err != nil {
			t.Fatalf("failed to insert provider %s: %v", prov["name"], err)
		}
	}
}

func TestNewDBConfigProvider(t *testing.T) {
	tests := []struct {
		name    string
		driver  string
		dsn     string
		wantErr bool
	}{
		{
			name:    "valid sqlite in-memory",
			driver:  "sqlite",
			dsn:     ":memory:",
			wantErr: false,
		},
		{
			name:    "empty driver",
			driver:  "",
			dsn:     ":memory:",
			wantErr: true,
		},
		{
			name:    "empty dsn",
			driver:  "sqlite",
			dsn:     "",
			wantErr: true,
		},
		{
			name:    "invalid driver",
			driver:  "mysql",
			dsn:     "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewDBConfigProvider(tt.driver, tt.dsn)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			defer func() { _ = provider.Close() }()

			if provider.db == nil {
				t.Error("expected non-nil db")
			}
			if provider.cache == nil {
				t.Error("expected non-nil cache")
			}
			if provider.driver != tt.driver {
				t.Errorf("expected driver %q, got %q", tt.driver, provider.driver)
			}
		})
	}
}

func TestDBConfigProvider_GetConfigByRef(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db, "ws-test-123")

	provider := &DBConfigProvider{db: db, driver: "sqlite"}
	provider.cache = NewCachedConfigProvider(provider, 5*time.Minute)
	defer func() { _ = provider.Close() }()

	ctx := context.Background()
	ref := auth.ConfigRef{
		WorkspaceID: "ws-test-123",
		Version:     "v1",
	}

	config, err := provider.GetConfigByRef(ctx, ref)
	if err != nil {
		t.Fatalf("GetConfigByRef failed: %v", err)
	}

	// Verify basic fields
	if config.WorkspaceID != "ws-test-123" {
		t.Errorf("expected workspace_id ws-test-123, got %q", config.WorkspaceID)
	}
	if config.Version != "v1" {
		t.Errorf("expected version v1, got %q", config.Version)
	}

	// Verify supermodels loaded
	if len(config.Supermodels) != 2 {
		t.Errorf("expected 2 supermodels, got %d", len(config.Supermodels))
	}

	// Check specific supermodel
	sm, ok := config.Supermodels["default-chat"]
	if !ok {
		t.Error("expected default-chat supermodel")
	} else {
		if sm.Name != "default-chat" {
			t.Errorf("expected name default-chat, got %q", sm.Name)
		}
		if sm.Default.Provider != "opencode-go" {
			t.Errorf("expected provider opencode-go, got %q", sm.Default.Provider)
		}
		if sm.Default.ModelID != "gpt-4" {
			t.Errorf("expected model_id gpt-4, got %q", sm.Default.ModelID)
		}
		if sm.Default.Temperature != 0.7 {
			t.Errorf("expected temperature 0.7, got %f", sm.Default.Temperature)
		}
		if sm.Default.MaxTokens != 4096 {
			t.Errorf("expected max_tokens 4096, got %d", sm.Default.MaxTokens)
		}

		// Check scenario
		if len(sm.Scenarios) != 1 {
			t.Errorf("expected 1 scenario, got %d", len(sm.Scenarios))
		}
		scenario, ok := sm.Scenarios["long_context"]
		if !ok {
			t.Error("expected long_context scenario")
		} else {
			if scenario.Provider != "opencode-zen" {
				t.Errorf("expected scenario provider opencode-zen, got %q", scenario.Provider)
			}
			if scenario.ContextWindow != 200000 {
				t.Errorf("expected context_window 200000, got %d", scenario.ContextWindow)
			}
		}
	}

	// Verify providers loaded
	if len(config.Providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(config.Providers))
	}

	prov, ok := config.Providers["opencode-go"]
	if !ok {
		t.Error("expected opencode-go provider")
	} else {
		if prov.Name != "opencode-go" {
			t.Errorf("expected name opencode-go, got %q", prov.Name)
		}
		if prov.Type != "opencode-go" {
			t.Errorf("expected type opencode-go, got %q", prov.Type)
		}
		if prov.BaseURL != "https://api.opencode-go.com/v1" {
			t.Errorf("expected base_url, got %q", prov.BaseURL)
		}
		if prov.TimeoutMs != 30000 {
			t.Errorf("expected timeout_ms 30000, got %d", prov.TimeoutMs)
		}
	}

	// Verify capability index
	if len(config.CapabilityIndex) != 3 { // 2 supermodels * default + 1 scenario
		t.Errorf("expected capability index entries, got %d", len(config.CapabilityIndex))
	}

	// Verify logging policy
	if config.LoggingPolicy.Level != "info" {
		t.Errorf("expected logging level info, got %q", config.LoggingPolicy.Level)
	}
	if !config.LoggingPolicy.LogRequests {
		t.Error("expected LogRequests to be true")
	}

	// Verify enforcement policy
	if !config.Enforcement.RequireAuth {
		t.Error("expected RequireAuth to be true")
	}
}

func TestDBConfigProvider_GetConfigByRef_NotFound(t *testing.T) {
	db := setupTestDB(t)
	// Don't seed any data

	provider := &DBConfigProvider{db: db, driver: "sqlite"}
	provider.cache = NewCachedConfigProvider(provider, 5*time.Minute)
	defer func() { _ = provider.Close() }()

	ctx := context.Background()
	ref := auth.ConfigRef{
		WorkspaceID: "non-existent",
		Version:     "v1",
	}

	_, err := provider.GetConfigByRef(ctx, ref)
	if err == nil {
		t.Error("expected error for non-existent workspace")
	}
	if !errors.Is(err, sql.ErrNoRows) && !containsString(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestDBConfigProvider_GetConfigByRef_VersionMismatch(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db, "ws-test-123")

	provider := &DBConfigProvider{db: db, driver: "sqlite"}
	provider.cache = NewCachedConfigProvider(provider, 5*time.Minute)
	defer func() { _ = provider.Close() }()

	ctx := context.Background()
	ref := auth.ConfigRef{
		WorkspaceID: "ws-test-123",
		Version:     "v2", // Different from v1 stored in DB
	}

	_, err := provider.GetConfigByRef(ctx, ref)
	if err == nil {
		t.Error("expected error for version mismatch")
	}
	if !containsString(err.Error(), "version mismatch") {
		t.Errorf("expected version mismatch error, got: %v", err)
	}
}

func TestDBConfigProvider_GetConfigByRef_EmptyVersion(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db, "ws-test-123")

	provider := &DBConfigProvider{db: db, driver: "sqlite"}
	provider.cache = NewCachedConfigProvider(provider, 5*time.Minute)
	defer func() { _ = provider.Close() }()

	ctx := context.Background()
	ref := auth.ConfigRef{
		WorkspaceID: "ws-test-123",
		Version:     "", // Empty version should use latest/current
	}

	config, err := provider.GetConfigByRef(ctx, ref)
	if err != nil {
		t.Fatalf("GetConfigByRef failed: %v", err)
	}

	if config.Version != "v1" {
		t.Errorf("expected version v1, got %q", config.Version)
	}
}

func TestDBConfigProvider_GetEffectiveConfig(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db, "ws-test-123")

	provider := &DBConfigProvider{db: db, driver: "sqlite"}
	provider.cache = NewCachedConfigProvider(provider, 5*time.Minute)
	defer func() { _ = provider.Close() }()

	ctx := context.Background()
	authCtx := &auth.AuthContext{
		WorkspaceID: "ws-test-123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws-test-123",
			Version:     "v1",
		},
	}

	config, err := provider.GetEffectiveConfig(ctx, authCtx)
	if err != nil {
		t.Fatalf("GetEffectiveConfig failed: %v", err)
	}

	if config.WorkspaceID != "ws-test-123" {
		t.Errorf("expected workspace_id ws-test-123, got %q", config.WorkspaceID)
	}

	// Verify caching: second call should be cached
	config2, err := provider.GetEffectiveConfig(ctx, authCtx)
	if err != nil {
		t.Fatalf("second GetEffectiveConfig failed: %v", err)
	}

	if config != config2 {
		t.Error("expected same config pointer (cached)")
	}
}

func TestDBConfigProvider_HealthCheck(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db, "ws-test-123")

	provider := &DBConfigProvider{db: db, driver: "sqlite"}
	provider.cache = NewCachedConfigProvider(provider, 5*time.Minute)

	ctx := context.Background()
	if err := provider.HealthCheck(ctx); err != nil {
		t.Errorf("HealthCheck failed: %v", err)
	}

	// Close db and verify health check fails
	_ = provider.Close()

	if err := provider.HealthCheck(ctx); err == nil {
		t.Error("expected HealthCheck to fail after Close")
	}
}

func TestDBConfigProvider_GetEffectiveConfig_CacheHit(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db, "ws-test-123")

	provider, err := NewDBConfigProvider("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	// Seed directly into provider's db
	workspaceConfig := map[string]interface{}{
		"logging": map[string]interface{}{"level": "info"},
		"enforcement": map[string]interface{}{
			"require_auth": false,
		},
	}
	workspaceJSON, _ := json.Marshal(workspaceConfig)
	_, err = provider.db.Exec(`
		INSERT INTO workspaces (id, config_json, version, updated_at)
		VALUES (?, ?, ?, ?)
	`, "ws-test-123", string(workspaceJSON), "v1", time.Now().Unix())
	if err != nil {
		t.Fatalf("failed to seed workspace: %v", err)
	}

	ctx := context.Background()
	authCtx := &auth.AuthContext{
		WorkspaceID: "ws-test-123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws-test-123",
			Version:     "v1",
		},
	}

	// First call - cache miss
	_, err = provider.GetEffectiveConfig(ctx, authCtx)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// Second call - cache hit
	_, err = provider.GetEffectiveConfig(ctx, authCtx)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	stats := provider.cache.GetStats()
	if stats.Hits != 1 {
		t.Errorf("expected 1 cache hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected 1 cache miss, got %d", stats.Misses)
	}
}

func TestDBConfigProvider_Invalidate(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db, "ws-test-123")

	provider := &DBConfigProvider{db: db, driver: "sqlite"}
	provider.cache = NewCachedConfigProvider(provider, 5*time.Minute)
	defer func() { _ = provider.Close() }()

	ctx := context.Background()
	authCtx := &auth.AuthContext{
		WorkspaceID: "ws-test-123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws-test-123",
			Version:     "v1",
		},
	}

	// Populate cache
	_, _ = provider.GetEffectiveConfig(ctx, authCtx)

	// Invalidate specific version
	err := provider.Invalidate(ctx, "ws-test-123", "v1")
	if err != nil {
		t.Fatalf("Invalidate failed: %v", err)
	}

	stats := provider.cache.GetStats()
	if stats.Invalidations != 1 {
		t.Errorf("expected 1 invalidation, got %d", stats.Invalidations)
	}

	// Invalidate all versions
	_, _ = provider.GetEffectiveConfig(ctx, authCtx) // Repopulate cache
	err = provider.Invalidate(ctx, "ws-test-123", "")
	if err != nil {
		t.Fatalf("Invalidate all failed: %v", err)
	}
}

func TestDBConfigProvider_ConcurrentAccess(t *testing.T) {
	// Use file-based SQLite for concurrent access to avoid connection issues
	tmpFile := t.TempDir() + "/test.db"
	provider, err := NewDBConfigProvider("sqlite", tmpFile)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	// Seed data
	workspaceConfig := map[string]interface{}{
		"logging":     map[string]interface{}{"level": "info"},
		"enforcement": map[string]interface{}{},
	}
	workspaceJSON, _ := json.Marshal(workspaceConfig)
	_, err = provider.db.Exec(`
		INSERT INTO workspaces (id, config_json, version, updated_at)
		VALUES (?, ?, ?, ?)
	`, "ws-concurrent", string(workspaceJSON), "v1", time.Now().Unix())
	if err != nil {
		t.Fatalf("failed to seed workspace: %v", err)
	}

	ctx := context.Background()
	authCtx := &auth.AuthContext{
		WorkspaceID: "ws-concurrent",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws-concurrent",
			Version:     "v1",
		},
	}

	var wg sync.WaitGroup
	numGoroutines := 50
	numCalls := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numCalls; j++ {
				_, err := provider.GetEffectiveConfig(ctx, authCtx)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		}()
	}

	wg.Wait()

	stats := provider.cache.GetStats()
	// With concurrent access, multiple goroutines may race past the cache check
	// before the first one populates it. We expect at least 1 miss, but could be more.
	// The key thing is that most calls should be hits.
	if stats.Misses == 0 {
		t.Error("expected at least 1 cache miss")
	}

	totalCalls := uint64(numGoroutines * numCalls)
	actualHits := stats.Hits
	// Should have mostly hits - at least 90% of total calls
	if actualHits < totalCalls*9/10 {
		t.Errorf("expected at least %d hits for good caching, got %d", totalCalls*9/10, actualHits)
	}
}

func TestDBConfigProvider_Close(t *testing.T) {
	provider, err := NewDBConfigProvider("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	// First close should succeed
	if err := provider.Close(); err != nil {
		t.Errorf("first Close failed: %v", err)
	}

	// Second close should be no-op (return nil)
	if err := provider.Close(); err != nil {
		t.Errorf("second Close should return nil: %v", err)
	}

	// db should be nil after close
	if provider.db != nil {
		t.Error("expected db to be nil after Close")
	}
}

func TestDBConfigProvider_ContextCancellation(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db, "ws-test-123")

	provider := &DBConfigProvider{db: db, driver: "sqlite"}
	provider.cache = NewCachedConfigProvider(provider, 5*time.Minute)
	defer func() { _ = provider.Close() }()

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ref := auth.ConfigRef{
		WorkspaceID: "ws-test-123",
		Version:     "v1",
	}

	_, err := provider.GetConfigByRef(ctx, ref)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestDBConfigProvider_MultipleWorkspaces(t *testing.T) {
	db := setupTestDB(t)

	// Seed multiple workspaces
	for i := 1; i <= 3; i++ {
		wsID := fmt.Sprintf("ws-%d", i)
		workspaceConfig := map[string]interface{}{
			"logging":     map[string]interface{}{"level": "info"},
			"enforcement": map[string]interface{}{},
		}
		workspaceJSON, _ := json.Marshal(workspaceConfig)
		_, err := db.Exec(`
			INSERT INTO workspaces (id, config_json, version, updated_at)
			VALUES (?, ?, ?, ?)
		`, wsID, string(workspaceJSON), fmt.Sprintf("v%d", i), time.Now().Unix())
		if err != nil {
			t.Fatalf("failed to insert workspace %d: %v", i, err)
		}
	}

	provider := &DBConfigProvider{db: db, driver: "sqlite"}
	provider.cache = NewCachedConfigProvider(provider, 5*time.Minute)
	defer func() { _ = provider.Close() }()

	ctx := context.Background()

	// Fetch each workspace
	for i := 1; i <= 3; i++ {
		wsID := fmt.Sprintf("ws-%d", i)
		ref := auth.ConfigRef{
			WorkspaceID: wsID,
			Version:     fmt.Sprintf("v%d", i),
		}

		config, err := provider.GetConfigByRef(ctx, ref)
		if err != nil {
			t.Fatalf("failed to get config for %s: %v", wsID, err)
		}
		if config.WorkspaceID != wsID {
			t.Errorf("expected workspace_id %s, got %q", wsID, config.WorkspaceID)
		}
		if config.Version != fmt.Sprintf("v%d", i) {
			t.Errorf("expected version v%d, got %q", i, config.Version)
		}
	}
}

func TestDBConfigProvider_ConcurrentHealthCheck(t *testing.T) {
	db := setupTestDB(t)

	provider := &DBConfigProvider{db: db, driver: "sqlite"}
	provider.cache = NewCachedConfigProvider(provider, 5*time.Minute)
	defer func() { _ = provider.Close() }()

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			if err := provider.HealthCheck(ctx); err != nil {
				t.Errorf("health check failed: %v", err)
			}
		}()
	}

	wg.Wait()
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStringHelper(s, substr))
}

func containsStringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
