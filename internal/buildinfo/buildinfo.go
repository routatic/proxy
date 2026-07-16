package buildinfo

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
)

var (
	// Version is the semantic version. Set via ldflags: -X github.com/routatic/proxy/internal/buildinfo.Version=vX.Y.Z
	Version = "dev"
	// Commit is the git commit SHA. Set via ldflags: -X github.com/routatic/proxy/internal/buildinfo.Commit=<sha>
	Commit = "none"
	// Date is the build timestamp (alias for BuildTime for compatibility).
	Date = "unknown"
	// BuildTime is the build timestamp. Set via ldflags.
	BuildTime = "unknown"
)

func init() {
	// If built with `go install` or module-aware build and no ldflags provided,
	// try to extract version info from the embedded build metadata.
	if info, ok := debug.ReadBuildInfo(); ok {
		if Version == "dev" {
			if info.Main.Version != "" && info.Main.Version != "(devel)" {
				Version = info.Main.Version
			}
		}
		// Capture VCS info if present (Go 1.18+)
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				if Commit == "none" && s.Value != "" {
					Commit = s.Value
				}
			case "vcs.time":
				if BuildTime == "unknown" && s.Value != "" {
					BuildTime = s.Value
					if Date == "unknown" {
						Date = s.Value
					}
				}
			}
		}
	}
}

// PID returns the current process ID.
func PID() int {
	return os.Getpid()
}

// PIDString returns the current process ID as a string.
func PIDString() string {
	return strconv.Itoa(os.Getpid())
}

// BinaryPath returns the absolute path to the running binary.
func BinaryPath() string {
	execPath, err := os.Executable()
	if err != nil {
		return "unknown"
	}
	absPath, err := filepath.Abs(execPath)
	if err != nil {
		return execPath
	}
	return absPath
}

// String returns a human-readable build info summary.
func String() string {
	return Version + " (" + Commit + ") built at " + Date
}
