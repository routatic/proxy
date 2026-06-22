// Package config provides configuration management for the routatic-proxy.
package config

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/auth"
)

// CacheStats tracks cache performance metrics.
type CacheStats struct {
	Hits          uint64
	Misses        uint64
	Evictions     uint64
	Invalidations uint64
}

// cachedEntry stores a cached configuration with its fetch timestamp.
type cachedEntry struct {
	config    *RuntimeConfig
	fetchedAt time.Time
}

// CacheKey generates a cache key from workspaceID and version.
func CacheKey(workspaceID, version string) string {
	return fmt.Sprintf("%s:%s", workspaceID, version)
}

// CachedConfigProvider wraps a ConfigProvider with TTL-based caching.
// It provides O(1) lookups after the first fetch and supports thread-safe
// operations with configurable invalidation.
type CachedConfigProvider struct {
	underlying ConfigProvider
	ttl        time.Duration
	maxSize    int // 0 = unlimited

	mu       sync.RWMutex
	cache    map[string]cachedEntry
	stats    CacheStats
	lruList  []string       // Simple LRU tracking (oldest first)
	lruIndex map[string]int // Position in lruList for O(1) removal

	// Optional logger/metrics callbacks
	onCacheHit  func(key string)
	onCacheMiss func(key string)
}

// NewCachedConfigProvider creates a new caching wrapper around the given provider.
//
// Parameters:
//   - underlying: the ConfigProvider to wrap and delegate to on cache misses
//   - ttl: time-to-live for cached entries; use 0 for no expiration (infinite TTL)
//
// Example:
//
//	provider := NewCachedConfigProvider(fileProvider, 5*time.Minute)
//	config, err := provider.GetEffectiveConfig(ctx, authCtx)
func NewCachedConfigProvider(underlying ConfigProvider, ttl time.Duration) *CachedConfigProvider {
	return &CachedConfigProvider{
		underlying: underlying,
		ttl:        ttl,
		cache:      make(map[string]cachedEntry),
		lruList:    make([]string, 0),
		lruIndex:   make(map[string]int),
	}
}

// SetMaxSize configures LRU eviction when cache exceeds the specified size.
// Call with 0 to disable size limits (default).
func (p *CachedConfigProvider) SetMaxSize(maxSize int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.maxSize = maxSize
}

// GetEffectiveConfig returns the runtime configuration for the authenticated request.
// It checks the cache first, and falls back to the underlying provider on cache miss.
func (p *CachedConfigProvider) GetEffectiveConfig(ctx context.Context, authCtx *auth.AuthContext) (*RuntimeConfig, error) {
	if authCtx == nil {
		return p.underlying.GetEffectiveConfig(ctx, authCtx)
	}

	key := CacheKey(authCtx.WorkspaceID, authCtx.ConfigRef.Version)

	// Try cache first (read lock)
	p.mu.RLock()
	entry, exists := p.cache[key]
	p.mu.RUnlock()

	if exists && !p.isExpired(entry) {
		p.recordHit(key)
		return entry.config, nil
	}

	// Cache miss or expired - fetch with write lock
	p.recordMiss(key)

	return p.fetchAndCache(ctx, key, func(ctx context.Context) (*RuntimeConfig, error) {
		return p.underlying.GetEffectiveConfig(ctx, authCtx)
	})
}

// GetConfigByRef retrieves a specific configuration version by reference.
// It caches the result for subsequent lookups.
func (p *CachedConfigProvider) GetConfigByRef(ctx context.Context, ref auth.ConfigRef) (*RuntimeConfig, error) {
	key := CacheKey(ref.WorkspaceID, ref.Version)

	// Try cache first (read lock)
	p.mu.RLock()
	entry, exists := p.cache[key]
	p.mu.RUnlock()

	if exists && !p.isExpired(entry) {
		p.recordHit(key)
		return entry.config, nil
	}

	// Cache miss or expired - fetch with write lock
	p.recordMiss(key)

	return p.fetchAndCache(ctx, key, func(ctx context.Context) (*RuntimeConfig, error) {
		return p.underlying.GetConfigByRef(ctx, ref)
	})
}

// Invalidate clears the cached configuration for the specified workspace and version.
// If version is empty, all versions for the workspace are invalidated.
func (p *CachedConfigProvider) Invalidate(ctx context.Context, workspaceID string, version string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if version != "" {
		// Invalidate specific entry
		key := CacheKey(workspaceID, version)
		p.removeFromLRU(key)
		delete(p.cache, key)
		p.stats.Invalidations++
	} else {
		// Invalidate all versions for this workspace
		prefix := workspaceID + ":"
		for key := range p.cache {
			if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
				p.removeFromLRU(key)
				delete(p.cache, key)
				p.stats.Invalidations++
			}
		}
	}

	return nil
}

// HealthCheck verifies the underlying provider is operational.
func (p *CachedConfigProvider) HealthCheck(ctx context.Context) error {
	return p.underlying.HealthCheck(ctx)
}

