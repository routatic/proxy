package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// rewriteTransport redirects all requests to the test server URL while
// preserving the original request method, headers, body, and path. This lets
// tests exercise code that hardcodes a production URL (like the GitHub API).
type rewriteTransport struct {
	target string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	target, err := url.Parse(t.target)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme = target.Scheme
	req.URL.Host = target.Host
	return http.DefaultTransport.RoundTrip(req)
}

// newTestClient returns an *http.Client whose requests are routed to the
// given test server, regardless of the URL the caller specifies.
func newTestClient(srv *httptest.Server) *http.Client {
	return &http.Client{
		Timeout:   10 * 1000_000_000, // 10s
		Transport: &rewriteTransport{target: srv.URL},
	}
}

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"v0.3.9", "0.3.9"},
		{"V0.3.9", "0.3.9"},
		{"0.3.9", "0.3.9"},
		{"  v1.0.0  ", "1.0.0"},
		{"dev", "dev"},
	}
	for _, tt := range tests {
		if got := normalizeVersion(tt.input); got != tt.want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"0.3.9", "0.3.9", 0},
		{"0.3.9", "0.3.10", -1},
		{"0.3.10", "0.3.9", 1},
		{"0.3.9", "0.4.0", -1},
		{"1.0.0", "0.9.9", 1},
		{"0.3.9-beta.1", "0.3.9", -1},
		{"0.3.9", "0.3.9-beta.1", 1},
		{"0.3.9-beta.1", "0.3.9-beta.2", -1},
		{"0.3", "0.3.0", 0},
	}
	for _, tt := range tests {
		got, err := compareSemver(tt.a, tt.b)
		if err != nil {
			t.Fatalf("compareSemver(%q, %q) error: %v", tt.a, tt.b, err)
		}
		if got != tt.want {
			t.Errorf("compareSemver(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCompareSemverInvalid(t *testing.T) {
	if _, err := compareSemver("abc", "0.3.9"); err == nil {
		t.Error("expected error for invalid version component")
	}
}

func TestNeedsUpdate(t *testing.T) {
	need, err := NeedsUpdate("v0.3.9", "v0.3.10", false)
	if err != nil || !need {
		t.Errorf("NeedsUpdate(v0.3.9, v0.3.10) = %v, %v; want true, nil", need, err)
	}

	need, err = NeedsUpdate("v0.3.10", "v0.3.9", false)
	if err != nil || need {
		t.Errorf("NeedsUpdate(v0.3.10, v0.3.9) = %v, %v; want false, nil", need, err)
	}

	need, err = NeedsUpdate("v0.3.9", "v0.3.9", false)
	if err != nil || need {
		t.Errorf("NeedsUpdate(v0.3.9, v0.3.9) = %v, %v; want false, nil", need, err)
	}

	need, err = NeedsUpdate("v0.3.9", "v0.3.9", true)
	if err != nil || !need {
		t.Errorf("NeedsUpdate(v0.3.9, v0.3.9, force) = %v, %v; want true, nil", need, err)
	}

	_, err = NeedsUpdate("dev", "v0.3.10", false)
	if err == nil {
		t.Error("NeedsUpdate(dev) expected error without force")
	}

	need, err = NeedsUpdate("dev", "v0.3.10", true)
	if err != nil || !need {
		t.Errorf("NeedsUpdate(dev, force) = %v, %v; want true, nil", need, err)
	}
}

func TestAssetName(t *testing.T) {
	asset, err := AssetName()
	if err != nil {
		t.Fatalf("AssetName() error: %v", err)
	}

	expected := "routatic-proxy_" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		expected += ".exe"
	}
	if asset != expected {
		t.Errorf("AssetName() = %q, want %q", asset, expected)
	}
}

func TestParseChecksums(t *testing.T) {
	body := "abc123  routatic-proxy_linux-amd64\ndef456  routatic-proxy_darwin-arm64\n"
	expected := "abc123"

	var got string
	for _, line := range strings.Split(body, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "routatic-proxy_linux-amd64" {
			got = fields[0]
			break
		}
	}
	if got != expected {
		t.Errorf("checksum parse = %q, want %q", got, expected)
	}
}

func TestCheckAndDownload(t *testing.T) {
	assetContent := []byte("fake binary content")
	assetHash := sha256.Sum256(assetContent)
	expectedAsset, err := AssetName()
	if err != nil {
		t.Fatalf("AssetName() error: %v", err)
	}
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(assetHash[:]), expectedAsset)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{
				"tag_name": "v0.4.0",
				"name": "v0.4.0",
				"published_at": "2026-06-22T12:00:00Z",
				"assets": [
					{"name": %q, "browser_download_url": "/download/asset"},
					{"name": "checksums.txt", "browser_download_url": "/download/checksums"}
				]
			}`, expectedAsset)
		case strings.HasSuffix(r.URL.Path, "/download/asset"):
			_, _ = w.Write(assetContent)
		case strings.HasSuffix(r.URL.Path, "/download/checksums"):
			_, _ = w.Write([]byte(checksums))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Override the default client so all requests (including the
	// hardcoded GitHub API URL) are routed to the test server.
	client := newTestClient(server)
	origClient := defaultClient
	defaultClient = client
	defer func() { defaultClient = origClient }()

	ctx := context.Background()
	info, err := Check(ctx)
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if info.TagName != "v0.4.0" {
		t.Errorf("TagName = %q, want v0.4.0", info.TagName)
	}
	if info.AssetURL == "" {
		t.Error("AssetURL is empty")
	}
	if info.ChecksumURL == "" {
		t.Error("ChecksumURL is empty")
	}

	tempPath, err := Download(ctx, info, "")
	if err != nil {
		t.Fatalf("Download() error: %v", err)
	}
	defer func() { _ = os.Remove(tempPath) }()

	data, err := os.ReadFile(tempPath)
	if err != nil {
		t.Fatalf("cannot read downloaded file: %v", err)
	}
	if string(data) != string(assetContent) {
		t.Errorf("downloaded content mismatch")
	}

	if err := VerifyChecksum(ctx, info, tempPath); err != nil {
		t.Errorf("VerifyChecksum() error: %v", err)
	}
}

func TestReplace(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "routatic-proxy")
	newBin := filepath.Join(dir, "routatic-proxy-new")

	if err := os.WriteFile(current, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newBin, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}

	backup, err := Replace(current, newBin)
	if err != nil {
		t.Fatalf("Replace() error: %v", err)
	}

	if _, err := os.Stat(current); err != nil {
		t.Errorf("new binary missing: %v", err)
	}
	got, err := os.ReadFile(current)
	if err != nil || string(got) != "new" {
		t.Errorf("new binary content = %q, want new", got)
	}

	if runtime.GOOS != "windows" {
		// On Unix the backup is removed immediately.
		if _, err := os.Stat(backup); !os.IsNotExist(err) {
			t.Errorf("expected backup %s to be removed on Unix", backup)
		}
	} else {
		// On Windows the backup is scheduled for later deletion.
		if _, err := os.Stat(backup); err != nil {
			t.Errorf("backup missing on Windows: %v", err)
		}
	}
}

func TestApplyNoUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{
			"tag_name": "v0.3.9",
			"name": "v0.3.9",
			"published_at": "2026-06-22T12:00:00Z",
			"assets": [{"name": "routatic-proxy_`+runtime.GOOS+`-`+runtime.GOARCH+`", "browser_download_url": "/asset"}]
		}`)
	}))
	defer server.Close()

	client := newTestClient(server)
	origClient := defaultClient
	defaultClient = client
	defer func() { defaultClient = origClient }()

	res, err := Apply(context.Background(), Options{CurrentVersion: "v0.3.9", CurrentBinaryPath: "/dev/null"})
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	if res.Updated {
		t.Error("expected no update")
	}
}
