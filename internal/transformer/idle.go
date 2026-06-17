// Package transformer handles request/response transformation and token counting.
package transformer

import (
	"context"
	"sync"
	"time"
)

// StartIdleWatchdog launches a goroutine that calls cancel() if no call to the
// returned ping function occurs within idleTimeout. The caller must invoke ping()
// after every successful byte read from the upstream stream.
//
// The watchdog stops when ctx is done (e.g., the stream completed or the caller
// cancelled the context).  Pass idleTimeout <= 0 to disable the watchdog (the
// returned ping is a no-op).
//
// Typical usage:
//
//	ping := StartIdleWatchdog(ctx, cancel, idleTimeout)
//	defer func() { cancel(); watchdogStop() }()   // no watchdogStop needed
//	// In the read loop:
//	n, err := body.Read(buf)
//	if n > 0 {
//	    ping()
//	    // process bytes
//	}
func StartIdleWatchdog(ctx context.Context, cancel context.CancelFunc, idleTimeout time.Duration) func() {
	if idleTimeout <= 0 {
		return func() {}
	}

	var mu sync.Mutex
	lastRead := time.Now()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(idleTimeout):
				mu.Lock()
				elapsed := time.Since(lastRead)
				mu.Unlock()
				if elapsed >= idleTimeout {
					cancel()
					return
				}
			}
		}
	}()

	return func() {
		mu.Lock()
		lastRead = time.Now()
		mu.Unlock()
	}
}
