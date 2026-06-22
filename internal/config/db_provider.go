// Package config provides configuration management for the routatic-proxy.
package config

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/auth"

	// SQLite driver
	_ "github.com/mattn/go-sqlite3"
	// Postgres driver
	_ "github.com/lib/pq"
)

// DBConfigProvider reads configuration from a database (SQLite or PostgreSQL).
// It uses standard database/sql with connection pooling and wraps itself with
// CachedConfigProvider to avoid DB hits on every request.
// Thread-safe via sync.RWMutex on the db handle.
type DBConfigProvider struct {
	db     *sql.DB
	mu     sync.RWMutex
	cache  *CachedConfigProvider
	driver string
}

// schemaDDL contains the SQL to create required tables.
// These are idempotent (CREATE TABLE IF NOT EXISTS).
const schemaDDL = `
CREATE TABLE IF NOT EXISTS workspaces (
    id TEXT PRIMARY KEY,
    config_json TEXT NOT NULL,
    version TEXT NOT NULL,
    updated_at INTEGER
);

CREATE TABLE IF NOT EXISTS supermodels (
    workspace_id TEXT,
    name TEXT,
    config_json TEXT,
    PRIMARY KEY (workspace_id, name)
);

CREATE TABLE IF NOT EXISTS providers (
    workspace_id TEXT,
    name TEXT,
    config_json TEXT,
    PRIMARY KEY (workspace_id, name)
);
`

// NewDBConfigProvider creates a new DBConfigProvider with the given driver and DSN.
// Supported drivers: "sqlite" (uses mattn/go-sqlite3), "postgres" (uses lib/pq).
// Auto-creates required tables on init.
// Returns error if connection fails or tables cannot be created.
func NewDBConfigProvider(driver, dsn string) (*DBConfigProvider, error) {
	if driver == "" {
		return nil, fmt.Errorf("driver cannot be empty")
	}
	if dsn == "" {
		return nil, fmt.Errorf("dsn cannot be empty")
	}

	// Map "sqlite" to "sqlite3" for the actual driver name
	dbDriver := driver
	if driver == "sqlite" {
		dbDriver = "sqlite3"
	}

	db, err := sql.Open(dbDriver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pooling
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(1 * time.Hour)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Create schema
	if _, err := db.ExecContext(ctx, schemaDDL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	provider := &DBConfigProvider{
		db:     db,
		driver: driver,
	}

	// Wrap with cache using a 5-minute TTL
	provider.cache = NewCachedConfigProvider(provider, 5*time.Minute)

	slog.Info("DBConfigProvider initialized",
		"driver", driver,
		"dsn", maskDSN(dsn),
	)

	return provider, nil
}

// GetEffectiveConfig returns the runtime configuration for the authenticated request.
// Delegates to the underlying CachedConfigProvider using GetConfigByRef.
func (p *DBConfigProvider) GetEffectiveConfig(ctx context.Context, authCtx *auth.AuthContext) (*RuntimeConfig, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if authCtx == nil {
		return nil, fmt.Errorf("auth context is required")
	}
	return p.cache.GetConfigByRef(ctx, authCtx.ConfigRef)
}

// GetConfigByRef retrieves a specific configuration version by reference.
// Fetches from database, compiles to RuntimeConfig, and caches the result.
// Thread-safe via database query with context.
func (p *DBConfigProvider) GetConfigByRef(ctx context.Context, ref auth.ConfigRef) (*RuntimeConfig, error) {
	if ref.WorkspaceID == "" {
		return nil, fmt.Errorf("workspace_id cannot be empty")
	}

	// Load workspace from database
	var configJSON, version string
	var updatedAt sql.NullInt64

	err := p.db.QueryRowContext(ctx, `
		SELECT config_json, version, updated_at
		FROM workspaces
		WHERE id = $1
	`, ref.WorkspaceID).Scan(&configJSON, &version, &updatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("workspace %q not found", ref.WorkspaceID)
		}
		return nil, fmt.Errorf("failed to query workspace: %w", err)
	}

	// If version is specified in ref, verify it matches
	// Empty version means "latest/current"
	if ref.Version != "" && ref.Version != version {
		return nil, fmt.Errorf("version mismatch: requested %q but found %q", ref.Version, version)
	}

	// Parse workspace config
	var dbWorkspace dbWorkspaceRecord
	if err := json.Unmarshal([]byte(configJSON), &dbWorkspace); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workspace config: %w", err)
	}

	// Load supermodels
	supermodels, err := p.loadSupermodels(ctx, ref.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to load supermodels: %w", err)
	}

	// Load providers
	providers, err := p.loadProviders(ctx, ref.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to load providers: %w", err)
	}

	// Compile to RuntimeConfig
	runtimeCfg, err := p.compileToRuntime(ref.WorkspaceID, version, dbWorkspace, supermodels, providers)
	if err != nil {
		return nil, fmt.Errorf("failed to compile runtime config: %w", err)
	}

	return runtimeCfg, nil
}

