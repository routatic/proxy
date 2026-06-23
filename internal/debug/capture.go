// Package debug provides request/response capture functionality for debugging.
package debug

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// CaptureLogger handles async capture of request/response data for debugging.
// It uses a buffered channel and background worker to avoid blocking the main request flow.
type CaptureLogger struct {
	storage   *Storage
	enabled   bool
	entryChan chan CaptureEntry
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// NewCaptureLogger creates a new async capture logger.
// Returns nil if capture is not enabled or storage is nil.
// The logger starts a background worker goroutine that processes capture entries.
func NewCaptureLogger(storage *Storage, enabled bool) *CaptureLogger {
	if !enabled || storage == nil {
		return nil
	}

	cl := &CaptureLogger{
		storage:   storage,
		enabled:   enabled,
		entryChan: make(chan CaptureEntry, 100),
	}

	// Start background worker
	cl.wg.Add(1)
	go cl.worker()

	return cl
}

// worker is the background goroutine that processes capture entries.
// It reads from entryChan and writes to storage until the channel is closed.
func (c *CaptureLogger) worker() {
	defer c.wg.Done()

	for entry := range c.entryChan {
		if err := c.storage.WriteEntry(entry); err != nil {
			slog.Warn("failed to write capture entry",
				"error", err,
				"request_id", entry.RequestID,
				"phase", entry.Phase,
			)
		}
	}
}

// CaptureOriginal captures the original incoming request data.
// This is called before any transformation occurs.
// The capture is performed asynchronously via the background worker.
func (c *CaptureLogger) CaptureOriginal(requestID string, data []byte) {
	if c == nil || !c.enabled {
		return
	}

	entry := CaptureEntry{
		Timestamp: time.Now(),
		Phase:     PhaseOriginal,
		RequestID: requestID,
		Data:      redactIfNeeded(data, c.storage.config.RedactAPIKeys),
	}

	c.sendEntry(entry)
}

// CaptureNormalized captures the request after normalization to the internal format.
// The provider parameter indicates which provider this request is being routed to.
// The capture is performed asynchronously via the background worker.
func (c *CaptureLogger) CaptureNormalized(requestID string, provider string, data []byte) {
	if c == nil || !c.enabled {
		return
	}

	entry := CaptureEntry{
		Timestamp: time.Now(),
		Phase:     PhaseNormalized,
		Provider:  provider,
		RequestID: requestID,
		Data:      redactIfNeeded(data, c.storage.config.RedactAPIKeys),
	}

	c.sendEntry(entry)
}

// CaptureUpstreamRequest captures the request as sent to the upstream provider.
// This is after all transformations have been applied.
// The capture is performed asynchronously via the background worker.
func (c *CaptureLogger) CaptureUpstreamRequest(requestID string, provider string, data []byte) {
	if c == nil || !c.enabled {
		return
	}

	entry := CaptureEntry{
		Timestamp: time.Now(),
		Phase:     PhaseUpstreamRequest,
		Provider:  provider,
		RequestID: requestID,
		Data:      redactIfNeeded(data, c.storage.config.RedactAPIKeys),
	}

	c.sendEntry(entry)
}

// CaptureUpstreamResponse captures the raw response from the upstream provider.
// This is before any transformation back to the Anthropic format.
// The capture is performed asynchronously via the background worker.
func (c *CaptureLogger) CaptureUpstreamResponse(requestID string, provider string, data []byte) {
	if c == nil || !c.enabled {
		return
	}

	entry := CaptureEntry{
		Timestamp: time.Now(),
		Phase:     PhaseUpstreamResponse,
		Provider:  provider,
		RequestID: requestID,
		Data:      redactIfNeeded(data, c.storage.config.RedactAPIKeys),
	}

	c.sendEntry(entry)
}

// CaptureTransformed captures the final response after transformation to Anthropic format.
// This is the response that will be sent back to the client.
// The capture is performed asynchronously via the background worker.
func (c *CaptureLogger) CaptureTransformed(requestID string, provider string, data []byte) {
	if c == nil || !c.enabled {
		return
	}

	entry := CaptureEntry{
		Timestamp: time.Now(),
		Phase:     PhaseTransformed,
		Provider:  provider,
		RequestID: requestID,
		Data:      redactIfNeeded(data, c.storage.config.RedactAPIKeys),
	}

	c.sendEntry(entry)
}

// sendEntry sends an entry to the background worker via the buffered channel.
// It uses a non-blocking select with default to avoid blocking if the channel is full.
func (c *CaptureLogger) sendEntry(entry CaptureEntry) {
	select {
	case c.entryChan <- entry:
		// Entry queued successfully
	default:
		// Channel is full, log and drop the entry to avoid blocking
		slog.Warn("capture channel full, dropping entry",
			"request_id", entry.RequestID,
			"phase", entry.Phase,
		)
	}
}

// redactIfNeeded applies RedactSensitive to data if redaction is enabled.
// Returns the original data as json.RawMessage if redaction is disabled.
// Returns the redacted data as json.RawMessage if redaction is enabled.
func redactIfNeeded(data []byte, redactEnabled bool) json.RawMessage {
	if !redactEnabled {
		return json.RawMessage(data)
	}

	redacted := RedactSensitive(data)
	return json.RawMessage(redacted)
}

// Close shuts down the capture logger.
// It closes the entry channel and waits for the background worker to finish.
// Any entries still in the channel will be processed before Close returns.
// Safe to call multiple times - only the first call will have effect.
func (c *CaptureLogger) Close() error {
	if c == nil {
		return nil
	}

	c.closeOnce.Do(func() {
		// Close the channel to signal the worker to exit
		close(c.entryChan)

		// Wait for the worker to finish processing remaining entries
		c.wg.Wait()
	})

	return nil
}
