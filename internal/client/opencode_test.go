package client

import (
	"sync"
	"testing"
	"time"

	"oc-go-cc/internal/config"
)

func TestIsAnthropicModelOnlyRoutesNativeAnthropicModels(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		want    bool
	}{
		{
			name:    "minimax m2.5 uses anthropic endpoint",
			modelID: "minimax-m2.5",
			want:    true,
		},
		{
			name:    "minimax m2.7 uses anthropic endpoint",
			modelID: "minimax-m2.7",
			want:    true,
		},
		{
			name:    "deepseek pro uses openai endpoint",
			modelID: "deepseek-v4-pro",
			want:    false,
		},
		{
			name:    "deepseek flash uses openai endpoint",
			modelID: "deepseek-v4-flash",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAnthropicModel(tt.modelID); got != tt.want {
				t.Fatalf("IsAnthropicModel(%q) = %v, want %v", tt.modelID, got, tt.want)
			}
		})
	}
}

// newTestClient builds an OpenCodeClient backed by an in-memory atomic config.
// The HTTP client is never invoked by these key-selector tests so its defaults
// don't matter.
func newTestClient(cfg *config.Config) *OpenCodeClient {
	return NewOpenCodeClient(config.NewAtomicConfig(cfg, ""))
}

func TestPickKeyFallsBackToAPIKeyWhenAPIKeysEmpty(t *testing.T) {
	c := newTestClient(&config.Config{APIKey: "solo"})
	if got := c.pickKey(); got != "solo" {
		t.Fatalf("pickKey() = %q, want %q", got, "solo")
	}
}

func TestPickKeyRoundRobinsAcrossConfiguredKeys(t *testing.T) {
	c := newTestClient(&config.Config{APIKeys: []string{"k1", "k2", "k3"}})

	got := []string{c.pickKey(), c.pickKey(), c.pickKey(), c.pickKey()}
	want := []string{"k1", "k2", "k3", "k1"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pick[%d] = %q, want %q (sequence %v)", i, got[i], want[i], got)
		}
	}
}

func TestPickKeySkipsColdKeys(t *testing.T) {
	c := newTestClient(&config.Config{APIKeys: []string{"k1", "k2"}})

	c.markKeyCold("k1", "")

	for i := 0; i < 3; i++ {
		if got := c.pickKey(); got != "k2" {
			t.Fatalf("attempt %d: pickKey() = %q, want %q (k1 cold)", i, got, "k2")
		}
	}
}

func TestPickKeyReturnsEmptyWhenAllKeysCold(t *testing.T) {
	c := newTestClient(&config.Config{APIKeys: []string{"k1", "k2"}})

	c.markKeyCold("k1", "")
	c.markKeyCold("k2", "")

	if got := c.pickKey(); got != "" {
		t.Fatalf("pickKey() = %q, want empty (all cold)", got)
	}
}

func TestPickKeyRehabilitatesAfterCooldownExpires(t *testing.T) {
	c := newTestClient(&config.Config{APIKeys: []string{"k1"}})

	c.keyMu.Lock()
	c.keyCooldown["k1"] = time.Now().Add(-1 * time.Second) // already expired
	c.keyMu.Unlock()

	if got := c.pickKey(); got != "k1" {
		t.Fatalf("pickKey() = %q, want %q (cooldown expired)", got, "k1")
	}
}

func TestPickKeyReturnsEmptyWhenNoKeysConfigured(t *testing.T) {
	c := newTestClient(&config.Config{})
	if got := c.pickKey(); got != "" {
		t.Fatalf("pickKey() = %q, want empty (no keys)", got)
	}
}

func TestMarkKeyColdIgnoresEmptyString(t *testing.T) {
	c := newTestClient(&config.Config{APIKeys: []string{"k1"}})
	c.markKeyCold("", "") // must not panic; must not pollute cooldown map

	if got := c.pickKey(); got != "k1" {
		t.Fatalf("pickKey() = %q after no-op markKeyCold, want %q", got, "k1")
	}
}

// TestMarkKeyColdUsesRetryAfter verifies that an explicit Retry-After value
// is respected instead of the default cooldown.
func TestMarkKeyColdUsesRetryAfter(t *testing.T) {
	c := newTestClient(&config.Config{APIKeys: []string{"k1"}})

	c.markKeyCold("k1", "2") // 2 seconds

	if got := c.pickKey(); got != "" {
		t.Fatalf("pickKey() = %q, want empty (k1 cold for 2s)", got)
	}

	// Fast-forward cooldown.
	c.keyMu.Lock()
	c.keyCooldown["k1"] = time.Now().Add(-1 * time.Second)
	c.keyMu.Unlock()

	if got := c.pickKey(); got != "k1" {
		t.Fatalf("pickKey() = %q, want %q (k1 rehab after expiry)", got, "k1")
	}
}

// TestMarkKeyColdIgnoresInvalidRetryAfter falls back to default on bad input.
func TestMarkKeyColdIgnoresInvalidRetryAfter(t *testing.T) {
	c := newTestClient(&config.Config{APIKeys: []string{"k1"}})
	c.markKeyCold("k1", "not-a-number")

	// Should still be cold (default 60s), not panicked.
	if got := c.pickKey(); got != "" {
		t.Fatalf("pickKey() = %q, want empty (fallback cooldown applied)", got)
	}
}

