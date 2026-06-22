package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestNewCloudAuthProvider tests the constructor
func TestNewCloudAuthProvider(t *testing.T) {
	provider := NewCloudAuthProvider("https://api.example.com/introspect", 5*time.Minute, "service-token")

	if provider.introspectionURL != "https://api.example.com/introspect" {
		t.Errorf("expected introspection URL %q, got %q",
			"https://api.example.com/introspect", provider.introspectionURL)
	}

	if provider.cacheTTL != 5*time.Minute {
		t.Errorf("expected cache TTL %v, got %v", 5*time.Minute, provider.cacheTTL)
	}

	if provider.serviceToken != "service-token" {
		t.Errorf("expected service token %q, got %q", "service-token", provider.serviceToken)
	}

	if provider.cache == nil {
		t.Error("cache should be initialized")
	}

	if provider.httpClient == nil {
		t.Error("HTTP client should be initialized")
	}

	if provider.httpClient.Timeout != 10*time.Second {
		t.Errorf("expected HTTP timeout %v, got %v", 10*time.Second, provider.httpClient.Timeout)
	}
}

// TestNewCloudAuthProvider_DefaultTTL tests that default TTL is applied
func TestNewCloudAuthProvider_DefaultTTL(t *testing.T) {
	// Zero TTL should default to 5 minutes
	provider := NewCloudAuthProvider("https://api.example.com/introspect", 0, "")

	if provider.cacheTTL != 5*time.Minute {
		t.Errorf("expected default TTL %v, got %v", 5*time.Minute, provider.cacheTTL)
	}
}

