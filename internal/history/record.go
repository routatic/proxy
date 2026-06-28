// Package history maintains an in-memory ring buffer of recent proxy requests.
package history

import "time"

// RequestRecord holds metadata for a single completed proxy request.
type RequestRecord struct {
	ID           string        // unique request ID
	Model        string        // actual upstream model used (e.g. "kimi-k2.6")
	Provider     string        // provider name (e.g. "opencode-go")
	Scenario     string        // routing scenario (e.g. "default", "complex")
	StartTime    time.Time     // when the request started
	Duration     time.Duration // total latency
	InputTokens  int           // input tokens from SSE usage event
	OutputTokens int           // output tokens from SSE usage event
	Streaming    bool          // whether this was a streaming request
	Success      bool          // whether it completed successfully
	ErrorMsg     string        // error message if failed
}
