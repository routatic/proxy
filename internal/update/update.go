package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// GitHubRelease represents a GitHub release
type GitHubRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// GetLatestRelease fetches the latest release for the specified channel ("stable" or "beta")
func GetLatestRelease(channel string) (*GitHubRelease, error) {
	var url string

	if channel == "beta" {
		// For beta, get all releases and find the latest prerelease with -beta- in tag
		url = "https://api.github.com/repos/routatic/proxy/releases?per_page=20"
	} else {
		// For stable, use the /latest endpoint
		url = "https://api.github.com/repos/routatic/proxy/releases/latest"
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	if channel == "beta" {
		// Parse as array of releases
		var releases []GitHubRelease
		if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
			return nil, fmt.Errorf("failed to parse releases: %w", err)
		}

		// Find the latest beta release (first prerelease with -beta- in tag)
		for _, release := range releases {
			if release.Prerelease && strings.Contains(release.TagName, "-beta-") {
				return &release, nil
			}
		}
		return nil, fmt.Errorf("no beta releases found")
	}

	// Parse as single release
	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release: %w", err)
	}

	return &release, nil
}

// GetAssetURL finds the download URL for the current platform
func GetAssetURL(release *GitHubRelease) (string, string, error) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Map Go architecture to asset naming
	archMap := map[string]string{
		"amd64": "amd64",
		"arm64": "arm64",
	}

	assetArch, ok := archMap[goarch]
	if !ok {
		return "", "", fmt.Errorf("unsupported architecture: %s", goarch)
	}

	// Build expected asset name
	// Format: routatic-proxy_{os}-{arch} or routatic-proxy_{os}-{arch}.exe for Windows
	ext := ""
	if goos == "windows" {
		ext = ".exe"
	}

	expectedName := fmt.Sprintf("routatic-proxy_%s-%s%s", goos, assetArch, ext)

	for _, asset := range release.Assets {
		if asset.Name == expectedName {
			return asset.BrowserDownloadURL, expectedName, nil
		}
	}

	return "", "", fmt.Errorf("no matching asset found for %s-%s", goos, goarch)
}

// DownloadAndInstall downloads the binary and replaces the current executable
func DownloadAndInstall(url, filename string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", "routatic-proxy-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	// Copy downloaded content to temp file
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	_ = tmpFile.Close()

	// Make executable on Unix
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0755); err != nil {
			return fmt.Errorf("failed to make executable: %w", err)
		}
	}

	// Get current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// On Windows, we can't replace a running executable directly
	// So we rename the old one and move the new one in place
	if runtime.GOOS == "windows" {
		oldPath := execPath + ".old"
		// Remove any previous .old file
		_ = os.Remove(oldPath)
		// Rename current executable
		if err := os.Rename(execPath, oldPath); err != nil {
			return fmt.Errorf("failed to rename current executable: %w", err)
		}
		// Move new executable into place
		if err := os.Rename(tmpPath, execPath); err != nil {
			// Try to restore old executable
			_ = os.Rename(oldPath, execPath)
			return fmt.Errorf("failed to install new executable: %w", err)
		}
		// Clean up old executable
		_ = os.Remove(oldPath)
	} else {
		// On Unix, we can directly replace
		if err := os.Rename(tmpPath, execPath); err != nil {
			return fmt.Errorf("failed to replace executable: %w", err)
		}
	}

	return nil
}

// IsNewerVersion reports whether candidate is a newer version than current.
// Uses semantic version comparison via golang.org/x/mod/semver.
func IsNewerVersion(current, candidate string) bool {
	if semver.IsValid(candidate) && semver.IsValid(current) {
		return semver.Compare(candidate, current) > 0
	}
	// Fallback for dev versions or non-semver strings
	return candidate != current && candidate > current
}

// CheckForUpdate checks if a newer version is available
func CheckForUpdate(currentVersion string, channel string) (*GitHubRelease, error) {
	release, err := GetLatestRelease(channel)
	if err != nil {
		return nil, err
	}

	// Compare versions using semantic versioning
	if IsNewerVersion(currentVersion, release.TagName) {
		return release, nil
	}

	return nil, nil // No update available
}
