package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   LogLevel  `json:"level"`
	Message string    `json:"message"`
	Field   string    `json:"field,omitempty"`
	Value   string    `json:"value,omitempty"`
}

type LogBuffer struct {
	mu      sync.RWMutex
	entries []LogEntry
	cap     int
	head    int
	count   int

	clients   map[chan LogEntry]struct{}
	clientsMu sync.RWMutex
}

func NewLogBuffer(cap int) *LogBuffer {
	if cap <= 0 {
		cap = 1000
	}
	return &LogBuffer{
		entries: make([]LogEntry, cap),
		cap:     cap,
		clients: make(map[chan LogEntry]struct{}),
	}
}

func (b *LogBuffer) Add(level LogLevel, msg string, field, value string) {
	b.mu.Lock()
	entry := LogEntry{
		Time:    time.Now(),
		Level:   level,
		Message: msg,
		Field:   field,
		Value:   value,
	}
	b.entries[b.head] = entry
	b.head = (b.head + 1) % b.cap
	if b.count < b.cap {
		b.count++
	}
	b.mu.Unlock()

	b.clientsMu.RLock()
	for ch := range b.clients {
		select {
		case ch <- entry:
		default:
		}
	}
	b.clientsMu.RUnlock()
}

func (b *LogBuffer) Last(n int) []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if n <= 0 || n > b.count {
		n = b.count
	}
	out := make([]LogEntry, n)
	for i := 0; i < n; i++ {
		idx := (b.head - 1 - i + b.cap) % b.cap
		out[i] = b.entries[idx]
	}
	return out
}

func (b *LogBuffer) Subscribe() chan LogEntry {
	ch := make(chan LogEntry, 100)
	b.clientsMu.Lock()
	b.clients[ch] = struct{}{}
	b.clientsMu.Unlock()
	return ch
}

func (b *LogBuffer) Unsubscribe(ch chan LogEntry) {
	b.clientsMu.Lock()
	delete(b.clients, ch)
	close(ch)
	b.clientsMu.Unlock()
}

func (b *LogBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.head = 0
	b.count = 0
}

type LogHandler struct {
	buffer *LogBuffer
	level  slog.Level
}

func NewLogHandler(buffer *LogBuffer, level slog.Level) *LogHandler {
	return &LogHandler{
		buffer: buffer,
		level:  level,
	}
}

func (h *LogHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (h *LogHandler) Handle(_ context.Context, r slog.Record) error {
	var level LogLevel
	switch {
	case r.Level >= slog.LevelError:
		level = LogLevelError
	case r.Level >= slog.LevelWarn:
		level = LogLevelWarn
	case r.Level >= slog.LevelInfo:
		level = LogLevelInfo
	default:
		level = LogLevelDebug
	}

	h.buffer.Add(level, r.Message, "", "")
	return nil
}

func (h *LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *LogHandler) WithGroup(name string) slog.Handler {
	return h
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, data any) {
	jsonData, _ := json.Marshal(data)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, jsonData)
	flusher.Flush()
}
