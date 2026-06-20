package config

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	names := []string{
		"ROUTATIC_PROXY_CONFIG",
		"ROUTATIC_PROXY_API_KEY",
		"ROUTATIC_PROXY_HOST",
		"ROUTATIC_PROXY_PORT",
		"ROUTATIC_PROXY_OPENCODE_URL",
		"ROUTATIC_PROXY_OPENCODE_ZEN_URL",
		"ROUTATIC_PROXY_LOG_LEVEL",
	}
	original := make(map[string]string, len(names))
	present := make(map[string]bool, len(names))
	for _, name := range names {
		original[name], present[name] = os.LookupEnv(name)
		_ = os.Unsetenv(name)
	}

	code := m.Run()

	for _, name := range names {
		if present[name] {
			_ = os.Setenv(name, original[name])
		} else {
			_ = os.Unsetenv(name)
		}
	}
	os.Exit(code)
}
