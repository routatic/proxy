# Update command spec

## Goal
Add a `routatic-proxy update` command that:
1. Checks the latest GitHub release of `routatic/proxy`.
2. Compares it with the currently running binary's version.
3. Downloads the correct release asset for the current OS/arch.
4. Verifies the SHA256 checksum if `checksums.txt` is available.
5. Replaces the current binary in-place (with a `.old` backup on all platforms).
6. Updates README/INSTALLATION docs to list the new command.

## Release asset naming

Repository: `routatic/proxy`
Latest release API: `https://api.github.com/repos/routatic/proxy/releases/latest`
Asset base URL pattern: `https://github.com/routatic/proxy/releases/download/<tag>/<asset>`

Assets follow the pattern `routatic-proxy_<os>-<arch>` with `.exe` suffix on Windows:

| GOOS   | GOARCH | Asset name                                   |
|--------|--------|----------------------------------------------|
| darwin | amd64  | `routatic-proxy_darwin-amd64`                |
| darwin | arm64  | `routatic-proxy_darwin-arm64`                |
| linux  | amd64  | `routatic-proxy_linux-amd64`                 |
| linux  | arm64  | `routatic-proxy_linux-arm64`                 |
| windows| amd64  | `routatic-proxy_windows-amd64.exe`           |
| windows| arm64  | `routatic-proxy_windows-arm64.exe`           |

`checksums.txt` contains one `sha256  filename` line per asset.

## Files

### `internal/updater/updater.go` (complex)

Pure-Go self-update logic. No Cobra/CLI code.

```go
package updater

const (
    Owner = "routatic"
    Repo  = "proxy"
)

// ReleaseInfo holds the data we need from the GitHub release API.
type ReleaseInfo struct {
    TagName    string
    Name       string
    Published  time.Time
    AssetName  string
    AssetURL   string
    ChecksumURL string // URL of checksums.txt if present, else empty
}

// Result is returned by Apply.
type Result struct {
    Updated     bool
    OldVersion  string
    NewVersion  string
    NewPath     string
    BackupPath  string
}

// Options controls update behavior.
type Options struct {
    CurrentVersion string
    Force          bool // install even if versions compare equal
    SkipChecksum   bool // skip SHA256 verification
    HTTPClient     *http.Client // optional; default 30s timeout
}

// Check fetches the latest release and returns its info.
func Check(ctx context.Context) (*ReleaseInfo, error)

// NeedsUpdate compares the current version with the release tag.
// Returns true if the release is newer, or if Force is set.
// Versions are normalized by stripping a leading "v" and comparing
// major.minor.patch numerically. Pre-release segments are compared
// lexicographically. "dev" is treated as older than any real release.
func NeedsUpdate(current, latest string, force bool) (bool, error)

// AssetName returns the expected asset name for the running platform.
func AssetName() (string, error)

// Download fetches the asset and returns a path to the temporary file.
func Download(ctx context.Context, info *ReleaseInfo, dir string) (tempPath string, err error)

// VerifyChecksum verifies the downloaded file against checksums.txt.
func VerifyChecksum(ctx context.Context, info *ReleaseInfo, assetPath string) error

// Replace swaps the running binary with the downloaded one.
// On Unix it renames current -> .old and temp -> current.
// On Windows it renames current -> .old, moves temp -> current, and
// schedules deletion of the .old file after a short delay because the
// running executable is locked until exit.
func Replace(currentPath, tempPath string) (backupPath string, err error)

// Apply orchestrates Check, NeedsUpdate, Download, VerifyChecksum, Replace.
func Apply(ctx context.Context, opts Options) (*Result, error)
```

Implementation details:
- Use `net/http` with a default 30-second timeout. Set `Accept: application/vnd.github+json` and a `User-Agent: routatic-proxy/<version>`.
- Parse the JSON release response manually into a small struct (do not add a GitHub SDK dependency).
- Find the asset by matching `info.AssetName()` against `asset.name`. If missing, error.
- Checksum parsing: read `checksums.txt`, split lines, find line ending with the asset name, compare lower-case hex SHA256.
- Download to `os.CreateTemp(dir, "routatic-proxy-update-*")`. On Windows the temp file should have a `.exe` extension so `os.Rename` behaves predictably. Use `filepath.Join(dir, base+".tmp")` when on Windows if the generic temp name is not `.exe`.
- After download, `os.Chmod(tempPath, 0755)` on Unix.
- `Replace`:
  - `backupPath = currentPath + ".old"`
  - Remove any existing `.old` first if possible.
  - `os.Rename(currentPath, backupPath)`.
  - `os.Rename(tempPath, currentPath)`.
  - On Windows, schedule deletion of `backupPath` with a short detached delay.

### `internal/updater/updater_test.go`

