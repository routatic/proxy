package update

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDownloadAndInstallTo_ReplacesBinary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("new-binary"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "routatic-proxy")
	if err := os.WriteFile(target, []byte("old-binary"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := downloadAndInstallTo(srv.URL, target); err != nil {
		t.Fatalf("downloadAndInstallTo() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-binary" {
		t.Errorf("installed content = %q, want %q", got, "new-binary")
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0111 == 0 {
			t.Errorf("installed binary is not executable: mode = %v", info.Mode().Perm())
		}
	}

	// No staging leftovers in the install directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".routatic-proxy-update-") {
			t.Errorf("staging file left behind: %s", e.Name())
		}
	}
}

// The staging file must live next to the target, not in the system temp dir:
// rename cannot cross filesystems, and /tmp is commonly a separate mount.
func TestDownloadAndInstallTo_StagesInInstallDir(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "routatic-proxy")
	if err := os.WriteFile(target, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}

	var staged []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// The staging file already exists by the time the body is served.
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Error(err)
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".routatic-proxy-update-") {
				staged = append(staged, e.Name())
			}
		}
		_, _ = w.Write([]byte("new"))
	}))
	defer srv.Close()

	if err := downloadAndInstallTo(srv.URL, target); err != nil {
		t.Fatalf("downloadAndInstallTo() error = %v", err)
	}
	if len(staged) != 1 {
		t.Errorf("expected exactly one staging file in the install dir during download, got %v", staged)
	}
}

func TestDownloadAndInstallTo_UnwritableDirFailsBeforeDownload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions are not enforced the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}

	dir := t.TempDir()
	installDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(installDir, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(installDir, "routatic-proxy")
	if err := os.WriteFile(target, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(installDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(installDir, 0755) })

	downloaded := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		downloaded = true
		_, _ = w.Write([]byte("new"))
	}))
	defer srv.Close()

	err := downloadAndInstallTo(srv.URL, target)
	if err == nil {
		t.Fatal("expected an error for an unwritable install directory")
	}
	if downloaded {
		t.Error("download started despite the install directory being unwritable")
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("error does not wrap fs.ErrPermission: %v", err)
	}
	for _, want := range []string{installDir, "sudo routatic-proxy update"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}

	// The old binary must be untouched.
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Errorf("target content = %q, want %q", got, "old")
	}
}

func TestPermissionError_PassesThroughOtherErrors(t *testing.T) {
	sentinel := errors.New("disk full")
	if got := permissionError("/some/dir", sentinel); got != sentinel {
		t.Errorf("permissionError() = %v, want the original error unchanged", got)
	}
}

func TestInstallPath_ResolvesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real-binary")
	if err := os.WriteFile(real, []byte("bin"), 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "linked-binary")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatal(err)
	}
	wantReal, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != wantReal {
		t.Errorf("EvalSymlinks(%q) = %q, want %q", link, resolved, wantReal)
	}

	// InstallPath resolves the running executable, which must be an existing path.
	got, err := InstallPath()
	if err != nil {
		t.Fatalf("InstallPath() error = %v", err)
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("InstallPath() = %q, which does not exist: %v", got, err)
	}
}

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		current, candidate string
		want               bool
	}{
		{"v0.6.3", "v0.6.4", true},
		{"v0.6.4", "v0.6.3", false},
		{"v0.6.3", "v0.6.3", false},
		{"v0.6.9", "v0.6.10", true},
		{"dev", "v0.6.3", true},
	}
	for _, tt := range tests {
		if got := IsNewerVersion(tt.current, tt.candidate); got != tt.want {
			t.Errorf("IsNewerVersion(%q, %q) = %v, want %v", tt.current, tt.candidate, got, tt.want)
		}
	}
}

// Real beta tags are v{VERSION}-beta.{N}; the old "-beta-" match found none of
// them, so the beta channel always reported "no beta releases found".
func TestLatestBeta_MatchesDottedBetaTags(t *testing.T) {
	releases := []GitHubRelease{
		{TagName: "v0.6.4-beta.1", Prerelease: true},
		{TagName: "v0.6.4-beta.5", Prerelease: true},
		{TagName: "v0.6.4-beta.2", Prerelease: true},
		{TagName: "v0.6.3", Prerelease: false},
	}
	latest := LatestBeta(releases)
	if latest == nil {
		t.Fatal("LatestBeta() = nil, want the newest beta release")
	}
	if latest.TagName != "v0.6.4-beta.5" {
		t.Errorf("LatestBeta() = %q, want %q", latest.TagName, "v0.6.4-beta.5")
	}
}

// GitHub orders by publish date; a re-published older beta must not win.
func TestLatestBeta_PicksBySemverNotListOrder(t *testing.T) {
	releases := []GitHubRelease{
		{TagName: "v0.6.4-beta.2", Prerelease: true},
		{TagName: "v0.6.4-beta.10", Prerelease: true},
	}
	if got := LatestBeta(releases); got == nil || got.TagName != "v0.6.4-beta.10" {
		t.Errorf("LatestBeta() = %v, want v0.6.4-beta.10", got)
	}
}

// Only the format get-versions.sh produces counts: stable releases, other
// prerelease kinds, and hyphenated spellings that the generator cannot emit.
func TestLatestBeta_IgnoresStableAndNonBetaPrereleases(t *testing.T) {
	releases := []GitHubRelease{
		{TagName: "v0.6.4", Prerelease: false},
		{TagName: "v0.7.0-rc.1", Prerelease: true},
		{TagName: "v0.7.0-beta-1", Prerelease: true},
	}
	if got := LatestBeta(releases); got != nil {
		t.Errorf("LatestBeta() = %v, want nil", got)
	}
}

func TestGetLatestRelease_BetaChannel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"tag_name":"v0.6.4-beta.5","prerelease":true,"assets":[]},
			{"tag_name":"v0.6.4-beta.1","prerelease":true,"assets":[]},
			{"tag_name":"v0.6.3","prerelease":false,"assets":[]}
		]`))
	}))
	defer srv.Close()

	release, err := getLatestReleaseFrom(srv.URL, "beta")
	if err != nil {
		t.Fatalf("getLatestReleaseFrom() error = %v", err)
	}
	if release.TagName != "v0.6.4-beta.5" {
		t.Errorf("tag = %q, want %q", release.TagName, "v0.6.4-beta.5")
	}
}

func TestGetLatestRelease_StableChannel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.6.3","prerelease":false,"assets":[]}`))
	}))
	defer srv.Close()

	release, err := getLatestReleaseFrom(srv.URL, "stable")
	if err != nil {
		t.Fatalf("getLatestReleaseFrom() error = %v", err)
	}
	if release.TagName != "v0.6.3" {
		t.Errorf("tag = %q, want %q", release.TagName, "v0.6.3")
	}
}
