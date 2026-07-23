package gui

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/catalog"
)

func TestHandleCatalogLock_NotSynced(t *testing.T) {
	s := &Server{catalogDir: t.TempDir()}

	req := httptest.NewRequest(http.MethodGet, "/api/catalog/lock", nil)
	rr := httptest.NewRecorder()
	s.handleCatalogLock(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp catalogLockResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Synced {
		t.Fatalf("synced = true, want false")
	}
	if resp.AgeSeconds != -1 {
		t.Fatalf("age_seconds = %d, want -1", resp.AgeSeconds)
	}
	if resp.SyncedAt != nil {
		t.Fatalf("synced_at unexpectedly set")
	}
}

func TestHandleCatalogLock_Synced(t *testing.T) {
	dir := t.TempDir()
	lock := &catalog.Lock{
		SourceURL: "https://example.com/catalog.json",
		SyncedAt:  time.Now().UTC().Add(-2 * time.Hour),
		SHA256:    "abc123",
		Bytes:     1234,
		TTLHours:  24,
	}
	if err := catalog.WriteLock(dir, lock); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	s := &Server{catalogDir: dir}
	req := httptest.NewRequest(http.MethodGet, "/api/catalog/lock", nil)
	rr := httptest.NewRecorder()
	s.handleCatalogLock(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp catalogLockResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Synced {
		t.Fatalf("synced = false, want true")
	}
	if resp.SHA256 != lock.SHA256 {
		t.Fatalf("sha256 = %q, want %q", resp.SHA256, lock.SHA256)
	}
	if resp.Bytes != lock.Bytes {
		t.Fatalf("bytes = %d, want %d", resp.Bytes, lock.Bytes)
	}
	if resp.TTLHours != lock.TTLHours {
		t.Fatalf("ttl_hours = %d, want %d", resp.TTLHours, lock.TTLHours)
	}
	if resp.AgeSeconds < 7199 || resp.AgeSeconds > 7201 {
		t.Fatalf("age_seconds = %d, want ~7200", resp.AgeSeconds)
	}
}

func TestHandleCatalogSync_NotConfigured(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodPost, "/api/catalog/sync", nil)
	rr := httptest.NewRecorder()
	s.handleCatalogSync(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleCatalogSync_Success(t *testing.T) {
	body := `{"models":{"openai/gpt-4":{"id":"openai/gpt-4","name":"GPT-4"}},"providers":{"openai":{}}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	dir := t.TempDir()
	s := &Server{catalogSourceURL: server.URL + "/catalog.json", catalogDir: dir}

	req := httptest.NewRequest(http.MethodPost, "/api/catalog/sync", nil)
	rr := httptest.NewRecorder()
	s.handleCatalogSync(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp catalogLockResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Synced {
		t.Fatalf("synced = false, want true")
	}
	if resp.Bytes != int64(len(body)) {
		t.Fatalf("bytes = %d, want %d", resp.Bytes, len(body))
	}
	if resp.TTLHours != 24 {
		t.Fatalf("ttl_hours = %d, want 24", resp.TTLHours)
	}

	// Verify the catalog and lock were actually written.
	if _, err := os.Stat(filepath.Join(dir, "catalog.json")); err != nil {
		t.Fatalf("catalog.json not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "catalog.lock.json")); err != nil {
		t.Fatalf("catalog.lock.json not written: %v", err)
	}
}

func TestHandleCatalogSync_UpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	s := &Server{catalogSourceURL: server.URL + "/catalog.json", catalogDir: t.TempDir()}

	req := httptest.NewRequest(http.MethodPost, "/api/catalog/sync", nil)
	rr := httptest.NewRecorder()
	s.handleCatalogSync(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHandleTestSend_RequestBodyTooLarge(t *testing.T) {
	s := &Server{proxyPort: 1}
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/test/send",
		bytes.NewReader(bytes.Repeat([]byte("x"), maxTestRequestBody+1)),
	)
	rr := httptest.NewRecorder()

	s.handleTestSend(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestHandleTestSend(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		setup      func(t *testing.T) (*Server, func())
		writer     func() http.ResponseWriter
		wantStatus int
		wantBody   string
	}{
		{
			name:       "method not allowed",
			method:     http.MethodGet,
			setup:      func(t *testing.T) (*Server, func()) { return &Server{}, func() {} },
			writer:     func() http.ResponseWriter { return httptest.NewRecorder() },
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   "method not allowed\n",
		},
		{
			name:   "successful post",
			method: http.MethodPost,
			setup: func(t *testing.T) (*Server, func()) {
				return startTestSendProxy(t, `{"ok":true}`)
			},
			writer:     func() http.ResponseWriter { return httptest.NewRecorder() },
			wantStatus: http.StatusOK,
			wantBody:   `{"ok":true}`,
		},
		{
			name:   "proxy connection failure",
			method: http.MethodPost,
			setup: func(t *testing.T) (*Server, func()) {
				listener, err := net.Listen("tcp4", "127.0.0.1:0")
				if err != nil {
					t.Fatalf("reserve proxy port: %v", err)
				}
				port := listener.Addr().(*net.TCPAddr).Port
				_ = listener.Close()
				return &Server{proxyPort: port}, func() {}
			},
			writer:     func() http.ResponseWriter { return httptest.NewRecorder() },
			wantStatus: http.StatusBadGateway,
			wantBody:   "proxy request failed:",
		},
		{
			name:   "client disconnect during stream",
			method: http.MethodPost,
			setup: func(t *testing.T) (*Server, func()) {
				return startTestSendProxy(t, strings.Repeat("x", 128<<10))
			},
			writer:     func() http.ResponseWriter { return &failingResponseWriter{} },
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, cleanup := tt.setup(t)
			defer cleanup()
			s.logger = slog.New(slog.NewTextHandler(io.Discard, nil))

			req := httptest.NewRequest(tt.method, "/api/test/send", strings.NewReader(`{"prompt":"hello"}`))
			w := tt.writer()
			s.handleTestSend(w, req)

			if got := responseStatus(w); got != tt.wantStatus {
				t.Fatalf("status = %d, want %d", got, tt.wantStatus)
			}
			if tt.wantBody != "" && !strings.Contains(responseBody(w), tt.wantBody) {
				t.Fatalf("body = %q, want it to contain %q", responseBody(w), tt.wantBody)
			}
		})
	}
}

func startTestSendProxy(t *testing.T, responseBody string) (*Server, func()) {
	t.Helper()

	proxy := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responseBody))
	}))
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for proxy: %v", err)
	}
	proxy.Listener = listener
	proxy.Start()

	return &Server{proxyPort: proxy.Listener.Addr().(*net.TCPAddr).Port}, proxy.Close
}

type failingResponseWriter struct {
	header http.Header
	status int
	writes int
}

func (w *failingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *failingResponseWriter) WriteHeader(status int) {
	w.status = status
}

func (w *failingResponseWriter) Write(p []byte) (int, error) {
	if w.writes > 0 {
		return 0, io.ErrClosedPipe
	}
	w.writes++
	return len(p), nil
}

func responseStatus(w http.ResponseWriter) int {
	switch w := w.(type) {
	case *httptest.ResponseRecorder:
		return w.Code
	case *failingResponseWriter:
		return w.status
	default:
		panic("unsupported response writer")
	}
}

func responseBody(w http.ResponseWriter) string {
	if recorder, ok := w.(*httptest.ResponseRecorder); ok {
		return recorder.Body.String()
	}
	return ""
}
