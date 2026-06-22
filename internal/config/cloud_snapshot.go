// Package config provides configuration management for the routatic-proxy.
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/auth"
)

// cachedSnapshot stores a cached configuration snapshot with its fetch timestamp.
type cachedSnapshot struct {
	config    *RuntimeConfig
	fetchedAt time.Time
}

// CloudSnapshotConfigProvider fetches configuration snapshots from a cloud endpoint.
// It implements the ConfigProvider interface with TTL-based caching and thread-safe
// operations.
type CloudSnapshotConfigProvider struct {
	snapshotURL  string
	ttl          time.Duration
	mu           sync.RWMutex
	snapshots    map[string]cachedSnapshot   // key: workspaceID:version
	inFlight     map[string]*inFlightRequest // key -> request in progress
	httpClient   *http.Client
	serviceToken string
}

// inFlightRequest tracks pending fetches for deduplication.
type inFlightRequest struct {
	mu     sync.Mutex
	cond   *sync.Cond
	done   bool
	config *RuntimeConfig
	err    error
}

// snapshotResponse represents the JSON structure returned by the cloud snapshot API.
// The API may return either a direct RuntimeConfig or a wrapped response.
type snapshotResponse struct {
	Version     string          `json:"version"`
	WorkspaceID string          `json:"workspace_id"`
	Config      json.RawMessage `json:"config"`
}

// NewCloudSnapshotConfigProvider creates a new CloudSnapshotConfigProvider.
//
// Parameters:
//   - snapshotURL: the base URL for the snapshot API endpoint
//   - ttl: time-to-live for cached snapshots; use 0 for no expiration
//   - serviceToken: optional Bearer token for authentication (can be empty)
//
// Example:
//
//	provider := NewCloudSnapshotConfigProvider("https://api.example.com/v1/snapshots", 5*time.Minute, "my-token")
//	config, err := provider.GetEffectiveConfig(ctx, authCtx)
func NewCloudSnapshotConfigProvider(snapshotURL string, ttl time.Duration, serviceToken string) *CloudSnapshotConfigProvider {
	return &CloudSnapshotConfigProvider{
		snapshotURL:  snapshotURL,
		ttl:          ttl,
		snapshots:    make(map[string]cachedSnapshot),
		inFlight:     make(map[string]*inFlightRequest),
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		serviceToken: serviceToken,
	}
}

// SetHTTPClient allows customizing the HTTP client used for requests.
// Useful for testing or for configuring custom TLS settings.
func (p *CloudSnapshotConfigProvider) SetHTTPClient(client *http.Client) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.httpClient = client
}

// cacheKey generates a cache key from workspaceID and version.
func (p *CloudSnapshotConfigProvider) cacheKey(workspaceID, version string) string {
	return fmt.Sprintf("%s:%s", workspaceID, version)
}

// isExpired checks if a cached snapshot has exceeded its TTL.
// Returns false for infinite TTL (ttl == 0).
func (p *CloudSnapshotConfigProvider) isExpired(entry cachedSnapshot) bool {
	if p.ttl == 0 {
		return false
	}
	return time.Since(entry.fetchedAt) > p.ttl
}

// fetchSnapshot retrieves a snapshot from the cloud endpoint.
func (p *CloudSnapshotConfigProvider) fetchSnapshot(ctx context.Context, workspaceID, version string) (*RuntimeConfig, error) {
	url := fmt.Sprintf("%s?workspace=%s&version=%s", p.snapshotURL, workspaceID, version)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	if p.serviceToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.serviceToken)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching snapshot: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("snapshot API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	// Try to unmarshal as wrapped response first
	var wrapped snapshotResponse
	if err := json.Unmarshal(body, &wrapped); err == nil && wrapped.Config != nil {
		// It's a wrapped response with a "config" field
		var config RuntimeConfig
		if err := json.Unmarshal(wrapped.Config, &config); err != nil {
			return nil, fmt.Errorf("unwrapping config: %w", err)
		}
		// Populate version and workspace_id if not set in inner config
		if config.WorkspaceID == "" {
			config.WorkspaceID = wrapped.WorkspaceID
		}
		if config.Version == "" {
			config.Version = wrapped.Version
		}
		return &config, nil
	}

	// Try direct RuntimeConfig unmarshaling
	var config RuntimeConfig
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, fmt.Errorf("parsing snapshot: %w", err)
	}

	return &config, nil
}

