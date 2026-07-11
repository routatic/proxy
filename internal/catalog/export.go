package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteFile marshals a catalog to JSON and writes it atomically to path.
func WriteFile(path string, catalog *Catalog) error {
	if catalog == nil {
		return fmt.Errorf("catalog is nil")
	}

	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal catalog: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write catalog temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename catalog file: %w", err)
	}

	return nil
}

// WriteFileToDir writes the catalog to dir/catalog.json atomically.
func WriteFileToDir(dir string, catalog *Catalog) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	return WriteFile(filepath.Join(dir, catalogFileName), catalog)
}
