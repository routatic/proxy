// Package auth provides authentication interfaces and types for the routatic-proxy.
package auth

import (
	"context"
	"errors"
	"net/http"
)

var (
	// ErrAuthenticationFailed is returned when credentials are invalid or missing.
	ErrAuthenticationFailed = errors.New("authentication failed")

	// ErrKeyRevoked is returned when the API key has been revoked.
	ErrKeyRevoked = errors.New("API key has been revoked")

	// ErrKeySuspended is returned when the API key is temporarily suspended.
	ErrKeySuspended = errors.New("API key is suspended")

	// ErrKeyExpired is returned when the API key has expired.
	ErrKeyExpired = errors.New("API key has expired")

	// ErrRateLimitExceeded is returned when rate limit has been exceeded.
	ErrRateLimitExceeded = errors.New("rate limit exceeded")

	// ErrInsufficientCredits is returned when billing credits are depleted.
	ErrInsufficientCredits = errors.New("insufficient credits")

	// ErrProviderNotAllowed is returned when the provider is not in AllowedProviders.
	ErrProviderNotAllowed = errors.New("provider not allowed for this subject")

	// ErrModelNotAllowed is returned when the model is not in AllowedModels.
	ErrModelNotAllowed = errors.New("model not allowed for this subject")
)

// ProviderType identifies the type of authentication provider implementation.
type ProviderType string

const (
	// ProviderTypeLocal is a file-based local key authentication provider.
	ProviderTypeLocal ProviderType = "local"

	// ProviderTypeCloud is a cloud-based authentication provider.
	ProviderTypeCloud ProviderType = "cloud"

	// ProviderTypeNone is a no-op provider that allows all requests.
	ProviderTypeNone ProviderType = "none"
)

// ProviderConfig holds common configuration for all auth provider implementations.
type ProviderConfig struct {
	// Type specifies the provider implementation type.
	Type ProviderType

	// CacheEnabled indicates whether authentication results should be cached.
	CacheEnabled bool

	// CacheTTL is the time-to-live for cached authentication results in seconds.
	// Zero means use default (300 seconds / 5 minutes).
	CacheTTL int

	// MaxCacheSize is the maximum number of entries to cache.
	// Zero means unlimited.
	MaxCacheSize int
}

// ProviderOption is a functional option for configuring auth providers.
type ProviderOption func(*providerOptions)

type providerOptions struct {
	config ProviderConfig
}

// WithCache enables authentication result caching with the specified TTL.
func WithCache(ttlSeconds int) ProviderOption {
	return func(o *providerOptions) {
		o.config.CacheEnabled = true
		o.config.CacheTTL = ttlSeconds
	}
}

// WithMaxCacheSize sets the maximum number of cached entries.
func WithMaxCacheSize(size int) ProviderOption {
	return func(o *providerOptions) {
		o.config.MaxCacheSize = size
	}
}

// StaticAuthProvider is an AuthProvider that returns a fixed AuthContext.
// Useful for development, testing, and local-only deployments.
type StaticAuthProvider struct {
	context *AuthContext
}

// NewStaticAuthProvider creates a new StaticAuthProvider with the given context.
func NewStaticAuthProvider(ctx *AuthContext) *StaticAuthProvider {
	return &StaticAuthProvider{context: ctx}
}

// Authenticate returns the static AuthContext, ignoring the request.
func (s *StaticAuthProvider) Authenticate(ctx context.Context, req *http.Request) (*AuthContext, error) {
	return s.context, nil
}

// RevokeCache is a no-op for static providers.
func (s *StaticAuthProvider) RevokeCache(ctx context.Context, keyID string) error {
	return nil
}

// HealthCheck always returns nil for static providers.
func (s *StaticAuthProvider) HealthCheck(ctx context.Context) error {
	return nil
}

// NewNoAuthProviderFunc returns a permissive AuthContext for unauthenticated requests.
// This is a convenience function that wraps NewNoAuthProvider for simple use cases.
// For more control, use NewNoAuthProvider directly.
func NewNoAuthProviderFunc(workspaceID string) AuthProvider {
	return NewNoAuthProvider(workspaceID)
}
