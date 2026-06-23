// Package debug provides request/response capture functionality for debugging.
package debug

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/config"
)

// Storage manages debug capture file storage with rotation.
type Storage struct {
	config      config.DebugCapture
	mu          sync.Mutex
	currentFile *os.File
	currentSize int64
	fileCount   int
}

// NewStorage creates a new debug storage manager.
// It ensures the debug directory exists and scans existing files to set initial fileCount.
func NewStorage(cfg config.DebugCapture) (*Storage, error) {
	s := &Storage{
		config: cfg,
	}

	if !cfg.Enabled {
		return s, nil
	}

	if err := s.ensureDirectory(); err != nil {
		return nil, err
	}

	if err := s.scanExistingFiles(); err != nil {
		return nil, err
	}

	return s, nil
}

// WriteEntry marshals the entry to JSON and writes it to the current file.
// It handles file rotation when MaxFileSize is exceeded and deletes oldest
// files when MaxFiles is exceeded. Thread-safe via mutex.
func (s *Storage) WriteEntry(entry CaptureEntry) error {
	if !s.config.Enabled {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Marshal entry to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling capture entry: %w", err)
	}

	// Append newline
	data = append(data, '\n')

	// Check if we need to rotate before writing
	if s.currentFile != nil && s.currentSize+int64(len(data)) > s.config.MaxFileSize {
		if err := s.rotateFile(); err != nil {
			return err
		}
	}

	// Create new file if none exists
	if s.currentFile == nil {
		if err := s.rotateFile(); err != nil {
			return err
		}
	}

	// Write data
	n, err := s.currentFile.Write(data)
	if err != nil {
		return fmt.Errorf("writing capture entry: %w", err)
	}

	s.currentSize += int64(n)

	// Sync to ensure data is persisted
	if err := s.currentFile.Sync(); err != nil {
		return fmt.Errorf("syncing capture file: %w", err)
	}

	return nil
}

// ensureDirectory creates the debug directory if it doesn't exist.
func (s *Storage) ensureDirectory() error {
	if s.config.Directory == "" {
		s.config.Directory = "~/.config/routatic-proxy/debug"
	}

	dir := expandHome(s.config.Directory)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating debug directory %s: %w", dir, err)
	}

	return nil
}

// rotateFile closes the current file and creates a new one.
func (s *Storage) rotateFile() error {
	// Close existing file
	if s.currentFile != nil {
		if err := s.currentFile.Close(); err != nil {
			return fmt.Errorf("closing capture file: %w", err)
		}
		s.fileCount++
	}

	// Check if we need to delete oldest files
	if s.config.MaxFiles > 0 && s.fileCount >= s.config.MaxFiles {
		if err := s.deleteOldestFiles(); err != nil {
			return err
		}
	}

	// Generate new filename
	filename := s.generateFilename()
	filepath := filepath.Join(expandHome(s.config.Directory), filename)

	// Create new file
	f, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("creating capture file %s: %w", filepath, err)
	}

	s.currentFile = f
	s.currentSize = 0

	return nil
}

// deleteOldestFiles removes the oldest debug files to keep within MaxFiles limit.
func (s *Storage) deleteOldestFiles() error {
	dir := expandHome(s.config.Directory)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading debug directory: %w", err)
	}

	// Collect debug files with their modification times
	type fileInfo struct {
		name    string
		modTime time.Time
	}

	var files []fileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "capture-") && strings.HasSuffix(name, ".jsonl") {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			files = append(files, fileInfo{name: name, modTime: info.ModTime()})
		}
	}

	// Sort by modification time (oldest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	// Delete oldest files to make room
	filesToDelete := len(files) - s.config.MaxFiles + 1
	if filesToDelete < 1 {
		filesToDelete = 0
	}

	for i := 0; i < filesToDelete && i < len(files); i++ {
		path := filepath.Join(dir, files[i].name)
		if err := os.Remove(path); err != nil {
			// Log but continue - don't fail on cleanup errors
			fmt.Fprintf(os.Stderr, "warning: failed to delete old capture file %s: %v\n", path, err)
		}
	}

	// Recalculate file count
	s.fileCount = len(files) - filesToDelete
	if s.fileCount < 0 {
		s.fileCount = 0
	}

	return nil
}

// generateFilename creates a timestamp-based filename for capture files.
func (s *Storage) generateFilename() string {
	timestamp := time.Now().UTC().Format("20060102-150405.000000")
	return fmt.Sprintf("capture-%s.jsonl", timestamp)
}

// scanExistingFiles counts existing capture files to set initial fileCount.
func (s *Storage) scanExistingFiles() error {
	dir := expandHome(s.config.Directory)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			s.fileCount = 0
			return nil
		}
		return fmt.Errorf("scanning debug directory: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "capture-") && strings.HasSuffix(name, ".jsonl") {
			count++
		}
	}

	s.fileCount = count
	return nil
}

// Close closes the current file if open.
func (s *Storage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentFile != nil {
		if err := s.currentFile.Close(); err != nil {
			return err
		}
		s.currentFile = nil
		s.currentSize = 0
	}

	return nil
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
