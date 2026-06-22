// Package auth provides authentication interfaces and types for the routatic-proxy.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CloudAuthProvider implements AuthProvider for cloud-based token authentication.
// It validates tokens by calling a cloud introspection endpoint and caches results
// with TTL for performance. All operations are thread-safe.
type CloudAuthProvider struct {
	introspectionURL string
	cacheTTL         time.Duration
	mu               sync.RWMutex
	cache            map[string]cachedAuth // key: token hash
	httpClient       *http.Client

	// serviceToken is used to authenticate with the introspection endpoint
	serviceToken string

	// refreshThreshold is the duration before TTL expiry when background refresh triggers
	refreshThreshold time.Duration
}

// cachedAuth holds cached authentication results with timestamp
type cachedAuth struct {
	authContext *AuthContext
	cachedAt    time.Time
}

// IntrospectionRequest is the JSON payload sent to the introspection endpoint
type IntrospectionRequest struct {
	Token string `json:"token"`
}

// IntrospectionResponse is the JSON response from the introspection endpoint
type IntrospectionResponse struct {
	Active           bool            `json:"active"`
	KeyID            string          `json:"key_id"`
	WorkspaceID      string          `json:"workspace_id"`
	SubjectID        string          `json:"subject_id"`
	SubjectType      string          `json:"subject_type"`
	Roles            []string        `json:"roles"`
	AllowedModels    []string        `json:"allowed_models"`
	AllowedProviders []string        `json:"allowed_providers"`
	RateLimits       RateLimitPolicy `json:"rate_limits"`
	Billing          BillingPolicy   `json:"billing"`
}

// NewCloudAuthProvider creates a new CloudAuthProvider with the given configuration.
// The introspectionURL is the endpoint to call for token validation.
// The cacheTTL determines how long to cache successful authentication results.
// The serviceToken is used to authenticate with the introspection endpoint.
func NewCloudAuthProvider(introspectionURL string, cacheTTL time.Duration, serviceToken string) *CloudAuthProvider {
	if cacheTTL <= 0 {
		cacheTTL = 5 * time.Minute
	}

	return &CloudAuthProvider{
		introspectionURL: introspectionURL,
		cacheTTL:         cacheTTL,
		cache:            make(map[string]cachedAuth),
		httpClient:       &http.Client{Timeout: 10 * time.Second},
		serviceToken:     serviceToken,
		refreshThreshold: cacheTTL / 4, // Refresh when 25% of TTL remains
	}
}

// NewCloudAuthProviderWithClient creates a new CloudAuthProvider with a custom HTTP client.
// This is useful for testing and for advanced configuration scenarios.
func NewCloudAuthProviderWithClient(introspectionURL string, cacheTTL time.Duration, serviceToken string, httpClient *http.Client) *CloudAuthProvider {
	provider := NewCloudAuthProvider(introspectionURL, cacheTTL, serviceToken)
	if httpClient != nil {
		provider.httpClient = httpClient
	}
	return provider
}

// Authenticate validates the request credentials by checking the cache first,
// then calling the cloud introspection endpoint if needed.
// Thread-safe.
func (p *CloudAuthProvider) Authenticate(ctx context.Context, req *http.Request) (*AuthContext, error) {
	// Extract the Bearer token from the Authorization header.
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		slog.Debug("missing Authorization header")
		return nil, ErrAuthenticationFailed
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		slog.Debug("invalid Authorization header format")
		return nil, ErrAuthenticationFailed
	}

	// Extract the token.
	token := strings.TrimPrefix(authHeader, "Bearer ")
	token = strings.TrimSpace(token)

	if token == "" {
		slog.Debug("empty bearer token")
		return nil, ErrAuthenticationFailed
	}

	// Hash the token for cache key.
	tokenHash := hashToken(token)

	// Check cache first (read lock).
	p.mu.RLock()
	cached, exists := p.cache[tokenHash]
	p.mu.RUnlock()

	if exists {
		// Check if cache entry is still valid.
		if time.Since(cached.cachedAt) < p.cacheTTL {
			slog.Debug("cache hit for token", "key_id", cached.authContext.KeyID)

			// Trigger background refresh if entry is nearing expiry.
			if time.Since(cached.cachedAt) > p.cacheTTL-p.refreshThreshold {
				go p.refreshCacheEntry(token, tokenHash)
			}

			return cached.authContext, nil
		}
		// Cache entry expired, will refresh below.
		slog.Debug("cache entry expired, refreshing", "key_id", cached.authContext.KeyID)
	}

	// Cache miss or expired - call introspection endpoint.
	return p.introspectAndCache(ctx, token, tokenHash)
}

// introspectAndCache calls the cloud introspection endpoint and caches the result.
// This is called when cache miss or expired.
func (p *CloudAuthProvider) introspectAndCache(ctx context.Context, token, tokenHash string) (*AuthContext, error) {
	// Call introspection endpoint with fail-closed behavior.
	authCtx, err := p.callIntrospectionEndpoint(ctx, token)
	if err != nil {
		// On introspection failure: fail closed (return error).
		slog.Error("introspection failed, failing closed", "error", err)
		return nil, ErrAuthenticationFailed
	}

	// Check if token is active.
	if authCtx.KeyStatus != KeyStatusActive {
		slog.Debug("token is not active", "key_id", authCtx.KeyID, "status", authCtx.KeyStatus)
		return nil, ErrAuthenticationFailed
	}

	// Cache the result.
	p.mu.Lock()
	p.cache[tokenHash] = cachedAuth{
		authContext: authCtx,
		cachedAt:    time.Now(),
	}
	p.mu.Unlock()

	slog.Debug("token introspection successful, cached", "key_id", authCtx.KeyID)
	return authCtx, nil
}

