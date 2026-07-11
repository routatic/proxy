package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const indexFileName = "provider_model_index.json"
const indexTmpFileName = "provider_model_index.json.tmp"

// ProviderModelIndex maps each enabled provider to the sorted keys of the
// models that declare support for that provider.
type ProviderModelIndex struct {
	ProviderModels map[string][]string `json:"provider_models"`
}

// BuildProviderIndex validates the catalog and builds an index from enabled
// provider name to sorted model keys. A provider with a nil Enabled field is
// treated as enabled; providers with Enabled explicitly set to false are
// skipped.
func BuildProviderIndex(catalog Catalog) (*ProviderModelIndex, error) {
	if len(catalog.Providers) == 0 {
		return nil, errors.New("catalog providers map is empty")
	}
	if len(catalog.Models) == 0 {
		return nil, errors.New("catalog models map is empty")
	}

	providerModels := make(map[string][]string)
	enabledCount := 0

	for providerName, provider := range catalog.Providers {
		if provider.Enabled != nil && !*provider.Enabled {
			continue
		}
		enabledCount++

		prefix := providerName + "/"
		for modelKey := range catalog.Models {
			if strings.HasPrefix(modelKey, prefix) {
				providerModels[providerName] = append(providerModels[providerName], modelKey)
			}
		}
	}

	if enabledCount == 0 {
		return nil, errors.New("no enabled providers in catalog")
	}

	// Deduplicate and sort each provider's model slice. A model could in
	// theory list the same provider twice.
	for providerName, models := range providerModels {
		seen := make(map[string]struct{}, len(models))
		unique := make([]string, 0, len(models))
		for _, modelKey := range models {
			if _, ok := seen[modelKey]; ok {
				continue
			}
			seen[modelKey] = struct{}{}
			unique = append(unique, modelKey)
		}
		sort.Strings(unique)
		providerModels[providerName] = unique
	}

	// Sorting the keys is not required by the contract, but it makes the
	// output deterministic and easier to test.
	if len(providerModels) == 0 {
		return nil, errors.New("no models reference enabled providers")
	}

	return &ProviderModelIndex{ProviderModels: providerModels}, nil
}

// Write marshals the index and writes it atomically to dir/provider_model_index.json.
func (idx *ProviderModelIndex) Write(dir string) error {
	if idx == nil {
		return errors.New("cannot write nil index")
	}

	data, err := json.Marshal(idx)
	if err != nil {
		return fmt.Errorf("marshal provider model index: %w", err)
	}

	tmpPath := filepath.Join(dir, indexTmpFileName)
	finalPath := filepath.Join(dir, indexFileName)

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write provider model index temp file: %w", err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename provider model index file: %w", err)
	}

	return nil
}

// ReadProviderIndex reads and unmarshals the provider model index from dir.
func ReadProviderIndex(dir string) (*ProviderModelIndex, error) {
	path := filepath.Join(dir, indexFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read provider model index: %w", err)
	}

	var idx ProviderModelIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse provider model index: %w", err)
	}

	return &idx, nil
}
