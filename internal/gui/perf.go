package gui

import (
	"net/http"
	"time"

	"github.com/routatic/proxy/internal/storage"
)

func (s *Server) handlePerformance(w http.ResponseWriter, r *http.Request) {
	rangeParam := r.URL.Query().Get("range")
	since := storage.ParseTimeRange(rangeParam)

	type modelPerf struct {
		Model   string `json:"model"`
		Count   int64  `json:"count"`
		Success int64  `json:"success"`
		Failed  int64  `json:"failed"`
		AvgMs   int64  `json:"avg_ms"`
		P50Ms   int64  `json:"p50_ms"`
		P90Ms   int64  `json:"p90_ms"`
		P99Ms   int64  `json:"p99_ms"`
		MinMs   int64  `json:"min_ms"`
		MaxMs   int64  `json:"max_ms"`
	}

	result := make(map[string]modelPerf)

	if s.storage != nil {
		latency := storage.NewLatency(s.storage)

		modelStats, err := latency.GetStats(since)
		if err == nil {
			for _, stat := range modelStats {
				result[stat.Model] = modelPerf{
					Model: stat.Model,
					Count: stat.Count,
					AvgMs: stat.Avg.Milliseconds(),
					P50Ms: stat.P50.Milliseconds(),
					P90Ms: stat.P90.Milliseconds(),
					P99Ms: stat.P99.Milliseconds(),
					MinMs: stat.Min.Milliseconds(),
					MaxMs: stat.Max.Milliseconds(),
				}
			}
		}

		successCounts, failureCounts, _ := latency.GetSuccessCounts(since)
		for model, count := range successCounts {
			if perf, exists := result[model]; exists {
				perf.Success = count
				result[model] = perf
			} else {
				result[model] = modelPerf{
					Model:   model,
					Success: count,
					Failed:  failureCounts[model],
				}
			}
		}
		for model, count := range failureCounts {
			if perf, exists := result[model]; exists {
				perf.Failed = count
				result[model] = perf
			}
		}
	} else if s.met != nil {
		snap := s.met.GetSnapshot()
		modelStats := s.met.GetModelLatencyStats()

		for _, stat := range modelStats {
			result[stat.Model] = modelPerf{
				Model: stat.Model,
				Count: stat.Count,
				AvgMs: stat.Avg.Milliseconds(),
				P50Ms: stat.P50.Milliseconds(),
				P90Ms: stat.P90.Milliseconds(),
				P99Ms: stat.P99.Milliseconds(),
				MinMs: stat.Min.Milliseconds(),
				MaxMs: stat.Max.Milliseconds(),
			}
		}

		for model, count := range snap.ModelCounts {
			if perf, exists := result[model]; exists {
				perf.Success = snap.ModelSuccess[model]
				perf.Failed = snap.ModelFailed[model]
				result[model] = perf
			} else {
				result[model] = modelPerf{
					Model:   model,
					Count:   count,
					Success: snap.ModelSuccess[model],
					Failed:  snap.ModelFailed[model],
				}
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
	since := storage.ParseTimeRange(rangeParam)

	type aggregate struct {
		TotalRequests int64 `json:"total_requests"`
		TotalSuccess  int64 `json:"total_success"`
		TotalFailed   int64 `json:"total_failed"`
		AvgLatencyMs  int64 `json:"avg_latency_ms"`
	}

	agg := aggregate{}

	if s.storage != nil {
		latency := storage.NewLatency(s.storage)
		latencyStats, err := latency.GetStats(since)
		if err == nil && len(latencyStats) > 0 {
			var totalCount int64
			var totalLatency time.Duration
			for _, stat := range latencyStats {
				totalCount += stat.Count
				totalLatency += stat.Avg * time.Duration(stat.Count)
			}
			if totalCount > 0 {
				agg.AvgLatencyMs = (totalLatency / time.Duration(totalCount)).Milliseconds()
			}
		}

		successCounts, failureCounts, _ := latency.GetSuccessCounts(since)
		for _, s := range successCounts {
			agg.TotalSuccess += s
		}
		for _, f := range failureCounts {
			agg.TotalFailed += f
		}
		agg.TotalRequests = agg.TotalSuccess + agg.TotalFailed
	} else if s.met != nil {
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
