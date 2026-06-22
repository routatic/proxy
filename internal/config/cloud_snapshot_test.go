// Package config provides configuration management for the routatic-proxy.
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/auth"
)

// TestNewCloudSnapshotConfigProvider tests the constructor.
func TestNewCloudSnapshotConfigProvider(t *testing.T) {
	provider := NewCloudSnapshotConfigProvider("https://api.example.com/snapshots", 5*time.Minute, "my-token")

	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
	if provider.snapshotURL != "https://api.example.com/snapshots" {
		t.Errorf("expected URL 'https://api.example.com/snapshots', got %q", provider.snapshotURL)
	}
	if provider.ttl != 5*time.Minute {
		t.Errorf("expected ttl 5m, got %v", provider.ttl)
	}
	if provider.serviceToken != "my-token" {
		t.Errorf("expected service token 'my-token', got %q", provider.serviceToken)
	}
	if provider.snapshots == nil {
		t.Error("expected non-nil snapshots map")
	}
	if provider.httpClient == nil {
		t.Error("expected non-nil http client")
	}
	if provider.SnapshotCount() != 0 {
		t.Errorf("expected 0 snapshots initially, got %d", provider.SnapshotCount())
	}
}

// TestCloudSnapshotConfigProvider_GetEffectiveConfig_Success tests basic successful fetch and caching.
func TestCloudSnapshotConfigProvider_GetEffectiveConfig_Success(t *testing.T) {
	callCount := 0
	expectedWorkspace := "ws_123"
	expectedVersion := "v1.2.3"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		// Verify request
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		q := r.URL.Query()
		if q.Get("workspace") != expectedWorkspace {
			t.Errorf("expected workspace %q, got %q", expectedWorkspace, q.Get("workspace"))
		}
		if q.Get("version") != expectedVersion {
			t.Errorf("expected version %q, got %q", expectedVersion, q.Get("version"))
		}

		// Return simple RuntimeConfig
		config := &RuntimeConfig{
			WorkspaceID: expectedWorkspace,
			Version:     expectedVersion,
			Supermodels: map[string]Supermodel{
				"default": {Name: "default"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "")

	authCtx := &auth.AuthContext{
		WorkspaceID: expectedWorkspace,
		ConfigRef: auth.ConfigRef{
			WorkspaceID: expectedWorkspace,
			Version:     expectedVersion,
		},
	}

	// First call - should fetch from server
	config1, err := provider.GetEffectiveConfig(context.Background(), authCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config1.WorkspaceID != expectedWorkspace {
		t.Errorf("expected workspace %q, got %q", expectedWorkspace, config1.WorkspaceID)
	}
	if config1.Version != expectedVersion {
		t.Errorf("expected version %q, got %q", expectedVersion, config1.Version)
	}

	// Second call - should be cached
	config2, err := provider.GetEffectiveConfig(context.Background(), authCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config2 != config1 {
		t.Error("expected cached config to be same pointer")
	}

	if callCount != 1 {
		t.Errorf("expected 1 server call, got %d", callCount)
	}
	if provider.SnapshotCount() != 1 {
		t.Errorf("expected 1 cached snapshot, got %d", provider.SnapshotCount())
	}
}

// TestCloudSnapshotConfigProvider_GetEffectiveConfig_WrappedResponse tests wrapped response format.
func TestCloudSnapshotConfigProvider_GetEffectiveConfig_WrappedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return wrapped response
		response := map[string]interface{}{
			"version":      "v1.2.3",
			"workspace_id": "ws_456",
			"config": map[string]interface{}{
				"workspace_id": "ws_456",
				"version":      "v1.2.3",
				"supermodels": map[string]interface{}{
					"default": map[string]interface{}{
						"name": "wrapped-model",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "")

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws_456",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_456",
			Version:     "v1.2.3",
		},
	}

	config, err := provider.GetEffectiveConfig(context.Background(), authCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.WorkspaceID != "ws_456" {
		t.Errorf("expected workspace 'ws_456', got %q", config.WorkspaceID)
	}
	if config.Version != "v1.2.3" {
		t.Errorf("expected version 'v1.2.3', got %q", config.Version)
	}
	if config.Supermodels["default"].Name != "wrapped-model" {
		t.Errorf("unexpected supermodel name")
	}
}

// TestCloudSnapshotConfigProvider_GetEffectiveConfig_WithAuthHeader tests Bearer token authentication.
func TestCloudSnapshotConfigProvider_GetEffectiveConfig_WithAuthHeader(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")

		config := &RuntimeConfig{
			WorkspaceID: "ws_123",
			Version:     "v1",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "secret-token")

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws_123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_123",
			Version:     "v1",
		},
	}

	_, err := provider.GetEffectiveConfig(context.Background(), authCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedAuth != "Bearer secret-token" {
		t.Errorf("expected 'Bearer secret-token', got %q", receivedAuth)
	}
}

// TestCloudSnapshotConfigProvider_GetEffectiveConfig_NilAuth tests nil auth context error.
func TestCloudSnapshotConfigProvider_GetEffectiveConfig_NilAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "")

	_, err := provider.GetEffectiveConfig(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil auth context")
	}
}

// TestCloudSnapshotConfigProvider_GetEffectiveConfig_ServerError tests error handling.
func TestCloudSnapshotConfigProvider_GetEffectiveConfig_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, "internal server error")
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "")

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws_123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_123",
			Version:     "v1",
		},
	}

	_, err := provider.GetEffectiveConfig(context.Background(), authCtx)
	if err == nil {
		t.Fatal("expected error for server error response")
	}
	if err.Error() != "snapshot API returned status 500: internal server error" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestCloudSnapshotConfigProvider_GetEffectiveConfig_NotFound tests 404 handling.
