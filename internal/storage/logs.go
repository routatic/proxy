package storage

import (
	"context"
	"database/sql"
	"time"
)

type Logs struct {
	db *Database
}

func NewLogs(db *Database) *Logs {
	return &Logs{db: db}
}

type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
	Field   string    `json:"field,omitempty"`
	Value   string    `json:"value,omitempty"`
}

func (l *Logs) Insert(level, message, field, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := l.db.DB().ExecContext(ctx, `
		INSERT INTO logs (level, message, field, value, recorded_at)
		VALUES (?, ?, ?, ?, ?)
	`, level, message, field, value, time.Now().Format(time.RFC3339Nano))

	return err
}

func (l *Logs) Last(n int) ([]LogEntry, error) {
	if n <= 0 {
		n = 200
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := l.db.DB().QueryContext(ctx, `
		SELECT level, message, field, value, recorded_at
		FROM logs
		ORDER BY recorded_at DESC
		LIMIT ?
	`, n)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanLogs(rows)
}

func (l *Logs) Since(since time.Time, level string) ([]LogEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var query string
	var args []interface{}

	if level != "" && level != "all" {
		query = `
			SELECT level, message, field, value, recorded_at
			FROM logs
			WHERE recorded_at >= ? AND level = ?
			ORDER BY recorded_at DESC
		`
		args = []interface{}{since.Format(time.RFC3339Nano), level}
	} else {
		query = `
			SELECT level, message, field, value, recorded_at
			FROM logs
			WHERE recorded_at >= ?
			ORDER BY recorded_at DESC
		`
		args = []interface{}{since.Format(time.RFC3339Nano)}
	}

	rows, err := l.db.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanLogs(rows)
}

func (l *Logs) DeleteBefore(before time.Time) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := l.db.DB().ExecContext(ctx, `
		DELETE FROM logs WHERE recorded_at < ?
	`, before.Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

func scanLogs(rows *sql.Rows) ([]LogEntry, error) {
	var entries []LogEntry
	for rows.Next() {
		var entry LogEntry
		var recordedAtStr string

		err := rows.Scan(
			&entry.Level,
			&entry.Message,
			&entry.Field,
			&entry.Value,
			&recordedAtStr,
		)
		if err != nil {
			return nil, err
		}

		entry.Time, _ = time.Parse(time.RFC3339Nano, recordedAtStr)
		entries = append(entries, entry)
	}

	return entries, rows.Err()
}
