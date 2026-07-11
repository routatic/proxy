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
	DatabasePath     string `json:"database_path"`
	RetentionDays    int    `json:"retention_days"`
	VacuumOnStartup  bool   `json:"vacuum_on_startup"`
	WALEnabled       bool   `json:"wal_enabled"`
}

var DefaultConfig = Config{
	DatabasePath:    "~/.local/share/routatic-proxy/data.db",
	RetentionDays:  7,
	VacuumOnStartup: false,
	WALEnabled:     true,
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
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	database := &Database{
		db:   db,
		path: path,
	}

	if err := database.initSchema(ctx); err != nil {
		database.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	if cfg.VacuumOnStartup {
		if _, err := db.ExecContext(ctx, "VACUUM"); err != nil {
			database.Close()
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
