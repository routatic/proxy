package client

import (
	"sync"
	"time"
)

// quotaCacheTTL mirrors CCS quota-response-cache.ts DEFAULT_CACHE_TTL_MS.
const quotaCacheTTL = 2 * time.Minute

// QuotaCache is a thread-safe TTL cache for upstream quota / usage lookups.
// Modeled after CCS quota-response-cache.ts.
type QuotaCache struct {
	mu    sync.RWMutex
	items map[string]quotaCacheEntry
}

type quotaCacheEntry struct {
	data      []byte
	cachedAt  time.Time
}

func NewQuotaCache() *QuotaCache {
	return &QuotaCache{items: make(map[string]quotaCacheEntry)}
}

// Get returns cached data if still valid.
func (c *QuotaCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	ent, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Since(ent.cachedAt) > quotaCacheTTL {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return nil, false
	}
	return ent.data, true
}

// Set stores data with current timestamp.
func (c *QuotaCache) Set(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = quotaCacheEntry{data: data, cachedAt: time.Now()}
}

// Evict removes a specific key.
func (c *QuotaCache) Evict(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}
