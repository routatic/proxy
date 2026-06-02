package handlers

import (
	"testing"

	"oc-go-cc/internal/config"
)

func TestAppendUniqueModels_DedupsByModelID(t *testing.T) {
	base := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "kimi-k2.6"},
		{Provider: "opencode-go", ModelID: "glm-5"},
	}
	extra := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "kimi-k2.6"}, // dup of base[0]
		{Provider: "opencode-go", ModelID: "mimo-v2-pro"},
		{Provider: "opencode-go", ModelID: "glm-5"}, // dup of base[1]
	}

	got := appendUniqueModels(base, extra)
	wantIDs := []string{"kimi-k2.6", "glm-5", "mimo-v2-pro"}

	if len(got) != len(wantIDs) {
		t.Fatalf("got %d models, want %d (got=%+v)", len(got), len(wantIDs), got)
	}
	for i, m := range got {
		if m.ModelID != wantIDs[i] {
			t.Errorf("position %d: got %s, want %s", i, m.ModelID, wantIDs[i])
		}
	}
}

func TestAppendUniqueModels_PreservesBaseOrder(t *testing.T) {
	base := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "a"},
		{Provider: "opencode-go", ModelID: "b"},
		{Provider: "opencode-go", ModelID: "c"},
	}
	// Extra starts with a model that would have come earlier in the chain
	// (b) and adds new models. The base order must be preserved.
	extra := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "b"}, // dup
		{Provider: "opencode-go", ModelID: "d"},
		{Provider: "opencode-go", ModelID: "e"},
	}

	got := appendUniqueModels(base, extra)
	wantIDs := []string{"a", "b", "c", "d", "e"}

	if len(got) != len(wantIDs) {
		t.Fatalf("got %d models, want %d (got=%+v)", len(got), len(wantIDs), got)
	}
	for i, m := range got {
		if m.ModelID != wantIDs[i] {
			t.Errorf("position %d: got %s, want %s", i, m.ModelID, wantIDs[i])
		}
	}
}

func TestAppendUniqueModels_EmptyExtra(t *testing.T) {
	base := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "a"},
	}
	got := appendUniqueModels(base, nil)
	if len(got) != 1 || got[0].ModelID != "a" {
		t.Errorf("expected unchanged base, got %+v", got)
	}
}

func TestAppendUniqueModels_AllDuplicates(t *testing.T) {
	base := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "a"},
		{Provider: "opencode-go", ModelID: "b"},
	}
	extra := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "a"},
		{Provider: "opencode-go", ModelID: "b"},
	}

	got := appendUniqueModels(base, extra)
	if len(got) != 2 {
		t.Errorf("expected 2 models, got %d (got=%+v)", len(got), got)
	}
}

func TestAppendUniqueModels_EmptyBase(t *testing.T) {
	extra := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "a"},
		{Provider: "opencode-go", ModelID: "b"},
	}
	got := appendUniqueModels(nil, extra)
	if len(got) != 2 {
		t.Errorf("expected 2 models, got %d (got=%+v)", len(got), got)
	}
}
