package gui

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/routatic/proxy/internal/storage"
)

// AnalyticsHandler serves analytics endpoints for the dashboard.
type AnalyticsHandler struct {
	store   *storage.Analytics
	latency *storage.Latency
}

// NewAnalyticsHandler creates a handler backed by the given database.
// It internally creates an Analytics store and a Latency store.
func NewAnalyticsHandler(db *storage.Database) *AnalyticsHandler {
	return &AnalyticsHandler{
		store:   storage.NewAnalytics(db),
		latency: storage.NewLatency(db),
	}
}

func (h *AnalyticsHandler) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (h *AnalyticsHandler) getDays(r *http.Request) int {
	daysStr := r.URL.Query().Get("days")
	if daysStr == "" {
		return 30
	}
	d, err := strconv.Atoi(daysStr)
	if err != nil || d <= 0 {
		return 30
	}
	return d
}

// Summary returns high-level KPIs and breakdowns.
func (h *AnalyticsHandler) Summary(w http.ResponseWriter, r *http.Request) {
	days := h.getDays(r)

	summary, err := h.store.GetTokenSummary(days)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	models, err := h.store.GetModelBreakdown(days)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	providers, err := h.store.GetProviderBreakdown(days)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]any{
		"summary":      summary,
		"models":       models,
		"providers":    providers,
		"generated_at": time.Now().Format(time.RFC3339),
	}
	h.writeJSON(w, resp)
}

// TokenTrend returns daily token/request aggregates.
func (h *AnalyticsHandler) TokenTrend(w http.ResponseWriter, r *http.Request) {
	days := h.getDays(r)
	trend, err := h.store.GetDailyTokenTrend(days)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.writeJSON(w, map[string]any{
		"days":  days,
		"trend": trend,
	})
}

// LatencyStats returns latency stats per model.
func (h *AnalyticsHandler) LatencyStats(w http.ResponseWriter, r *http.Request) {
	days := h.getDays(r)
	since := time.Now().AddDate(0, 0, -days)
	stats, err := h.latency.GetStats(since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.writeJSON(w, map[string]any{
		"days":  days,
		"stats": stats,
	})
}
