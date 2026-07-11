package catalog

import (
	"sort"
	"testing"
)

func boolPtr(b bool) *bool {
	return &b
}

func newFixtureCatalog() *IndexedCatalog {
	return &IndexedCatalog{
		Catalog: Catalog{
			Providers: map[string]Provider{
				"opencode-go": {
					Name:    "opencode-go",
					BaseURL: "https://go.opencode.ai/v1",
					Enabled: boolPtr(true),
				},
				"openrouter": {
					Name:    "openrouter",
					BaseURL: "https://openrouter.ai/api/v1",
					Enabled: boolPtr(true),
				},
				"disabled-provider": {
					Name:    "disabled-provider",
					BaseURL: "https://disabled.example/v1",
					Enabled: boolPtr(false),
				},
			},
			Models: map[string]Model{
				"opencode-go/deepseek-v4-flash": {
					ID:        "opencode-go/deepseek-v4-flash",
					Name:      "DeepSeek V4 Flash",
					ToolCall:  true,
					Reasoning: false,
					Limit:     &Limit{Context: 128000},
				},
				"opencode-go/kimi-k2.6": {
					ID:        "opencode-go/kimi-k2.6",
					Name:      "Kimi K2.6",
					ToolCall:  false,
					Reasoning: false,
					Limit:     &Limit{Context: 256000},
				},
				"openrouter/kimi-k2.6": {
					ID:        "openrouter/kimi-k2.6",
					Name:      "Kimi K2.6",
					ToolCall:  false,
					Reasoning: false,
					Limit:     &Limit{Context: 256000},
				},
				"opencode-go/legacy-name": {
					ID:        "opencode-go/legacy-name",
					Name:      "Old Model",
					ToolCall:  false,
					Reasoning: false,
				},
				"disabled-provider/only-disabled": {
					ID:        "disabled-provider/only-disabled",
					Name:      "Only Disabled",
					ToolCall:  false,
					Reasoning: false,
				},
			},
		},
	}
}

func TestParseModelRef(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		want    Selector
		wantErr bool
	}{
		{
			name: "lab/model@provider",
			ref:  "deepseek/deepseek-v4-flash@opencode-go",
			want: Selector{
				Provider: "opencode-go",
				Model:    "deepseek-v4-flash",
				Alias:    "deepseek/deepseek-v4-flash",
			},
		},
		{
			name: "model@provider",
			ref:  "kimi-k2.6@opencode-go",
			want: Selector{
				Provider: "opencode-go",
				Model:    "kimi-k2.6",
				Alias:    "kimi-k2.6",
			},
		},
		{
			name: "short model only",
			ref:  "kimi-k2.6",
			want: Selector{
				Provider: "",
				Model:    "kimi-k2.6",
				Alias:    "kimi-k2.6",
			},
		},
		{
			name: "lab/model without provider",
			ref:  "deepseek/deepseek-v4-flash",
			want: Selector{
				Provider: "",
				Model:    "deepseek-v4-flash",
				Alias:    "deepseek/deepseek-v4-flash",
			},
		},
		{
			name:    "empty reference",
			ref:     "",
			wantErr: true,
		},
		{
			name:    "multiple @ separators",
			ref:     "a@b@c",
			wantErr: true,
		},
		{
			name:    "empty model id",
			ref:     "@provider",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseModelRef(tt.ref)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseModelRef(%q) error = %v, wantErr %v", tt.ref, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got != tt.want {
				t.Errorf("ParseModelRef(%q) = %+v, want %+v", tt.ref, got, tt.want)
			}
		})
	}
}

func TestResolve_Canonical(t *testing.T) {
	ic := newFixtureCatalog()
	sel := Selector{Provider: "opencode-go", Model: "deepseek-v4-flash", Alias: "deepseek/deepseek-v4-flash"}

	got, err := ic.Resolve(sel)
	if err != nil {
		t.Fatalf("Resolve(%+v) unexpected error: %v", sel, err)
	}

	if got.Provider != "opencode-go" {
		t.Errorf("Provider = %q, want %q", got.Provider, "opencode-go")
	}
	if got.ModelID != "deepseek-v4-flash" {
		t.Errorf("ModelID = %q, want %q", got.ModelID, "deepseek-v4-flash")
	}
	if got.CanonicalName != "opencode-go/deepseek-v4-flash" {
		t.Errorf("CanonicalName = %q, want %q", got.CanonicalName, "opencode-go/deepseek-v4-flash")
	}
	if got.DisplayName != "DeepSeek V4 Flash" {
		t.Errorf("DisplayName = %q, want %q", got.DisplayName, "DeepSeek V4 Flash")
	}
	if got.BaseURL != "https://go.opencode.ai/v1" {
		t.Errorf("BaseURL = %q, want %q", got.BaseURL, "https://go.opencode.ai/v1")
	}
	if !got.Tools {
		t.Errorf("Tools = %v, want true", got.Tools)
	}
}