Tests:
- `TestNormalizeVersion` — strip leading `v`, keep `dev`.
- `TestCompareVersions` — equal, newer, older, pre-release ordering, `dev` older than release.
- `TestAssetName` — for each supported platform via `runtime.GOOS/GOARCH` override table.
- `TestParseChecksums` — parses `sha256  filename` lines and ignores others.
- `TestFindAsset` — picks the correct asset from a mocked list.
- `TestDownload` — uses `httptest` to serve a small asset and checksum, verifies temp file.

### `cmd/routatic-proxy/update.go` (simple)

Cobra command. Add `updateCmd()` and register it in `main.go` with `rootCmd.AddCommand(updateCmd())`.

```go
func updateCmd() *cobra.Command {
    var check, yes, force, skipChecksum bool
    cmd := &cobra.Command{
        Use:   "update",
        Short: "Update routatic-proxy to the latest release",
        RunE: func(cmd *cobra.Command, args []string) error {
            ctx := cmd.Context()
            if check {
                info, err := updater.Check(ctx)
                if err != nil { return err }
                needs, err := updater.NeedsUpdate(version, info.TagName, false)
                if err != nil { return err }
                if needs {
                    fmt.Printf("Update available: %s -> %s\n", version, info.TagName)
                } else {
                    fmt.Printf("Already up to date (%s)\n", version)
                }
                return nil
            }

            if version == "dev" && !force {
                return fmt.Errorf("current binary has version 'dev'; use --force to update anyway")
            }

            info, err := updater.Check(ctx)
            if err != nil { return err }
            needs, err := updater.NeedsUpdate(version, info.TagName, force)
            if err != nil { return err }
            if !needs {
                fmt.Printf("Already up to date (%s)\n", version)
                return nil
            }

            if !yes {
                fmt.Printf("Update %s -> %s? [y/N] ", version, info.TagName)
                var resp string
                if _, err := fmt.Scanln(&resp); err != nil {
                    return fmt.Errorf("aborted")
                }
                if strings.ToLower(strings.TrimSpace(resp)) != "y" {
                    return fmt.Errorf("update cancelled")
                }
            }

            currentPath, err := os.Executable()
            if err != nil { return err }
            currentPath = resolveExecutablePath(currentPath)

            result, err := updater.Apply(ctx, updater.Options{
                CurrentVersion: version,
                Force:          force,
                SkipChecksum:   skipChecksum,
            })
            if err != nil { return err }

            fmt.Printf("Updated %s -> %s\n", result.OldVersion, result.NewVersion)
            fmt.Printf("New binary: %s\n", result.NewPath)
            if result.BackupPath != "" {
                fmt.Printf("Backup: %s\n", result.BackupPath)
            }
            return nil
        },
    }
    cmd.Flags().BoolVarP(&check, "check", "c", false, "Only check for updates")
    cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
    cmd.Flags().BoolVarP(&force, "force", "f", false, "Update even if already on the latest version")
    cmd.Flags().BoolVar(&skipChecksum, "skip-checksum", false, "Skip SHA256 checksum verification")
    return cmd
}
```

Notes:
- Use existing `resolveExecutablePath` from `internal/daemon`? It is unexported. `cmd/routatic-proxy` already has its own `resolveExecutablePath`? No, that's in `internal/daemon/paths.go`. The command package can implement its own small helper or import `internal/daemon`. For consistency, import `internal/daemon` and use `daemon.DefaultPaths().BinaryPath` or `daemon.FindBinary()`. The simplest: use `daemon.FindBinary()` to get the canonical path.
- The command must import `internal/updater`.

### `cmd/routatic-proxy/main.go`

Add `rootCmd.AddCommand(updateCmd())` next to the other `AddCommand` calls.

### Documentation updates

#### `README.md`
- Add to the command reference block (~line 132):
  ```
  routatic-proxy update              Update to the latest release
  routatic-proxy update --check      Show if an update is available
  routatic-proxy update --yes        Update without prompting
  ```
- Add a bullet under **Features** (optional): "Self-Update — check and install the latest release with one command".

#### `INSTALLATION.md`
- Add an "Updating" section near the top or bottom with the `routatic-proxy update` command and notes about Homebrew/Scoop users using their package manager instead.

## Verification

- `go test ./internal/updater/ -v`
- `go test ./...`
- `make lint`
- Build: `make build` and run `./bin/routatic-proxy update --check` (will report "dev" unless built with a version tag).

## Acceptance criteria
- `routatic-proxy update --check` prints whether an update is available.
- `routatic-proxy update` downloads the correct asset, verifies checksum, and replaces the binary (with confirmation).
- `routatic-proxy update --yes --force` works without interaction.
- README and INSTALLATION mention the new command.
- All tests and lint pass.
