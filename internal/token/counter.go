// Package token provides token counting utilities using tiktoken encoding.
package token

import (
	"container/list"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

// Counter handles token counting for text and message arrays.
type Counter struct {
	tiktoken *tiktoken.Tiktoken
	encoding string
	cacheMu  sync.Mutex
	cache    *tokenCache
}

const defaultCacheCapacity = 8192

type tokenCache struct {
	enabled  bool
	capacity int
	ll       *list.List
	items    map[[32]byte]*list.Element
}

type tokenCacheEntry struct {
	key   [32]byte
	count int
}

func newTokenCache(enabled bool, capacity int) *tokenCache {
	if capacity <= 0 {
		capacity = defaultCacheCapacity
	}
	return &tokenCache{
		enabled:  enabled,
		capacity: capacity,
		ll:       list.New(),
		items:    make(map[[32]byte]*list.Element, capacity),
	}
}

func (c *tokenCache) get(key [32]byte) (int, bool) {
	if !c.enabled {
		return 0, false
	}
	elem, ok := c.items[key]
	if !ok {
		return 0, false
	}
	c.ll.MoveToFront(elem)
	return elem.Value.(tokenCacheEntry).count, true
}

func (c *tokenCache) put(key [32]byte, count int) {
	if !c.enabled {
		return
	}
	if elem, ok := c.items[key]; ok {
		elem.Value = tokenCacheEntry{key: key, count: count}
		c.ll.MoveToFront(elem)
		return
	}
	elem := c.ll.PushFront(tokenCacheEntry{key: key, count: count})
	c.items[key] = elem
	if c.ll.Len() > c.capacity {
		oldest := c.ll.Back()
		if oldest != nil {
			c.ll.Remove(oldest)
			delete(c.items, oldest.Value.(tokenCacheEntry).key)
		}
	}
}

// defaultCacheDir returns a user-writable cache directory for tiktoken files.
// Uses TIKTOKEN_CACHE_DIR or DATA_GYM_CACHE_DIR if already set; otherwise
// defaults to ~/.cache/routatic-proxy/tiktoken to avoid /tmp permission issues.
func defaultCacheDir() string {
	if d := os.Getenv("TIKTOKEN_CACHE_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("DATA_GYM_CACHE_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "data-gym-cache")
	}
	return filepath.Join(home, ".cache", "routatic-proxy", "tiktoken")
}

// NewCounter creates a new token counter with cl100k_base encoding.
func NewCounter() (*Counter, error) {
	// Set process-wide env var before tiktoken loads any encoding files.
	// This is safe because NewCounter is called once at startup.
	_ = os.Setenv("TIKTOKEN_CACHE_DIR", defaultCacheDir())

	enc, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return nil, fmt.Errorf("failed to get encoding: %w", err)
	}
	return &Counter{
		tiktoken: enc,
		encoding: "cl100k_base",
		cache:    newTokenCache(true, defaultCacheCapacity),
	}, nil
}

// ConfigureCache atomically replaces the token count cache. Replacing the
// cache drops old entries so a config reload cannot retain stale state.
func (c *Counter) ConfigureCache(enabled bool, capacity int) {
	c.cacheMu.Lock()
	c.cache = newTokenCache(enabled, capacity)
	c.cacheMu.Unlock()
}

// CacheStats returns the current cache size, capacity, and hit/miss counters.
// It is intended for diagnostics and tests.
func (c *Counter) CacheStats() (size, capacity int, enabled bool) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if c.cache == nil {
		return 0, 0, false
	}
	return c.cache.ll.Len(), c.cache.capacity, c.cache.enabled
}

// CountTokens counts tokens in a string.
func (c *Counter) CountTokens(text string) (int, error) {
	encoding := c.encoding
	if encoding == "" {
		encoding = "unknown"
	}
	key := sha256.Sum256(append([]byte(encoding+"\x00"), []byte(text)...))
	c.cacheMu.Lock()
	if c.cache != nil {
		if count, ok := c.cache.get(key); ok {
			c.cacheMu.Unlock()
			return count, nil
		}
	}
	c.cacheMu.Unlock()

	tokens := c.tiktoken.Encode(text, nil, nil)
	count := len(tokens)

	c.cacheMu.Lock()
	if c.cache != nil {
		c.cache.put(key, count)
	}
	c.cacheMu.Unlock()
	return count, nil
}

// MessageContent represents a single message in a conversation.
type MessageContent struct {
	Role        string
	Content     string
	ExtraTokens int
}

// CountMessages counts tokens in a message array.
// Estimates tokens for system prompt + messages with formatting overhead.
func (c *Counter) CountMessages(system string, messages []MessageContent) (int, error) {
	// Base tokens for message formatting
	total := 3 // Start token

	if system != "" {
		sysTokens, err := c.CountTokens(system)
		if err != nil {
			return 0, err
		}
		total += sysTokens + 5 // System prompt overhead
	}

	for _, msg := range messages {
		msgTokens, err := c.CountTokens(msg.Content)
		if err != nil {
			return 0, err
		}
		total += msgTokens + 5 // Per-message overhead
		total += msg.ExtraTokens
	}

	return total, nil
}