func TestResolve_ProviderMissing(t *testing.T) {
	ic := newFixtureCatalog()
	sel := Selector{Model: "deepseek-v4-flash"}

	_, err := ic.Resolve(sel)
	if err == nil {
		t.Fatal("Resolve: expected error for missing provider, got nil")
	}
}

func TestResolve_UnknownProvider(t *testing.T) {
	ic := newFixtureCatalog()
	sel := Selector{Provider: "unknown", Model: "deepseek-v4-flash"}

	_, err := ic.Resolve(sel)
	if err == nil {
		t.Fatal("Resolve: expected error for unknown provider, got nil")
	}
}

func TestResolve_ModelNotOnProvider(t *testing.T) {
	ic := newFixtureCatalog()
	sel := Selector{Provider: "openrouter", Model: "deepseek-v4-flash"}

	_, err := ic.Resolve(sel)
	if err == nil {
		t.Fatal("Resolve: expected error for model not on provider, got nil")
	}
}

func TestResolveShort_Legacy(t *testing.T) {
	ic := newFixtureCatalog()

	got, err := ic.ResolveShort("DeepSeek V4 Flash")
	if err != nil {
		t.Fatalf("ResolveShort(%q) unexpected error: %v", "DeepSeek V4 Flash", err)
	}
	if got.Provider != "opencode-go" {
		t.Errorf("Provider = %q, want %q", got.Provider, "opencode-go")
	}
	if got.ModelID != "deepseek-v4-flash" {
		t.Errorf("ModelID = %q, want %q", got.ModelID, "deepseek-v4-flash")
	}
	if got.CanonicalName != "opencode-go/deepseek-v4-flash" {
		t.Errorf("CanonicalName = %q, want %q", got.CanonicalName, "opencode-go/deepseek-v4-flash")
	}
}

func TestResolveShort_Name(t *testing.T) {
	ic := newFixtureCatalog()

	got, err := ic.ResolveShort("Old Model")
	if err != nil {
		t.Fatalf("ResolveShort(%q) unexpected error: %v", "Old Model", err)
	}
	if got.Provider != "opencode-go" {
		t.Errorf("Provider = %q, want %q", got.Provider, "opencode-go")
	}
	if got.ModelID != "legacy-name" {
		t.Errorf("ModelID = %q, want %q", got.ModelID, "legacy-name")
	}
	if got.CanonicalName != "opencode-go/legacy-name" {
		t.Errorf("CanonicalName = %q, want %q", got.CanonicalName, "opencode-go/legacy-name")
	}
}

func TestResolveShort_DisabledProvider(t *testing.T) {
	ic := newFixtureCatalog()

	_, err := ic.ResolveShort("only-disabled")
	if err == nil {
		t.Fatal("ResolveShort: expected error for model with only disabled provider, got nil")
	}
}

func TestResolveShort_AmbiguousModel(t *testing.T) {
	ic := newFixtureCatalog()

	_, err := ic.ResolveShort("kimi-k2.6")
	if err == nil {
		t.Fatal("ResolveShort: expected ambiguity error for model on multiple providers, got nil")
	}
	if !containsAll(err.Error(), "ambiguous", "kimi-k2.6", "opencode-go", "openrouter") {
		t.Errorf("error message = %q, want mention of ambiguity and available providers", err.Error())
	}
}

func TestResolveShort_UnambiguousWithSingleEnabledProvider(t *testing.T) {
	ic := newFixtureCatalog()

	provider := ic.Providers["openrouter"]
	provider.Enabled = boolPtr(false)
	ic.Providers["openrouter"] = provider

	got, err := ic.ResolveShort("kimi-k2.6")
	if err != nil {
		t.Fatalf("ResolveShort: unexpected error: %v", err)
	}
	if got.Provider != "opencode-go" {
		t.Errorf("Provider = %q, want %q", got.Provider, "opencode-go")
	}
	if got.ModelID != "kimi-k2.6" {
		t.Errorf("ModelID = %q, want %q", got.ModelID, "kimi-k2.6")
	}
}

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || containsAt(s, sub))
}

func containsAt(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestListProviderModels(t *testing.T) {
	ic := newFixtureCatalog()

	got := ic.ListProviderModels("opencode-go")
	if len(got) != 3 {
		t.Fatalf("ListProviderModels(%q) returned %d models, want 3", "opencode-go", len(got))
	}

	ids := make([]string, len(got))
	for i, m := range got {
		ids[i] = m.ModelID
	}
	sort.Strings(ids)
	want := []string{"deepseek-v4-flash", "kimi-k2.6", "legacy-name"}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("model ids = %v, want %v", ids, want)
			break
		}
	}

	if ic.ListProviderModels("unknown") != nil {
		t.Error("ListProviderModels(\"unknown\"): expected nil for unknown provider")
	}

	for _, m := range got {
		if m.ModelID == "disabled-provider/only-disabled" {
			t.Errorf("unexpected disabled-only model %q in opencode-go list", m.ModelID)
		}
		if m.Provider != "opencode-go" {
			t.Errorf("model %q has provider %q, want %q", m.ModelID, m.Provider, "opencode-go")
		}
	}
}
