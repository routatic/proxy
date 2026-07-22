package gui

import (
	"bytes"
	"encoding/json"
	"io"
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

func TestHandleTestSend_MethodNotAllowed(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/test/send", nil)
	rr := httptest.NewRecorder()

	s.handleTestSend(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
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

func TestHandleTestSend_ForwardsRequest(t *testing.T) {
	const requestBody = `{"model":"test-model","messages":[{"role":"user","content":"hello"}]}`

	var gotMethod string
	var gotContentType string
	var gotBody []byte
	proxy := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for proxy: %v", err)
	}
	proxy.Listener = listener
	proxy.Start()
	defer proxy.Close()

	s := &Server{proxyPort: proxy.Listener.Addr().(*net.TCPAddr).Port}
	req := httptest.NewRequest(http.MethodPost, "/api/test/send", strings.NewReader(requestBody))
	rr := httptest.NewRecorder()

	s.handleTestSend(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("forwarded method = %q, want %q", gotMethod, http.MethodPost)
	}
	if gotContentType != "application/json" {
		t.Fatalf("forwarded content type = %q, want application/json", gotContentType)
	}
	if string(gotBody) != requestBody {
		t.Fatalf("forwarded body = %q, want %q", gotBody, requestBody)
	}
	if rr.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("response content type = %q, want application/json", rr.Header().Get("Content-Type"))
	}
	if rr.Body.String() != `{"ok":true}` {
		t.Fatalf("response body = %q, want %q", rr.Body.String(), `{"ok":true}`)
	}
}
