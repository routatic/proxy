package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/routatic/proxy/internal/history"
)

// Requests provides methods for reading and writing request records.
type Requests struct {
	db *Database
}

// NewRequests creates a new Requests repository backed by the given database.
func NewRequests(db *Database) *Requests {
	return &Requests{db: db}
}

// Insert stores a request record, replacing any existing record with the same ID.
func (r *Requests) Insert(rec history.RequestRecord) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	attempt := rec.Attempt
	if attempt < 1 {
		attempt = 1
	}

	_, err := r.db.DB().ExecContext(ctx, `
		INSERT OR REPLACE INTO requests (
			id, model, provider, scenario, start_time, duration_ms,
			input_tokens, output_tokens, streaming, success, error_msg, attempt
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		rec.ID,
		rec.Model,
		rec.Provider,
		rec.Scenario,
		rec.StartTime.Format(time.RFC3339Nano),
		rec.Duration.Milliseconds(),
		rec.InputTokens,
		rec.OutputTokens,
		boolToInt(rec.Streaming),
		boolToInt(rec.Success),
		rec.ErrorMsg,
		attempt,
	)

	return err
}

// Last returns the most recent n request records ordered by start time.
func (r *Requests) Last(n int) ([]history.RequestRecord, error) {
	if n <= 0 {
		n = 1000
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := r.db.DB().QueryContext(ctx, `
		SELECT id, model, provider, scenario, start_time, duration_ms,
		       input_tokens, output_tokens, streaming, success, error_msg, attempt
		FROM requests
		ORDER BY start_time DESC
		LIMIT ?
	`, n)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanRequests(rows)
}

// Since returns all request records with start time after the given time.
func (r *Requests) Since(since time.Time) ([]history.RequestRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := r.db.DB().QueryContext(ctx, `
		SELECT id, model, provider, scenario, start_time, duration_ms,
		       input_tokens, output_tokens, streaming, success, error_msg, attempt
		FROM requests
		WHERE start_time >= ?
		ORDER BY start_time DESC
	`, since.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanRequests(rows)
}

// Count returns the total number of request records.
func (r *Requests) Count() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int64
	err := r.db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM requests`).Scan(&count)
	return count, err
}

// CountSince returns the number of request records with start time after the given time.
func (r *Requests) CountSince(since time.Time) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int64
	err := r.db.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM requests WHERE start_time >= ?
	`, since.Format(time.RFC3339Nano)).Scan(&count)
	return count, err
}

// DeleteBefore removes request records older than the given time.
func (r *Requests) DeleteBefore(before time.Time) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := r.db.DB().ExecContext(ctx, `
		DELETE FROM requests WHERE created_at < ?
	`, before.Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

func scanRequests(rows *sql.Rows) ([]history.RequestRecord, error) {
	var records []history.RequestRecord
	for rows.Next() {
		var rec history.RequestRecord
		var startTimeStr string
		var streaming, success int

		var attempt sql.NullInt64
		err := rows.Scan(
			&rec.ID,
			&rec.Model,
			&rec.Provider,
			&rec.Scenario,
			&startTimeStr,
			&rec.Duration,
			&rec.InputTokens,
			&rec.OutputTokens,
			&streaming,
			&success,
			&rec.ErrorMsg,
			&attempt,
		)
		if attempt.Valid {
			rec.Attempt = int(attempt.Int64)
		} else {
			rec.Attempt = 1
		}
		if err != nil {
			return nil, err
		}

		rec.StartTime, _ = time.Parse(time.RFC3339Nano, startTimeStr)
		rec.Streaming = streaming == 1
		rec.Success = success == 1
		rec.Duration = time.Duration(rec.Duration) * time.Millisecond

		records = append(records, rec)
	}

	return records, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
