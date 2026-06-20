// Package provider implements the core.Provider interface for all supported
// upstream LLM providers.
package provider

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/routatic/proxy/internal/config"
)

// baseProvider holds shared HTTP transport and key rotation used by all
// provider implementations in this package.
type baseProvider struct {
	atomic     *config.AtomicConfig
	httpClient *http.Client
	keyCounter atomic.Uint64
}

// newBaseProvider creates a baseProvider with a shared HTTP transport tuned
// for high-concurrency upstream calls.
func newBaseProvider(atomic *config.AtomicConfig) baseProvider {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		MaxConnsPerHost:     50,
		DisableKeepAlives:   false,
		Proxy:               http.ProxyFromEnvironment,
	}
	return baseProvider{
		atomic: atomic,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

// nextAPIKey returns the next API key in round-robin order from the given pool.
func (b *baseProvider) nextAPIKey(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	n := uint64(len(keys))
	old := b.keyCounter.Add(1)
	return keys[(old-1)%n]
}