// TestCloudAuthProvider_Authenticate_Success tests successful authentication
func TestCloudAuthProvider_Authenticate_Success(t *testing.T) {
	// Create a mock introspection server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Verify content type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Parse request body
		var req IntrospectionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Return successful introspection response
		response := IntrospectionResponse{
			Active:           true,
			KeyID:            "key_123",
			WorkspaceID:      "ws_456",
			SubjectID:        "user_789",
			SubjectType:      "user",
			Roles:            []string{"admin"},
			AllowedModels:    []string{"gpt-4", "claude-3"},
			AllowedProviders: []string{"openai"},
			RateLimits: RateLimitPolicy{
				RequestsPerMinute: 60,
				TokensPerMinute:   10000,
			},
			Billing: BillingPolicy{
				Plan:             "pro",
				CreditsRemaining: 50000,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewCloudAuthProvider(server.URL, 5*time.Minute, "service-token")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	authCtx, err := provider.Authenticate(ctx, req)
	if err != nil {
		t.Fatalf("authentication failed: %v", err)
	}

	// Verify AuthContext fields
	if authCtx.KeyID != "key_123" {
		t.Errorf("expected KeyID %q, got %q", "key_123", authCtx.KeyID)
	}

	if authCtx.WorkspaceID != "ws_456" {
		t.Errorf("expected WorkspaceID %q, got %q", "ws_456", authCtx.WorkspaceID)
	}

	if authCtx.Identity.Type != SubjectTypeUser {
		t.Errorf("expected SubjectType %q, got %q", SubjectTypeUser, authCtx.Identity.Type)
	}

	if authCtx.Identity.ID != "user_789" {
		t.Errorf("expected Identity.ID %q, got %q", "user_789", authCtx.Identity.ID)
	}

	if authCtx.KeyStatus != KeyStatusActive {
		t.Errorf("expected KeyStatus %q, got %q", KeyStatusActive, authCtx.KeyStatus)
	}

	if !authCtx.HasRole("admin") {
		t.Errorf("expected role 'admin', got %v", authCtx.Roles)
	}

	if !authCtx.IsAllowedModel("gpt-4") {
		t.Errorf("expected model 'gpt-4' to be allowed")
	}

	if !authCtx.IsAllowedProvider("openai") {
		t.Errorf("expected provider 'openai' to be allowed")
	}

	if authCtx.RateLimits.RequestsPerMinute != 60 {
		t.Errorf("expected 60 requests per minute, got %d", authCtx.RateLimits.RequestsPerMinute)
	}

	if authCtx.Billing.Plan != "pro" {
		t.Errorf("expected plan 'pro', got %q", authCtx.Billing.Plan)
	}

	if authCtx.Billing.CreditsRemaining != 50000 {
		t.Errorf("expected 50000 credits, got %d", authCtx.Billing.CreditsRemaining)
	}
}

// TestCloudAuthProvider_Authenticate_MissingHeader tests missing Authorization header
func TestCloudAuthProvider_Authenticate_MissingHeader(t *testing.T) {
	provider := NewCloudAuthProvider("http://localhost", 5*time.Minute, "")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// No Authorization header

	ctx := context.Background()
	_, err := provider.Authenticate(ctx, req)

	if err != ErrAuthenticationFailed {
		t.Errorf("expected ErrAuthenticationFailed, got %v", err)
	}
}

// TestCloudAuthProvider_Authenticate_InvalidHeaderFormat tests invalid Authorization header format
func TestCloudAuthProvider_Authenticate_InvalidHeaderFormat(t *testing.T) {
	provider := NewCloudAuthProvider("http://localhost", 5*time.Minute, "")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Basic c29tZTp0b2tlbg==") // Basic auth instead of Bearer

	ctx := context.Background()
	_, err := provider.Authenticate(ctx, req)

	if err != ErrAuthenticationFailed {
		t.Errorf("expected ErrAuthenticationFailed, got %v", err)
	}
}

// TestCloudAuthProvider_Authenticate_EmptyToken tests empty bearer token
func TestCloudAuthProvider_Authenticate_EmptyToken(t *testing.T) {
	provider := NewCloudAuthProvider("http://localhost", 5*time.Minute, "")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer   ") // Empty with whitespace

	ctx := context.Background()
	_, err := provider.Authenticate(ctx, req)

	if err != ErrAuthenticationFailed {
		t.Errorf("expected ErrAuthenticationFailed, got %v", err)
	}
}

// TestCloudAuthProvider_Authenticate_InactiveToken tests authentication with inactive token
func TestCloudAuthProvider_Authenticate_InactiveToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := IntrospectionResponse{
			Active:      false,
			KeyID:       "key_inactive",
			WorkspaceID: "ws_123",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewCloudAuthProvider(server.URL, 5*time.Minute, "")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer inactive-token")

	ctx := context.Background()
	_, err := provider.Authenticate(ctx, req)

	if err != ErrAuthenticationFailed {
		t.Errorf("expected ErrAuthenticationFailed for inactive token, got %v", err)
	}
}

// TestCloudAuthProvider_Authenticate_ServerError tests fail-closed behavior on server error
func TestCloudAuthProvider_Authenticate_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	provider := NewCloudAuthProvider(server.URL, 5*time.Minute, "")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer some-token")

	ctx := context.Background()
	_, err := provider.Authenticate(ctx, req)

	if err != ErrAuthenticationFailed {
		t.Errorf("expected ErrAuthenticationFailed on server error (fail closed), got %v", err)
	}
}

// TestCloudAuthProvider_Authenticate_Timeout tests fail-closed behavior on timeout
func TestCloudAuthProvider_Authenticate_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create provider with very short client timeout
	provider := NewCloudAuthProvider(server.URL, 5*time.Minute, "")
	provider.httpClient.Timeout = 10 * time.Millisecond

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer some-token")

	ctx := context.Background()
	_, err := provider.Authenticate(ctx, req)

	if err != ErrAuthenticationFailed {
		t.Errorf("expected ErrAuthenticationFailed on timeout (fail closed), got %v", err)
	}
}

// TestCloudAuthProvider_Authenticate_NetworkError tests fail-closed behavior on network error
func TestCloudAuthProvider_Authenticate_NetworkError(t *testing.T) {
	// Create a server that will be closed immediately
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.Close() // Close immediately so requests will fail

	provider := NewCloudAuthProvider(server.URL, 5*time.Minute, "")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer some-token")

	ctx := context.Background()
	_, err := provider.Authenticate(ctx, req)

	if err != ErrAuthenticationFailed {
		t.Errorf("expected ErrAuthenticationFailed on network error (fail closed), got %v", err)
	}
}

