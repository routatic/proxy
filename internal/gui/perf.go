package gui

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/metrics"
)

func (s *Server) handlePerformance(w http.ResponseWriter, r *http.Request) {
	var modelStats []metrics.ModelLatencyStats
	if s.met != nil {
		modelStats = s.met.GetModelLatencyStats()
	}

	successCounts := make(map[string]int64)
	failureCounts := make(map[string]int64)

	if s.hist != nil {
		records := s.hist.Last(0)
		for _, rec := range records {
			if rec.Success {
				successCounts[rec.Model]++
			} else {
				failureCounts[rec.Model]++
			}
		}
	}

	type modelPerf struct {
		Model    string `json:"model"`
		Count    int64  `json:"count"`
		Success  int64  `json:"success"`
		Failed   int64  `json:"failed"`
		AvgMs    int64  `json:"avg_ms"`
		P50Ms    int64  `json:"p50_ms"`
		P90Ms    int64  `json:"p90_ms"`
		P99Ms    int64  `json:"p99_ms"`
		MinMs    int64  `json:"min_ms"`
		MaxMs    int64  `json:"max_ms"`
	}

	result := make(map[string]modelPerf)

	for _, stat := range modelStats {
		perf := modelPerf{
			Model:  stat.Model,
			Count:  stat.Count,
			AvgMs:  stat.Avg.Milliseconds(),
			P50Ms:  stat.P50.Milliseconds(),
			P90Ms:  stat.P90.Milliseconds(),
			P99Ms:  stat.P99.Milliseconds(),
			MinMs:  stat.Min.Milliseconds(),
			MaxMs:  stat.Max.Milliseconds(),
			Success: successCounts[stat.Model],
			Failed:  failureCounts[stat.Model],
		}
		result[stat.Model] = perf
	}

	for model, success := range successCounts {
		if _, exists := result[model]; !exists {
			result[model] = modelPerf{
				Model:   model,
				Success: success,
				Failed:  failureCounts[model],
			}
		}
	}

	var output []modelPerf
	for _, perf := range result {
		output = append(output, perf)
	}

	writeJSON(w, output)
}

func (s *Server) handlePerformanceAggregate(w http.ResponseWriter, r *http.Request) {
	rangeParam := r.URL.Query().Get("range")
	var since time.Time
	switch rangeParam {
	case "1h":
		since = time.Now().Add(-1 * time.Hour)
	case "24h":
		since = time.Now().Add(-24 * time.Hour)
	case "7d":
		since = time.Now().Add(-7 * 24 * time.Hour)
	default:
		since = time.Time{}
	}

	type aggregate struct {
		TotalRequests int64 `json:"total_requests"`
		TotalSuccess  int64 `json:"total_success"`
		TotalFailed   int64 `json:"total_failed"`
		AvgLatencyMs  int64 `json:"avg_latency_ms"`
		ConnectionTime time.Time `json:"connection_time"`
	}

	agg := aggregate{}

	if s.met != nil {
		snap := s.met.GetSnapshot()
		agg.TotalRequests = snap.RequestsReceived
		agg.TotalSuccess = snap.RequestsSuccess
		agg.TotalFailed = snap.RequestsFailed
		if len(snap.Latencies) > 0 {
			var sum time.Duration
			for _, lat := range snap.Latencies {
				sum += lat
			}
			agg.AvgLatencyMs = (sum / time.Duration(len(snap.Latencies))).Milliseconds()
		}
	}

	writeJSON(w, agg)
}
