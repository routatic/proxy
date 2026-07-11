// Package storage provides SQLite-based persistent storage for the proxy.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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
