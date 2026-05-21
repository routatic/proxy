package token

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultCacheDir(t *testing.T) {
	tests := []struct {
		name          string
		tiktokenCache string
		datagymCache  string
		homeDir       string
		want          string
	}{
		{
			name:          "respects TIKTOKEN_CACHE_DIR",
			tiktokenCache: "/custom/path",
			want:          "/custom/path",
		},
		{
			name:         "respects DATA_GYM_CACHE_DIR",
			datagymCache: "/other/path",
			want:         "/other/path",
		},
		{
			name: "falls back to home cache",
			want: filepath.Join(mustHome(), ".cache", "oc-go-cc", "tiktoken"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.tiktokenCache != "" {
				t.Setenv("TIKTOKEN_CACHE_DIR", tt.tiktokenCache)
			} else {
				t.Setenv("TIKTOKEN_CACHE_DIR", "")
			}
			if tt.datagymCache != "" {
				t.Setenv("DATA_GYM_CACHE_DIR", tt.datagymCache)
			} else {
				t.Setenv("DATA_GYM_CACHE_DIR", "")
			}

			got := defaultCacheDir()
			if got != tt.want {
				t.Errorf("defaultCacheDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultCacheDir_HomeDirFallback(t *testing.T) {
	// When UserHomeDir fails (HOME unset), fall back to temp dir.
	t.Setenv("TIKTOKEN_CACHE_DIR", "")
	t.Setenv("DATA_GYM_CACHE_DIR", "")
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", "")
	t.Cleanup(func() { t.Setenv("HOME", origHome) })

	got := defaultCacheDir()
	want := filepath.Join(os.TempDir(), "data-gym-cache")
	if got != want {
		t.Errorf("defaultCacheDir() = %q, want %q", got, want)
	}
}

func TestDefaultCacheDir_TiktokenTakesPrecedence(t *testing.T) {
	t.Setenv("TIKTOKEN_CACHE_DIR", "/tiktoken/path")
	t.Setenv("DATA_GYM_CACHE_DIR", "/datagym/path")

	got := defaultCacheDir()
	if got != "/tiktoken/path" {
		t.Errorf("defaultCacheDir() = %q, want /tiktoken/path", got)
	}
}

func mustHome() string {
	h, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return h
}