func TestCloudSnapshotConfigProvider_GetEffectiveConfig_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, "configuration not found")
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "")

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws_unknown",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_unknown",
			Version:     "v99",
		},
	}

	_, err := provider.GetEffectiveConfig(context.Background(), authCtx)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if err.Error() != "snapshot API returned status 404: configuration not found" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestCloudSnapshotConfigProvider_GetEffectiveConfig_InvalidJSON tests invalid JSON handling.
func TestCloudSnapshotConfigProvider_GetEffectiveConfig_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "not valid json {{ ")
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "")

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws_123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_123",
			Version:     "v1",
		},
	}

	_, err := provider.GetEffectiveConfig(context.Background(), authCtx)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// TestCloudSnapshotConfigProvider_GetConfigByRef tests GetConfigByRef functionality.
func TestCloudSnapshotConfigProvider_GetConfigByRef(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		config := &RuntimeConfig{
			WorkspaceID: r.URL.Query().Get("workspace"),
			Version:     r.URL.Query().Get("version"),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "")

	ref := auth.ConfigRef{
		WorkspaceID: "ws_789",
		Version:     "v2.0",
	}

	// First call - cache miss
	config1, err := provider.GetConfigByRef(context.Background(), ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config1.WorkspaceID != "ws_789" {
		t.Errorf("expected workspace 'ws_789', got %q", config1.WorkspaceID)
	}

	// Second call - cache hit
	_, err = provider.GetConfigByRef(context.Background(), ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 server call, got %d", callCount)
	}
}

// TestCloudSnapshotConfigProvider_TTLExpiration tests TTL-based expiration.
func TestCloudSnapshotConfigProvider_TTLExpiration(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		config := &RuntimeConfig{
			WorkspaceID: "ws_123",
			Version:     "v1",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 50*time.Millisecond, "")

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws_123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_123",
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

	if callCount != 2 {
		t.Errorf("expected 2 calls after TTL expiry, got %d", callCount)
	}
}

// TestCloudSnapshotConfigProvider_InfiniteTTL tests that TTL=0 means no expiration.
func TestCloudSnapshotConfigProvider_InfiniteTTL(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		config := &RuntimeConfig{
			WorkspaceID: "ws_123",
			Version:     "v1",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 0, "") // Infinite TTL

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws_123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_123",
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

	// Should still be in cache
	_, err = provider.GetEffectiveConfig(context.Background(), authCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 call with infinite TTL, got %d", callCount)
	}
}

// TestCloudSnapshotConfigProvider_Invalidate_SpecificVersion tests invalidating specific version.
func TestCloudSnapshotConfigProvider_Invalidate_SpecificVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		config := &RuntimeConfig{
			WorkspaceID: r.URL.Query().Get("workspace"),
			Version:     r.URL.Query().Get("version"),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "")

	// Populate cache with multiple versions
	v1 := &auth.AuthContext{
		WorkspaceID: "ws_123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_123",
			Version:     "v1",
		},
	}
	v2 := &auth.AuthContext{
		WorkspaceID: "ws_123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_123",
			Version:     "v2",
		},
	}

	_, _ = provider.GetEffectiveConfig(context.Background(), v1)
	_, _ = provider.GetEffectiveConfig(context.Background(), v2)

	if provider.SnapshotCount() != 2 {
		t.Fatalf("expected 2 cached snapshots, got %d", provider.SnapshotCount())
	}

	// Invalidate v1
	err := provider.Invalidate(context.Background(), "ws_123", "v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if provider.SnapshotCount() != 1 {
		t.Errorf("expected 1 cached snapshot, got %d", provider.SnapshotCount())
	}

	// v2 should still be in cache
	_, err = provider.GetEffectiveConfig(context.Background(), v2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestCloudSnapshotConfigProvider_Invalidate_AllVersions tests invalidating all versions of a workspace.
func TestCloudSnapshotConfigProvider_Invalidate_AllVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		config := &RuntimeConfig{
			WorkspaceID: r.URL.Query().Get("workspace"),
			Version:     r.URL.Query().Get("version"),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "")

	// Populate cache with multiple workspaces and versions
	ws1v1 := &auth.AuthContext{
		WorkspaceID: "ws_1",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_1",
			Version:     "v1",
		},
	}
	ws1v2 := &auth.AuthContext{
		WorkspaceID: "ws_1",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_1",
			Version:     "v2",
		},
	}
	ws2v1 := &auth.AuthContext{
		WorkspaceID: "ws_2",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_2",
			Version:     "v1",
		},
	}

	_, _ = provider.GetEffectiveConfig(context.Background(), ws1v1)
	_, _ = provider.GetEffectiveConfig(context.Background(), ws1v2)
	_, _ = provider.GetEffectiveConfig(context.Background(), ws2v1)

	if provider.SnapshotCount() != 3 {
		t.Fatalf("expected 3 cached snapshots, got %d", provider.SnapshotCount())
	}

	// Invalidate all versions of ws_1
	err := provider.Invalidate(context.Background(), "ws_1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if provider.SnapshotCount() != 1 {
		t.Errorf("expected 1 cached snapshot after invalidation, got %d", provider.SnapshotCount())
	}
}

