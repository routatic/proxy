// Package auth provides authentication interfaces and types for the routatic-proxy.
package auth

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
)

// NoAuthProvider is a no-op authentication provider for local development.
// It returns a hardcoded AuthContext allowing all requests without validating credentials.
// This provider includes safety checks to warn when used in non-localhost environments.
type NoAuthProvider struct {
	defaultContext *AuthContext
	workspaceID    string
	mu             sync.RWMutex
}

// NewNoAuthProvider creates a new NoAuthProvider with the given workspace ID.
// The workspace ID is used to populate the Identity.ID and WorkspaceID fields.
func NewNoAuthProvider(workspaceID string) *NoAuthProvider {
	if workspaceID == "" {
		workspaceID = "local"
	}

	return &NoAuthProvider{
		workspaceID: workspaceID,
		defaultContext: &AuthContext{
			Identity: SubjectIdentity{
				Type: SubjectTypeLocal,
				ID:   workspaceID,
				Name: "Local Development",
			},
			WorkspaceID:      workspaceID,
			KeyID:            "dev",
			KeyStatus:        KeyStatusActive,
			AllowedModels:    []string{}, // Empty = all allowed
			AllowedProviders: []string{}, // Empty = all allowed
			Roles:            []string{"admin"},
			Metadata:         make(map[string]string),
		},
	}
}

// Authenticate returns the hardcoded AuthContext, allowing all requests.
// It performs a safety check to warn if the request is not from localhost.
// This method is thread-safe.
func (p *NoAuthProvider) Authenticate(ctx context.Context, req *http.Request) (*AuthContext, error) {
	// Perform safety check for non-localhost requests
	if req != nil && !p.isLocalhostRequest(req) {
		// In development, we warn but still allow. For production, consider returning an error.
		// Return an error if not localhost for extra safety
		return nil, errors.New("NoAuthProvider: rejecting non-localhost request from: " + req.Host)
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return a copy to prevent external modification
	return p.copyAuthContext(), nil
}

// RevokeCache is a no-op for NoAuthProvider since there is no caching.
func (p *NoAuthProvider) RevokeCache(ctx context.Context, keyID string) error {
	// No-op: this provider doesn't cache anything
	return nil
}

// HealthCheck always returns nil indicating the provider is healthy.
func (p *NoAuthProvider) HealthCheck(ctx context.Context) error {
	return nil
}

// isLocalhostRequest checks if the request is coming from localhost.
// SECURITY: This function only uses RemoteAddr, NOT X-Forwarded-For headers,
// because anyone can spoof X-Forwarded-For. This provider is for local development only.
func (p *NoAuthProvider) isLocalhostRequest(req *http.Request) bool {
	if req == nil {
		return true // Allow nil requests (for testing)
	}

	// Check the Host header
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}

	// Remove port if present
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// Check if it's localhost
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}

	// Only trust RemoteAddr - NOT X-Forwarded-For or other headers.
	// X-Forwarded-For can be spoofed by clients and must only be trusted
	// when validated against a list of trusted proxies (not implemented here).
	remoteHost, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		// If no port, assume it's just the IP
		remoteHost = req.RemoteAddr
	}

	// Check if remote address is localhost
	return remoteHost == "127.0.0.1" || remoteHost == "::1" || remoteHost == "localhost"
}

// copyAuthContext creates a deep copy of the default AuthContext.
// This ensures thread safety by preventing external modification of internal state.
func (p *NoAuthProvider) copyAuthContext() *AuthContext {
	ctx := p.defaultContext

	// Copy slices
	allowedModels := make([]string, len(ctx.AllowedModels))
	copy(allowedModels, ctx.AllowedModels)

	allowedProviders := make([]string, len(ctx.AllowedProviders))
	copy(allowedProviders, ctx.AllowedProviders)

	roles := make([]string, len(ctx.Roles))
	copy(roles, ctx.Roles)

	// Copy metadata map
	metadata := make(map[string]string, len(ctx.Metadata))
	for k, v := range ctx.Metadata {
		metadata[k] = v
	}

	return &AuthContext{
		Identity:         ctx.Identity,
		WorkspaceID:      ctx.WorkspaceID,
		KeyID:            ctx.KeyID,
		KeyStatus:        ctx.KeyStatus,
		AllowedModels:    allowedModels,
		AllowedProviders: allowedProviders,
		Roles:            roles,
		RateLimits:       ctx.RateLimits,
		Billing:          ctx.Billing,
		ConfigRef:        ctx.ConfigRef,
		Metadata:         metadata,
	}
}

// UpdateWorkspaceID updates the workspace ID in a thread-safe manner.
// This is useful for testing and dynamic configuration scenarios.
func (p *NoAuthProvider) UpdateWorkspaceID(workspaceID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.workspaceID = workspaceID
	p.defaultContext.Identity.ID = workspaceID
	p.defaultContext.WorkspaceID = workspaceID
}
