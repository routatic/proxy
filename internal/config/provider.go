package config

import (
	"slices"
	"strings"
)

// Canonical provider identifiers. These are the values accepted in a
// ModelConfig's `provider` field, and they name the upstream that serves the
// model. config is the lowest layer that knows about all four providers (it
// holds their connection settings), so the names live here and higher layers
// alias them.
const (
	ProviderOpenCodeGo  = "opencode-go"
	ProviderOpenCodeZen = "opencode-zen"
	ProviderAWSBedrock  = "aws-bedrock"
	ProviderOpenRouter  = "openrouter"
)

// KnownProviders lists every canonical provider name, in the order used for
// error messages and documentation.
var KnownProviders = []string{
	ProviderOpenCodeGo,
	ProviderOpenCodeZen,
	ProviderAWSBedrock,
	ProviderOpenRouter,
}

// NormalizeProvider maps a configured provider string to its canonical form.
// Underscores are normalized to hyphens so that both "aws_bedrock" and
// "aws-bedrock" resolve identically. An empty string yields
// ProviderOpenCodeGo, which is the default at request time.
func NormalizeProvider(provider string) string {
	if provider == "" {
		return ProviderOpenCodeGo
	}
	if strings.IndexByte(provider, '_') >= 0 {
		return strings.ReplaceAll(provider, "_", "-")
	}
	return provider
}

// IsKnownProvider reports whether provider names a supported upstream. The
// empty string is known: it means "use the default provider".
func IsKnownProvider(provider string) bool {
	return slices.Contains(KnownProviders, NormalizeProvider(provider))
}

// quotedKnownProviders renders the known provider names for error messages,
// e.g. `"opencode-go", "opencode-zen", "aws-bedrock" or "openrouter"`.
func quotedKnownProviders() string {
	quoted := make([]string, len(KnownProviders))
	for i, p := range KnownProviders {
		quoted[i] = `"` + p + `"`
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + " or " + quoted[len(quoted)-1]
}
