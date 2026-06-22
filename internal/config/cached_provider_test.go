// Package config provides configuration management for the routatic-proxy.
package config

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/auth"
)

// mockConfigProvider is a test double for ConfigProvider.
type mockConfigProvider struct {
	mu           sync.Mutex
	callCount    int
	configs      map[string]*RuntimeConfig
	healthErr    error
	getConfigErr error
}

func newMockConfigProvider() *mockConfigProvider {
	return &mockConfigProvider{
		configs: make(map[string]*RuntimeConfig),
	}
}

func (m *mockConfigProvider) GetEffectiveConfig(ctx context.Context, authCtx *auth.AuthContext) (*RuntimeConfig, error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()

	if m.getConfigErr != nil {
		return nil, m.getConfigErr
	}

	if authCtx == nil {
		return &RuntimeConfig{WorkspaceID: "default", Version: "v1"}, nil
	}

	key := CacheKey(authCtx.WorkspaceID, authCtx.ConfigRef.Version)
	if cfg, ok := m.configs[key]; ok {
		return cfg, nil
	}

	return &RuntimeConfig{
		WorkspaceID: authCtx.WorkspaceID,
		Version:     authCtx.ConfigRef.Version,
	}, nil
}

func (m *mockConfigProvider) GetConfigByRef(ctx context.Context, ref auth.ConfigRef) (*RuntimeConfig, error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()

	if m.getConfigErr != nil {
		return nil, m.getConfigErr
	}

	key := CacheKey(ref.WorkspaceID, ref.Version)
	if cfg, ok := m.configs[key]; ok {
		return cfg, nil
	}

	return &RuntimeConfig{
		WorkspaceID: ref.WorkspaceID,
		Version:     ref.Version,
	}, nil
}

func (m *mockConfigProvider) Invalidate(ctx context.Context, workspaceID string, version string) error {
	return nil
}

func (m *mockConfigProvider) HealthCheck(ctx context.Context) error {
	return m.healthErr
}

func (m *mockConfigProvider) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func TestNewCachedConfigProvider(t *testing.T) {
	mock := newMockConfigProvider()
	provider := NewCachedConfigProvider(mock, 5*time.Minute)

	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
	if provider.ttl != 5*time.Minute {
		t.Errorf("expected ttl 5m, got %v", provider.ttl)
	}
	if provider.cache == nil {
		t.Error("expected non-nil cache")
	}
}

func TestCachedConfigProvider_GetEffectiveConfig_CacheHit(t *testing.T) {
	mock := newMockConfigProvider()
	provider := NewCachedConfigProvider(mock, 5*time.Minute)

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws-123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws-123",
			Version:     "v1",
		},
	}

	// First call - cache miss
	_, err := provider.GetEffectiveConfig(context.Background(), authCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second call - should be cache hit
	_, err = provider.GetEffectiveConfig(context.Background(), authCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.getCallCount() != 1 {
		t.Errorf("expected 1 underlying call, got %d", mock.getCallCount())
	}

	stats := provider.GetStats()
	if stats.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}
}

