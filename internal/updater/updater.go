package updater

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	Owner = "routatic"
	Repo  = "proxy"
)

var defaultClient = &http.Client{Timeout: 30 * time.Second}

// userAgent builds a User-Agent header value. If version is non-empty it is
// appended to the product name as required by the GitHub API spec.
func userAgent(version string) string {
	if version == "" {
		return "routatic-proxy"
	}
	return "routatic-proxy/" + version
}

// ReleaseInfo holds the data we need from the GitHub release API.
type ReleaseInfo struct {
	TagName     string
	Name        string
	Published   time.Time
	AssetName   string
	AssetURL    string
	ChecksumURL string
}

// Result is returned by Apply.
type Result struct {
	Updated    bool
	OldVersion string
	NewVersion string
	NewPath    string
	BackupPath string
}

// Options controls update behavior.
type Options struct {
	CurrentVersion    string
	CurrentBinaryPath string
	Force             bool
	SkipChecksum      bool
	HTTPClient        *http.Client
}

// Check fetches the latest release from GitHub.
func Check(ctx context.Context) (*ReleaseInfo, error) {
	return checkWithClient(ctx, defaultClient, "")
}

func checkWithClient(ctx context.Context, client *http.Client, version string) (*ReleaseInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", Owner, Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent(version))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot fetch latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release API returned %s", resp.Status)
	}

	var payload struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		PublishedAt string `json:"published_at"`
		Assets      []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("cannot decode release response: %w", err)
	}

	published, err := time.Parse(time.RFC3339, payload.PublishedAt)
	if err != nil {
		published = time.Time{}
	}

	assetName, err := AssetName()
	if err != nil {
		return nil, err
	}

	info := &ReleaseInfo{
		TagName:   payload.TagName,
		Name:      payload.Name,
		Published: published,
		AssetName: assetName,
	}

	for _, asset := range payload.Assets {
		switch asset.Name {
		case assetName:
			info.AssetURL = asset.BrowserDownloadURL
		case "checksums.txt":
			info.ChecksumURL = asset.BrowserDownloadURL
		}
	}

	if info.AssetURL == "" {
		return nil, fmt.Errorf("release does not contain asset %q", assetName)
	}
	return info, nil
}

// NeedsUpdate compares the current version with the release tag.
// Versions are normalized by stripping a leading "v" and comparing
// major.minor.patch numerically. Pre-release segments are compared
// lexicographically. "dev" is treated as older than any real release
// unless Force is true.
func NeedsUpdate(current, latest string, force bool) (bool, error) {
	current = normalizeVersion(current)
	latest = normalizeVersion(latest)

	if current == "dev" {
		if force {
			return true, nil
		}
		return false, fmt.Errorf("current version is %q; use --force to update anyway", current)
	}

	if current == "" {
		if force {
			return true, nil
		}
		return false, fmt.Errorf("current version is empty; use --force to update anyway")
	}

	cmp, err := compareSemver(current, latest)
	if err != nil {
		return false, fmt.Errorf("cannot compare versions: %w", err)
	}
	return cmp < 0 || force, nil
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return strings.TrimSpace(v)
}

// compareSemver returns -1 if a < b, 0 if equal, 1 if a > b.
// It expects inputs like "0.3.9" or "0.3.9-beta.1".
func compareSemver(a, b string) (int, error) {
	aPre, bPre := "", ""
	if i := strings.Index(a, "-"); i >= 0 {
		aPre = a[i+1:]
		a = a[:i]
	}
	if i := strings.Index(b, "-"); i >= 0 {
		bPre = b[i+1:]
		b = b[:i]
	}

	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	for i := 0; i < len(aParts) || i < len(bParts); i++ {
		ai, bi := 0, 0
		if i < len(aParts) {
			n, err := strconv.Atoi(aParts[i])
			if err != nil {
				return 0, fmt.Errorf("invalid version component %q", aParts[i])
			}
			ai = n
		}
		if i < len(bParts) {
			n, err := strconv.Atoi(bParts[i])
			if err != nil {
				return 0, fmt.Errorf("invalid version component %q", bParts[i])
			}
			bi = n
		}
		if ai < bi {
			return -1, nil
		}
		if ai > bi {
			return 1, nil
		}
	}

	// A version without a pre-release is newer than one with a pre-release.
	if aPre == "" && bPre != "" {
		return 1, nil
	}
	if aPre != "" && bPre == "" {
		return -1, nil
	}
	if aPre == bPre {
		return 0, nil
	}
	if aPre < bPre {
		return -1, nil
	}
	return 1, nil
}