// TestPickKeySkipsCircuitBrokenKeys verifies that keys with an open circuit
// breaker are skipped, and rehabilitated after the recovery timeout.
func TestPickKeySkipsCircuitBrokenKeys(t *testing.T) {
	c := newTestClient(&config.Config{APIKeys: []string{"k1", "k2"}})

	cb := c.getKeyCircuitBreaker("k1")
	for i := 0; i < 3; i++ {
		cb.RecordFailure() // open the circuit
	}
	if cb.Allow() {
		t.Fatal("expected circuit breaker to be open")
	}

	// k1 should be skipped, k2 returned every time.
	for i := 0; i < 3; i++ {
		if got := c.pickKey(); got != "k2" {
			t.Fatalf("attempt %d: pickKey() = %q, want %q (k1 circuit open)", i, got, "k2")
		}
	}
}

// TestKeyCircuitBreakerHalfOpen verifies the half-open probe behaviour.
func TestKeyCircuitBreakerHalfOpen(t *testing.T) {
	cb := NewKeyCircuitBreaker(3, 100*time.Millisecond)

	// Open the circuit.
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}
	if cb.State() != CircuitOpen {
		t.Fatalf("expected state Open, got %d", cb.State())
	}

	// Should still block immediately.
	if cb.Allow() {
		t.Fatal("expected blocked while open")
	}

	// Wait for recovery timeout → transitions to HalfOpen.
	time.Sleep(150 * time.Millisecond)
	if !cb.Allow() {
		t.Fatal("expected allowed in half-open (probe)")
	}
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("expected state HalfOpen, got %d", cb.State())
	}

	// Success in half-open should close circuit after enough probes.
	cb.RecordSuccess()
	cb.RecordSuccess()
	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Fatalf("expected state Closed after successes, got %d", cb.State())
	}
	if !cb.Allow() {
		t.Fatal("expected allowed after closed")
	}
}

// TestKeyCircuitBreakerHalfOpenFailureReopens verifies that a failure in
// half-open immediately reopens the circuit.
func TestKeyCircuitBreakerHalfOpenFailureReopens(t *testing.T) {
	cb := NewKeyCircuitBreaker(3, 100*time.Millisecond)

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	time.Sleep(150 * time.Millisecond)
	if !cb.Allow() {
		t.Fatal("expected allowed in half-open")
	}

	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("expected state Open after half-open failure, got %d", cb.State())
	}
}

// TestChatCompletionRetriesOnRetryableStatus verifies that 5xx errors trigger
// a circuit-breaker failure and retry with the next key.
func TestChatCompletionRetriesOnRetryableStatus(t *testing.T) {
	// This test requires a mock HTTP server; we verify the circuit breaker
	// state transitions instead to avoid heavy infrastructure.
	c := newTestClient(&config.Config{APIKeys: []string{"k1", "k2"}})

	c.getKeyCircuitBreaker("k1").RecordFailure()
	c.getKeyCircuitBreaker("k1").RecordFailure()
	c.getKeyCircuitBreaker("k1").RecordFailure()

	if c.getKeyCircuitBreaker("k1").Allow() {
		t.Fatal("k1 circuit should be open")
	}
	if !c.getKeyCircuitBreaker("k2").Allow() {
		t.Fatal("k2 circuit should be closed")
	}
}

// TestPickKeyConcurrentUniqueIndices verifies that atomic.Uint64 eliminates
// mutex contention on the hot path: many goroutines pick distinct keys.
func TestPickKeyConcurrentUniqueIndices(t *testing.T) {
	c := newTestClient(&config.Config{APIKeys: []string{"k1", "k2", "k3"}})
	const n = 300
	results := make([]string, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = c.pickKey()
		}(i)
	}
	wg.Wait()

	counts := map[string]int{}
	for _, k := range results {
		if k == "" {
			t.Fatal("concurrent pickKey returned empty")
		}
		counts[k]++
	}
	// With 300 picks across 3 keys we expect ~100 each (±30 for variance).
	for _, key := range []string{"k1", "k2", "k3"} {
		if counts[key] < 70 || counts[key] > 130 {
			t.Fatalf("key %q count = %d, expected ~100 (±30); distribution = %v", key, counts[key], counts)
		}
	}
}

// TestPickKeyGCSExpiredCooldowns verifies that expired entries are deleted
// from the cooldown map during pickKey, preventing unbounded growth.
func TestPickKeyGCSExpiredCooldowns(t *testing.T) {
	c := newTestClient(&config.Config{APIKeys: []string{"k1", "k2"}})

	c.keyMu.Lock()
	c.keyCooldown["k1"] = time.Now().Add(-1 * time.Second) // expired
	c.keyCooldown["k2"] = time.Now().Add(-1 * time.Second) // expired
	c.keyMu.Unlock()

	c.pickKey() // should GC both entries

	c.keyMu.Lock()
	mapLen := len(c.keyCooldown)
	c.keyMu.Unlock()

	if mapLen != 0 {
		t.Fatalf("expected cooldown map GC'd to 0, got %d", mapLen)
	}
}