func TestCachedConfigProvider_GetEffectiveConfig_CacheMissNilAuth(t *testing.T) {
	mock := newMockConfigProvider()
	provider := NewCachedConfigProvider(mock, 5*time.Minute)

	// Call with nil auth context - should bypass cache
	_, err := provider.GetEffectiveConfig(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Call again - should still bypass cache
	_, err = provider.GetEffectiveConfig(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.getCallCount() != 2 {
		t.Errorf("expected 2 underlying calls (cache bypassed), got %d", mock.getCallCount())
	}
}

func TestCachedConfigProvider_GetEffectiveConfig_TTLExpiration(t *testing.T) {
	mock := newMockConfigProvider()
	provider := NewCachedConfigProvider(mock, 50*time.Millisecond)

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws-123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws-123",
			Version:     "v1",
		},
	}

	// First call
	_, err := provider.GetEffectiveConfig(context.Background(), authCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for TTL to expire
	time.Sleep(100 * time.Millisecond)

	// Second call should trigger new fetch
	_, err = provider.GetEffectiveConfig(context.Background(), authCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.getCallCount() != 2 {
		t.Errorf("expected 2 underlying calls after TTL expiry, got %d", mock.getCallCount())
	}
}

func TestCachedConfigProvider_GetEffectiveConfig_InfiniteTTL(t *testing.T) {
	mock := newMockConfigProvider()
	provider := NewCachedConfigProvider(mock, 0) // Infinite TTL

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws-123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws-123",
			Version:     "v1",
		},
	}

	// First call
	_, err := provider.GetEffectiveConfig(context.Background(), authCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Second call should still be cache hit
	_, err = provider.GetEffectiveConfig(context.Background(), authCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.getCallCount() != 1 {
		t.Errorf("expected 1 underlying call with infinite TTL, got %d", mock.getCallCount())
	}
}

func TestCachedConfigProvider_GetConfigByRef(t *testing.T) {
	mock := newMockConfigProvider()
	provider := NewCachedConfigProvider(mock, 5*time.Minute)

	ref := auth.ConfigRef{
		WorkspaceID: "ws-123",
		Version:     "v1",
	}

	// First call - cache miss
	_, err := provider.GetConfigByRef(context.Background(), ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second call - cache hit
	_, err = provider.GetConfigByRef(context.Background(), ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.getCallCount() != 1 {
		t.Errorf("expected 1 underlying call, got %d", mock.getCallCount())
	}

	stats := provider.GetStats()
	if stats.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.Hits)
	}
}

func TestCachedConfigProvider_Invalidate_SpecificVersion(t *testing.T) {
	mock := newMockConfigProvider()
	provider := NewCachedConfigProvider(mock, 5*time.Minute)

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws-123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws-123",
			Version:     "v1",
		},
	}

	// Populate cache
	_, _ = provider.GetEffectiveConfig(context.Background(), authCtx)

	// Invalidate specific version
	err := provider.Invalidate(context.Background(), "ws-123", "v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Next call should be cache miss
	_, _ = provider.GetEffectiveConfig(context.Background(), authCtx)

	if mock.getCallCount() != 2 {
		t.Errorf("expected 2 underlying calls after invalidation, got %d", mock.getCallCount())
	}

	stats := provider.GetStats()
	if stats.Invalidations != 1 {
		t.Errorf("expected 1 invalidation, got %d", stats.Invalidations)
	}
}

func TestCachedConfigProvider_Invalidate_AllVersions(t *testing.T) {
	mock := newMockConfigProvider()
	provider := NewCachedConfigProvider(mock, 5*time.Minute)

	// Populate cache with multiple versions
	v1 := &auth.AuthContext{
		WorkspaceID: "ws-123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws-123",
			Version:     "v1",
		},
	}
	v2 := &auth.AuthContext{
		WorkspaceID: "ws-123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws-123",
			Version:     "v2",
		},
	}
	other := &auth.AuthContext{
		WorkspaceID: "ws-456",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws-456",
			Version:     "v1",
		},
	}

	_, _ = provider.GetEffectiveConfig(context.Background(), v1)
	_, _ = provider.GetEffectiveConfig(context.Background(), v2)
	_, _ = provider.GetEffectiveConfig(context.Background(), other)

	// Invalidate all versions for ws-123
	err := provider.Invalidate(context.Background(), "ws-123", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// These should be cache misses
	_, _ = provider.GetEffectiveConfig(context.Background(), v1)
	_, _ = provider.GetEffectiveConfig(context.Background(), v2)

	// This should still be cache hit
	_, _ = provider.GetEffectiveConfig(context.Background(), other)

	if mock.getCallCount() != 5 {
		t.Errorf("expected 5 underlying calls, got %d", mock.getCallCount())
	}

	stats := provider.GetStats()
	if stats.Invalidations != 2 {
		t.Errorf("expected 2 invalidations, got %d", stats.Invalidations)
	}
}

func TestCachedConfigProvider_HealthCheck(t *testing.T) {
	mock := newMockConfigProvider()
	provider := NewCachedConfigProvider(mock, 5*time.Minute)

	// Healthy
	err := provider.HealthCheck(context.Background())
	if err != nil {
		t.Errorf("expected nil error for healthy provider, got %v", err)
	}

	// Unhealthy
	mock.healthErr = errors.New("unhealthy")
	err = provider.HealthCheck(context.Background())
	if err == nil {
		t.Error("expected error for unhealthy provider")
	}
}