// TestCloudSnapshotConfigProvider_HealthCheck_Success tests successful health check.
func TestCloudSnapshotConfigProvider_HealthCheck_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "healthy")
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "")

	err := provider.HealthCheck(context.Background())
	if err != nil {
		t.Errorf("expected no error for healthy server, got %v", err)
	}
}

// TestCloudSnapshotConfigProvider_HealthCheck_Error tests health check with error response.
func TestCloudSnapshotConfigProvider_HealthCheck_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, "unhealthy")
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "")

	err := provider.HealthCheck(context.Background())
	if err == nil {
		t.Error("expected error for unhealthy server")
	}
}

// TestCloudSnapshotConfigProvider_HealthCheck_3xx tests health check with 3xx redirect.
func TestCloudSnapshotConfigProvider_HealthCheck_3xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPermanentRedirect)
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "")

	// 3xx should be treated as unhealthy
	err := provider.HealthCheck(context.Background())
	if err == nil {
		t.Error("expected error for 3xx response")
	}
}

// TestCloudSnapshotConfigProvider_ConcurrentAccess tests thread safety.
func TestCloudSnapshotConfigProvider_ConcurrentAccess(t *testing.T) {
	callCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()

		// Add small delay to increase chance of race conditions
		time.Sleep(10 * time.Millisecond)

		config := &RuntimeConfig{
			WorkspaceID: r.URL.Query().Get("workspace"),
			Version:     r.URL.Query().Get("version"),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "")

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws_123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_123",
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

	// Due to double-check pattern, should only have 1 underlying call
	mu.Lock()
	if callCount != 1 {
		t.Errorf("expected 1 server call, got %d", callCount)
	}
	mu.Unlock()

	if provider.SnapshotCount() != 1 {
		t.Errorf("expected 1 cached snapshot, got %d", provider.SnapshotCount())
	}
}

