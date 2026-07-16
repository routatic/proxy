// Package storage provides SQLite-based persistent storage for the proxy.
package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Database struct {
	db   *sql.DB
	path string
	mu   sync.RWMutex
}

type Config struct {
	DatabasePath    string `json:"database_path"`
	RetentionDays   int    `json:"retention_days"`
	VacuumOnStartup bool   `json:"vacuum_on_startup"`
	WALEnabled      bool   `json:"wal_enabled"`
}

var DefaultConfig = Config{
	DatabasePath:    "~/.local/share/routatic-proxy/data.db",
	RetentionDays:   7,
	VacuumOnStartup: false,
	WALEnabled:      true,
}

func Open(cfg Config) (*Database, error) {
	path := expandPath(cfg.DatabasePath)

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	dsn := path
	if cfg.WALEnabled {
		dsn = path + "?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	database := &Database{
		db:   db,
		path: path,
	}

	if err := database.initSchema(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	// Lightweight migrations for new columns (safe on existing DBs)
	if err := database.migrateAddAttemptColumn(ctx); err != nil {
		// Non-fatal; log and continue so the proxy still works
		slog.Warn("migration warning", "err", err)
	}

	// Seed default model prices so analytics dashboard shows meaningful
	// cost numbers immediately for new/existing installs. Idempotent.
	_ = database.SeedDefaultModelPrices(ctx)

	if cfg.VacuumOnStartup {
		if _, err := db.ExecContext(ctx, "VACUUM"); err != nil {
			_ = database.Close()
			return nil, fmt.Errorf("vacuum: %w", err)
		}
	}

	return database, nil
}

func (d *Database) initSchema(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS requests (
		id TEXT PRIMARY KEY,
		model TEXT NOT NULL,
		provider TEXT,
		scenario TEXT,
		start_time TIMESTAMP NOT NULL,
		duration_ms INTEGER,
		input_tokens INTEGER,
		output_tokens INTEGER,
		streaming INTEGER,
		success INTEGER,
		error_msg TEXT,
		attempt INTEGER DEFAULT 1,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_requests_start_time ON requests(start_time);
	CREATE INDEX IF NOT EXISTS idx_requests_model ON requests(model);
	CREATE INDEX IF NOT EXISTS idx_requests_created_at ON requests(created_at);

	CREATE TABLE IF NOT EXISTS latency_samples (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		model TEXT NOT NULL,
		latency_ms INTEGER NOT NULL,
		recorded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_latency_model_time ON latency_samples(model, recorded_at);
	CREATE INDEX IF NOT EXISTS idx_latency_recorded_at ON latency_samples(recorded_at);

	CREATE TABLE IF NOT EXISTS logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		level TEXT NOT NULL,
		message TEXT,
		field TEXT,
		value TEXT,
		recorded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_logs_recorded_at ON logs(recorded_at);
	CREATE INDEX IF NOT EXISTS idx_logs_level ON logs(level);

	CREATE TABLE IF NOT EXISTS schema_info (
		key TEXT PRIMARY KEY,
		value TEXT
	);

	INSERT OR IGNORE INTO schema_info (key, value) VALUES ('version', '1');

	CREATE TABLE IF NOT EXISTS providers (
		name TEXT PRIMARY KEY,
		base_url TEXT,
		api_key TEXT,
		enabled INTEGER DEFAULT 1,
		anthropic_tools_disabled INTEGER DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_providers_enabled ON providers(enabled);

	CREATE TABLE IF NOT EXISTS models (
		id TEXT PRIMARY KEY,
		provider TEXT NOT NULL,
		name TEXT NOT NULL,
		display_name TEXT,
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

	CREATE TABLE IF NOT EXISTS scenarios (
		key TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		requires_tools INTEGER,
		requires_vision INTEGER,
		requires_reasoning INTEGER,
		min_context_window INTEGER,
		preferred_providers TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_scenarios_name ON scenarios(name);
	`

	_, err := d.db.ExecContext(ctx, schema)
	return err
}

// migrateAddAttemptColumn adds the 'attempt' column to the requests table if it does not exist.
// This is used for fallback-rate analytics.
func (d *Database) migrateAddAttemptColumn(ctx context.Context) error {
	// Try to add the column. SQLite will error if it already exists.
	_, err := d.db.ExecContext(ctx, `ALTER TABLE requests ADD COLUMN attempt INTEGER DEFAULT 1`)
	if err != nil {
		// Ignore "duplicate column" errors
		if strings.Contains(err.Error(), "duplicate column") {
			return nil
		}
		return err
	}
	return nil
}

func (d *Database) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

func (d *Database) DB() *sql.DB {
	return d.db
}

func (d *Database) Path() string {
	return d.path
}

func (d *Database) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return d.db.BeginTx(ctx, opts)
}

func expandPath(path string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// priceEntry defines a pricing rule for models whose id/name/display_name
// contains the match substring. Applied only if current cost is 0/NULL.
type priceEntry struct {
	Match  string  `json:"match"`
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

//go:embed seed_prices.json
var defaultModelPrices []byte

// SeedDefaultModelPrices inserts realistic default pricing for common models
// (GLM, Kimi, Qwen, Grok, DeepSeek, Claude, GPT, MiniMax, Nemotron, MiMo, etc.)
// so that /api/analytics/* endpoints immediately show non-zero USD costs
// without requiring user config or catalog rates.
//
// The seeder is idempotent: it only updates rows where cost_input_per_m or
// cost_output_per_m is NULL or 0. This preserves any prices already set by
// catalog sync (internal/catalog) or user overrides.
//
// Prices are approximate current public list prices (per million tokens, USD)
// sourced from official provider documentation and pricing pages as of
// July 2026: OpenAI (GPT), Anthropic (Claude), Z.ai (GLM), Moonshot (Kimi),
// Alibaba (Qwen), xAI (Grok), DeepSeek, MiniMax, NVIDIA (Nemotron), and
// others. Free-tier variants are explicitly zeroed. Update seed_prices.json
// to refresh values; the JSON is embedded at build time.
func (d *Database) SeedDefaultModelPrices(ctx context.Context) error {
	if len(defaultModelPrices) == 0 {
		return nil
	}

	var entries []priceEntry
	if err := json.Unmarshal(defaultModelPrices, &entries); err != nil {
		return fmt.Errorf("parse seed_prices.json: %w", err)
	}

	for _, e := range entries {
		if e.Match == "" {
			continue
		}
		_, err := d.db.ExecContext(ctx, `
			UPDATE models
			SET cost_input_per_m = ?, cost_output_per_m = ?
			WHERE (cost_input_per_m IS NULL OR cost_input_per_m = 0)
			  AND (id LIKE '%' || ? || '%'
			    OR name LIKE '%' || ? || '%'
			    OR display_name LIKE '%' || ? || '%')
		`, e.Input, e.Output, e.Match, e.Match, e.Match)
		if err != nil {
			return fmt.Errorf("seed price for %q: %w", e.Match, err)
		}
	}
	return nil
}
