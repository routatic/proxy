package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
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
	url := "https://api.github.com/repos/routatic/proxy/releases/latest"
	if channel == "beta" {
		// The /latest endpoint skips prereleases, so betas need the full list.
		url = "https://api.github.com/repos/routatic/proxy/releases?per_page=20"
	}
	return getLatestReleaseFrom(url, channel)
}

// getLatestReleaseFrom fetches and decodes a release listing from an explicit
// URL. The response shape depends on the channel: a single object for stable,
// an array for beta.
func getLatestReleaseFrom(url, channel string) (*GitHubRelease, error) {
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

		if latest := LatestBeta(releases); latest != nil {
			return latest, nil
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

// isBetaTag reports whether a tag names a beta build.
//
// `.github/scripts/get-versions.sh` is the only producer of beta tags and emits
// exactly `v{VERSION}-beta.{N}` (e.g. v0.6.4-beta.5). Match that literally: the
// dot is what a previous "-beta-" check got wrong, and it matched no real tag.
func isBetaTag(tag string) bool {
	return strings.Contains(tag, "-beta.")
}

// LatestBeta returns the highest-versioned beta prerelease, or nil when the
// list contains none. Selection is by semantic version rather than by the
// order GitHub returns, so a re-published or back-dated release cannot make an
// older beta look like the newest one.
func LatestBeta(releases []GitHubRelease) *GitHubRelease {
	var latest *GitHubRelease
	for i := range releases {
		r := &releases[i]
		if !r.Prerelease || !isBetaTag(r.TagName) {
			continue
		}
		if latest == nil || IsNewerVersion(latest.TagName, r.TagName) {
			latest = r
		}
	}
	return latest
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

// InstallPath returns the path of the binary an update would replace. Symlinks
// are resolved so that a symlinked install (e.g. a Homebrew shim in
// /usr/local/bin pointing into the Cellar) is replaced at its real location
// instead of clobbering the link.
func InstallPath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(execPath); err == nil {
		return resolved, nil
	}
	return execPath, nil
}

// permissionError turns an OS permission failure on the install directory into
// an actionable message. The raw "rename ...: permission denied" tells the user
// nothing about what to do next.
func permissionError(dir string, err error) error {
	if !errors.Is(err, fs.ErrPermission) {
		return err
	}
	hint := "re-run with elevated privileges (sudo routatic-proxy update)"
	if runtime.GOOS == "windows" {
		hint = "re-run from an Administrator terminal"
	}
	return fmt.Errorf("install directory %s is not writable by the current user: %s, or update through the package manager you installed with (e.g. brew upgrade routatic-proxy, scoop update routatic-proxy): %w", dir, hint, err)
}

// DownloadAndInstall downloads the binary and replaces the current executable.
//
// The download lands in the install directory rather than the system temp dir:
// the final step must be a rename, and rename cannot cross filesystems, so a
// /tmp staging file fails with EXDEV whenever /tmp is a separate mount (tmpfs
// on most Linux systems). Staging in the target directory also surfaces a
// read-only or root-owned install location before spending the download.
func DownloadAndInstall(url string) error {
	execPath, err := InstallPath()
	if err != nil {
		return err
	}
	return downloadAndInstallTo(url, execPath)
}

// downloadAndInstallTo performs the install against an explicit target path.
func downloadAndInstallTo(url, execPath string) error {
	installDir := filepath.Dir(execPath)

	tmpFile, err := os.CreateTemp(installDir, ".routatic-proxy-update-*")
	if err != nil {
		return permissionError(installDir, fmt.Errorf("failed to stage update in %s: %w", installDir, err))
	}
	tmpPath := tmpFile.Name()
	// Removing a path that was already renamed into place is a no-op error.
	defer func() { _ = os.Remove(tmpPath) }()

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_ = tmpFile.Close()
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write staged update: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close staged update: %w", err)
	}

	// Make executable on Unix. CreateTemp uses 0600, which would leave the
	// installed binary unrunnable.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0755); err != nil {
			return fmt.Errorf("failed to make executable: %w", err)
		}
	}

	// On Windows a running executable is locked and cannot be overwritten, so
	// move it aside first. Unix allows replacing the inode under a running
	// process, so the rename is enough.
	if runtime.GOOS == "windows" {
		oldPath := execPath + ".old"
		_ = os.Remove(oldPath)
		if err := os.Rename(execPath, oldPath); err != nil {
			return permissionError(installDir, fmt.Errorf("failed to rename current executable: %w", err))
		}
		if err := os.Rename(tmpPath, execPath); err != nil {
			_ = os.Rename(oldPath, execPath)
			return permissionError(installDir, fmt.Errorf("failed to install new executable: %w", err))
		}
		_ = os.Remove(oldPath)
		return nil
	}

	if err := os.Rename(tmpPath, execPath); err != nil {
		return permissionError(installDir, fmt.Errorf("failed to replace executable: %w", err))
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
