package handlers

import (
	"time"

	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/storage"
)

type StorageWriter interface {
	InsertRequest(rec history.RequestRecord) error
	InsertLatency(model string, latency time.Duration) error
}

type StorageAdapter struct {
	requests *storage.Requests
	latency  *storage.Latency
}

func NewStorageAdapter(db *storage.Database) *StorageAdapter {
	return &StorageAdapter{
		requests: storage.NewRequests(db),
		latency:  storage.NewLatency(db),
	}
}

func (s *StorageAdapter) InsertRequest(rec history.RequestRecord) error {
	return s.requests.Insert(rec)
}

func (s *StorageAdapter) InsertLatency(model string, latency time.Duration) error {
	return s.latency.Insert(model, latency)
}
