// Package metrics provides in-memory metrics for the proxy.
package metrics

import (
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ModelKey returns the canonical provider/model label used by metrics.
func ModelKey(provider, model string) string {
	if strings.TrimSpace(provider) == "" {
		return model
	}
	return provider + "/" + model
}

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
	storageDropped   atomic.Int64

	// Latency tracking
	mu                sync.RWMutex
	latencies         durationRing
	maxLatencySamples int

	// Request-stage timing
	stageMu        sync.RWMutex
	stageLatencies map[string]durationRing
	ttft           durationRing

	// By model
	modelCounts map[string]*atomic.Int64
	modelMu     sync.RWMutex

	// Per-model success/failure for accurate success rates
	modelSuccess   map[string]int64
	modelSuccessMu sync.RWMutex
	modelFailed    map[string]int64
	modelFailedMu  sync.RWMutex

	// Per-model latency tracking
	modelLatencies     map[string]*durationRing
	modelLatMu         sync.RWMutex
	maxPerModelSamples int
}

const (
	defaultMaxLatencySamples  = 1000
	defaultMaxPerModelSamples = 200
	defaultMaxStageSamples    = 1000
	StageRequestParse         = "request_parse"
	StageTokenCount           = "token_count"
	StageRouting              = "routing"
	StageNormalization        = "normalization"
	StageUpstream             = "upstream"
	StageResponseTransform    = "response_transform"
	StageStorageEnqueue       = "storage_enqueue"
)

// durationRing stores a bounded rolling window of durations.
type durationRing struct {
	values []time.Duration
	next   int
	count  int
}

func newDurationRing(capacity int) durationRing {
	return durationRing{values: make([]time.Duration, capacity)}
}

func (r *durationRing) Add(value time.Duration) {
	if len(r.values) == 0 {
		return
	}
	r.values[r.next] = value
	r.next = (r.next + 1) % len(r.values)
	if r.count < len(r.values) {
		r.count++
	}
}

func (r *durationRing) Snapshot() []time.Duration {
	if r.count == 0 {
		return nil
	}
	out := make([]time.Duration, r.count)
	start := 0
	if r.count == len(r.values) {
		start = r.next
	}
	for i := range out {
		out[i] = r.values[(start+i)%len(r.values)]
	}
	return out
}