// GetEffectiveConfig returns the runtime configuration for the authenticated request.
// It fetches from the cloud if not cached or expired, using the workspace and
// version from the auth context.
func (p *CloudSnapshotConfigProvider) GetEffectiveConfig(ctx context.Context, authCtx *auth.AuthContext) (*RuntimeConfig, error) {
	if authCtx == nil {
		return nil, fmt.Errorf("auth context is required")
	}

	key := p.cacheKey(authCtx.WorkspaceID, authCtx.ConfigRef.Version)

	// Check cache first (read lock)
	p.mu.RLock()
	entry, exists := p.snapshots[key]
	p.mu.RUnlock()

	if exists && !p.isExpired(entry) {
		return entry.config, nil
	}

	// Cache miss or expired - fetch with write lock
	return p.fetchAndCache(ctx, key, authCtx.WorkspaceID, authCtx.ConfigRef.Version)
}

// GetConfigByRef retrieves a specific configuration version by reference.
// It uses the workspace and version from the ConfigRef.
func (p *CloudSnapshotConfigProvider) GetConfigByRef(ctx context.Context, ref auth.ConfigRef) (*RuntimeConfig, error) {
	key := p.cacheKey(ref.WorkspaceID, ref.Version)

	// Check cache first (read lock)
	p.mu.RLock()
	entry, exists := p.snapshots[key]
	p.mu.RUnlock()

	if exists && !p.isExpired(entry) {
		return entry.config, nil
	}

	// Cache miss or expired - fetch with write lock
	return p.fetchAndCache(ctx, key, ref.WorkspaceID, ref.Version)
}

// fetchAndCache fetches from the cloud endpoint and caches the result.
// Uses in-flight request deduplication to prevent multiple concurrent fetches
// for the same key.
func (p *CloudSnapshotConfigProvider) fetchAndCache(ctx context.Context, key, workspaceID, version string) (*RuntimeConfig, error) {
	p.mu.Lock()

	// Check again under write lock
	if entry, exists := p.snapshots[key]; exists && !p.isExpired(entry) {
		p.mu.Unlock()
		return entry.config, nil
	}

	// Check if someone is already fetching for this key
	if inflight, exists := p.inFlight[key]; exists {
		p.mu.Unlock()
		// Wait for existing fetch to complete
		return p.waitForInFlight(ctx, inflight)
	}

	// We are the one who will fetch
	req := &inFlightRequest{}
	req.cond = sync.NewCond(&req.mu)
	p.inFlight[key] = req
	p.mu.Unlock()

	// Do the fetch
	config, err := p.fetchSnapshot(ctx, workspaceID, version)

	// Mark as done and store result
	req.mu.Lock()
	req.config = config
	req.err = err
	req.done = true
	req.cond.Broadcast()
	req.mu.Unlock()

	// Remove from in-flight and store in cache (if success)
	p.mu.Lock()
	delete(p.inFlight, key)
	if err == nil {
		p.snapshots[key] = cachedSnapshot{
			config:    config,
			fetchedAt: time.Now(),
		}
	}
	p.mu.Unlock()

	return config, err
}

// waitForInFlight waits for an in-flight request to complete.
// Returns the result of the shared fetch operation.
func (p *CloudSnapshotConfigProvider) waitForInFlight(ctx context.Context, req *inFlightRequest) (*RuntimeConfig, error) {
	req.mu.Lock()
	defer req.mu.Unlock()

	// Wait for request to complete
	for !req.done {
		// Use Wait with timeout support
		done := make(chan struct{})
		go func() {
			req.cond.Wait()
			close(done)
		}()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-done:
			// Continue
		}
	}

	return req.config, req.err
}

// Invalidate clears the cached configuration for the specified workspace and version.
// If version is empty, all versions for the workspace are invalidated.
func (p *CloudSnapshotConfigProvider) Invalidate(ctx context.Context, workspaceID string, version string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if version != "" {
		// Invalidate specific entry
		key := p.cacheKey(workspaceID, version)
		delete(p.snapshots, key)
	} else {
		// Invalidate all versions for this workspace
		prefix := workspaceID + ":"
		for key := range p.snapshots {
			if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
				delete(p.snapshots, key)
			}
		}
	}

	return nil
}

// HealthCheck pings the snapshot endpoint to verify connectivity.
// It makes a lightweight HEAD request to check if the endpoint is reachable.
func (p *CloudSnapshotConfigProvider) HealthCheck(ctx context.Context) error {
	// Use HEAD request for efficiency - no response body needed
	url := p.snapshotURL

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return fmt.Errorf("creating health check request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	if p.serviceToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.serviceToken)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Accept only 2xx status as healthy
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// SnapshotCount returns the number of cached snapshots.
// Safe for concurrent use, mainly useful for testing.
func (p *CloudSnapshotConfigProvider) SnapshotCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.snapshots)
}