// TestCloudAuthProvider_Authenticate_CacheHit tests caching behavior
func TestCloudAuthProvider_Authenticate_CacheHit(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		response := IntrospectionResponse{
			Active:      true,
			KeyID:       "key_cached",
			WorkspaceID: "ws_cached",
			SubjectType: "service",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewCloudAuthProvider(server.URL, 5*time.Minute, "")

	// First call - should hit the server
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.Header.Set("Authorization", "Bearer cache-test-token")

	ctx := context.Background()
	authCtx1, err := provider.Authenticate(ctx, req1)
	if err != nil {
		t.Fatalf("first authentication failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 server call after first auth, got %d", callCount)
	}

	// Second call with same token - should hit cache
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.Header.Set("Authorization", "Bearer cache-test-token")

	authCtx2, err := provider.Authenticate(ctx, req2)
	if err != nil {
		t.Fatalf("second authentication failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 server call after cache hit, got %d", callCount)
	}

	// AuthContext should be the same
	if authCtx1.KeyID != authCtx2.KeyID {
		t.Errorf("expected same KeyID from cache")
	}
}

// TestCloudAuthProvider_Authenticate_CacheExpiration tests cache TTL expiration
func TestCloudAuthProvider_Authenticate_CacheExpiration(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		response := IntrospectionResponse{
			Active:      true,
			KeyID:       "key_expiring",
			WorkspaceID: "ws_expiring",
			SubjectType: "service",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Use a very short TTL for testing
	provider := NewCloudAuthProvider(server.URL, 100*time.Millisecond, "")

	token := "Bearer expiring-token"

	// First call
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.Header.Set("Authorization", token)

	ctx := context.Background()
	_, err := provider.Authenticate(ctx, req1)
	if err != nil {
		t.Fatalf("first authentication failed: %v", err)
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Second call - cache should have expired
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.Header.Set("Authorization", token)

	_, err = provider.Authenticate(ctx, req2)
	if err != nil {
		t.Fatalf("second authentication failed: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 server calls after cache expiration, got %d", callCount)
	}
}

// TestCloudAuthProvider_RevokeCache tests cache revocation
func TestCloudAuthProvider_RevokeCache(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		response := IntrospectionResponse{
			Active:      true,
			KeyID:       "key_revoke_test",
			WorkspaceID: "ws_1",
			SubjectType: "service",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewCloudAuthProvider(server.URL, 5*time.Minute, "")

	// Authenticate to populate cache
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer revoke-token")

	ctx := context.Background()
	_, err := provider.Authenticate(ctx, req)
	if err != nil {
		t.Fatalf("authentication failed: %v", err)
	}

	// Should have 1 call
	if callCount != 1 {
		t.Errorf("expected 1 call after auth, got %d", callCount)
	}

	// Revoke cache for the key
	err = provider.RevokeCache(ctx, "key_revoke_test")
	if err != nil {
		t.Fatalf("revoke cache failed: %v", err)
	}

	// Authenticate again - should trigger new introspection
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.Header.Set("Authorization", "Bearer revoke-token")

	_, err = provider.Authenticate(ctx, req2)
	if err != nil {
		t.Fatalf("second authentication failed: %v", err)
	}

	// Should have 2 calls now
	if callCount != 2 {
		t.Errorf("expected 2 calls after revoke and re-auth, got %d", callCount)
	}
}

// TestCloudAuthProvider_RevokeCache_NonExistent tests revoking non-existent key
func TestCloudAuthProvider_RevokeCache_NonExistent(t *testing.T) {
	provider := NewCloudAuthProvider("http://localhost", 5*time.Minute, "")

	ctx := context.Background()
	err := provider.RevokeCache(ctx, "non-existent-key")
	if err != nil {
		t.Errorf("revoking non-existent key should not error, got %v", err)
	}
}

// TestCloudAuthProvider_HealthCheck tests health check
func TestCloudAuthProvider_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 401, which is OK for health check - endpoint is reachable
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	provider := NewCloudAuthProvider(server.URL, 5*time.Minute, "")

	ctx := context.Background()
	err := provider.HealthCheck(ctx)
	if err != nil {
		t.Errorf("health check should pass for reachable endpoint, got %v", err)
	}
}

// TestCloudAuthProvider_HealthCheck_Unreachable tests health check with unreachable endpoint
func TestCloudAuthProvider_HealthCheck_Unreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.Close() // Close immediately

	provider := NewCloudAuthProvider(server.URL, 5*time.Minute, "")

	ctx := context.Background()
	err := provider.HealthCheck(ctx)
	if err == nil {
		t.Error("health check should fail for unreachable endpoint")
	}
}

// TestCloudAuthProvider_ClearCache tests clearing all cache entries
func TestCloudAuthProvider_ClearCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := IntrospectionResponse{
			Active:      true,
			KeyID:       "key_clear",
			WorkspaceID: "ws_1",
			SubjectType: "service",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewCloudAuthProvider(server.URL, 5*time.Minute, "")

	// Authenticate multiple tokens
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer token-"+string(rune('0'+i)))
		_, _ = provider.Authenticate(ctx, req)
	}

	total, _ := provider.CacheStats()
	if total != 5 {
		t.Errorf("expected 5 cache entries, got %d", total)
	}

	// Clear cache
	provider.ClearCache()

	total, _ = provider.CacheStats()
	if total != 0 {
		t.Errorf("expected 0 cache entries after clear, got %d", total)
	}
}

// TestCloudAuthProvider_CacheStats tests cache statistics
func TestCloudAuthProvider_CacheStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := IntrospectionResponse{
			Active:      true,
			KeyID:       "key_stats",
			WorkspaceID: "ws_1",
			SubjectType: "service",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Very short TTL for testing expiration
	provider := NewCloudAuthProvider(server.URL, 50*time.Millisecond, "")

	// Add one entry
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer stats-token")
	ctx := context.Background()
	_, _ = provider.Authenticate(ctx, req)

	total, expired := provider.CacheStats()
	if total != 1 {
		t.Errorf("expected 1 total entry, got %d", total)
	}
	if expired != 0 {
		t.Errorf("expected 0 expired entries, got %d", expired)
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	total, expired = provider.CacheStats()
	if total != 1 {
		t.Errorf("expected 1 total entry (not removed), got %d", total)
	}
	if expired != 1 {
		t.Errorf("expected 1 expired entry, got %d", expired)
	}
}

// TestCloudAuthProvider_ConcurrentAccess tests thread safety
func TestCloudAuthProvider_ConcurrentAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := IntrospectionResponse{
			Active:      true,
			KeyID:       "key_concurrent",
			WorkspaceID: "ws_1",
			SubjectType: "service",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewCloudAuthProvider(server.URL, 5*time.Minute, "")

	ctx := context.Background()
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Concurrent authenticate from different goroutines
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Authorization", "Bearer concurrent-token-"+string(rune('A'+idx%10)))
			_, err := provider.Authenticate(ctx, req)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	// Concurrent revokes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = provider.RevokeCache(ctx, "key_concurrent")
		}()
	}

	// Cache operations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			provider.CacheStats()
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent operation failed: %v", err)
	}
}

