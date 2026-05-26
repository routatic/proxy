package handlers

import (
	"encoding/json"
	"net/http"

	"oc-go-cc/internal/buildinfo"
	"oc-go-cc/internal/metrics"
	"oc-go-cc/internal/router"
	"oc-go-cc/internal/status"
	"oc-go-cc/internal/token"
	"oc-go-cc/pkg/types"
)

// HealthHandler handles health checks and token counting endpoints.
type HealthHandler struct {
	tokenCounter    *token.Counter
	fallbackHandler *router.FallbackHandler
	metrics         *metrics.Metrics
	statusStore     *status.Store
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(tokenCounter *token.Counter, fallbackHandler *router.FallbackHandler, metrics *metrics.Metrics, statusStore *status.Store) *HealthHandler {
	return &HealthHandler{
		tokenCounter:    tokenCounter,
		fallbackHandler: fallbackHandler,
		metrics:         metrics,
		statusStore:     statusStore,
	}
}

// HandleHealth handles GET /health.
func (h *HealthHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	// Get metrics snapshot
	snapshot := h.metrics.GetSnapshot()

	// Get circuit breaker states
	cbStates := map[string]string{}
	if h.fallbackHandler != nil {
		cbStates = h.fallbackHandler.GetCircuitStates()
	}

	response := map[string]interface{}{
		"status":     "ok",
		"service":    "oc-go-cc",
		"version":    buildinfo.Version,
		"build_time": buildinfo.BuildTime,
		"pid":        buildinfo.PID(),
		"binary":     buildinfo.BinaryPath(),
		"metrics": map[string]interface{}{
			"requests_received": snapshot.RequestsReceived,
			"requests_success":  snapshot.RequestsSuccess,
			"requests_failed":   snapshot.RequestsFailed,
			"requests_streamed": snapshot.RequestsStreamed,
			"upstream_calls":    snapshot.UpstreamCalls,
			"rate_limited":      snapshot.RateLimited,
			"deduplicated":      snapshot.Deduplicated,
			"p95_latency_ms":    snapshot.CalculateP95().Milliseconds(),
			"p99_latency_ms":    snapshot.CalculateP99().Milliseconds(),
		},
		"circuit_breakers": cbStates,
		"models":           snapshot.ModelCounts,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func (h *HealthHandler) HandleStatusline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if h.statusStore == nil {
		_ = json.NewEncoder(w).Encode(status.NewStore(0).Snapshot())
		return
	}
	_ = json.NewEncoder(w).Encode(h.statusStore.Snapshot())
}

// HandleCountTokens handles POST /v1/messages/count_tokens.
func (h *HealthHandler) HandleCountTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body types.MessageRequest

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Count tokens.
	systemText, err := systemAndToolsTokenText(body.SystemText(), body.Tools)
	if err != nil {
		http.Error(w, "failed to process tools", http.StatusBadRequest)
		return
	}
	messages := tokenMessagesFromAnthropic(body.Messages)
	count, err := h.tokenCounter.CountMessages(systemText, messages)
	if err != nil {
		http.Error(w, "failed to count tokens", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]int{
		"input_tokens": count,
		"token_count":  count,
	})
}
