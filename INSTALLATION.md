# Installation

## Homebrew (macOS & Linux)

```bash
brew tap routatic/tap
brew install routatic-proxy
```

## Scoop (Windows)

```powershell
scoop bucket add routatic https://github.com/routatic/scoop-bucket
scoop install routatic-proxy
```

## Build from Source

```bash
git clone https://github.com/routatic/proxy.git
cd proxy
make build

# Binary is at bin/routatic-proxy
# bin/oc-go-cc is created as a compatibility alias
# Optionally install to $GOPATH/bin
make install
```

## Download a Release Binary

Download the latest release for your platform from the [Releases page](https://github.com/routatic/proxy/releases):

| Platform              | File                         |
| --------------------- | ---------------------------- |
| macOS (Apple Silicon) | `routatic-proxy_darwin-arm64`      |
| macOS (Intel)         | `routatic-proxy_darwin-amd64`      |
| Linux (x86_64)        | `routatic-proxy_linux-amd64`       |
| Linux (ARM64)         | `routatic-proxy_linux-arm64`       |
| Windows (x86_64)      | `routatic-proxy_windows-amd64.exe` |
| Windows (ARM64)       | `routatic-proxy_windows-arm64.exe` |

```bash
# macOS Apple Silicon
curl -L -o routatic-proxy https://github.com/routatic/proxy/releases/latest/download/routatic-proxy_darwin-arm64
chmod +x routatic-proxy
sudo mv routatic-proxy /usr/local/bin/

# Windows (PowerShell)
Invoke-WebRequest -Uri "https://github.com/routatic/proxy/releases/latest/download/routatic-proxy_windows-amd64.exe" -OutFile "routatic-proxy.exe"
Move-Item -Path "routatic-proxy.exe" -Destination "$env:LOCALAPPDATA\Microsoft\WindowsApps\routatic-proxy.exe"
```

Homebrew and Scoop installs also provide `oc-go-cc` as an alias for `routatic-proxy`.

## Fedora / RHEL (RPM)

Every release publishes RPMs for `x86_64` and `aarch64`, so `dnf` handles
upgrades and removal for you:

```bash
VERSION=0.6.3                       # pick a version from the Releases page
ARCH=$(uname -m)                    # x86_64 or aarch64
sudo dnf install "https://github.com/routatic/proxy/releases/download/v${VERSION}/routatic-proxy-${VERSION}-1.${ARCH}.rpm"
```

The package installs the binary to `/usr/bin/routatic-proxy`, a config template
to `/etc/routatic-proxy/config.json` (marked `noreplace`, so upgrades never
overwrite your edits), and an optional systemd **user** unit you can opt into
with `systemctl --user enable --now routatic-proxy`. The RPMs are not GPG-signed
yet — verify against `checksums.txt` on the release page. Note that
`routatic-proxy update` is for standalone binaries; on an RPM install, upgrade
through `dnf` instead.

See [docs/fedora-setup.md](docs/fedora-setup.md) for the full Fedora guide,
including the systemd and troubleshooting details.

## macOS GUI (DMG)

macOS users can install the app bundle instead of the CLI:

1. Open the [Releases page](https://github.com/routatic/proxy/releases)
2. Download `RoutaticProxy.dmg` from the latest release
3. Open it and drag the app into your Applications folder
4. Launch routatic-proxy from Launchpad or Applications

The app runs as a menu bar item rather than a window. Its menu shows the proxy's
current status and offers **Open Console...** for the dashboard, **Start Proxy** /
**Stop Proxy**, and a **Start on Boot** toggle. The same functionality is
available from the CLI via `routatic-proxy start`, `stop`, `status`, and
`autostart enable`.

## Docker

### Pull the prebuilt image

Prebuilt multi-arch images (linux/amd64, linux/arm64) are published to GitHub Container Registry:

```bash
# Latest stable release
docker pull ghcr.io/routatic/proxy:latest

# Latest beta (newest prerelease build)
docker pull ghcr.io/routatic/proxy:beta

# A specific stable release
docker pull ghcr.io/routatic/proxy:v1.0.0

docker run -d --restart unless-stopped --name routatic-proxy \
  --env-file .env -p 3456:3456 ghcr.io/routatic/proxy:latest
```

### Quick start with Makefile

```bash
cp .env.example .env
# Edit .env and put your API key
make docker-up
```

Stop the container:

```bash
make docker-stop
```

### Build and run manually

```bash
docker build -t routatic-proxy .
docker run -d --restart unless-stopped --name routatic-proxy --env-file .env -p 3456:3456 routatic-proxy
```

### Use a custom config

The Docker image uses `configs/config.json` by default (or `configs/config.example.json` as fallback). Override with a volume:

```bash
docker run -d --restart unless-stopped --name routatic-proxy --env-file .env -p 3456:3456 \
  -v /path/to/your/config.json:/etc/routatic-proxy/config.json:ro \
  routatic-proxy
```

## Requirements

- An [OpenCode Go](https://opencode.ai/auth) subscription and API key
- Go 1.21+ (only needed if building from source)
- Docker (only needed for Docker setup)

## Updating

If you downloaded a release binary directly (or built from source), update in place with the built-in command:

```bash
# See whether a newer release is available without changing anything
routatic-proxy update check

# Download the matching release and replace the running binary
routatic-proxy update
```

The updater queries the [routatic/proxy releases on GitHub](https://github.com/routatic/proxy/releases) for your current [channel](#beta-releases), picks the asset matching your OS/arch, and replaces the running binary. It resolves symlinks first, so a symlinked install is replaced at its real location instead of overwriting the link.

`update` installs without prompting, so wrap it in your own confirmation if you script it.

A binary built from source reports the version it was built with. Versions that are not valid release tags (such as `dev`) are treated as older than any release, so `update` will move you onto the newest published build.

### If the update fails with a permission error

The updater writes to the directory the binary lives in. When that directory belongs to root (`/usr/local/bin` is the common case), the update stops **before downloading** and tells you so:

```
install directory /usr/local/bin is not writable by the current user:
re-run with elevated privileges (sudo routatic-proxy update), or update
through the package manager you installed with ...
```

Either re-run with `sudo` (Unix) or from an Administrator terminal (Windows), or use whichever package manager installed the binary. The existing binary is left untouched when this happens.

### Managed installs

If you installed via a package manager, use it rather than `routatic-proxy update` — each tracks the same releases and handles uninstall/reinstall cleanly:

| Install method | Update with |
| -------------- | ----------- |
| Homebrew | `brew upgrade routatic-proxy` |
| Scoop | `scoop update routatic-proxy` |
| RPM (Fedora/RHEL) | `sudo dnf upgrade routatic-proxy` |
| Docker | `docker pull ghcr.io/routatic/proxy:latest` |

### Verifying a download

Every release publishes `checksums.txt`. `routatic-proxy update` does not verify checksums itself, so for a manual download compare the hash yourself:

```bash
curl -LO https://github.com/routatic/proxy/releases/latest/download/checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

## Beta Releases

Beta builds are published automatically from every push to `main`, so they carry the newest features and fixes before a stable release is cut. They are marked as prereleases on GitHub and versioned `v{NEXT_PATCH}-beta.{N}` — for example `v0.6.4-beta.5` is the fifth beta leading up to `v0.6.4`. The counter restarts at `1` once that version ships as stable. See [RELEASE_PROCESS.md](RELEASE_PROCESS.md) for how the two channels are built.

Beta releases publish the same assets as stable ones: platform binaries, `RoutaticProxy.dmg`, RPMs, and Docker images. They receive the same automated test and build pipeline, but no manual release review — expect the occasional rough edge, and please [open an issue](https://github.com/routatic/proxy/issues) when you hit one.

### Switch a binary install to the beta channel

```bash
# Opt in — this only changes which releases `update` looks at
routatic-proxy update-channel beta

# Show the current channel
routatic-proxy update-channel

# Install the newest beta
routatic-proxy update
```

The choice persists in `update-channel.json` in your user config directory (`~/.config/routatic-proxy/` on Linux, `~/Library/Application Support/routatic-proxy/` on macOS, `%AppData%\routatic-proxy\` on Windows). It affects only the `update` command — the proxy itself behaves identically on either channel.

### Going back to stable

```bash
routatic-proxy update-channel stable
```

This stops future betas, but it does **not** downgrade the binary you are running: a beta is newer than the current stable release, so `update` will correctly report that you are already up to date. To move back onto a stable build now, reinstall it explicitly:

```bash
# Replace with your platform's asset name from the table above
curl -L -o routatic-proxy https://github.com/routatic/proxy/releases/latest/download/routatic-proxy_linux-amd64
chmod +x routatic-proxy
sudo mv routatic-proxy /usr/local/bin/
```

Homebrew and Scoop users can reinstall the stable package instead (`brew reinstall routatic-proxy`, `scoop install routatic-proxy`).

### Install a specific beta manually

Prereleases are not served by the `/latest/download/` path, so reference the tag directly:

```bash
VERSION=v0.6.4-beta.5   # pick a prerelease from the Releases page
curl -L -o routatic-proxy "https://github.com/routatic/proxy/releases/download/${VERSION}/routatic-proxy_linux-amd64"
chmod +x routatic-proxy
sudo mv routatic-proxy /usr/local/bin/
```

The same tag works for the DMG (`RoutaticProxy.dmg`) and the RPMs, whose filenames flatten the prerelease suffix to dots:

```bash
sudo dnf install "https://github.com/routatic/proxy/releases/download/v0.6.4-beta.5/routatic-proxy-0.6.4.beta.5-1.$(uname -m).rpm"
```

Beta RPMs carry an RPM-native tilde version (`0.6.4~beta.5`), which sorts *below* `0.6.4`. So a beta RPM upgrades cleanly to the stable release once it ships — unlike the standalone binary, `dnf` handles the return to stable for you.

### Beta with Docker

```bash
# Rolling pointer to the newest beta
docker pull ghcr.io/routatic/proxy:beta

# A specific beta build
docker pull ghcr.io/routatic/proxy:v0.6.4-beta.5
```

`beta` always moves to the newest prerelease, so pin the exact tag if you need a reproducible deployment.

### Package managers and beta

Homebrew, Scoop, and the RPM `dnf` path all track **stable releases only**. To run a beta, use `routatic-proxy update-channel beta` on a standalone binary, a manual download, or the Docker `beta` tag.
