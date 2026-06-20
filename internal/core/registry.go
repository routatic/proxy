package core

import (
	"fmt"
	"sync"
)

// ProviderRegistry provides thread-safe access to registered providers.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewProviderRegistry creates a new provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]Provider),
	}
}

// Register adds a provider. Returns an error if the name is already registered.
func (r *ProviderRegistry) Register(p Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := p.Name()
	if _, ok := r.providers[name]; ok {
		return fmt.Errorf("provider %q already registered", name)
	}
	r.providers[name] = p
	return nil
}

// Get retrieves a provider by name. Returns false if not found.
func (r *ProviderRegistry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// MustGet retrieves a provider by name, panicking if missing.
func (r *ProviderRegistry) MustGet(name string) Provider {
	if p, ok := r.Get(name); ok {
		return p
	}
	panic(fmt.Sprintf("provider %q not registered", name))
}

// List returns all registered provider names.
func (r *ProviderRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for n := range r.providers {
		names = append(names, n)
	}
	return names
}