// TestCloudAuthProvider_SubjectTypes tests different subject types
func TestCloudAuthProvider_SubjectTypes(t *testing.T) {
	tests := []struct {
		subjectType  string
		expectedType SubjectType
	}{
		{"user", SubjectTypeUser},
		{"workspace", SubjectTypeWorkspace},
		{"service", SubjectTypeService},
		{"unknown", SubjectTypeService}, // Default to service for unknown
	}

	for _, tt := range tests {
		t.Run(tt.subjectType, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				response := IntrospectionResponse{
					Active:      true,
					KeyID:       "key_" + tt.subjectType,
					WorkspaceID: "ws_1",
					SubjectType: tt.subjectType,
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			provider := NewCloudAuthProvider(server.URL, 5*time.Minute, "")

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Authorization", "Bearer token-"+tt.subjectType)

			ctx := context.Background()
			authCtx, err := provider.Authenticate(ctx, req)
			if err != nil {
				t.Fatalf("authentication failed: %v", err)
			}

			if authCtx.Identity.Type != tt.expectedType {
				t.Errorf("expected SubjectType %q, got %q", tt.expectedType, authCtx.Identity.Type)
			}
		})
	}
}

// TestCloudAuthProvider_ServiceToken tests that service token is sent correctly
func TestCloudAuthProvider_ServiceToken(t *testing.T) {
	receivedAuth := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")

		response := IntrospectionResponse{
			Active:      true,
			KeyID:       "key_service",
			WorkspaceID: "ws_1",
			SubjectType: "service",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewCloudAuthProvider(server.URL, 5*time.Minute, "my-service-token")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer user-token")

	ctx := context.Background()
	_, err := provider.Authenticate(ctx, req)
	if err != nil {
		t.Fatalf("authentication failed: %v", err)
	}

	if !strings.HasPrefix(receivedAuth, "Bearer ") {
		t.Errorf("expected Authorization header to start with 'Bearer ', got %q", receivedAuth)
	}

	token := strings.TrimPrefix(receivedAuth, "Bearer ")
	if token != "my-service-token" {
		t.Errorf("expected service token %q, got %q", "my-service-token", token)
	}
}

// TestCloudAuthProvider_NewCloudAuthProviderWithClient tests custom HTTP client
func TestCloudAuthProvider_NewCloudAuthProviderWithClient(t *testing.T) {
	customClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	provider := NewCloudAuthProviderWithClient(
		"https://api.example.com/introspect",
		5*time.Minute,
		"service-token",
		customClient,
	)

	if provider.httpClient != customClient {
		t.Error("expected HTTP client to be the custom client")
	}

	if provider.httpClient.Timeout != 30*time.Second {
		t.Errorf("expected timeout %v, got %v", 30*time.Second, provider.httpClient.Timeout)
	}
}

// TestCloudAuthProvider_BackgroundRefresh tests background refresh behavior
func TestCloudAuthProvider_BackgroundRefresh(t *testing.T) {
	var callCountMu sync.Mutex
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCountMu.Lock()
		callCount++
		callCountMu.Unlock()
		response := IntrospectionResponse{
			Active:      true,
			KeyID:       "key_refresh",
			WorkspaceID: "ws_1",
			SubjectType: "service",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Short TTL with refresh threshold at 25%
	provider := NewCloudAuthProvider(server.URL, 400*time.Millisecond, "")
	provider.refreshThreshold = 100 * time.Millisecond // Trigger refresh after 300ms

	// First call
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.Header.Set("Authorization", "Bearer refresh-token")

	ctx := context.Background()
	_, err := provider.Authenticate(ctx, req1)
	if err != nil {
		t.Fatalf("first authentication failed: %v", err)
	}

	// Wait for entry to age past refresh threshold but not expire
	time.Sleep(350 * time.Millisecond)

	// Trigger potential refresh
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.Header.Set("Authorization", "Bearer refresh-token")
	_, err = provider.Authenticate(ctx, req2)
	if err != nil {
		t.Fatalf("second authentication failed: %v", err)
	}

	// Give background refresh time to complete
	time.Sleep(100 * time.Millisecond)

	// Should have 2 calls now (initial + background refresh)
	callCountMu.Lock()
	finalCount := callCount
	callCountMu.Unlock()
	if finalCount < 1 {
		t.Errorf("expected at least 1 call, got %d", finalCount)
	}
}

// TestCloudAuthProvider_BackgroundRefresh_InactiveToken tests that inactive tokens
// are removed from cache during background refresh
func TestCloudAuthProvider_BackgroundRefresh_InactiveToken(t *testing.T) {
	active := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := IntrospectionResponse{
			Active:      active,
			KeyID:       "key_inactive_refresh",
			WorkspaceID: "ws_1",
			SubjectType: "service",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewCloudAuthProvider(server.URL, 5*time.Minute, "")

	// First call - token is active
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.Header.Set("Authorization", "Bearer inactive-refresh-token")

	ctx := context.Background()
	_, err := provider.Authenticate(ctx, req1)
	if err != nil {
		t.Fatalf("first authentication failed: %v", err)
	}

	total, _ := provider.CacheStats()
	if total != 1 {
		t.Errorf("expected 1 cache entry, got %d", total)
	}

	// Simulate token becoming inactive
	active = false

	// Manually trigger refresh (simulate aging entry)
	provider.mu.RLock()
	for tokenHash, cached := range provider.cache {
		if cached.authContext.KeyID == "key_inactive_refresh" {
			// Force cached at time to be old
			provider.mu.RUnlock()
			provider.mu.Lock()
			updated := cached
			updated.cachedAt = time.Now().Add(-4 * time.Minute) // 4 min old
			provider.cache[tokenHash] = updated
			provider.mu.Unlock()
			break
		}
	}

	// Trigger refresh
	go provider.refreshCacheEntry("inactive-refresh-token", hashToken("inactive-refresh-token"))

	// Wait for background refresh
	time.Sleep(200 * time.Millisecond)

	// Cache should now be empty (token removed)
	total, _ = provider.CacheStats()
	if total != 0 {
		t.Errorf("expected 0 cache entries after inactive refresh, got %d", total)
	}
}

// TestHashToken_Specific tests the hashToken function with specific values
func TestHashToken_Specific(t *testing.T) {
	token := "my-secret-token"
	hash1 := hashToken(token)
	hash2 := hashToken(token)

	if hash1 != hash2 {
		t.Error("hashing same token should produce same hash")
	}

	// Different token should produce different hash
	hash3 := hashToken("different-token")
	if hash1 == hash3 {
		t.Error("different tokens should produce different hashes")
	}

	// Hash should be hex string
	if len(hash1) != 64 { // SHA256 produces 256 bits = 64 hex chars
		t.Errorf("expected 64 char hex hash, got %d chars", len(hash1))
	}
}

// TestCloudAuthProvider_Integration tests a complete integration scenario
func TestCloudAuthProvider_Integration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate introspection request
		var req IntrospectionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Return different responses based on token
		switch req.Token {
		case "valid-admin-token":
			_ = json.NewEncoder(w).Encode(IntrospectionResponse{
				Active:           true,
				KeyID:            "admin_key",
				WorkspaceID:      "workspace_123",
				SubjectID:        "user_admin",
				SubjectType:      "user",
				Roles:            []string{"admin", "user"},
				AllowedModels:    []string{}, // Empty = allow all
				AllowedProviders: []string{}, // Empty = allow all
				RateLimits: RateLimitPolicy{
					RequestsPerMinute: 1000,
					TokensPerMinute:   100000,
				},
				Billing: BillingPolicy{
					Plan:             "enterprise",
					CreditsRemaining: 100000,
				},
			})
		case "expired-token":
			_ = json.NewEncoder(w).Encode(IntrospectionResponse{
				Active:      false,
				KeyID:       "expired_key",
				WorkspaceID: "workspace_123",
			})
		case "limited-token":
			_ = json.NewEncoder(w).Encode(IntrospectionResponse{
				Active:           true,
				KeyID:            "limited_key",
				WorkspaceID:      "workspace_456",
				SubjectID:        "service_account",
				SubjectType:      "service",
				Roles:            []string{"service"},
				AllowedModels:    []string{"gpt-3.5-turbo"},
				AllowedProviders: []string{"openai"},
				RateLimits: RateLimitPolicy{
					RequestsPerMinute: 10,
				},
			})
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer server.Close()

	provider := NewCloudAuthProvider(server.URL, 5*time.Minute, "service-token")

	// Test 1: Valid admin token
	t.Run("AdminToken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer valid-admin-token")

		ctx := context.Background()
		authCtx, err := provider.Authenticate(ctx, req)
		if err != nil {
			t.Fatalf("admin token authentication failed: %v", err)
		}

		if !authCtx.HasRole("admin") {
			t.Error("admin token should have admin role")
		}

		// Empty allowed lists should mean allow all
		if !authCtx.IsAllowedModel("any-model") {
			t.Error("empty AllowedModels should allow all")
		}

		if authCtx.Billing.Plan != "enterprise" {
			t.Errorf("expected enterprise plan, got %q", authCtx.Billing.Plan)
		}
	})

	// Test 2: Expired token fails
	t.Run("ExpiredToken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer expired-token")

		ctx := context.Background()
		_, err := provider.Authenticate(ctx, req)
		if err != ErrAuthenticationFailed {
			t.Errorf("expected ErrAuthenticationFailed for expired token, got %v", err)
		}
	})

	// Test 3: Limited service token
	t.Run("LimitedToken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer limited-token")

		ctx := context.Background()
		authCtx, err := provider.Authenticate(ctx, req)
		if err != nil {
			t.Fatalf("limited token authentication failed: %v", err)
		}

		if authCtx.Identity.Type != SubjectTypeService {
			t.Errorf("expected service subject type, got %q", authCtx.Identity.Type)
		}

		if authCtx.IsAllowedModel("gpt-4") {
			t.Error("limited token should not allow gpt-4")
		}

		if !authCtx.IsAllowedModel("gpt-3.5-turbo") {
			t.Error("limited token should allow gpt-3.5-turbo")
		}

		if authCtx.IsAllowedProvider("anthropic") {
			t.Error("limited token should not allow anthropic provider")
		}
	})
}

