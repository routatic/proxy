// Package config provides configuration management for the routatic-proxy.
package config

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/routatic/proxy/internal/auth"
)

var (
	// ErrAuthContextRequired is returned when auth context is nil but required.
	ErrAuthContextRequired = errors.New("auth context is required")

	// ErrConfigNotLoaded is returned when configuration has not been loaded.
	ErrConfigNotLoaded = errors.New("configuration not loaded")
)

// ConfigRef is re-exported from auth package for convenience.
type ConfigRef = auth.ConfigRef

// ConfigProvider returns runtime-ready config for authenticated requests.
// Implementations may be backed by file-based configs, cloud snapshots,
// or cached/multi-layered sources.
type ConfigProvider interface {
	// GetEffectiveConfig returns the runtime configuration for the authenticated request.
	// The auth context determines which workspace and permissions apply.
	GetEffectiveConfig(ctx context.Context, authCtx *auth.AuthContext) (*RuntimeConfig, error)

	// GetConfigByRef retrieves a specific configuration version by reference.
	// Useful for rollbacks or previewing specific config versions.
	GetConfigByRef(ctx context.Context, ref auth.ConfigRef) (*RuntimeConfig, error)

	// Invalidate clears any cached configuration for the specified workspace and version.
	// Call this when external configuration changes are detected.
	Invalidate(ctx context.Context, workspaceID string, version string) error

	// HealthCheck verifies the provider is operational.
	HealthCheck(ctx context.Context) error
}

// StaticConfigProvider is a ConfigProvider that always returns a fixed RuntimeConfig.
// Useful for development, testing, and backward compatibility during migrations.
// Thread-safe via sync.RWMutex.
type StaticConfigProvider struct {
	config map[string]*RuntimeConfig // key: workspaceID
	mu     sync.RWMutex              // guards config map
}

// NewStaticConfigProvider creates a new StaticConfigProvider that returns the given config
// for all workspaces. Pass nil to return ErrConfigNotLoaded.
//
// This provider is useful for:
//   - Backward compatibility during migration to provider-based architecture
//   - Development and testing scenarios
//   - Simple deployments without workspace-specific configs
func NewStaticConfigProvider(cfg *RuntimeConfig) *StaticConfigProvider {
	configs := make(map[string]*RuntimeConfig)
	if cfg != nil {
		configs[cfg.WorkspaceID] = cfg
		// Also index by "default" and "" for flexibility
		configs["default"] = cfg
		configs[""] = cfg
	}
	return &StaticConfigProvider{
		config: configs,
	}
}

// NewStaticConfigProviderWithWorkspaces creates a new StaticConfigProvider with
// workspace-specific configurations.
func NewStaticConfigProviderWithWorkspaces(configs map[string]*RuntimeConfig) *StaticConfigProvider {
	return &StaticConfigProvider{
		config: configs,
	}
}

// GetEffectiveConfig returns the runtime configuration for the authenticated request.
// For StaticConfigProvider, it returns the configured config based on workspaceID,
// falling back to "default" or the first available config.
// Safe for concurrent use.
func (p *StaticConfigProvider) GetEffectiveConfig(ctx context.Context, authCtx *auth.AuthContext) (*RuntimeConfig, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	// Determine workspace ID
	workspaceID := ""
	if authCtx != nil {
		workspaceID = authCtx.WorkspaceID
	}

	// Try workspace-specific config
	if cfg, ok := p.config[workspaceID]; ok && cfg != nil {
		return cfg, nil
	}

	// Try "default" key
	if cfg, ok := p.config["default"]; ok && cfg != nil {
		return cfg, nil
	}

	// Try empty key
	if cfg, ok := p.config[""]; ok && cfg != nil {
		return cfg, nil
	}

	// Try any available config
	for _, cfg := range p.config {
		if cfg != nil {
			return cfg, nil
		}
	}

	return nil, ErrConfigNotLoaded
}

// GetConfigByRef retrieves a specific configuration version by reference.
// For StaticConfigProvider, this ignores the ref and returns the effective config.
// Safe for concurrent use.
func (p *StaticConfigProvider) GetConfigByRef(ctx context.Context, ref auth.ConfigRef) (*RuntimeConfig, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	// Use ref.WorkspaceID to look up config
	workspaceID := ref.WorkspaceID
	if workspaceID == "" {
		workspaceID = "default"
	}

	// Try workspace-specific config
	if cfg, ok := p.config[workspaceID]; ok && cfg != nil {
		return cfg, nil
	}

	// Fall back to any available config
	for _, cfg := range p.config {
		if cfg != nil {
			return cfg, nil
		}
	}

	return nil, ErrConfigNotLoaded
}

