package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	catalogFileName = "catalog.json"
	tmpFileName     = "catalog.json.tmp"
	maxCatalogBytes = 50 << 20 // 50 MiB
	defaultTTLHours = 24
)

// envelope validates that the top-level catalog JSON contains the expected
// models and providers objects.
type envelope struct {
	Models    map[string]json.RawMessage `json:"models"`
	Providers map[string]json.RawMessage `json:"providers"`
}

// Sync downloads the models.dev catalog from sourceURL, validates its shape,
// writes it atomically to destDir/catalog.json, and persists a lock file.
func Sync(sourceURL, destDir string) (*Lock, error) {
	if sourceURL == "" {
		return nil, fmt.Errorf("source URL is required")
	}
	if destDir == "" {
		return nil, fmt.Errorf("destination directory is required")
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("create destination directory: %w", err)
	}

	req, err := http.NewRequest(http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch catalog: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status: %d", resp.StatusCode)
	}

	limited := http.MaxBytesReader(nil, resp.Body, maxCatalogBytes)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	if env.Models == nil || env.Providers == nil {
		return nil, fmt.Errorf("catalog must contain models and providers objects")
	}

	var catalog Catalog
	if err := json.Unmarshal(body, &catalog); err != nil {
		return nil, fmt.Errorf("parse catalog contents: %w", err)
	}

	idx, err := BuildProviderIndex(catalog)
	if err != nil {
		return nil, fmt.Errorf("build provider index: %w", err)
	}

	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])

	tmpPath := filepath.Join(destDir, tmpFileName)
	if err := os.WriteFile(tmpPath, body, 0644); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("write catalog temp file: %w", err)
	}

	finalPath := filepath.Join(destDir, catalogFileName)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("rename catalog file: %w", err)
	}

	if err := idx.Write(destDir); err != nil {
		_ = os.Remove(tmpPath)
		_ = os.Remove(filepath.Join(destDir, indexTmpFileName))
		_ = os.Remove(filepath.Join(destDir, indexFileName))
		return nil, fmt.Errorf("write provider index: %w", err)
	}

	lock := &Lock{
		SourceURL: sourceURL,
		SyncedAt:  time.Now().UTC(),
		SHA256:    hash,
		Bytes:     int64(len(body)),
		TTLHours:  defaultTTLHours,
	}

	if err := WriteLock(destDir, lock); err != nil {
		return nil, fmt.Errorf("write lock: %w", err)
	}

	return lock, nil
}