// New creates a new metrics instance.
func New() *Metrics {
	return &Metrics{
		maxLatencySamples:  defaultMaxLatencySamples,
		maxPerModelSamples: defaultMaxPerModelSamples,
		latencies:          newDurationRing(defaultMaxLatencySamples),
		ttft:               newDurationRing(defaultMaxLatencySamples),
		stageLatencies:     make(map[string]durationRing),
		modelCounts:        make(map[string]*atomic.Int64),
		modelSuccess:       make(map[string]int64),
		modelFailed:        make(map[string]int64),
		modelLatencies:     make(map[string]*durationRing),
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

// RecordStorageDrop records a completion record dropped from the bounded
// asynchronous storage queue.
func (m *Metrics) RecordStorageDrop() {
	m.storageDropped.Add(1)
}

func (m *Metrics) recordLatency(latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latencies.Add(latency)
}

func (m *Metrics) recordModelLatency(model string, latency time.Duration) {
	m.modelLatMu.Lock()
	defer m.modelLatMu.Unlock()

	ring, ok := m.modelLatencies[model]
	if !ok {
		r := newDurationRing(m.maxPerModelSamples)
		ring = &r
		m.modelLatencies[model] = ring
	}
	ring.Add(latency)
}

// RecordStage records a bounded timing sample for a known request stage.
func (m *Metrics) RecordStage(stage string, duration time.Duration) {
	m.stageMu.Lock()
	defer m.stageMu.Unlock()

	ring, ok := m.stageLatencies[stage]
	if !ok {
		ring = newDurationRing(defaultMaxStageSamples)
		m.stageLatencies[stage] = ring
	}
	ring.Add(duration)
	m.stageLatencies[stage] = ring
}

// RecordTTFT records the time to first non-empty model content for a streaming
// request.
func (m *Metrics) RecordTTFT(duration time.Duration) {
	m.stageMu.Lock()
	defer m.stageMu.Unlock()
	m.ttft.Add(duration)
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
	latencies := m.latencies.Snapshot()
	m.mu.RUnlock()

	m.stageMu.RLock()
	ttft := m.ttft.Snapshot()
	stageLatencies := make(map[string][]time.Duration, len(m.stageLatencies))
	for stage, ring := range m.stageLatencies {
		samples := ring.Snapshot()
		stageLatencies[stage] = samples
	}
	m.stageMu.RUnlock()

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
		StorageDropped:   m.storageDropped.Load(),
		Latencies:        latencies,
		ModelCounts:      modelCounts,
		ModelSuccess:     modelSuccess,
		ModelFailed:      modelFailed,
		TTFT:             ttft,
		StageLatencies:   stageLatencies,
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
	StorageDropped   int64
	Latencies        []time.Duration
	ModelCounts      map[string]int64
	ModelSuccess     map[string]int64 // Per-model success counts
	ModelFailed      map[string]int64 // Per-model failure counts
	TTFT             []time.Duration
	StageLatencies   map[string][]time.Duration
}

// ModelLatencyStats holds latency statistics for a single model.
type ModelLatencyStats struct {
	Model string
	Count int64
	Avg   time.Duration
	P50   time.Duration
	P90   time.Duration
	P95   time.Duration
	P99   time.Duration
	Min   time.Duration
	Max   time.Duration
}

// GetModelLatencyStats returns latency statistics for all models.
func (m *Metrics) GetModelLatencyStats() []ModelLatencyStats {
	m.modelLatMu.RLock()
	samplesByModel := make(map[string][]time.Duration, len(m.modelLatencies))
	for model, ring := range m.modelLatencies {
		samples := ring.Snapshot()
		if len(samples) == 0 {
			continue
		}
		samplesByModel[model] = samples
	}
	m.modelLatMu.RUnlock()

	stats := make([]ModelLatencyStats, 0, len(samplesByModel))
	for model, samples := range samplesByModel {
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
	p95Idx := int(math.Ceil(float64(count)*0.95)) - 1
	p99Idx := int(math.Ceil(float64(count)*0.99)) - 1
	if p50Idx < 0 {
		p50Idx = 0
	}
	if p90Idx < 0 {
		p90Idx = 0
	}
	if p95Idx < 0 {
		p95Idx = 0
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
	if p95Idx >= count {
		p95Idx = count - 1
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
		P95:   sorted[p95Idx],
		P99:   sorted[p99Idx],
		Min:   sorted[0],
		Max:   sorted[count-1],
	}
}

// CalculateP95 calculates the p95 latency from the snapshot.
func (s Snapshot) CalculateP95() time.Duration {
	p95, _ := s.Percentiles()
	return p95
}

// CalculateP99 calculates the p99 latency from the snapshot.
func (s Snapshot) CalculateP99() time.Duration {
	_, p99 := s.Percentiles()
	return p99
}

// Percentiles returns p95 and p99 from the snapshot's latency samples.
// It sorts one copy so callers can safely provide samples in any order.
func (s Snapshot) Percentiles() (time.Duration, time.Duration) {
	return percentiles(s.Latencies)
}

// TTFTPercentiles returns p95 and p99 for streaming time-to-first-token samples.
func (s Snapshot) TTFTPercentiles() (time.Duration, time.Duration) {
	return percentiles(s.TTFT)
}

func percentiles(samples []time.Duration) (time.Duration, time.Duration) {
	sortedSamples := append([]time.Duration(nil), samples...)
	sort.Slice(sortedSamples, func(i, j int) bool { return sortedSamples[i] < sortedSamples[j] })
	return percentile(sortedSamples, 0.95), percentile(sortedSamples, 0.99)
}

func percentile(sortedSamples []time.Duration, fraction float64) time.Duration {
	if len(sortedSamples) == 0 {
		return 0
	}
	index := int(math.Ceil(float64(len(sortedSamples))*fraction)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sortedSamples) {
		index = len(sortedSamples) - 1
	}
	return sortedSamples[index]
}
