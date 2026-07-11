package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// catalogFixture is a minimal catalog with three providers and one model each.
const catalogFixture = `{
  "providers": {
    "opencode-go": {"name": "opencode-go", "base_url": "https://opencode.ai/zen/go/v1/chat/completions", "enabled": true},
    "opencode-zen": {"name": "opencode-zen", "base_url": "https://opencode.ai/zen/v1/chat/completions", "enabled": true},
    "openrouter": {"name": "openrouter", "base_url": "https://openrouter.ai/api/v1", "enabled": true}
  },
  "models": {
    "opencode-go/model-go": {"id": "opencode-go/model-go", "name": "Model Go"},
    "opencode-zen/model-zen": {"id": "opencode-zen/model-zen", "name": "Model Zen"},
    "openrouter/model-router": {"id": "openrouter/model-router", "name": "Model Router"}
  }
}`

func writeTestConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func writeTestCatalog(t *testing.T, dir, content string) {
	t.Helper()
	catalogDir := filepath.Join(dir, "catalog")
	if err := os.MkdirAll(catalogDir, 0755); err != nil {
		t.Fatalf("mkdir catalog: %v", err)
	}
	if err := os.WriteFile(filepath.Join(catalogDir, "catalog.json"), []byte(content), 0644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
}

func newCaptureCommand(t *testing.T) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	cmd := &cobra.Command{Use: "routatic-proxy"}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	return cmd, buf
}

func TestRunModelsList_ProviderFilter(t *testing.T) {
	tmp := t.TempDir()
	configPath := writeTestConfig(t, tmp, `{"api_key": "test-global-key"}`)
	writeTestCatalog(t, tmp, catalogFixture)

	cmd, buf := newCaptureCommand(t)
	t.Setenv("ROUTATIC_PROXY_CONFIG", configPath)

	if err := runModelsList(cmd, configPath, "opencode-zen"); err != nil {
		t.Fatalf("runModelsList error: %v", err)
	}

	out := buf.String()
	want := "opencode-zen/model-zen"
	if !strings.Contains(out, want) {
		t.Fatalf("output missing %q:\n%s", want, out)
	}
	for _, unexpected := range []string{"opencode-go/model-go", "openrouter/model-router"} {
		if strings.Contains(out, unexpected) {
			t.Fatalf("output should not contain %q:\n%s", unexpected, out)
		}
	}
	if !strings.Contains(out, "Use these model IDs") {
		t.Fatalf("output missing usage footer:\n%s", out)
	}
}

func TestRunModelsList_EnabledProviders(t *testing.T) {
	tmp := t.TempDir()
	// A global API key enables every provider, so all catalog providers appear.
	configPath := writeTestConfig(t, tmp, `{"api_key": "test-global-key"}`)
	writeTestCatalog(t, tmp, catalogFixture)

	cmd, buf := newCaptureCommand(t)
	t.Setenv("ROUTATIC_PROXY_CONFIG", configPath)

	if err := runModelsList(cmd, configPath, ""); err != nil {
		t.Fatalf("runModelsList error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"opencode-go/model-go", "opencode-zen/model-zen", "openrouter/model-router"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunModelsList_UnknownProvider(t *testing.T) {
	tmp := t.TempDir()
	configPath := writeTestConfig(t, tmp, `{"api_key": "test-global-key"}`)
	writeTestCatalog(t, tmp, catalogFixture)

	cmd, buf := newCaptureCommand(t)
	t.Setenv("ROUTATIC_PROXY_CONFIG", configPath)

	if err := runModelsList(cmd, configPath, "unknown"); err != nil {
		t.Fatalf("runModelsList error: %v", err)
	}

	out := buf.String()
	want := `No models found for provider "unknown".`
	if !strings.Contains(out, want) {
		t.Fatalf("output missing %q:\n%s", want, out)
	}
	if strings.Contains(out, "Use these model IDs") {
		t.Fatalf("usage footer should not appear when no models are found:\n%s", out)
	}
}

func TestRunModelsList_MissingCatalog(t *testing.T) {
	tmp := t.TempDir()
	configPath := writeTestConfig(t, tmp, `{"api_key": "test-global-key"}`)
	// Intentionally do not write catalog.json.

	cmd, _ := newCaptureCommand(t)
	t.Setenv("ROUTATIC_PROXY_CONFIG", configPath)

	err := runModelsList(cmd, configPath, "")
	if err == nil {
		t.Fatal("expected error for missing catalog, got nil")
	}
	want := "catalog not found; run 'routatic-proxy catalog sync' first"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error containing %q, got: %v", want, err)
	}
}
