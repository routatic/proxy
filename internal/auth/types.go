// Package auth provides authentication interfaces and types for the routatic-proxy.
//
// The auth package defines the contracts for authenticating requests and
// managing authentication contexts. Implementations include local key-based
// authentication, cloud-based authentication, and no-op authentication for
// development.
package auth

// SubjectIdentity represents the authenticated subject (user, workspace, or service).
type SubjectIdentity struct {
	// Type indicates the subject type (workspace, user, service, or local).
	Type SubjectType

	// ID is the unique identifier for this subject.
	ID string

	// Name is the human-readable display name for this subject.
	Name string

	// Email is the contact email for user and service subjects.
	Email string
}

// SubjectType represents the type of authenticated subject.
type SubjectType string

const (
	// SubjectTypeWorkspace represents an organization workspace account.
	SubjectTypeWorkspace SubjectType = "workspace"

	// SubjectTypeUser represents an individual user account.
	SubjectTypeUser SubjectType = "user"

	// SubjectTypeService represents a service account or API key.
	SubjectTypeService SubjectType = "service"

	// SubjectTypeLocal represents a locally configured subject (no cloud auth).
	SubjectTypeLocal SubjectType = "local"
)

// KeyStatus represents the current state of an API key.
type KeyStatus string

const (
	// KeyStatusActive indicates the key is valid and can be used.
	KeyStatusActive KeyStatus = "active"

	// KeyStatusRevoked indicates the key has been manually revoked.
	KeyStatusRevoked KeyStatus = "revoked"

	// KeyStatusSuspended indicates the key is temporarily suspended.
	KeyStatusSuspended KeyStatus = "suspended"
	// KeyStatusExpired indicates the key has passed its expiration date.
	KeyStatusExpired KeyStatus = "expired"
)

// RateLimitPolicy defines rate limiting parameters for a subject.
type RateLimitPolicy struct {
	// RequestsPerSecond is the maximum requests allowed per second.
	// Zero means no limit.
	RequestsPerSecond int `json:"requests_per_second,omitempty" yaml:"requests_per_second,omitempty"`

	// RequestsPerMinute is the maximum requests allowed per minute.
	// Zero means no limit.
	RequestsPerMinute int `json:"requests_per_minute,omitempty" yaml:"requests_per_minute,omitempty"`

	// RequestsPerHour is the maximum requests allowed per hour.
	// Zero means no limit.
	RequestsPerHour int `json:"requests_per_hour,omitempty" yaml:"requests_per_hour,omitempty"`

	// RequestsPerDay is the maximum requests allowed per day.
	// Zero means no limit.
	RequestsPerDay int `json:"requests_per_day,omitempty" yaml:"requests_per_day,omitempty"`

	// TokensPerMinute is the maximum tokens (input + output) allowed per minute.
	// Zero means no limit.
	TokensPerMinute int `json:"tokens_per_minute,omitempty" yaml:"tokens_per_minute,omitempty"`

	// BurstSize allows temporary bursts above the rate limit.
	// Zero means no burst allowed.
	BurstSize int `json:"burst_size,omitempty" yaml:"burst_size,omitempty"`
}

// BillingPolicy defines billing-related configuration.
// This is only populated when running in cloud mode.
type BillingPolicy struct {
	// Plan is the billing plan identifier (e.g., "free", "pro", "enterprise").
	Plan string

	// CreditsRemaining is the remaining credit balance in the smallest currency unit.
	// Negative values indicate unlimited credits.
	CreditsRemaining int64

	// CreditsConsumed is the total credits consumed this billing period.
	CreditsConsumed int64

	// BillingPeriodStart is the start of the current billing period in Unix seconds.
	BillingPeriodStart int64

	// BillingPeriodEnd is the end of the current billing period in Unix seconds.
	BillingPeriodEnd int64
}

// ConfigRef references a specific configuration context for the subject.
type ConfigRef struct {
	// WorkspaceID is the unique identifier for the workspace/tenant.
	WorkspaceID string

	// Version is the configuration version for caching and invalidation.
	Version string

	// LastModified is the Unix timestamp of the last configuration change.
	LastModified int64
}
