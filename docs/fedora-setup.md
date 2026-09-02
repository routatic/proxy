# Fedora 44 Setup Guide

This guide covers setting up, configuring, and using routatic-proxy on Fedora 44.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Installation Methods](#installation-methods)
- [Configuration](#configuration)
- [Running the Proxy](#running-the-proxy)
- [Configuring Claude Code](#configuring-claude-code)
- [Systemd Service Setup](#systemd-service-setup)
- [Auto-start on Login](#auto-start-on-login)
- [Troubleshooting](#troubleshooting)
- [Updating](#updating)

---

## Prerequisites

Before installing routatic-proxy, ensure you have:

1. **An OpenCode account** with an API key from [opencode.ai](https://opencode.ai/)
2. **Claude Code CLI** installed (optional, for using with Claude Code)
3. **Basic familiarity** with the terminal

### System Requirements

- Fedora 44 (or compatible Fedora version)
- Internet connectivity for API calls
- At least 100MB disk space

---

## Installation Methods

### Method 1: RPM Package with dnf (Recommended)

Every release publishes RPMs for `x86_64` and `aarch64`. Installing the RPM puts
the binary at `/usr/bin/routatic-proxy`, ships an optional systemd **user** unit,
and lets `dnf` handle upgrades and removal.

```bash
# Pick the version you want from the Releases page, e.g. v0.5.3
VERSION=0.5.3

# x86_64 (most common)
sudo dnf install "https://github.com/routatic/proxy/releases/download/v${VERSION}/routatic-proxy-${VERSION}-1.x86_64.rpm"

# aarch64 (ARM64)
sudo dnf install "https://github.com/routatic/proxy/releases/download/v${VERSION}/routatic-proxy-${VERSION}-1.aarch64.rpm"

# Verify installation
routatic-proxy --version
```

To always grab the newest release without hardcoding a version:

```bash
RPM_URL=$(curl -fsSL https://api.github.com/repos/routatic/proxy/releases/latest \
  | grep -o 'https://[^"]*\.'"$(uname -m)"'\.rpm')
sudo dnf install "$RPM_URL"
```

Beta channel RPMs carry a `~beta.N` version suffix (RPM version `0.6.4~beta.5`),
which RPM sorts *below* the matching stable `0.6.4` release — so a later
`dnf upgrade` moves you onto stable cleanly. Note that the *filename* renders
that suffix with dots, so install a beta by tag like this:

```bash
sudo dnf install "https://github.com/routatic/proxy/releases/download/v0.6.4-beta.5/routatic-proxy-0.6.4.beta.5-1.$(uname -m).rpm"
```

See [Beta Releases](../INSTALLATION.md#beta-releases) for the full beta workflow.

What the package installs:

| Path | Purpose |
|------|---------|
| `/usr/bin/routatic-proxy` | The binary (`/usr/bin/oc-go-cc` is a symlink to it) |
| `/etc/routatic-proxy/config.json` | System-wide config template, `%config(noreplace)` — your edits survive upgrades |
| `/usr/lib/systemd/user/routatic-proxy.service` | Optional systemd user unit, disabled by default |
| `/usr/share/doc/routatic-proxy/` | README, configuration, troubleshooting, this guide |
| `/usr/share/licenses/routatic-proxy/LICENSE` | AGPL-3.0-only license text |

Removing it:

```bash
sudo dnf remove routatic-proxy
```

#### A Note on Signatures

The published RPMs are **not GPG-signed yet**, so `dnf` will not be able to verify
their provenance. Until signing lands, verify the download against the
`checksums.txt` asset attached to the same release:

```bash
curl -fsSLO "https://github.com/routatic/proxy/releases/download/v${VERSION}/checksums.txt"
sha256sum -c checksums.txt --ignore-missing
```

The plan is to sign releases with a project GPG key (and likely publish a COPR
repository so `dnf` can resolve upgrades directly). Neither exists today.

### Method 2: Download Pre-built Binary

Download the latest Linux binary from the [Releases page](https://github.com/routatic/proxy/releases):

```bash
# Download for x86_64 (most common)
curl -L -o routatic-proxy https://github.com/routatic/proxy/releases/latest/download/routatic-proxy_linux-amd64

# Download for ARM64 (aarch64)
curl -L -o routatic-proxy https://github.com/routatic/proxy/releases/latest/download/routatic-proxy_linux-arm64

# Make executable and move to PATH
chmod +x routatic-proxy
sudo mv routatic-proxy /usr/local/bin/

# Verify installation
routatic-proxy --version
```

### Method 3: Build from Source

Building from source requires Go 1.25.0 or later.

#### Install Go on Fedora 44

```bash
# Install Go using dnf
sudo dnf install golang

# Verify Go installation
go version
```

If the dnf version is older than 1.25.0, install Go manually:

```bash
# Download Go 1.25 (or latest)
wget https://go.dev/dl/go1.25.0.linux-amd64.tar.gz

# Extract to /usr/local
sudo tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz

# Add to PATH (add to ~/.bashrc for persistence)
export PATH=$PATH:/usr/local/go/bin

# Verify
go version
```

#### Build routatic-proxy

```bash
# Clone the repository
git clone https://github.com/routatic/proxy.git
cd proxy

# Build the binary
make build

# The binary is now at bin/routatic-proxy
# Optionally install system-wide
sudo make install

# Verify
routatic-proxy --version
```

### Method 4: Docker

Install Docker on Fedora 44:

```bash
# Install Docker
sudo dnf install docker docker-compose

# Enable and start Docker
sudo systemctl enable --now docker

# Add your user to docker group (optional, for non-root access)
sudo usermod -aG docker $USER
# Log out and back in for group changes to take effect
```

Run routatic-proxy with Docker:

```bash
# Clone the repository
git clone https://github.com/routatic/proxy.git
cd proxy

# Create environment file with your API key
cp .env.example .env
# Edit .env and add your API key

# Build and run
make docker-up

# Or manually:
docker build -t routatic-proxy .
docker run -d --restart unless-stopped --name routatic-proxy \
  --env-file .env -p 3456:3456 routatic-proxy
```

---

## Configuration

### Initialize Configuration

```bash
# Create default config file
routatic-proxy init
```

This creates `~/.config/routatic-proxy/config.json` with default settings.

### Configure API Key

You have three options for setting your API key:

#### Option 1: Environment Variable (Recommended)

```bash
# Add to ~/.bashrc for persistence
echo 'export ROUTATIC_PROXY_API_KEY=sk-opencode-your-key-here' >> ~/.bashrc
source ~/.bashrc
```

#### Option 2: Edit Config File

```bash
# Edit the config file
nano ~/.config/routatic-proxy/config.json
```

Find the `api_key` field and replace it:

```json
{
  "api_key": "sk-opencode-your-key-here",
  ...
}
```

#### Option 3: Provider-Specific Keys

For advanced setups with multiple providers:

```bash
# OpenCode Go key
export ROUTATIC_PROXY_OPENCODE_GO_API_KEY=sk-opencode-go-key

# OpenCode Zen key
export ROUTATIC_PROXY_OPENCODE_ZEN_API_KEY=sk-opencode-zen-key

# AWS Bedrock key
export ROUTATIC_PROXY_AWS_BEDROCK_API_KEY=your-bedrock-key
```

### Validate Configuration

```bash
routatic-proxy validate
```

### View Available Models

```bash
routatic-proxy models
```

---

## Running the Proxy

### Foreground Mode

```bash
routatic-proxy serve
```

The proxy runs on `http://127.0.0.1:3456` by default. Press `Ctrl+C` to stop.

### Background Mode

```bash
# Start in background
routatic-proxy serve -b

# Check status
routatic-proxy status

# Stop the proxy
routatic-proxy stop
```

### Custom Port

```bash
routatic-proxy serve --port 8080
```

---

## Configuring Claude Code

### Install Claude Code CLI

If you haven't installed Claude Code yet:

```bash
# Install via npm (requires Node.js)
npm install -g @anthropic-ai/claude-code

# Or download directly
curl -L https://claude.ai/code/install.sh | bash
```

### Environment Variables

Set the environment variables to route Claude Code through routatic-proxy:

```bash
# Add to ~/.bashrc for persistence
echo 'export ANTHROPIC_BASE_URL=http://127.0.0.1:3456' >> ~/.bashrc
echo 'export ANTHROPIC_AUTH_TOKEN=unused' >> ~/.bashrc
source ~/.bashrc
```

### Run Claude Code

```bash
claude
```

Claude Code will now route all requests through routatic-proxy to your configured upstream providers.

---

## Systemd Service Setup

routatic-proxy is a per-user proxy listening on loopback and reading its config
from `~/.config/routatic-proxy`, so the supported unit is a **systemd user
service**, not a machine-level daemon.

### Option A: Packaged User Service (Recommended)

The RPM ships `/usr/lib/systemd/user/routatic-proxy.service`, disabled by default.
Opt in per user — no root needed after installation:

```bash
# Optional: put your API key (and any other overrides) in the unit's env file
mkdir -p ~/.config/routatic-proxy
echo 'ROUTATIC_PROXY_API_KEY=sk-opencode-your-key-here' > ~/.config/routatic-proxy/env
chmod 600 ~/.config/routatic-proxy/env

# Enable and start for your user
systemctl --user enable --now routatic-proxy

# Check status and logs
systemctl --user status routatic-proxy
journalctl --user -u routatic-proxy -f
```

Managing it:

```bash
systemctl --user restart routatic-proxy
systemctl --user stop routatic-proxy
systemctl --user disable routatic-proxy
```

By default a user service stops when your last session ends. To keep the proxy
running after logout:

```bash
sudo loginctl enable-linger "$USER"
```

### Option B: System-wide Service (Manual)

Only needed if the proxy must serve something other than your own login session
(for example a shared host). This unit is not shipped by the package — write it
yourself.

#### Create Service File

```bash
sudo nano /etc/systemd/system/routatic-proxy.service
```

Paste the following content:

```ini
[Unit]
Description=Routatic Proxy Service
After=network.target

[Service]
Type=simple
User=%USER%
Group=%USER%
WorkingDirectory=/home/%USER%
ExecStart=/usr/bin/routatic-proxy serve
Restart=on-failure
RestartSec=5

# Environment variables
Environment="ROUTATIC_PROXY_API_KEY=sk-opencode-your-key-here"

# Or load from a file
# EnvironmentFile=/home/%USER%/.config/routatic-proxy/env

# Security settings
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=read-only
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

Replace `%USER%` with your actual username. If you installed from a tarball or
source rather than the RPM, point `ExecStart` at wherever the binary actually
lives (for example `/usr/local/bin/routatic-proxy`).

#### Enable and Start Service

```bash
# Reload systemd daemon
sudo systemctl daemon-reload

# Enable auto-start on boot
sudo systemctl enable routatic-proxy

# Start the service
sudo systemctl start routatic-proxy

# Check status
sudo systemctl status routatic-proxy

# View logs
journalctl -u routatic-proxy -f
```

#### Managing the Service

```bash
# Stop
sudo systemctl stop routatic-proxy

# Restart
sudo systemctl restart routatic-proxy

# View logs
journalctl -u routatic-proxy --since "1 hour ago"
```

---

## Auto-start on Login

For per-user auto-start without systemd:

```bash
# Enable autostart
routatic-proxy autostart enable

# Check status
routatic-proxy autostart status

# Disable autostart
routatic-proxy autostart disable
```

---

## Troubleshooting

### Common Issues

#### 1. Port Already in Use

```bash
# Check what's using port 3456
sudo ss -tlnp | grep 3456

# Kill the process if needed
sudo kill -9 <PID>

# Or use a different port
routatic-proxy serve --port 8080
```

#### 2. Permission Denied

```bash
# Ensure the binary is executable
chmod +x /usr/local/bin/routatic-proxy

# Check config directory permissions
ls -la ~/.config/routatic-proxy/
```

#### 3. Connection Refused

```bash
# Check if the proxy is running
routatic-proxy status

# Check firewall (Fedora uses firewalld)
sudo firewall-cmd --list-ports
sudo firewall-cmd --add-port=3456/tcp --permanent
sudo firewall-cmd --reload
```

#### 4. API Key Not Recognized

```bash
# Verify environment variable
echo $ROUTATIC_PROXY_API_KEY

# Check config file
cat ~/.config/routatic-proxy/config.json | grep api_key

# Validate config
routatic-proxy validate
```

### Debug Mode

Enable verbose logging for troubleshooting:

```bash
# Set log level via environment
export ROUTATIC_PROXY_LOG_LEVEL=debug
routatic-proxy serve

# Or in config file
# ~/.config/routatic-proxy/config.json:
{
  "logging": {
    "level": "debug",
    "requests": true
  }
}
```

### SELinux Considerations

Fedora uses SELinux by default. If you encounter permission issues:

```bash
# Check SELinux status
sestatus

# If enforcing and having issues, check audit logs
sudo ausearch -m avc -ts recent

# For custom binary locations, you may need to set context
sudo chcon -t bin_t /usr/local/bin/routatic-proxy
```

### View Logs

```bash
# If running as systemd service
journalctl -u routatic-proxy -f

# If running in background mode
# Logs go to stdout, view with:
routatic-proxy logs
```

---

## Updating

### RPM Update

There is no COPR repository yet, so point `dnf` at the new release's RPM:

```bash
VERSION=0.5.4
sudo dnf upgrade "https://github.com/routatic/proxy/releases/download/v${VERSION}/routatic-proxy-${VERSION}-1.$(uname -m).rpm"
```

Your `/etc/routatic-proxy/config.json` edits are preserved; if the packaged
template changed, the new version lands beside it as `config.json.rpmnew`.

> Don't use `routatic-proxy update` on an RPM install — it replaces the binary
> behind `dnf`'s back and leaves the package database out of sync.

### Binary Update

For a standalone binary (not the RPM), use the built-in updater:

```bash
# Check for updates without installing
routatic-proxy update check

# Update to the latest version on your channel
routatic-proxy update

# Opt in to beta builds (see INSTALLATION.md#beta-releases)
routatic-proxy update-channel beta
```

If the binary lives in a root-owned directory such as `/usr/local/bin`, run it
with `sudo` — the updater stops before downloading and says so when it cannot
write there.

### Manual Update

```bash
# Download new version
curl -L -o routatic-proxy https://github.com/routatic/proxy/releases/latest/download/routatic-proxy_linux-amd64
chmod +x routatic-proxy
sudo mv routatic-proxy /usr/local/bin/
```

---

## Additional Resources

- [CONFIGURATION.md](../CONFIGURATION.md) - Full configuration reference
- [MODELS.md](../MODELS.md) - Model capabilities and routing
- [TROUBLESHOOTING.md](../TROUBLESHOOTING.md) - General troubleshooting guide
- [CONTRIBUTING.md](../CONTRIBUTING.md) - Development setup

---

## Getting Help

- **Discord**: [Join the community](https://discord.gg/pUrfwfTFxM)
- **GitHub Issues**: [Report bugs or request features](https://github.com/routatic/proxy/issues)
