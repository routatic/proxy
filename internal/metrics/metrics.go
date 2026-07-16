// Package metrics provides in-memory metrics for the proxy.
package metrics

import (
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics holds in-memory metrics for the proxy.
type Metrics struct {
	// Counters (atomic)
	requestsReceived atomic.Int64
	requestsStreamed atomic.Int64
	requestsSuccess  atomic.Int64
	requestsFailed   atomic.Int64
	upstreamCalls    atomic.Int64
	rateLimited      atomic.Int64
	deduplicated     atomic.Int64

	// Latency tracking
	mu                sync.RWMutex
	latencies         []time.Duration
	maxLatencySamples int

	// By model
	modelCounts map[string]*atomic.Int64
	modelMu     sync.RWMutex

	// Per-model success/failure for accurate success rates
	modelSuccess   map[string]int64
	modelSuccessMu sync.RWMutex
	modelFailed    map[string]int64
	modelFailedMu  sync.RWMutex

	// Per-model latency tracking
	modelLatencies     map[string][]time.Duration
	modelLatMu         sync.RWMutex
	maxPerModelSamples int
}

// New creates a new metrics instance.
func New() *Metrics {
	return &Metrics{
		maxLatencySamples:  1000,
		maxPerModelSamples: 100,
		modelCounts:        make(map[string]*atomic.Int64),
		modelSuccess:       make(map[string]int64),
		modelFailed:        make(map[string]int64),
		modelLatencies:     make(map[string][]time.Duration),
	}
}

// RecordRequest records an incoming request.
func (m *Metrics) RecordRequest(streaming bool) {
	m.requestsReceived.Add(1)
	if streaming {
		m.requestsStreamed.Add(1)
	}
}

// RecordSuccess records a successful request.
func (m *Metrics) RecordSuccess(model string, latency time.Duration) {
	m.requestsSuccess.Add(1)
	m.upstreamCalls.Add(1)
	m.recordLatency(latency)
	m.recordModel(model)
	m.recordModelLatency(model, latency)

	m.modelSuccessMu.Lock()
	m.modelSuccess[model]++
	m.modelSuccessMu.Unlock()
}

// RecordFailure records a failed request.
func (m *Metrics) RecordFailure() {
	m.requestsFailed.Add(1)
}

// RecordFailureForModel records a failed request for a specific model.
func (m *Metrics) RecordFailureForModel(model string) {
	m.requestsFailed.Add(1)

	m.modelFailedMu.Lock()
	m.modelFailed[model]++
	m.modelFailedMu.Unlock()
}

// RecordRateLimited records a rate-limited request.
func (m *Metrics) RecordRateLimited() {
	m.rateLimited.Add(1)
}

// RecordDeduplicated records a deduplicated request.
func (m *Metrics) RecordDeduplicated() {
	m.deduplicated.Add(1)
}

func (m *Metrics) recordLatency(latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Keep last N samples for p95/p99
	if len(m.latencies) >= m.maxLatencySamples {
		// Shift and add new
		m.latencies = m.latencies[1:]
	}
	m.latencies = append(m.latencies, latency)
}

func (m *Metrics) recordModelLatency(model string, latency time.Duration) {
	m.modelLatMu.Lock()
	defer m.modelLatMu.Unlock()

	samples := m.modelLatencies[model]
	if len(samples) >= m.maxPerModelSamples {
		samples = samples[1:]
	}
	m.modelLatencies[model] = append(samples, latency)
}

func (m *Metrics) recordModel(model string) {
	m.modelMu.Lock()
	defer m.modelMu.Unlock()

	if _, exists := m.modelCounts[model]; !exists {
		m.modelCounts[model] = &atomic.Int64{}
	}
	m.modelCounts[model].Add(1)
}

// GetSnapshot returns a snapshot of current metrics.
func (m *Metrics) GetSnapshot() Snapshot {
	m.mu.RLock()
	latencies := make([]time.Duration, len(m.latencies))
	copy(latencies, m.latencies)
	m.mu.RUnlock()

	modelCounts := make(map[string]int64)
	m.modelMu.RLock()
	for k, v := range m.modelCounts {
		modelCounts[k] = v.Load()
	}
	m.modelMu.RUnlock()

	// Collect per-model success/failure counts
	modelSuccess := make(map[string]int64)
	modelFailed := make(map[string]int64)
	m.modelSuccessMu.RLock()
	for k, v := range m.modelSuccess {
		modelSuccess[k] = v
	}
	m.modelSuccessMu.RUnlock()

	m.modelFailedMu.RLock()
	for k, v := range m.modelFailed {
		modelFailed[k] = v
	}
	m.modelFailedMu.RUnlock()

	return Snapshot{
		RequestsReceived: m.requestsReceived.Load(),
		RequestsStreamed: m.requestsStreamed.Load(),
		RequestsSuccess:  m.requestsSuccess.Load(),
		RequestsFailed:   m.requestsFailed.Load(),
		UpstreamCalls:    m.upstreamCalls.Load(),
		RateLimited:      m.rateLimited.Load(),
		Deduplicated:     m.deduplicated.Load(),
		Latencies:        latencies,
		ModelCounts:      modelCounts,
		ModelSuccess:     modelSuccess,
		ModelFailed:      modelFailed,
	}
}

// Snapshot represents a point-in-time view of metrics.
type Snapshot struct {
	RequestsReceived int64
	RequestsStreamed int64
	RequestsSuccess  int64
	RequestsFailed   int64
	UpstreamCalls    int64
	RateLimited      int64
	Deduplicated     int64
	Latencies        []time.Duration
	ModelCounts      map[string]int64
	ModelSuccess     map[string]int64 // Per-model success counts
	ModelFailed      map[string]int64 // Per-model failure counts
}

// ModelLatencyStats holds latency statistics for a single model.
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

// GetModelLatencyStats returns latency statistics for all models.
func (m *Metrics) GetModelLatencyStats() []ModelLatencyStats {
	m.modelLatMu.RLock()
	defer m.modelLatMu.RUnlock()

	var stats []ModelLatencyStats
	for model, samples := range m.modelLatencies {
		if len(samples) == 0 {
			continue
		}
		stats = append(stats, calculateModelStats(model, samples))
	}
	return stats
}

func calculateModelStats(model string, samples []time.Duration) ModelLatencyStats {
	if len(samples) == 0 {
		return ModelLatencyStats{Model: model}
	}

	sorted := make([]time.Duration, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}

	count := len(sorted)
	avg := sum / time.Duration(count)

	p50Idx := int(math.Ceil(float64(count)*0.50)) - 1
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
		Avg:   avg,
		P50:   sorted[p50Idx],
		P90:   sorted[p90Idx],
		P99:   sorted[p99Idx],
		Min:   sorted[0],
		Max:   sorted[count-1],
	}
}

// CalculateP95 calculates the p95 latency from the snapshot.
func (s Snapshot) CalculateP95() time.Duration {
	if len(s.Latencies) == 0 {
		return 0
	}
	index := int(float64(len(s.Latencies)) * 0.95)
	if index >= len(s.Latencies) {
		index = len(s.Latencies) - 1
	}
	return s.Latencies[index]
}

// CalculateP99 calculates the p99 latency from the snapshot.
func (s Snapshot) CalculateP99() time.Duration {
	if len(s.Latencies) == 0 {
		return 0
	}
	index := int(float64(len(s.Latencies)) * 0.99)
	if index >= len(s.Latencies) {
		index = len(s.Latencies) - 1
	}
	return s.Latencies[index]
}