// TestCloudSnapshotConfigProvider_DifferentWorkspaces tests isolation between workspaces.
func TestCloudSnapshotConfigProvider_DifferentWorkspaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workspace := r.URL.Query().Get("workspace")
		version := r.URL.Query().Get("version")

		config := &RuntimeConfig{
			WorkspaceID: workspace,
			Version:     version,
			Supermodels: map[string]Supermodel{
				"default": {Name: "model-for-" + workspace},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "")

	ws1 := &auth.AuthContext{
		WorkspaceID: "ws_alpha",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_alpha",
			Version:     "v1",
		},
	}
	ws2 := &auth.AuthContext{
		WorkspaceID: "ws_beta",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_beta",
			Version:     "v1",
		},
	}

	config1, err := provider.GetEffectiveConfig(context.Background(), ws1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config1.Supermodels["default"].Name != "model-for-ws_alpha" {
		t.Errorf("unexpected model name: %s", config1.Supermodels["default"].Name)
	}

	config2, err := provider.GetEffectiveConfig(context.Background(), ws2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config2.Supermodels["default"].Name != "model-for-ws_beta" {
		t.Errorf("unexpected model name: %s", config2.Supermodels["default"].Name)
	}

	// Verify both are cached
	if provider.SnapshotCount() != 2 {
		t.Errorf("expected 2 cached snapshots, got %d", provider.SnapshotCount())
	}
}

// TestCloudSnapshotConfigProvider_ContextCancellation tests handling of cancelled context.
func TestCloudSnapshotConfigProvider_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws_123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_123",
			Version:     "v1",
		},
	}

	_, err := provider.GetEffectiveConfig(ctx, authCtx)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

// TestCloudSnapshotConfigProvider_cacheKey tests the cache key generation.
func TestCloudSnapshotConfigProvider_cacheKey(t *testing.T) {
	provider := NewCloudSnapshotConfigProvider("http://example.com", 5*time.Minute, "")

	tests := []struct {
		workspaceID string
		version     string
		expected    string
	}{
		{"ws_123", "v1", "ws_123:v1"},
		{"my-workspace", "latest", "my-workspace:latest"},
		{"", "", ":"},
		{"ws:123", "v:1", "ws:123:v:1"},
		{"ws-test_123.456", "v1.2.3", "ws-test_123.456:v1.2.3"},
	}

	for _, tt := range tests {
		result := provider.cacheKey(tt.workspaceID, tt.version)
		if result != tt.expected {
			t.Errorf("cacheKey(%q, %q) = %q, expected %q",
				tt.workspaceID, tt.version, result, tt.expected)
		}
	}
}

