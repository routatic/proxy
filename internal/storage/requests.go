package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/routatic/proxy/internal/history"
)

type Requests struct {
	db *Database
}

func NewRequests(db *Database) *Requests {
	return &Requests{db: db}
}

func (r *Requests) Insert(rec history.RequestRecord) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.db.DB().ExecContext(ctx, `
		INSERT OR REPLACE INTO requests (
			id, model, provider, scenario, start_time, duration_ms,
			input_tokens, output_tokens, streaming, success, error_msg
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
	)

	return err
}

func (r *Requests) Last(n int) ([]history.RequestRecord, error) {
	if n <= 0 {
		n = 1000
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := r.db.DB().QueryContext(ctx, `
		SELECT id, model, provider, scenario, start_time, duration_ms,
		       input_tokens, output_tokens, streaming, success, error_msg
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

func (r *Requests) Since(since time.Time) ([]history.RequestRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := r.db.DB().QueryContext(ctx, `
		SELECT id, model, provider, scenario, start_time, duration_ms,
		       input_tokens, output_tokens, streaming, success, error_msg
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

func (r *Requests) Count() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int64
	err := r.db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM requests`).Scan(&count)
	return count, err
}

func (r *Requests) CountSince(since time.Time) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int64
	err := r.db.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM requests WHERE start_time >= ?
	`, since.Format(time.RFC3339Nano)).Scan(&count)
	return count, err
}

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
		)
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
