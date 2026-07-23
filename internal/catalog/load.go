package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
)

// IndexedCatalog is a Catalog with an additional index from provider name to
// the models that declare support for that provider.
type IndexedCatalog struct {
	Catalog
	ProviderModels map[string][]Model
}

// ModelsForProvider returns the models that support the named provider.
// It returns nil when the provider has no indexed models.
func (ic *IndexedCatalog) ModelsForProvider(provider string) []Model {
	return ic.ProviderModels[provider]
}

// Load reads a catalog from path, validates its contents, and returns an
// indexed view.
func Load(path string) (*IndexedCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read catalog file: %w", err)
	}

	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("parse catalog json: %w", err)
	}

	if err := validateCatalog(&catalog); err != nil {
		return nil, err
	}

	idx := &IndexedCatalog{
		Catalog:        catalog,
		ProviderModels: make(map[string][]Model, len(catalog.Providers)),
	}

	for key, model := range catalog.Models {
		provider := ProviderFromModelKey(key)
		if provider != "" {
			idx.ProviderModels[provider] = append(idx.ProviderModels[provider], model)
		}
	}

	return idx, nil
}

func validateCatalog(catalog *Catalog) error {
	if len(catalog.Providers) == 0 {
		return errors.New("catalog providers map is empty")
	}
	if len(catalog.Models) == 0 {
		return errors.New("catalog models map is empty")
	}

	var toDelete []string
	for key := range catalog.Models {
		provider := ProviderFromModelKey(key)
		if provider == "" {
			return fmt.Errorf("model key %q does not include a provider prefix (expected format: provider/model)", key)
		}
		if _, ok := catalog.Providers[provider]; !ok {
			slog.Warn("skipping model with unknown provider", "model", key, "provider", provider)
			toDelete = append(toDelete, key)
		}
	}

	for _, key := range toDelete {
		delete(catalog.Models, key)
	}

	if len(toDelete) > 0 && len(catalog.Models) == 0 {
		return errors.New("all models reference unknown providers, catalog is unusable")
	}

	return nil
}
