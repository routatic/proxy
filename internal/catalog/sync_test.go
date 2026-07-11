package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSync(t *testing.T) {
	validCatalog := `{"models":{"openai/gpt-4":{"id":"openai/gpt-4","name":"gpt-4"}},"providers":{"openai":{}}}`
	validHash := sha256.Sum256([]byte(validCatalog))

	cases := []struct {
		name        string
		body        string
		wantErr     bool
		wantLock    bool
		wantCatalog bool
	}{
		{
			name:        "successful sync writes catalog and lock",
			body:        validCatalog,
			wantErr:     false,
			wantLock:    true,
			wantCatalog: true,
		},
		{
			name:        "missing providers object returns error and leaves no catalog",
			body:        `{"models":{"openai/gpt-4":{"id":"openai/gpt-4"}}}`,
			wantErr:     true,
			wantLock:    false,
			wantCatalog: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			destDir := t.TempDir()
			lock, err := Sync(server.URL, destDir)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if tc.wantLock && lock == nil {
				t.Fatalf("expected non-nil lock")
			}
			if !tc.wantLock && lock != nil {
				t.Fatalf("expected nil lock, got %+v", lock)
			}

			catalogPath := filepath.Join(destDir, catalogFileName)
			if tc.wantCatalog {
				data, err := os.ReadFile(catalogPath)
				if err != nil {
					t.Fatalf("expected catalog file: %v", err)
				}
				if string(data) != tc.body {
					t.Fatalf("catalog content mismatch: got %q, want %q", string(data), tc.body)
				}

				indexPath := filepath.Join(destDir, indexFileName)
				if _, err := os.Stat(indexPath); err != nil {
					t.Fatalf("expected index file: %v", err)
				}
			} else {
				if _, err := os.Stat(catalogPath); !os.IsNotExist(err) {
					t.Fatalf("expected no catalog file, got %v", err)
				}
				if _, err := os.Stat(filepath.Join(destDir, indexFileName)); !os.IsNotExist(err) {
					t.Fatalf("expected no index file, got %v", err)
				}
			}

			lockPath := filepath.Join(destDir, lockFileName)
			if tc.wantLock {
				read, err := ReadLock(destDir)
				if err != nil {
					t.Fatalf("expected readable lock: %v", err)
				}
				if read.SourceURL != server.URL {
					t.Fatalf("lock source URL mismatch: got %q, want %q", read.SourceURL, server.URL)
				}
				if read.SHA256 != hex.EncodeToString(validHash[:]) {
					t.Fatalf("lock SHA256 mismatch: got %q, want %q", read.SHA256, hex.EncodeToString(validHash[:]))
				}
				if read.Bytes != int64(len(tc.body)) {
					t.Fatalf("lock bytes mismatch: got %d, want %d", read.Bytes, len(tc.body))
				}
				if read.TTLHours != defaultTTLHours {
					t.Fatalf("lock TTL mismatch: got %d, want %d", read.TTLHours, defaultTTLHours)
				}
			} else {
				if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
					t.Fatalf("expected no lock file, got %v", err)
				}
			}

			tmpPath := filepath.Join(destDir, tmpFileName)
			if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
				t.Fatalf("expected no temp file left behind, got %v", err)
			}
		})
	}
}

func TestSyncOversized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		prefix := []byte(`{"models":{`)
		_, _ = w.Write(prefix)
		padding := strings.Repeat("0", maxCatalogBytes+1)
		_, _ = w.Write([]byte(padding))
		_, _ = w.Write([]byte(`},"providers":{}}`))
	}))
	defer server.Close()

	destDir := t.TempDir()
	_, err := Sync(server.URL, destDir)
	if err == nil {
		t.Fatalf("expected error for oversized response, got nil")
	}

	for _, name := range []string{catalogFileName, tmpFileName, lockFileName, indexFileName, indexTmpFileName} {
		path := filepath.Join(destDir, name)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected no %s after oversized sync failure, got %v", name, err)
		}
	}
}

func TestSyncNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer server.Close()

	destDir := t.TempDir()
	_, err := Sync(server.URL, destDir)
	if err == nil {
		t.Fatalf("expected error for non-OK status, got nil")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("%d", http.StatusInternalServerError)) {
		t.Fatalf("expected status in error, got %v", err)
	}
}

func TestSyncMissingModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"providers":{"openai":{}}}`))
	}))
	defer server.Close()

	destDir := t.TempDir()
	_, err := Sync(server.URL, destDir)
	if err == nil {
		t.Fatalf("expected error for missing models object, got nil")
	}
	if _, err := os.Stat(filepath.Join(destDir, catalogFileName)); !os.IsNotExist(err) {
		t.Fatalf("expected no catalog file, got %v", err)
	}
}

func TestSyncCreatesDestDir(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":{"y/x":{"id":"y/x","name":"x"}},"providers":{"y":{}}}`))
	}))
	defer server.Close()

	base := t.TempDir()
	destDir := filepath.Join(base, "nested", "catalog")
	_, err := Sync(server.URL, destDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, catalogFileName)); err != nil {
		t.Fatalf("expected catalog file: %v", err)
	}
}