// GetStats returns a copy of current cache statistics.
// Safe for concurrent use.
func (p *CachedConfigProvider) GetStats() CacheStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stats
}

// SetOnCacheHit sets a callback invoked on cache hits.
func (p *CachedConfigProvider) SetOnCacheHit(fn func(key string)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onCacheHit = fn
}

// SetOnCacheMiss sets a callback invoked on cache misses.
func (p *CachedConfigProvider) SetOnCacheMiss(fn func(key string)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onCacheMiss = fn
}

// isExpired checks if a cached entry has exceeded its TTL.
// Returns false for infinite TTL (ttl == 0).
func (p *CachedConfigProvider) isExpired(entry cachedEntry) bool {
	if p.ttl == 0 {
		return false
	}
	return time.Since(entry.fetchedAt) > p.ttl
}

// fetchAndCache fetches from the underlying provider and caches the result.
// Must be called with proper locking coordination.
func (p *CachedConfigProvider) fetchAndCache(
	ctx context.Context,
	key string,
	fetchFn func(context.Context) (*RuntimeConfig, error),
) (*RuntimeConfig, error) {
	// Double-check pattern: check again after acquiring write lock
	p.mu.Lock()

	// Another goroutine may have fetched while we waited for lock
	if entry, exists := p.cache[key]; exists && !p.isExpired(entry) {
		p.recordHitLocked(key)
		p.mu.Unlock()
		return entry.config, nil
	}
	p.mu.Unlock()

	// Fetch from underlying provider (outside lock to allow concurrent fetches of different keys)
	config, err := fetchFn(ctx)
	if err != nil {
		return nil, err
	}

	// Re-acquire lock to store in cache
	p.mu.Lock()
	defer p.mu.Unlock()

	// Apply LRU eviction if at capacity
	if p.maxSize > 0 && len(p.cache) >= p.maxSize {
		p.evictLRU()
	}

	// Store in cache
	p.cache[key] = cachedEntry{
		config:    config,
		fetchedAt: time.Now(),
	}
	p.addToLRU(key)

	return config, nil
}

// recordHit records a cache hit with proper locking.
func (p *CachedConfigProvider) recordHit(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.recordHitLocked(key)
}

// recordHitLocked records a cache hit (must hold write lock).
func (p *CachedConfigProvider) recordHitLocked(key string) {
	p.stats.Hits++
	p.touchLRU(key)
	if p.onCacheHit != nil {
		p.onCacheHit(key)
	}
}

// recordMiss records a cache miss with proper locking.
func (p *CachedConfigProvider) recordMiss(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stats.Misses++
	if p.onCacheMiss != nil {
		p.onCacheMiss(key)
	}
}

// addToLRU adds a key to the LRU tracking (must hold write lock).
func (p *CachedConfigProvider) addToLRU(key string) {
	if p.maxSize == 0 {
		return // LRU disabled
	}
	p.lruList = append(p.lruList, key)
	p.lruIndex[key] = len(p.lruList) - 1
}

// touchLRU moves a key to the end of the LRU list (most recently used).
// Must hold write lock.
func (p *CachedConfigProvider) touchLRU(key string) {
	if p.maxSize == 0 {
		return // LRU disabled
	}
	idx, exists := p.lruIndex[key]
	if !exists {
		return
	}

	// Remove from current position
	p.lruList = append(p.lruList[:idx], p.lruList[idx+1:]...)

	// Update indices for shifted elements
	for i := idx; i < len(p.lruList); i++ {
		p.lruIndex[p.lruList[i]] = i
	}

	// Add to end (most recently used)
	p.lruList = append(p.lruList, key)
	p.lruIndex[key] = len(p.lruList) - 1
}

// removeFromLRU removes a key from LRU tracking (must hold write lock).
func (p *CachedConfigProvider) removeFromLRU(key string) {
	if p.maxSize == 0 {
		return // LRU disabled
	}
	idx, exists := p.lruIndex[key]
	if !exists {
		return
	}

	// Remove from list
	p.lruList = append(p.lruList[:idx], p.lruList[idx+1:]...)
	delete(p.lruIndex, key)

	// Update indices for shifted elements
	for i := idx; i < len(p.lruList); i++ {
		p.lruIndex[p.lruList[i]] = i
	}
}

// evictLRU removes the least recently used entry from the cache.
// Must hold write lock.
func (p *CachedConfigProvider) evictLRU() {
	if len(p.lruList) == 0 {
		return
	}

	// Remove oldest entry (first in list)
	oldestKey := p.lruList[0]
	p.lruList = p.lruList[1:]
	delete(p.lruIndex, oldestKey)
	delete(p.cache, oldestKey)
	p.stats.Evictions++

	// Update remaining indices
	for i, key := range p.lruList {
		p.lruIndex[key] = i
	}
}
