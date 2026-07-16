package catalog

import (
	"sort"
	"testing"
)

func loadFixtureCatalog(t *testing.T) *IndexedCatalog {
	t.Helper()
	ic, err := Load("testdata/catalog.json")
	if err != nil {
		t.Fatalf("Load(testdata/catalog.json) failed: %v", err)
	}
	return ic
}

func TestFixtureResolve_Canonical(t *testing.T) {
	tests := []struct {
		name            string
		ref             string
		wantProvider    string
		wantModelID     string
		wantDisplayName string
		wantBaseURL     string
		wantAPIKey      string
		wantTools       bool
	}{
		{
			name:            "deepseek via opencode-go",
			ref:             "deepseek/deepseek-v4-flash@opencode-go",
			wantProvider:    "opencode-go",
			wantModelID:     "deepseek-v4-flash",
			wantDisplayName: "DeepSeek V4 Flash",
			wantBaseURL:     "https://go.opencode.ai/v1",
			wantAPIKey:      "go-key",
			wantTools:       true,
		},
		{
			name:         "vision via openrouter",
			ref:          "vision-model@openrouter",
			wantProvider: "openrouter",
			wantModelID:  "vision-model",
			wantBaseURL:  "https://openrouter.ai/api/v1",
			wantAPIKey:   "or-key",
			wantTools:    true,
		},
	}

	ic := loadFixtureCatalog(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel, err := ParseModelRef(tt.ref)
			if err != nil {
				t.Fatalf("ParseModelRef(%q) failed: %v", tt.ref, err)
			}

			got, err := ic.Resolve(sel)
			if err != nil {
				t.Fatalf("Resolve(%q) unexpected error: %v", tt.ref, err)
			}

			if got.Provider != tt.wantProvider {
				t.Errorf("Provider = %q, want %q", got.Provider, tt.wantProvider)
			}
			if got.ModelID != tt.wantModelID {
				t.Errorf("ModelID = %q, want %q", got.ModelID, tt.wantModelID)
			}
			if tt.wantDisplayName != "" && got.DisplayName != tt.wantDisplayName {
				t.Errorf("DisplayName = %q, want %q", got.DisplayName, tt.wantDisplayName)
			}
			if tt.wantBaseURL != "" && got.BaseURL != tt.wantBaseURL {
				t.Errorf("BaseURL = %q, want %q", got.BaseURL, tt.wantBaseURL)
			}
			if tt.wantAPIKey != "" && got.APIKey != tt.wantAPIKey {
				t.Errorf("APIKey = %q, want %q", got.APIKey, tt.wantAPIKey)
			}
			if got.Tools != tt.wantTools {
				t.Errorf("Tools = %v, want %v", got.Tools, tt.wantTools)
			}
		})
	}
}

func TestFixtureResolve_Invalid(t *testing.T) {
	tests := []struct {
		name string
		ref  string
	}{
		{
			name: "empty provider",
			ref:  "deepseek-v4-flash@",
		},
		{
			name: "unknown provider",
			ref:  "deepseek-v4-flash@unknown",
		},
		{
			name: "model not on provider",
			ref:  "deepseek-v4-flash@openrouter",
		},
	}

	ic := loadFixtureCatalog(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel, err := ParseModelRef(tt.ref)
			if err != nil {
				t.Fatalf("ParseModelRef(%q) failed: %v", tt.ref, err)
			}

			_, err = ic.Resolve(sel)
			if err == nil {
				t.Fatalf("Resolve(%q) expected error, got nil", tt.ref)
			}
		})
	}
}

func TestFixtureResolveShort(t *testing.T) {
	tests := []struct {
		name         string
		short        string
		wantProvider string
		wantModelID  string
		wantErr      bool
	}{
		{
			name:         "first enabled provider",
			short:        "DeepSeek V4 Flash",
			wantProvider: "opencode-go",
			wantModelID:  "deepseek-v4-flash",
		},
		{
			name:         "resolve by key suffix",
			short:        "deepseek-v4-flash",
			wantProvider: "opencode-go",
			wantModelID:  "deepseek-v4-flash",
		},
		{
			name:    "only disabled provider",
			short:   "Only Disabled",
			wantErr: true,
		},
	}

	ic := loadFixtureCatalog(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ic.ResolveShort(tt.short)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveShort(%q) error = %v, wantErr %v", tt.short, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if got.Provider != tt.wantProvider {
				t.Errorf("Provider = %q, want %q", got.Provider, tt.wantProvider)
			}
			if got.ModelID != tt.wantModelID {
				t.Errorf("ModelID = %q, want %q", got.ModelID, tt.wantModelID)
			}
		})
	}
}

func TestFixtureResolve_ListProviderModels(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		wantIDs  []string
		wantNil  bool
	}{
		{
			name:     "opencode-go models",
			provider: "opencode-go",
			wantIDs:  []string{"deepseek-v4-flash", "large-context", "tie-small-context"},
		},
		{
			name:     "openrouter models",
			provider: "openrouter",
			wantIDs:  []string{"reasoning-model", "vision-model"},
		},
		{
			name:     "unknown provider",
			provider: "unknown",
			wantNil:  true,
		},
	}

	ic := loadFixtureCatalog(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ic.ListProviderModels(tt.provider)

			if tt.wantNil {
				if got != nil {
					t.Fatalf("ListProviderModels(%q) = %v, want nil", tt.provider, got)
				}
				return
			}

			ids := make([]string, len(got))
			for i, m := range got {
				ids[i] = m.ModelID
			}
			sort.Strings(ids)
			sort.Strings(tt.wantIDs)

			if len(ids) != len(tt.wantIDs) {
				t.Fatalf("ListProviderModels(%q) returned %d models, want %d: got %v", tt.provider, len(ids), len(tt.wantIDs), ids)
			}
			for i := range tt.wantIDs {
				if ids[i] != tt.wantIDs[i] {
					t.Errorf("model ids = %v, want %v", ids, tt.wantIDs)
					break
				}
			}
		})
	}
}
