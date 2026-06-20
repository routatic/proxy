package transformer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestNewCtxReader_PassesThroughUncanceled(t *testing.T) {
	ctx := context.Background()
	in := strings.NewReader("hello world")
	r := NewCtxReader(ctx, in)

	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll err = %v, want nil", err)
	}
	if string(got) != "hello world" {
		t.Fatalf("ReadAll = %q, want %q", got, "hello world")
	}
}

func TestNewCtxReader_AbortsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled before the first Read

	r := NewCtxReader(ctx, strings.NewReader("anything"))
	buf := make([]byte, 16)
	n, err := r.Read(buf)
	if n != 0 {
		t.Fatalf("Read returned n=%d, want 0", n)
	}
	if !errors.Is(err, ErrStreamReadCanceled) {
		t.Fatalf("Read err = %v, want ErrStreamReadCanceled", err)
	}
}

func TestNewCtxReader_AbortsOnDeadlineExpiry(t *testing.T) {
	// 1ns deadline fires almost immediately.
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond) // ensure deadline has passed

	r := NewCtxReader(ctx, strings.NewReader("anything"))
	buf := make([]byte, 16)
	_, err := r.Read(buf)
	if !errors.Is(err, ErrStreamReadCanceled) {
		t.Fatalf("Read err = %v, want ErrStreamReadCanceled", err)
	}
}

func TestNewCtxReader_NilReaderReturnsNil(t *testing.T) {
	if got := NewCtxReader(context.Background(), nil); got != nil {
		t.Fatalf("NewCtxReader(nil) = %v, want nil", got)
	}
}

func TestNewCtxReadCloser_ClosesUnderlying(t *testing.T) {
	ctx := context.Background()
	br := &bufferReadCloser{Reader: bytes.NewReader([]byte("ok"))}
	rc := NewCtxReadCloser(ctx, br)
	if rc == nil {
		t.Fatal("NewCtxReadCloser returned nil")
	}

	// Underlying body is exposed via the underlying *bytes.Reader; close
	// should still flip the closer flag.
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll err = %v, want nil", err)
	}
	if string(got) != "ok" {
		t.Fatalf("ReadAll = %q, want %q", got, "ok")
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close err = %v, want nil", err)
	}
	if !br.closed {
		t.Fatal("underlying Close was not called")
	}
}

func TestNewCtxReadCloser_NilReturnsNil(t *testing.T) {
	if got := NewCtxReadCloser(context.Background(), nil); got != nil {
		t.Fatalf("NewCtxReadCloser(nil) = %v, want nil", got)
	}
}

type bufferReadCloser struct {
	io.Reader
	closed bool
}

func (b *bufferReadCloser) Close() error {
	b.closed = true
	return nil
}