// Invalidate clears cached configuration for the specified workspace and version.
// If version is empty, all versions for the workspace are invalidated.
// Delegates to CachedConfigProvider.
func (p *DBConfigProvider) Invalidate(ctx context.Context, workspaceID string, version string) error {
	return p.cache.Invalidate(ctx, workspaceID, version)
}

// HealthCheck verifies the database connection is operational.
// Performs a lightweight ping with context.
func (p *DBConfigProvider) HealthCheck(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	p.mu.RLock()
	db := p.db
	p.mu.RUnlock()

	if db == nil {
		return fmt.Errorf("database connection is nil")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}

// Close closes the database connection.
// Safe to call multiple times - subsequent calls are no-ops.
// Thread-safe via mutex.
func (p *DBConfigProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.db == nil {
		return nil
	}

	err := p.db.Close()
	p.db = nil

	if err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}

	slog.Info("DBConfigProvider closed")
	return nil
}

// loadSupermodels loads all supermodels for a workspace from the database.
func (p *DBConfigProvider) loadSupermodels(ctx context.Context, workspaceID string) (map[string]*dbSupermodelRecord, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT name, config_json
		FROM supermodels
		WHERE workspace_id = $1
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	supermodels := make(map[string]*dbSupermodelRecord)
	for rows.Next() {
		var name, configJSON string
		if err := rows.Scan(&name, &configJSON); err != nil {
			return nil, err
		}

		var sm dbSupermodelRecord
		if err := json.Unmarshal([]byte(configJSON), &sm); err != nil {
			return nil, fmt.Errorf("failed to unmarshal supermodel %q: %w", name, err)
		}
		sm.Name = name
		supermodels[name] = &sm
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return supermodels, nil
}

// loadProviders loads all providers for a workspace from the database.
func (p *DBConfigProvider) loadProviders(ctx context.Context, workspaceID string) (map[string]*dbProviderRecord, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT name, config_json
		FROM providers
		WHERE workspace_id = $1
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	providers := make(map[string]*dbProviderRecord)
	for rows.Next() {
		var name, configJSON string
		if err := rows.Scan(&name, &configJSON); err != nil {
			return nil, err
		}

		var prov dbProviderRecord
		if err := json.Unmarshal([]byte(configJSON), &prov); err != nil {
			return nil, fmt.Errorf("failed to unmarshal provider %q: %w", name, err)
		}
		prov.Name = name
		providers[name] = &prov
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return providers, nil
}

// compileToRuntime compiles database records to a RuntimeConfig.
func (p *DBConfigProvider) compileToRuntime(
	workspaceID, version string,
	workspace dbWorkspaceRecord,
	supermodels map[string]*dbSupermodelRecord,
	providers map[string]*dbProviderRecord,
) (*RuntimeConfig, error) {
	// Compile supermodels
	compiledSupermodels := make(map[string]Supermodel)
	for name, sm := range supermodels {
		compiledSupermodels[name] = Supermodel{
			Name:        sm.Name,
			Description: sm.Description,
			Default:     sm.Default,
			Scenarios:   sm.Scenarios,
		}
	}

	// Compile providers
	compiledProviders := make(map[string]ProviderConfig)
	for name, prov := range providers {
		compiledProviders[name] = ProviderConfig{
			Name:             prov.Name,
			Type:             prov.Type,
			BaseURL:          prov.BaseURL,
			AnthropicBaseURL: prov.AnthropicBaseURL,
			ResponsesBaseURL: prov.ResponsesBaseURL,
			GeminiBaseURL:    prov.GeminiBaseURL,
			APIKey:           prov.APIKey,
			APIKeys:          prov.APIKeys,
			TimeoutMs:        prov.TimeoutMs,
			StreamTimeoutMs:  prov.StreamTimeoutMs,
			Headers:          prov.Headers,
		}
	}

	// Build capability index
	capabilityIndex := make(map[string]ModelCapabilities)
	for _, sm := range compiledSupermodels {
		capabilityIndex[sm.Default.ModelID] = inferCapabilities(sm.Default)
		for _, scenario := range sm.Scenarios {
			capabilityIndex[scenario.ModelID] = inferCapabilitiesFromScenario(scenario)
		}
	}

	// Default routing policies
	routingPolicies := []RoutingPolicy{
		{
			Name:     "long_context",
			Priority: 100,
			Conditions: RoutingConditions{
				ContextThreshold: 81920,
			},
		},
		{
			Name:     "complex",
			Priority: 90,
			Conditions: RoutingConditions{
				HasTools: boolPtr(true),
			},
		},
		{
			Name:     "think",
			Priority: 80,
			Conditions: RoutingConditions{
				Scenarios: []string{"reasoning"},
			},
		},
		{
			Name:     "background",
			Priority: 10,
			Conditions: RoutingConditions{
				Streaming: boolPtr(false),
			},
		},
		{
			Name:       "default",
			Priority:   0,
			Conditions: RoutingConditions{},
		},
	}

	return &RuntimeConfig{
		WorkspaceID:     workspaceID,
		Version:         version,
		Supermodels:     compiledSupermodels,
		Providers:       compiledProviders,
		CapabilityIndex: capabilityIndex,
		RoutingPolicies: routingPolicies,
		LoggingPolicy:   workspace.Logging,
		Enforcement:     workspace.Enforcement,
	}, nil
}

// inferCapabilities extracts model capabilities from ModelConfig.
func inferCapabilities(mc ModelConfig) ModelCapabilities {
	supportsTools := mc.SupportsTools == nil || *mc.SupportsTools
	return ModelCapabilities{
		ModelID:           mc.ModelID,
		Provider:          mc.Provider,
		MaxContextWindow:  mc.ContextWindow,
		MaxOutputTokens:   mc.MaxOutputTokens,
		SupportsTools:     supportsTools,
		SupportsVision:    mc.Vision,
		SupportsStreaming: true,
		SupportsThinking:  mc.ReasoningEffort != "",
		WireFormats:       []string{"openai", "anthropic"},
	}
}

// inferCapabilitiesFromScenario extracts model capabilities from ScenarioConfig.
func inferCapabilitiesFromScenario(sc ScenarioConfig) ModelCapabilities {
	return ModelCapabilities{
		ModelID:           sc.ModelID,
		Provider:          sc.Provider,
		MaxContextWindow:  sc.ContextWindow,
		MaxOutputTokens:   sc.MaxOutputTokens,
		SupportsTools:     true,
		SupportsVision:    false,
		SupportsStreaming: true,
		SupportsThinking:  sc.ReasoningEffort != "",
		WireFormats:       []string{"openai", "anthropic"},
	}
}

// maskDSN masks sensitive parts of the DSN for logging.
func maskDSN(dsn string) string {
	// Simple masking: show only first few chars
	if len(dsn) > 20 {
		return dsn[:20] + "..."
	}
	if len(dsn) > 10 {
		return dsn[:10] + "..."
	}
	return "***"
}

// dbWorkspaceRecord represents the workspace config stored in the database.
type dbWorkspaceRecord struct {
	Logging     LoggingPolicy     `json:"logging,omitempty"`
	Enforcement EnforcementPolicy `json:"enforcement,omitempty"`
}

// dbSupermodelRecord represents a supermodel config stored in the database.
type dbSupermodelRecord struct {
	Name        string                    `json:"name"`
	Description string                    `json:"description,omitempty"`
	Default     ModelConfig               `json:"default"`
	Scenarios   map[string]ScenarioConfig `json:"scenarios,omitempty"`
}

// dbProviderRecord represents a provider config stored in the database.
type dbProviderRecord struct {
	Name             string            `json:"name"`
	Type             string            `json:"type"`
	BaseURL          string            `json:"base_url"`
	AnthropicBaseURL string            `json:"anthropic_base_url,omitempty"`
	ResponsesBaseURL string            `json:"responses_base_url,omitempty"`
	GeminiBaseURL    string            `json:"gemini_base_url,omitempty"`
	APIKey           string            `json:"api_key,omitempty"`
	APIKeys          []string          `json:"api_keys,omitempty"`
	TimeoutMs        int               `json:"timeout_ms"`
	StreamTimeoutMs  int               `json:"stream_timeout_ms,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
}