// TestCloudSnapshotConfigProvider_isExpired tests the expiration check.
func TestCloudSnapshotConfigProvider_isExpired(t *testing.T) {
	// With TTL
	provider := NewCloudSnapshotConfigProvider("http://example.com", 100*time.Millisecond, "")

	fresh := cachedSnapshot{
		config:    &RuntimeConfig{WorkspaceID: "ws_123"},
		fetchedAt: time.Now(),
	}
	if provider.isExpired(fresh) {
		t.Error("fresh entry should not be expired")
	}

	// Simulate expired entry by manipulating fetchedAt
	expired := cachedSnapshot{
		config:    &RuntimeConfig{WorkspaceID: "ws_123"},
		fetchedAt: time.Now().Add(-200 * time.Millisecond),
	}
	if !provider.isExpired(expired) {
		t.Error("old entry should be expired")
	}

	// With infinite TTL
	providerInf := NewCloudSnapshotConfigProvider("http://example.com", 0, "")
	if providerInf.isExpired(expired) {
		t.Error("entry with infinite TTL should not expire")
	}
}

// TestCloudSnapshotConfigProvider_WrappedConfigPopulatesFields tests that wrapped
// response populates missing fields from wrapper.
func TestCloudSnapshotConfigProvider_WrappedConfigPopulatesFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return config without version/workspace_id in inner config
		response := map[string]interface{}{
			"version":      "v2.0.0",
			"workspace_id": "ws_special",
			"config": map[string]interface{}{
				// Missing workspace_id and version in inner config
				"supermodels": map[string]interface{}{
					"default": map[string]interface{}{
						"name": "model",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "")

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws_special",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_special",
			Version:     "v2.0.0",
		},
	}

	config, err := provider.GetEffectiveConfig(context.Background(), authCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.WorkspaceID != "ws_special" {
		t.Errorf("expected workspace 'ws_special', got %q", config.WorkspaceID)
	}
	if config.Version != "v2.0.0" {
		t.Errorf("expected version 'v2.0.0', got %q", config.Version)
	}
}

// TestCloudSnapshotConfigProvider_SetHTTPClient tests custom HTTP client.
func TestCloudSnapshotConfigProvider_SetHTTPClient(t *testing.T) {
	customClient := &http.Client{Timeout: 60 * time.Second}

	provider := NewCloudSnapshotConfigProvider("http://example.com", 5*time.Minute, "")
	provider.SetHTTPClient(customClient)

	if provider.httpClient != customClient {
		t.Error("expected custom HTTP client to be set")
	}
}

// TestCloudSnapshotConfigProvider_NilResponseBody tests error when response is truncated.
func TestCloudSnapshotConfigProvider_NetworkError(t *testing.T) {
	// Create a server that immediately closes connection
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Skip("hijacking not supported")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "")

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws_123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_123",
			Version:     "v1",
		},
	}

	_, err := provider.GetEffectiveConfig(context.Background(), authCtx)
	if err == nil {
		t.Error("expected error for network failure")
	}
}

// TestCloudSnapshotConfigProvider_DoubleCheckPattern tests the double-check pattern.
func TestCloudSnapshotConfigProvider_DoubleCheckPattern(t *testing.T) {
	callCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()

		time.Sleep(50 * time.Millisecond) // Simulate slow fetch

		config := &RuntimeConfig{
			WorkspaceID: "ws_123",
			Version:     "v1",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(config)
	}))
	defer server.Close()

	provider := NewCloudSnapshotConfigProvider(server.URL, 5*time.Minute, "")

	authCtx := &auth.AuthContext{
		WorkspaceID: "ws_123",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "ws_123",
			Version:     "v1",
		},
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Two concurrent calls
	go func() {
		defer wg.Done()
		_, _ = provider.GetEffectiveConfig(context.Background(), authCtx)
	}()
	go func() {
		defer wg.Done()
		_, _ = provider.GetEffectiveConfig(context.Background(), authCtx)
	}()

	wg.Wait()

	// Due to double-check, should only be 1 server call
	mu.Lock()
	if callCount != 1 {
		t.Errorf("expected 1 server call due to double-check, got %d", callCount)
	}
	mu.Unlock()
}