// Invalidate is a no-op for StaticConfigProvider since the config is fixed.
func (p *StaticConfigProvider) Invalidate(ctx context.Context, workspaceID string, version string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Static provider doesn't support invalidation - config is fixed
	return nil
}

// HealthCheck verifies the provider has a valid configuration loaded.
// Safe for concurrent use.
func (p *StaticConfigProvider) HealthCheck(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	// Check if we have any valid config
	for _, cfg := range p.config {
		if cfg != nil {
			return nil
		}
	}

	return ErrConfigNotLoaded
}

// SetConfig updates the configuration for a workspace.
// Safe for concurrent use.
func (p *StaticConfigProvider) SetConfig(workspaceID string, cfg *RuntimeConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if cfg == nil {
		delete(p.config, workspaceID)
		return
	}
	p.config[workspaceID] = cfg
}

// IsNoOp returns true if this provider has no valid configuration loaded.
// Safe for concurrent use.
func (p *StaticConfigProvider) IsNoOp() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, cfg := range p.config {
		if cfg != nil {
			return false
		}
	}
	return true
}

// ProviderOption configures a ConfigProvider.
type ProviderOption func(*ProviderOptions)

// ProviderOptions holds configuration options for providers.
type ProviderOptions struct {
	WorkspaceID string
	Version     string
}

// WithWorkspace sets the workspace ID for provider operations.
func WithWorkspace(workspaceID string) ProviderOption {
	return func(o *ProviderOptions) {
		o.WorkspaceID = workspaceID
	}
}

// WithVersion sets the config version for provider operations.
func WithVersion(version string) ProviderOption {
	return func(o *ProviderOptions) {
		o.Version = version
	}
}

// ValidateProvider performs basic validation on a ConfigProvider.
func ValidateProvider(p ConfigProvider) error {
	ctx := context.Background()
	return p.HealthCheck(ctx)
}

// CompositeConfigProvider chains multiple ConfigProviders together.
// It tries providers in order and returns the first successful result.
type CompositeConfigProvider struct {
	providers []ConfigProvider
}

// NewCompositeConfigProvider creates a new ConfigProvider that chains multiple
// providers together. Providers are tried in order until one succeeds.
func NewCompositeConfigProvider(providers ...ConfigProvider) *CompositeConfigProvider {
	// Filter out nil providers
	var valid []ConfigProvider
	for _, p := range providers {
		if p != nil {
			valid = append(valid, p)
		}
	}
	return &CompositeConfigProvider{providers: valid}
}

// GetEffectiveConfig tries each provider in order until one succeeds.
func (p *CompositeConfigProvider) GetEffectiveConfig(ctx context.Context, authCtx *auth.AuthContext) (*RuntimeConfig, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if authCtx == nil {
		return nil, ErrAuthContextRequired
	}

	var lastErr error
	for _, provider := range p.providers {
		cfg, err := provider.GetEffectiveConfig(ctx, authCtx)
		if err == nil {
			return cfg, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrConfigNotLoaded
}

// GetConfigByRef tries each provider in order until one succeeds.
func (p *CompositeConfigProvider) GetConfigByRef(ctx context.Context, ref auth.ConfigRef) (*RuntimeConfig, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var lastErr error
	for _, provider := range p.providers {
		cfg, err := provider.GetConfigByRef(ctx, ref)
		if err == nil {
			return cfg, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrConfigNotLoaded
}

// Invalidate invalidates the configuration in all underlying providers.
func (p *CompositeConfigProvider) Invalidate(ctx context.Context, workspaceID string, version string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	var lastErr error
	for _, provider := range p.providers {
		if err := provider.Invalidate(ctx, workspaceID, version); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// HealthCheck verifies all providers are healthy.
func (p *CompositeConfigProvider) HealthCheck(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	for i, provider := range p.providers {
		if err := provider.HealthCheck(ctx); err != nil {
			return fmt.Errorf("provider %d health check failed: %w", i, err)
		}
	}
	return nil
}