// callIntrospectionEndpoint makes the HTTP request to the cloud introspection endpoint.
// Returns the AuthContext on success, error on failure.
func (p *CloudAuthProvider) callIntrospectionEndpoint(ctx context.Context, token string) (*AuthContext, error) {
	// Prepare request body.
	reqBody := IntrospectionRequest{Token: token}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling introspection request: %w", err)
	}

	// Create HTTP request.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.introspectionURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating introspection request: %w", err)
	}

	// Set headers.
	req.Header.Set("Content-Type", "application/json")
	if p.serviceToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.serviceToken)
	}

	// Make the request.
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("introspection request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle non-2xx responses as failures (fail closed).
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Read and log response body for debugging.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		slog.Debug("introspection endpoint returned non-2xx status",
			"status", resp.StatusCode,
			"body", string(body))
		return nil, fmt.Errorf("introspection endpoint returned status %d", resp.StatusCode)
	}

	// Parse response.
	var introspectResp IntrospectionResponse
	if err := json.NewDecoder(resp.Body).Decode(&introspectResp); err != nil {
		return nil, fmt.Errorf("decoding introspection response: %w", err)
	}

	// Convert response to AuthContext.
	return p.responseToAuthContext(&introspectResp), nil
}

// responseToAuthContext converts an IntrospectionResponse to an AuthContext.
func (p *CloudAuthProvider) responseToAuthContext(resp *IntrospectionResponse) *AuthContext {
	// Determine key status based on active field.
	keyStatus := KeyStatusActive
	if !resp.Active {
		keyStatus = KeyStatusRevoked // Default to revoked if not active
	}

	// Map subject type.
	subjectType := SubjectTypeService
	switch resp.SubjectType {
	case "user":
		subjectType = SubjectTypeUser
	case "workspace":
		subjectType = SubjectTypeWorkspace
	case "service":
		subjectType = SubjectTypeService
	}

	return &AuthContext{
		Identity: SubjectIdentity{
			Type: subjectType,
			ID:   resp.SubjectID,
			Name: resp.KeyID, // Using key_id as name for now
		},
		WorkspaceID:      resp.WorkspaceID,
		KeyID:            resp.KeyID,
		KeyStatus:        keyStatus,
		AllowedModels:    resp.AllowedModels,
		AllowedProviders: resp.AllowedProviders,
		Roles:            resp.Roles,
		RateLimits:       resp.RateLimits,
		Billing:          resp.Billing,
		ConfigRef: ConfigRef{
			WorkspaceID:  resp.WorkspaceID,
			Version:      "cloud",
			LastModified: time.Now().Unix(),
		},
		Metadata: make(map[string]string),
	}
}

// refreshCacheEntry performs background refresh of a cache entry.
// This is called in a goroutine and should not block the caller.
func (p *CloudAuthProvider) refreshCacheEntry(token, tokenHash string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slog.Debug("background refresh started", "token_hash_prefix", tokenHash[:8])

	authCtx, err := p.callIntrospectionEndpoint(ctx, token)
	if err != nil {
		slog.Error("background refresh failed", "error", err)
		return
	}

	// Only update cache if token is still active.
	if authCtx.KeyStatus != KeyStatusActive {
		slog.Debug("background refresh: token no longer active, removing from cache",
			"key_id", authCtx.KeyID)
		p.mu.Lock()
		delete(p.cache, tokenHash)
		p.mu.Unlock()
		return
	}

	p.mu.Lock()
	p.cache[tokenHash] = cachedAuth{
		authContext: authCtx,
		cachedAt:    time.Now(),
	}
	p.mu.Unlock()

	slog.Debug("background refresh completed", "key_id", authCtx.KeyID)
}

// RevokeCache invalidates any cached authentication state for the given key ID.
// Thread-safe.
func (p *CloudAuthProvider) RevokeCache(ctx context.Context, keyID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Find and remove cache entries matching the key ID.
	// Since cache is keyed by token hash, we need to iterate.
	for tokenHash, cached := range p.cache {
		if cached.authContext.KeyID == keyID {
			delete(p.cache, tokenHash)
			slog.Info("revoked cache entry", "key_id", keyID)
		}
	}

	return nil
}

// HealthCheck verifies the authentication provider is healthy by making
// a lightweight request to the introspection endpoint.
// Thread-safe.
func (p *CloudAuthProvider) HealthCheck(ctx context.Context) error {
	// Perform a lightweight check by making an HTTP HEAD request.
	// HEAD is more efficient than GET as it doesn't return a response body.
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, p.introspectionURL, nil)
	if err != nil {
		return fmt.Errorf("creating health check request: %w", err)
	}

	// Try to connect - we expect this might fail authentication,
	// but we just want to check the endpoint is reachable.
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("introspection endpoint unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Any HTTP response (even 401/403) means the endpoint is reachable.
	// Only network errors or timeouts should cause health check to fail.
	return nil
}

// ClearCache removes all entries from the cache.
// Thread-safe.
func (p *CloudAuthProvider) ClearCache() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.cache = make(map[string]cachedAuth)
	slog.Info("cloud auth cache cleared")
}

// CacheStats returns statistics about the current cache state.
// Thread-safe.
func (p *CloudAuthProvider) CacheStats() (total int, expired int) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	now := time.Now()
	total = len(p.cache)

	for _, cached := range p.cache {
		if now.Sub(cached.cachedAt) >= p.cacheTTL {
			expired++
		}
	}

	return total, expired
}
