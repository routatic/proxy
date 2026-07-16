package transformer

import (
	"context"
	"errors"
	"io"
)

// ErrStreamReadCanceled is returned by ctxReader.Read when its context is canceled
// or its deadline expires.
var ErrStreamReadCanceled = errors.New("stream read canceled by context")

// ctxReader wraps an io.Reader and aborts Read when ctx is done.
//
// http.Client.Timeout is checked only at request start; once headers arrive, the
// body is streamed and the net/http transport does not enforce any further deadline.
// Without this wrapper, a slow upstream mid-stream can stall a streaming proxy
// forever. The wrapper does not preempt a read that is already blocked in the
// transport — it surfaces a context-canceled error on the next call.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

// NewCtxReader wraps r so that its next Read returns ErrStreamReadCanceled when
// ctx is canceled or its deadline expires.
func NewCtxReader(ctx context.Context, r io.Reader) io.Reader {
	if r == nil {
		return nil
	}
	return &ctxReader{ctx: ctx, r: r}
}

// NewCtxReadCloser is like NewCtxReader but preserves the io.Closer on the
// returned value. If rc is nil, returns nil.
func NewCtxReadCloser(ctx context.Context, rc io.ReadCloser) io.ReadCloser {
	if rc == nil {
		return nil
	}
	return &ctxReadCloser{
		ctxReader: ctxReader{ctx: ctx, r: rc},
		closer:    rc,
	}
}

type ctxReadCloser struct {
	ctxReader
	closer io.Closer
}

func (c *ctxReadCloser) Close() error {
	return c.closer.Close()
}

func (c *ctxReader) Read(p []byte) (int, error) {
	select {
	case <-c.ctx.Done():
		return 0, ErrStreamReadCanceled
	default:
	}

	n, err := c.r.Read(p)
	if n > 0 {
		// Data still valid even if the deadline fired mid-read; the next
		// Read will surface the cancellation.
		return n, err
	}

	select {
	case <-c.ctx.Done():
		return 0, ErrStreamReadCanceled
	default:
		return n, err
	}
}
