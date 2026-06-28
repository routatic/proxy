// Package history maintains an in-memory ring buffer of recent proxy requests.
package history

import "sync"

const defaultMaxRecords = 1000

// History is a thread-safe ring buffer of RequestRecord entries.
// Uses head/tail indices for O(1) insert instead of O(n) slice shift.
type History struct {
	mu      sync.RWMutex
	records []RequestRecord
	head    int // write position
	count   int // number of records stored
	cap     int // max capacity
}

// New creates a History that retains at most maxRecords entries.
// If maxRecords is 0, the default of 1000 is used.
func New(maxRecords int) *History {
	if maxRecords <= 0 {
		maxRecords = defaultMaxRecords
	}
	return &History{
		records: make([]RequestRecord, maxRecords),
		cap:     maxRecords,
	}
}

// Add appends a record to the history. If the buffer is full, the oldest
// entry is evicted (ring-buffer behaviour). O(1) time complexity.
func (h *History) Add(r RequestRecord) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records[h.head] = r
	h.head = (h.head + 1) % h.cap
	if h.count < h.cap {
		h.count++
	}
}

// Last returns up to n most-recent records in newest-first order.
// If n <= 0 all records are returned.
func (h *History) Last(n int) []RequestRecord {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if n <= 0 || n > h.count {
		n = h.count
	}
	out := make([]RequestRecord, n)
	// Iterate backwards from head-1 (most recent) to head-n (oldest of the n).
	for i := 0; i < n; i++ {
		idx := (h.head - 1 - i + h.cap) % h.cap
		out[i] = h.records[idx]
	}
	return out
}

// Len returns the current number of records stored.
func (h *History) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.count
}
