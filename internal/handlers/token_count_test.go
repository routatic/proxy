package handlers

import (
	"testing"

	"github.com/routatic/proxy/pkg/types"
)

func TestImageTokenEstimateFromBase64_Zero(t *testing.T) {
	got := imageTokenEstimateFromBase64(0)
	if got != 1500 {
		t.Errorf("got %d, want 1500", got)
	}
}

func TestImageTokenEstimateFromBase64_Small(t *testing.T) {
	// ~5KB base64 → ~3.7KB raw → ~50 tokens → clamped to 300
	got := imageTokenEstimateFromBase64(5000)
	if got != 300 {
		t.Errorf("got %d, want 300", got)
	}
}

func TestImageTokenEstimateFromBase64_Medium(t *testing.T) {
	// ~150KB base64 → ~112KB raw → ~1500 tokens
	got := imageTokenEstimateFromBase64(150000)
	if got != 1500 {
		t.Errorf("got %d, want 1500", got)
	}
}

func TestImageTokenEstimateFromBase64_Large(t *testing.T) {
	// ~400KB base64 → ~300KB raw → ~4000 tokens (at clamp boundary)
	got := imageTokenEstimateFromBase64(400000)
	if got != 4000 {
		t.Errorf("got %d, want 4000", got)
	}
}

func TestImageTokenEstimateFromBase64_Overflow(t *testing.T) {
	// ~1MB base64 → clamped to 4000
	got := imageTokenEstimateFromBase64(1000000)
	if got != 4000 {
		t.Errorf("got %d, want 4000", got)
	}
}

func TestImageTokenEstimate_NoImageBlocks(t *testing.T) {
	blocks := []types.ContentBlock{
		{Type: "text", Text: "hello"},
		{Type: "tool_use", Name: "test"},
	}
	got := imageTokenEstimate(blocks)
	if got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestImageTokenEstimate_Base64Image(t *testing.T) {
	blocks := []types.ContentBlock{
		{Type: "text", Text: "hello"},
		{Type: "image", Source: &types.ImageSource{Data: "AAAA", Type: "base64", MediaType: "image/png"}},
	}
	got := imageTokenEstimate(blocks)
	if got != 300 {
		t.Errorf("got %d, want 300", got)
	}
}

func TestImageTokenEstimate_URLImage(t *testing.T) {
	blocks := []types.ContentBlock{
		{Type: "image", Source: &types.ImageSource{URL: "https://example.com/img.png"}},
	}
	got := imageTokenEstimate(blocks)
	if got != 1500 {
		t.Errorf("got %d, want 1500", got)
	}
}

func TestImageTokenEstimate_MultipleImages(t *testing.T) {
	blocks := []types.ContentBlock{
		{Type: "image", Source: &types.ImageSource{URL: "https://example.com/a.png"}},
		{Type: "image", Source: &types.ImageSource{URL: "https://example.com/b.png"}},
	}
	got := imageTokenEstimate(blocks)
	if got != 3000 {
		t.Errorf("got %d, want 3000", got)
	}
}

func TestImageTokenEstimate_NilSource(t *testing.T) {
	blocks := []types.ContentBlock{
		{Type: "image"},
	}
	got := imageTokenEstimate(blocks)
	if got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}
