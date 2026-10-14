// Package provider implements the core.Provider interface for all supported
// upstream LLM providers.
package provider

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
)

const routaticUserAgent = "routatic-proxy"

// baseProvider holds shared HTTP transport and key rotation used by all
// provider implementations in this package.
type baseProvider struct {
	atomic     *config.AtomicConfig
	httpClient *http.Client
	keyCounter atomic.Uint64
}

func validateRequest(p core.Provider, req *core.NormalizedRequest, model config.ModelConfig) error {
	caps, ok := p.ModelCapabilities(model.ModelID)
	if !ok {
		return &core.CompatibilityError{Provider: p.Name(), ModelID: model.ModelID, Reason: "model is not known by provider"}
	}
	return core.ValidateRequestCompatibility(req, model, caps, p.WireFormat(model))
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

// applyHeaders applies the configurable User-Agent override and any custom
// request headers to an outgoing upstream request. An empty UserAgent keeps
// the provider's default (defaultUserAgent); a nil/empty Headers map is a
// no-op. Custom headers are applied last so they can override UA and any
// previously set header.
func applyHeaders(httpReq *http.Request, hc config.HeadersConfig, defaultUserAgent string) {
	ua := hc.UserAgent
	if ua == "" {
		ua = defaultUserAgent
	}
	httpReq.Header.Set("User-Agent", ua)
	for name, value := range hc.Headers {
		httpReq.Header.Set(name, value)
	}
}

const (
	openCodeClientHeader  = "x-opencode-client"
	openCodeProjectHeader = "x-opencode-project"
	openCodeRequestHeader = "x-opencode-request"
	openCodeClientValue   = "desktop"
	openCodeProjectValue  = "global"
	base62Alphabet        = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

// setOpenCodeFingerprint stamps the identity headers the native OpenCode CLI
// sends, so upstream gating treats proxied traffic as a first-party client.
// A per-request x-opencode-request id is generated; config headers applied
// afterwards can override any of these.
func setOpenCodeFingerprint(h http.Header) {
	h.Set(openCodeClientHeader, openCodeClientValue)
	h.Set(openCodeProjectHeader, openCodeProjectValue)
	h.Set(openCodeRequestHeader, newOpenCodeRequestID())
}

// newOpenCodeRequestID returns a "msg_" id matching the native client's
// shape: the prefix, 12 lowercase hex chars, then 14 base62 chars.
func newOpenCodeRequestID() string {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "msg_00000000000000000000000000"
	}
	var b strings.Builder
	b.Grow(len("msg_") + 26)
	b.WriteString("msg_")
	b.WriteString(hex.EncodeToString(raw[:6]))
	for _, x := range raw[6:] {
		b.WriteByte(base62Alphabet[int(x)%len(base62Alphabet)])
	}
	return b.String()
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
