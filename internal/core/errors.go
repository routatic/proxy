package core

import (
	"errors"
	"fmt"
)

// Sentinel errors for common provider and routing failures.
var (
	ErrModelNotFound         = errors.New("model not found")
	ErrProviderNotFound      = errors.New("provider not found")
	ErrUnsupportedCapability = errors.New("capability not supported by model")
	ErrRateLimited           = errors.New("rate limited by provider")
	ErrStreamIdle            = errors.New("upstream stream idle")
	ErrClientDisconnected    = errors.New("client disconnected")
)

// NormalizedError wraps a provider error with structured context.
type NormalizedError struct {
	Kind       string // "api_error", "rate_limit", "invalid_request", etc.
	Message    string
	Retryable  bool
	StatusCode int
	Provider   string
	ModelID    string
}

// CompatibilityError means the selected provider/model cannot represent the
// request. It is safe to skip during fallback without recording a provider
// failure.
type CompatibilityError struct {
	Provider string
	ModelID  string
	Reason   string
}

func (e *CompatibilityError) Error() string {
	return fmt.Sprintf("model %s/%s is incompatible with request: %s", e.Provider, e.ModelID, e.Reason)
}

func (e *CompatibilityError) IsCompatibility() bool { return true }

// Error implements the error interface.
func (e *NormalizedError) Error() string {
	return e.Message
}

// IsRetryable returns true if the error is safe to retry with a fallback model.
func (e *NormalizedError) IsRetryable() bool {
	return e.Retryable
}