// TestCloudAuthProvider_ErrorScenarios tests various error scenarios
func TestCloudAuthProvider_ErrorScenarios(t *testing.T) {
	tests := []struct {
		name          string
		serverHandler http.HandlerFunc
		expectedErr   error
		description   string
	}{
		{
			name: "5xx error",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			expectedErr: ErrAuthenticationFailed,
			description: "server returns 503",
		},
		{
			name: "4xx error",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
			},
			expectedErr: ErrAuthenticationFailed,
			description: "server returns 400",
		},
		{
			name: "invalid json response",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("not valid json"))
			},
			expectedErr: ErrAuthenticationFailed,
			description: "server returns invalid JSON",
		},
		{
			name: "empty response body",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
			},
			expectedErr: ErrAuthenticationFailed,
			description: "server returns empty body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverHandler))
			defer server.Close()

			provider := NewCloudAuthProvider(server.URL, 5*time.Minute, "")

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Authorization", "Bearer test-token")

			ctx := context.Background()
			_, err := provider.Authenticate(ctx, req)

			if !errors.Is(err, tt.expectedErr) {
				t.Errorf("%s: expected error %v, got %v", tt.description, tt.expectedErr, err)
			}
		})
	}
}

// BenchmarkCloudAuthProvider_Authenticate benchmarks the authenticate function
func BenchmarkCloudAuthProvider_Authenticate(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := IntrospectionResponse{
			Active:      true,
			KeyID:       "bench_key",
			WorkspaceID: "ws_1",
			SubjectType: "service",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewCloudAuthProvider(server.URL, 5*time.Minute, "")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer bench-token")

	ctx := context.Background()

	// Warm up cache
	_, _ = provider.Authenticate(ctx, req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Use cache hit
		_, _ = provider.Authenticate(ctx, req)
	}
}

// BenchmarkCloudAuthProvider_Authenticate_CacheMiss benchmarks cache miss path
func BenchmarkCloudAuthProvider_Authenticate_CacheMiss(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := IntrospectionResponse{
			Active:      true,
			KeyID:       "bench_key",
			WorkspaceID: "ws_1",
			SubjectType: "service",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewCloudAuthProvider(server.URL, 5*time.Minute, "")
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Use unique tokens to ensure cache misses
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer bench-token-%d", i))
		_, _ = provider.Authenticate(ctx, req)
	}
}
