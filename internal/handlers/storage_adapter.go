package handlers

import (
	"context"
	"log/slog"
	"sync"

	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/storage"
)

type StorageWriter interface {
	RecordCompletion(rec history.RequestRecord)
	Shutdown(ctx context.Context) error
}

type StorageAdapter struct {
	requests  *storage.Requests
	metrics   *metrics.Metrics
	queue     chan history.RequestRecord
	stop      chan struct{}
	stopOnce  sync.Once
	closeOnce sync.Once
	queueMu   sync.RWMutex
	closed    bool
	wg        sync.WaitGroup
}

func NewStorageAdapter(db *storage.Database, metricSink *metrics.Metrics) *StorageAdapter {
	s := &StorageAdapter{
		requests: storage.NewRequests(db),
		metrics:  metricSink,
		queue:    make(chan history.RequestRecord, 1024),
		stop:     make(chan struct{}),
	}
	s.wg.Add(1)
	go s.run()
	return s
}

func (s *StorageAdapter) run() {
	defer s.wg.Done()
	for {
		select {
		case rec, ok := <-s.queue:
			if !ok {
				return
			}
			if err := s.requests.Insert(rec); err != nil {
				slog.Warn("failed to persist request completion", "request_id", rec.ID, "error", err)
			}
		case <-s.stop:
			s.drainBuffered()
			return
		}
	}
}

// RecordCompletion enqueues a completed request without blocking the request
// path. If the bounded queue is full, the newest record is dropped.
func (s *StorageAdapter) RecordCompletion(rec history.RequestRecord) {
	s.queueMu.RLock()
	defer s.queueMu.RUnlock()
	if s.closed {
		return
	}
	select {
	case s.queue <- rec:
	default:
		if s.metrics != nil {
			s.metrics.RecordStorageDrop()
		}
		slog.Warn("storage completion queue full; dropping newest record", "request_id", rec.ID)
	}
}

func (s *StorageAdapter) drainBuffered() {
	for {
		select {
		case rec, ok := <-s.queue:
			if !ok {
				return
			}
			if err := s.requests.Insert(rec); err != nil {
				slog.Warn("failed to persist request completion", "request_id", rec.ID, "error", err)
			}
		default:
			return
		}
	}
}

// Shutdown stops accepting completions and drains accepted records until ctx
// expires.
func (s *StorageAdapter) Shutdown(ctx context.Context) error {
	s.closeOnce.Do(func() {
		s.queueMu.Lock()
		s.closed = true
		close(s.queue)
		s.queueMu.Unlock()
	})
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		s.stopOnce.Do(func() { close(s.stop) })
		s.wg.Wait()
		return ctx.Err()
	}
}
