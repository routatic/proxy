package config

import "testing"

func TestNormalizeProvider(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ProviderOpenCodeGo},
		{"opencode-go", ProviderOpenCodeGo},
		{"opencode_zen", ProviderOpenCodeZen},
		{"aws_bedrock", ProviderAWSBedrock},
		{"aws-bedrock", ProviderAWSBedrock},
		{"openrouter", ProviderOpenRouter},
		{"mystery", "mystery"},
	}
	for _, tt := range tests {
		if got := NormalizeProvider(tt.in); got != tt.want {
			t.Errorf("NormalizeProvider(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsKnownProvider(t *testing.T) {
	for _, provider := range append([]string{"", "aws_bedrock", "opencode_zen"}, KnownProviders...) {
		if !IsKnownProvider(provider) {
			t.Errorf("IsKnownProvider(%q) = false, want true", provider)
		}
	}
	for _, provider := range []string{"mystery", "bedrock", "open-router"} {
		if IsKnownProvider(provider) {
			t.Errorf("IsKnownProvider(%q) = true, want false", provider)
		}
	}
}

func TestQuotedKnownProviders(t *testing.T) {
	want := `"opencode-go", "opencode-zen", "aws-bedrock" or "openrouter"`
	if got := quotedKnownProviders(); got != want {
		t.Errorf("quotedKnownProviders() = %q, want %q", got, want)
	}
}
