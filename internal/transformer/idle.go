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
// The watchdog goroutine exits when ctx is done (e.g., the stream completed or
// the caller cancelled the context). The caller MUST cancel ctx when the stream
// is finished to avoid leaking the goroutine.
//
// Pass idleTimeout <= 0 to disable the watchdog (the returned ping is a no-op).
//
// Typical usage:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	ping := StartIdleWatchdog(ctx, cancel, idleTimeout)
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
	timer := time.NewTimer(idleTimeout)

	go func() {
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				cancel()
				return
			}
		}
	}()

	return func() {
		mu.Lock()
		// Reset the timer on every ping so the deadline is always idleTimeout
		// from the most recent byte, not from the last timer fire.
		if !timer.Stop() {
			// Timer already fired; drain the channel to avoid a spurious wake.
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(idleTimeout)
		mu.Unlock()
	}
}