// AssetName returns the expected release asset name for the running platform.
func AssetName() (string, error) {
	key := runtime.GOOS + "-" + runtime.GOARCH
	switch key {
	case "darwin-amd64":
		return "routatic-proxy_darwin-amd64", nil
	case "darwin-arm64":
		return "routatic-proxy_darwin-arm64", nil
	case "linux-amd64":
		return "routatic-proxy_linux-amd64", nil
	case "linux-arm64":
		return "routatic-proxy_linux-arm64", nil
	case "windows-amd64":
		return "routatic-proxy_windows-amd64.exe", nil
	case "windows-arm64":
		return "routatic-proxy_windows-arm64.exe", nil
	default:
		return "", fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

// Download fetches the release asset and returns the path to the temporary file.
func Download(ctx context.Context, info *ReleaseInfo, dir string) (string, error) {
	return downloadWithClient(ctx, defaultClient, "", info, dir)
}

func downloadWithClient(ctx context.Context, client *http.Client, version string, info *ReleaseInfo, dir string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.AssetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent(version))

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot download asset: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("asset download returned %s", resp.Status)
	}

	pattern := "routatic-proxy-update-*"
	if strings.HasSuffix(info.AssetName, ".exe") {
		pattern += ".exe"
	}
	out, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("cannot create temporary file: %w", err)
	}
	defer func() {
		if err != nil {
			_ = out.Close()
			_ = os.Remove(out.Name())
		}
	}()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", fmt.Errorf("cannot write asset: %w", err)
	}
	if err := out.Close(); err != nil {
		return "", fmt.Errorf("cannot close asset file: %w", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(out.Name(), 0755); err != nil {
			return "", fmt.Errorf("cannot make asset executable: %w", err)
		}
	}

	return out.Name(), nil
}

// VerifyChecksum verifies the downloaded file against checksums.txt.
func VerifyChecksum(ctx context.Context, info *ReleaseInfo, assetPath string) error {
	return verifyChecksumWithClient(ctx, defaultClient, "", info, assetPath)
}

func verifyChecksumWithClient(ctx context.Context, client *http.Client, version string, info *ReleaseInfo, assetPath string) error {
	if info.ChecksumURL == "" {
		return fmt.Errorf("no checksums.txt available")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.ChecksumURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent(version))

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot download checksums: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("checksum download returned %s", resp.Status)
	}

	expected := ""
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[1] == info.AssetName {
			expected = fields[0]
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("cannot read checksums: %w", err)
	}
	if expected == "" {
		return fmt.Errorf("checksum not found for %s", info.AssetName)
	}

	f, err := os.Open(assetPath)
	if err != nil {
		return fmt.Errorf("cannot open downloaded asset: %w", err)
	}
	defer func() { _ = f.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return fmt.Errorf("cannot hash asset: %w", err)
	}
	got := hex.EncodeToString(hash.Sum(nil))

	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", info.AssetName, expected, got)
	}
	return nil
}

// Replace swaps the running binary with the downloaded one.
// It returns the path to the backup file.
func Replace(currentPath, tempPath string) (string, error) {
	backupPath := currentPath + ".old"

	// Remove any stale backup from a previous update.
	_ = os.Remove(backupPath)

	if err := os.Rename(currentPath, backupPath); err != nil {
		return "", fmt.Errorf("cannot back up current binary: %w", err)
	}

	if err := os.Rename(tempPath, currentPath); err != nil {
		// Best-effort rollback.
		_ = os.Rename(backupPath, currentPath)
		return "", fmt.Errorf("cannot install new binary: %w", err)
	}

	cleanupBackup(backupPath)
	return backupPath, nil
}

// cleanupBackup is replaced on Windows where the running binary is locked.
var cleanupBackup = func(path string) { _ = os.Remove(path) }

// Apply orchestrates the full update.
func Apply(ctx context.Context, opts Options) (*Result, error) {
	client := opts.HTTPClient
	if client == nil {
		client = defaultClient
	}

	info, err := checkWithClient(ctx, client, opts.CurrentVersion)
	if err != nil {
		return nil, err
	}

	needs, err := NeedsUpdate(opts.CurrentVersion, info.TagName, opts.Force)
	if err != nil {
		return nil, err
	}

	if !needs {
		return &Result{
			Updated:    false,
			OldVersion: opts.CurrentVersion,
			NewVersion: info.TagName,
		}, nil
	}

	tempPath, err := downloadWithClient(ctx, client, opts.CurrentVersion, info, "")
	if err != nil {
		return nil, err
	}

	if !opts.SkipChecksum && info.ChecksumURL != "" {
		if err := verifyChecksumWithClient(ctx, client, opts.CurrentVersion, info, tempPath); err != nil {
			_ = os.Remove(tempPath)
			return nil, err
		}
	}

	currentPath := opts.CurrentBinaryPath
	if currentPath == "" {
		p, err := os.Executable()
		if err != nil {
			_ = os.Remove(tempPath)
			return nil, fmt.Errorf("cannot locate current binary: %w", err)
		}
		currentPath = p
	}

	backupPath, err := Replace(currentPath, tempPath)
	if err != nil {
		_ = os.Remove(tempPath)
		return nil, err
	}

	return &Result{
		Updated:    true,
		OldVersion: opts.CurrentVersion,
		NewVersion: info.TagName,
		NewPath:    currentPath,
		BackupPath: backupPath,
	}, nil
}
