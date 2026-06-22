// Package auth provides authentication interfaces and types for the routatic-proxy.
package auth

import (
	"context"
	"net/http"
	"slices"
)

// AuthContext holds the complete authentication and authorization context
// for an incoming request. It is populated by the AuthProvider and passed
// through the request lifecycle.
//
// Fields are grouped semantically:
//   - Identity: who is making the request
//   - Access Control: what they're allowed to do
//   - Limits: rate limits and billing
//   - Configuration: config references and metadata
type AuthContext struct {
	// Identity contains the authenticated subject information.
	Identity SubjectIdentity

	// WorkspaceID is the unique identifier for the workspace/organization.
	// May be the same as Identity.ID for workspace-level subjects.
	WorkspaceID string

	// KeyID is the identifier for the API key used to authenticate.
	// May be empty for unauthenticated requests.
	KeyID string

	// KeyStatus indicates the current state of the API key.
	KeyStatus KeyStatus

	// AllowedModels is the list of model IDs this subject can use.
	// If empty, all models in the global config are allowed.
	AllowedModels []string

	// AllowedProviders is the list of provider IDs this subject can use.
	// If empty, all providers in the global config are allowed.
	AllowedProviders []string

	// Roles contains the subject's assigned roles for RBAC.
	// Common roles: "admin", "user", "read-only", "service".
	Roles []string

	// RateLimits defines the rate limiting policy for this subject.
	RateLimits RateLimitPolicy

	// Billing contains billing information for cloud mode.
	// This is lazily populated and may be zero-valued for local-only deployments.
	Billing BillingPolicy

	// ConfigRef references the active configuration for this subject.
	// Uses the ConfigRef type from types.go
	ConfigRef ConfigRef

	// Metadata contains additional provider-specific metadata.
	// Keys are provider-specific and should be prefixed to avoid collisions.
	Metadata map[string]string
}

// Authenticate authenticates an HTTP request and returns an AuthContext.
// The implementation should extract credentials from the request headers
// (typically "Authorization: Bearer <token>"), validate them, and return
// the authentication context.
type AuthProvider interface {
	// Authenticate validates the request credentials and returns an AuthContext.
	// Returns an error if authentication fails or credentials are invalid.
	Authenticate(ctx context.Context, req *http.Request) (*AuthContext, error)

	// RevokeCache invalidates any cached authentication state for the given key ID.
	// This is called when a key is revoked or modified externally.
	RevokeCache(ctx context.Context, keyID string) error

	// HealthCheck verifies the authentication provider is healthy and reachable.
	// Returns nil if healthy, otherwise returns an error describing the issue.
	HealthCheck(ctx context.Context) error
}

// HasRole checks if the subject has the specified role.
func (a *AuthContext) HasRole(role string) bool {
	return slices.Contains(a.Roles, role)
}

// HasAnyRole checks if the subject has any of the specified roles.
func (a *AuthContext) HasAnyRole(roles ...string) bool {
	return slices.ContainsFunc(roles, func(role string) bool {
		return a.HasRole(role)
	})
}

// IsAllowedModel checks if the subject can use the specified model.
func (a *AuthContext) IsAllowedModel(modelID string) bool {
	if len(a.AllowedModels) == 0 {
		return true
	}
	return slices.Contains(a.AllowedModels, modelID)
}

// IsAllowedProvider checks if the subject can use the specified provider.
func (a *AuthContext) IsAllowedProvider(providerID string) bool {
	if len(a.AllowedProviders) == 0 {
		return true
	}
	return slices.Contains(a.AllowedProviders, providerID)
}

// Key returns the value for the given metadata key, or empty string if not present.
func (a *AuthContext) Key(key string) string {
	if a.Metadata == nil {
		return ""
	}
	return a.Metadata[key]
}

// WithMetadata returns a copy of the AuthContext with the given key-value pair
// added to metadata.
func (a *AuthContext) WithMetadata(key, value string) *AuthContext {
	// Create a copy of the struct.
	copy := *a

	// Deep copy the metadata map.
	if a.Metadata == nil {
		copy.Metadata = make(map[string]string)
	} else {
		copy.Metadata = make(map[string]string, len(a.Metadata))
		for k, v := range a.Metadata {
			copy.Metadata[k] = v
		}
	}
	copy.Metadata[key] = value
	return &copy
}
