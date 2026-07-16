package storage

import (
	"context"
	"time"
)

// Analytics provides aggregated metrics for the dashboard.
type Analytics struct {
	db *Database
}

// NewAnalytics creates a new Analytics store.
func NewAnalytics(db *Database) *Analytics {
	return &Analytics{db: db}
}

// TokenSummary holds high-level token and request metrics for a time window.
type TokenSummary struct {
	TotalRequests int64     `json:"total_requests"`
	InputTokens   int64     `json:"input_tokens"`
	OutputTokens  int64     `json:"output_tokens"`
	SuccessRate   float64   `json:"success_rate"` // 0-1
	EstCostUSD    float64   `json:"est_cost_usd"`
	PeriodStart   time.Time `json:"period_start"`
	PeriodEnd     time.Time `json:"period_end"`
}

// GetTokenSummary returns aggregated token/request metrics for the last N days.
func (a *Analytics) GetTokenSummary(days int) (*TokenSummary, error) {
	if days <= 0 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -days)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var summary TokenSummary
	summary.PeriodStart = since
	summary.PeriodEnd = time.Now()

	row := a.db.DB().QueryRowContext(ctx, `
		SELECT
			COUNT(*) AS total_requests,
			COALESCE(SUM(input_tokens), 0) AS input_tokens,
			COALESCE(SUM(output_tokens), 0) AS output_tokens,
			CASE
				WHEN COUNT(*) > 0 THEN CAST(SUM(success) AS FLOAT) / COUNT(*)
				ELSE 0
			END AS success_rate,
			COALESCE(
				(SUM(input_tokens * COALESCE(m.cost_input_per_m, 0)) +
				 SUM(output_tokens * COALESCE(m.cost_output_per_m, 0))) / 1000000,
				0
			) AS est_cost_usd
		FROM requests r
		LEFT JOIN models m ON m.id = r.model
		WHERE r.created_at >= ?
	`, since.Format(time.RFC3339Nano))

	if err := row.Scan(&summary.TotalRequests, &summary.InputTokens, &summary.OutputTokens, &summary.SuccessRate, &summary.EstCostUSD); err != nil {
		return nil, err
	}
	return &summary, nil
}

// ModelBreakdown holds per-model usage and performance stats.
type ModelBreakdown struct {
	Model        string  `json:"model"`
	Provider     string  `json:"provider"`
	Requests     int64   `json:"requests"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	SuccessRate  float64 `json:"success_rate"`
	EstCostUSD   float64 `json:"est_cost_usd"` // based on models.cost_* if available
}

// GetModelBreakdown returns usage stats per model for the last N days.
func (a *Analytics) GetModelBreakdown(days int) ([]ModelBreakdown, error) {
	if days <= 0 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -days)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := a.db.DB().QueryContext(ctx, `
		SELECT 
			r.model,
			COALESCE(r.provider, '') AS provider,
			COUNT(*) AS requests,
			COALESCE(SUM(r.input_tokens), 0) AS input_tokens,
			COALESCE(SUM(r.output_tokens), 0) AS output_tokens,
			COALESCE(AVG(l.latency_ms), 0) AS avg_latency_ms,
			CASE 
				WHEN COUNT(*) > 0 THEN CAST(SUM(r.success) AS FLOAT) / COUNT(*)
				ELSE 0 
			END AS success_rate,
			COALESCE(
				(SUM(r.input_tokens * COALESCE(m.cost_input_per_m, 0)) + 
				 SUM(r.output_tokens * COALESCE(m.cost_output_per_m, 0))) / 1000000,
				0
			) AS est_cost_usd
		FROM requests r
		LEFT JOIN latency_samples l 
			ON l.model = r.model 
			AND l.recorded_at >= ?
		LEFT JOIN models m 
			ON m.id = r.model
		WHERE r.created_at >= ?
		GROUP BY r.model, r.provider
		ORDER BY requests DESC
	`, since.Format(time.RFC3339Nano), since.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []ModelBreakdown
	for rows.Next() {
		var mb ModelBreakdown
		if err := rows.Scan(
			&mb.Model,
			&mb.Provider,
			&mb.Requests,
			&mb.InputTokens,
			&mb.OutputTokens,
			&mb.AvgLatencyMs,
			&mb.SuccessRate,
			&mb.EstCostUSD,
		); err != nil {
			return nil, err
		}
		result = append(result, mb)
	}
	return result, rows.Err()
}

// ProviderBreakdown holds per-provider aggregates.
type ProviderBreakdown struct {
	Provider     string  `json:"provider"`
	Requests     int64   `json:"requests"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	FallbackRate float64 `json:"fallback_rate"` // % of requests that were fallbacks
	EstCostUSD   float64 `json:"est_cost_usd"`
}

// GetProviderBreakdown returns usage by provider (with fallback rate).
func (a *Analytics) GetProviderBreakdown(days int) ([]ProviderBreakdown, error) {
	if days <= 0 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -days)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := a.db.DB().QueryContext(ctx, `
		SELECT 
			COALESCE(provider, 'unknown') AS provider,
			COUNT(*) AS requests,
			COALESCE(SUM(input_tokens), 0) AS input_tokens,
			COALESCE(SUM(output_tokens), 0) AS output_tokens,
			COALESCE(
				(SUM(input_tokens) * 0 + SUM(output_tokens) * 0) / 1000000, -- placeholder; real cost via model join if needed
				0
			) AS est_cost_usd,
			-- Fallback rate: attempts > 1 are fallbacks; old rows (NULL/0) treated as primary (rate 0)
			COALESCE(100.0 * COUNT(CASE WHEN COALESCE(attempt, 1) > 1 THEN 1 END) / NULLIF(COUNT(*), 0), 0) AS fallback_rate
		FROM requests
		WHERE created_at >= ?
		GROUP BY provider
		ORDER BY requests DESC
	`, since.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []ProviderBreakdown
	for rows.Next() {
		var pb ProviderBreakdown
		if err := rows.Scan(&pb.Provider, &pb.Requests, &pb.InputTokens, &pb.OutputTokens, &pb.EstCostUSD, &pb.FallbackRate); err != nil {
			return nil, err
		}
		result = append(result, pb)
	}
	return result, rows.Err()
}

// DailyTokenPoint is a single day in the token trend.
type DailyTokenPoint struct {
	Date         string `json:"date"` // YYYY-MM-DD
	Requests     int64  `json:"requests"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}

// GetDailyTokenTrend returns daily token/request aggregates for the last N days.
func (a *Analytics) GetDailyTokenTrend(days int) ([]DailyTokenPoint, error) {
	if days <= 0 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -days)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := a.db.DB().QueryContext(ctx, `
		SELECT 
			DATE(created_at) AS day,
			COUNT(*) AS requests,
			COALESCE(SUM(input_tokens), 0) AS input_tokens,
			COALESCE(SUM(output_tokens), 0) AS output_tokens
		FROM requests
		WHERE created_at >= ?
		GROUP BY DATE(created_at)
		ORDER BY day ASC
	`, since.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []DailyTokenPoint
	for rows.Next() {
		var p DailyTokenPoint
		if err := rows.Scan(&p.Date, &p.Requests, &p.InputTokens, &p.OutputTokens); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
