package storage

import (
	"context"
	"math"
	"sort"
	"time"
)

type Latency struct {
	db *Database
}

func NewLatency(db *Database) *Latency {
	return &Latency{db: db}
}

func (l *Latency) Insert(model string, latency time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := l.db.DB().ExecContext(ctx, `
		INSERT INTO latency_samples (model, latency_ms, recorded_at)
		VALUES (?, ?, ?)
	`, model, latency.Milliseconds(), time.Now().Format(time.RFC3339Nano))

	return err
}

type ModelLatencyStats struct {
	Model string
	Count int64
	Avg   time.Duration
	P50   time.Duration
	P90   time.Duration
	P99   time.Duration
	Min   time.Duration
	Max   time.Duration
}

func (l *Latency) GetStats(since time.Time) ([]ModelLatencyStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	query := `
		SELECT model, latency_ms
		FROM latency_samples
		WHERE recorded_at >= ?
		ORDER BY model
	`

	rows, err := l.db.DB().QueryContext(ctx, query, since.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	samplesByModel := make(map[string][]int64)
	for rows.Next() {
		var model string
		var latencyMs int64
		if err := rows.Scan(&model, &latencyMs); err != nil {
			return nil, err
		}
		samplesByModel[model] = append(samplesByModel[model], latencyMs)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	var stats []ModelLatencyStats
	for model, samples := range samplesByModel {
		stats = append(stats, calculateStats(model, samples))
	}

	return stats, nil
}

func (l *Latency) GetSuccessCounts(since time.Time) (map[string]int64, map[string]int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT model, success, COUNT(*)
		FROM requests
		WHERE start_time >= ?
		GROUP BY model, success
	`

	rows, err := l.db.DB().QueryContext(ctx, query, since.Format(time.RFC3339Nano))
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	successCounts := make(map[string]int64)
	failureCounts := make(map[string]int64)

	for rows.Next() {
		var model string
		var success, count int64
		if err := rows.Scan(&model, &success, &count); err != nil {
			return nil, nil, err
		}
		if success == 1 {
			successCounts[model] = count
		} else {
			failureCounts[model] = count
		}
	}

	return successCounts, failureCounts, rows.Err()
}

func (l *Latency) DeleteBefore(before time.Time) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := l.db.DB().ExecContext(ctx, `
		DELETE FROM latency_samples WHERE recorded_at < ?
	`, before.Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

func calculateStats(model string, samples []int64) ModelLatencyStats {
	if len(samples) == 0 {
		return ModelLatencyStats{Model: model}
	}

	sorted := make([]int64, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var sum int64
	for _, ms := range sorted {
		sum += ms
	}

	count := len(sorted)
	avg := sum / int64(count)

	p50Idx := int(float64(count)*0.50) - 1
	p90Idx := int(math.Ceil(float64(count)*0.90)) - 1
	p99Idx := int(math.Ceil(float64(count)*0.99)) - 1
	if p50Idx < 0 {
		p50Idx = 0
	}
	if p90Idx < 0 {
		p90Idx = 0
	}
	if p99Idx < 0 {
		p99Idx = 0
	}
	if p50Idx >= count {
		p50Idx = count - 1
	}
	if p90Idx >= count {
		p90Idx = count - 1
	}
	if p99Idx >= count {
		p99Idx = count - 1
	}

	return ModelLatencyStats{
		Model: model,
		Count: int64(count),
		Avg:   time.Duration(avg) * time.Millisecond,
		P50:   time.Duration(sorted[p50Idx]) * time.Millisecond,
		P90:   time.Duration(sorted[p90Idx]) * time.Millisecond,
		P99:   time.Duration(sorted[p99Idx]) * time.Millisecond,
		Min:   time.Duration(sorted[0]) * time.Millisecond,
		Max:   time.Duration(sorted[count-1]) * time.Millisecond,
	}
}

func ParseTimeRange(rangeParam string) time.Time {
	switch rangeParam {
	case "1h":
		return time.Now().Add(-1 * time.Hour)
	case "24h":
		return time.Now().Add(-24 * time.Hour)
	case "7d":
		return time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		return time.Now().Add(-30 * 24 * time.Hour)
	default:
		return time.Time{}
	}
}
