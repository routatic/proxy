package status

import (
	"sync"
	"time"

	"github.com/routatic/proxy/internal/buildinfo"
	"github.com/routatic/proxy/internal/router"
)

type Snapshot struct {
	SchemaVersion int             `json:"schema_version"`
	UpdatedAt     string          `json:"updated_at"`
	AgeMS         int64           `json:"age_ms"`
	Source        string          `json:"source"`
	Stale         bool            `json:"stale"`
	Proxy         ProxySnapshot   `json:"proxy"`
	Request       RequestSnapshot `json:"request"`
	Routing       RoutingSnapshot `json:"routing"`
	Context       ContextSnapshot `json:"context"`
	Models        ModelsSnapshot  `json:"models"`
}

type ProxySnapshot struct {
	Version string `json:"version"`
	PID     int    `json:"pid"`
	Binary  string `json:"binary"`
}

type RequestSnapshot struct {
	RequestID string `json:"request_id"`
	Streaming bool   `json:"streaming"`
}

type RoutingSnapshot struct {
	Scenario string `json:"scenario"`
	ModelID  string `json:"model_id"`
}

type ContextSnapshot struct {
	InputTokens int `json:"input_tokens"`
	MaxTokens   int `json:"max_tokens"`
	Percent     int `json:"pct"`
}

type ModelsSnapshot struct {
	SkippedFallbacks []router.SkippedModel `json:"skipped_fallbacks,omitempty"`
}

type Store struct {
	mu       sync.RWMutex
	seq      uint64
	updated  time.Time
	snapshot Snapshot
	ttl      time.Duration
}

func NewStore(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = 10 * time.Second
	}
	return &Store{ttl: ttl}
}

func (s *Store) Update(seq uint64, snap Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if seq < s.seq {
		return
	}
	now := time.Now().UTC()
	s.seq = seq
	s.updated = now
	snap.SchemaVersion = 1
	snap.UpdatedAt = now.Format(time.RFC3339Nano)
	snap.AgeMS = 0
	snap.Source = "proxy"
	snap.Proxy = ProxySnapshot{
		Version: buildinfo.Version,
		PID:     buildinfo.PID(),
		Binary:  buildinfo.BinaryPath(),
	}
	s.snapshot = snap
}

func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap := s.snapshot

	// Deep-copy the SkippedFallbacks slice to avoid sharing the backing
	// array with a concurrent Update().
	if len(snap.Models.SkippedFallbacks) > 0 {
		skipped := make([]router.SkippedModel, len(snap.Models.SkippedFallbacks))
		copy(skipped, snap.Models.SkippedFallbacks)
		snap.Models.SkippedFallbacks = skipped
	}

	if s.updated.IsZero() {
		snap.SchemaVersion = 1
		snap.Source = "empty"
		snap.Stale = true
		snap.Proxy = ProxySnapshot{
			Version: buildinfo.Version,
			PID:     buildinfo.PID(),
			Binary:  buildinfo.BinaryPath(),
		}
		return snap
	}
	age := time.Since(s.updated)
	snap.AgeMS = age.Milliseconds()
	snap.Stale = age > s.ttl
	return snap
}
