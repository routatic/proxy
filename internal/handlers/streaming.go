package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/transformer"
)

// StreamProxy handles SSE stream forwarding from various upstream wire formats
// to Anthropic-format SSE events. It wraps transformer.StreamHandler and
// dispatches by WireFormat.
type StreamProxy struct {
	handler *transformer.StreamHandler
}

// NewStreamProxy creates a new StreamProxy.
func NewStreamProxy() *StreamProxy {
	return &StreamProxy{
		handler: transformer.NewStreamHandler(),
	}
}

// ProxyStream proxies an upstream SSE stream to the response writer, transforming
// events from the wire format to Anthropic SSE events.
func (sp *StreamProxy) ProxyStream(
	w http.ResponseWriter,
	body io.ReadCloser,
	wireFormat core.WireFormat,
	modelID string,
	clientCtx context.Context,
	idleTimeout time.Duration,
	cancel context.CancelFunc,
) error {
	switch wireFormat {
	case core.WireFormatAnthropic:
		return sp.proxyAnthropicPassthroughStream(w, body, idleTimeout, clientCtx, cancel)
	case core.WireFormatOpenAIResponses:
		return sp.handler.ProxyResponsesStream(w, body, modelID, clientCtx, idleTimeout, cancel)
	case core.WireFormatGemini:
		return sp.handler.ProxyGeminiStream(w, body, modelID, clientCtx, idleTimeout, cancel)
	default:
		return sp.proxyOpenAIStream(w, body, modelID, clientCtx, idleTimeout, cancel)
	}
}

// proxyOpenAIStream delegates to the transformer's ProxyStream.
func (sp *StreamProxy) proxyOpenAIStream(
	w http.ResponseWriter,
	body io.ReadCloser,
	modelID string,
	clientCtx context.Context,
	idleTimeout time.Duration,
	cancel context.CancelFunc,
) error {
	return sp.handler.ProxyStream(w, body, modelID, clientCtx, idleTimeout, cancel)
}

// proxyAnthropicPassthroughStream forwards raw Anthropic SSE bytes directly to
// the client, with an idle watchdog. No transformation is needed since the
// upstream already speaks Anthropic format.
func (sp *StreamProxy) proxyAnthropicPassthroughStream(
	w http.ResponseWriter,
	body io.ReadCloser,
	idleTimeout time.Duration,
	clientCtx context.Context,
	cancel context.CancelFunc,
) error {
	defer func() { _ = body.Close() }()
	defer cancel()

	buf := make([]byte, 4096)
	ping := transformer.StartIdleWatchdog(clientCtx, cancel, idleTimeout)
	for {
		select {
		case <-clientCtx.Done():
			if clientCtx.Err() == nil {
				return transformer.ErrStreamIdle
			}
			return transformer.ErrClientDisconnected
		default:
		}
		n, rerr := body.Read(buf)
		if n > 0 {
			ping()
			if _, werr := w.Write(buf[:n]); werr != nil {
				return transformer.ErrClientDisconnected
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			if transformer.IsIdleTimeout(rerr) {
				return transformer.ErrStreamIdle
			}
			if errors.Is(rerr, context.Canceled) || errors.Is(rerr, transformer.ErrStreamReadCanceled) || clientCtx.Err() == context.Canceled {
				if clientCtx.Err() == nil {
					return transformer.ErrStreamIdle
				}
				return transformer.ErrClientDisconnected
			}
			return fmt.Errorf("failed to copy response: %w", rerr)
		}
	}
}
