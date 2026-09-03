package core

import (
	"context"
	"net/http"
)

// OpenCodeSessionHeader is the header sent to OpenCode Go upstreams identifying
// the conversation. Its value is the Claude Code conversation UUID verbatim.
const OpenCodeSessionHeader = "x-opencode-session"

type sessionIDKey struct{}

// WithSessionID returns a context carrying the OpenCode session ID.
func WithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, id)
}

// SessionIDFromContext returns the OpenCode session ID carried by ctx, or ""
// when the context has none.
func SessionIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(sessionIDKey{}).(string)
	return id
}

// SetOpenCodeSessionHeader sets the OpenCode session header on h when ctx
// carries a session ID. Uses Set (never Add) so a reused header map cannot
// accumulate values.
func SetOpenCodeSessionHeader(h http.Header, ctx context.Context) {
	if id := SessionIDFromContext(ctx); id != "" {
		h.Set(OpenCodeSessionHeader, id)
	}
}