func TestCachedConfigProvider_ConcurrentAccess(t *testing.T) {
	mock := newMockConfigProvider()
	provider := NewCachedConfigProvider(mock, 5*time.Minute)

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws-123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws-123",
			Version:     "v1",
		},
	}

	var wg sync.WaitGroup
	numGoroutines := 50
	numCalls := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numCalls; j++ {
				_, err := provider.GetEffectiveConfig(context.Background(), authCtx)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		}()
	}

	wg.Wait()

	// Should only have 1 underlying call due to caching
	if mock.getCallCount() != 1 {
		t.Errorf("expected 1 underlying call, got %d", mock.getCallCount())
	}

	stats := provider.GetStats()
	expectedHits := uint64(numGoroutines*numCalls - 1)
	if stats.Hits != expectedHits {
		t.Errorf("expected %d hits, got %d", expectedHits, stats.Hits)
	}
}

func TestCachedConfigProvider_SetMaxSize_Eviction(t *testing.T) {
	mock := newMockConfigProvider()
	provider := NewCachedConfigProvider(mock, 5*time.Minute)
	provider.SetMaxSize(2)

	// Add 3 configs (exceeds max size of 2)
	for i := 1; i <= 3; i++ {
		authCtx := &auth.AuthContext{
			WorkspaceID: fmt.Sprintf("ws-%d", i),
			ConfigRef: auth.ConfigRef{
				WorkspaceID: fmt.Sprintf("ws-%d", i),
				Version:     "v1",
			},
		}
		_, _ = provider.GetEffectiveConfig(context.Background(), authCtx)
	}

	// First config should have been evicted
	authCtx1 := &auth.AuthContext{
		WorkspaceID: "ws-1",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws-1",
			Version:     "v1",
		},
	}
	_, _ = provider.GetEffectiveConfig(context.Background(), authCtx1)

	// Should have triggered a new fetch
	if mock.getCallCount() != 4 {
		t.Errorf("expected 4 underlying calls (3 initial + 1 after eviction), got %d", mock.getCallCount())
	}

	stats := provider.GetStats()
	// 2 evictions: first when ws-3 exceeded maxSize (evicted ws-1),
	// second when ws-1 was re-inserted (evicted ws-2)
	if stats.Evictions != 2 {
		t.Errorf("expected 2 evictions, got %d", stats.Evictions)
	}
}

func TestCachedConfigProvider_Callbacks(t *testing.T) {
	mock := newMockConfigProvider()
	provider := NewCachedConfigProvider(mock, 5*time.Minute)

	var hitCalled, missCalled bool
	provider.SetOnCacheHit(func(key string) {
		hitCalled = true
	})
	provider.SetOnCacheMiss(func(key string) {
		missCalled = true
	})

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws-123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws-123",
			Version:     "v1",
		},
	}

	// First call - miss
	_, _ = provider.GetEffectiveConfig(context.Background(), authCtx)
	if !missCalled {
		t.Error("expected miss callback to be called")
	}

	// Second call - hit
	_, _ = provider.GetEffectiveConfig(context.Background(), authCtx)
	if !hitCalled {
		t.Error("expected hit callback to be called")
	}
}

func TestCachedConfigProvider_DoubleCheckPattern(t *testing.T) {
	mock := newMockConfigProvider()
	provider := NewCachedConfigProvider(mock, 5*time.Minute)

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws-123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws-123",
			Version:     "v1",
		},
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Two concurrent calls - only one should fetch
	go func() {
		defer wg.Done()
		_, _ = provider.GetEffectiveConfig(context.Background(), authCtx)
	}()
	go func() {
		defer wg.Done()
		_, _ = provider.GetEffectiveConfig(context.Background(), authCtx)
	}()

	wg.Wait()

	// Due to double-check pattern, should only be 1 underlying call
	if mock.getCallCount() != 1 {
		t.Errorf("expected 1 underlying call due to double-check, got %d", mock.getCallCount())
	}
}

func TestCacheKey(t *testing.T) {
	tests := []struct {
		workspaceID string
		version     string
		expected    string
	}{
		{"ws-123", "v1", "ws-123:v1"},
		{"my-workspace", "latest", "my-workspace:latest"},
		{"", "", ":"},
		{"ws:123", "v:1", "ws:123:v:1"},
	}

	for _, tt := range tests {
		result := CacheKey(tt.workspaceID, tt.version)
		if result != tt.expected {
			t.Errorf("CacheKey(%q, %q) = %q, expected %q", tt.workspaceID, tt.version, result, tt.expected)
		}
	}
}
