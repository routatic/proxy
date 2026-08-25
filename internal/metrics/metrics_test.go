package metrics

import (
	"testing"
	"time"
)

func TestSnapshotPercentilesUseSortedSamples(t *testing.T) {
	t.Parallel()

	snapshot := Snapshot{Latencies: []time.Duration{
		90 * time.Millisecond,
		10 * time.Millisecond,
		50 * time.Millisecond,
		20 * time.Millisecond,
		80 * time.Millisecond,
	}}

	if got, want := snapshot.CalculateP95(), 90*time.Millisecond; got != want {
		t.Fatalf("CalculateP95() = %s, want %s", got, want)
	}
	if got, want := snapshot.CalculateP99(), 90*time.Millisecond; got != want {
		t.Fatalf("CalculateP99() = %s, want %s", got, want)
	}
}

func TestMetricsRetainsLatestGlobalSamples(t *testing.T) {
	t.Parallel()

	ring := newDurationRing(3)
	ring.Add(time.Second)
	ring.Add(2 * time.Second)
	ring.Add(3 * time.Second)
	ring.Add(4 * time.Second)

	got := ring.Snapshot()
	want := []time.Duration{2 * time.Second, 3 * time.Second, 4 * time.Second}
	if !equalDurations(got, want) {
		t.Fatalf("latencies = %v, want %v", got, want)
	}
}

func TestMetricsModelLatencyUsesBoundedSamples(t *testing.T) {
	t.Parallel()

	m := New()
	m.recordModelLatency("provider/model", time.Second)
	m.recordModelLatency("provider/model", 2*time.Second)
	m.recordModelLatency("provider/model", 3*time.Second)

	stats := m.GetModelLatencyStats()
	if len(stats) != 1 {
		t.Fatalf("got %d model stats, want 1", len(stats))
	}
	if got, want := stats[0].Count, int64(3); got != want {
		t.Fatalf("Count = %d, want %d", got, want)
	}
}

func TestMetricsRecordsStageAndTTFT(t *testing.T) {
	t.Parallel()

	m := New()
	m.RecordStage("token_count", 2*time.Millisecond)
	m.RecordTTFT(12 * time.Millisecond)

	snapshot := m.GetSnapshot()
	if got, want := percentile(snapshot.TTFT, 0.95), 12*time.Millisecond; got != want {
		t.Fatalf("TTFT p95 = %s, want %s", got, want)
	}
	if got := snapshot.StageLatencies["token_count"]; len(got) != 1 || got[0] != 2*time.Millisecond {
		t.Fatalf("token_count stage samples = %v", got)
	}
}

func equalDurations(a, b []time.Duration) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
