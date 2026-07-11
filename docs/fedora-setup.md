# Fedora 44 Setup Guide

This guide covers setting up, configuring, and using routatic-proxy on Fedora 44.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Installation Methods](#installation-methods)
- [Configuration](#configuration)
- [Running the Proxy](#running-the-proxy)
- [Configuring Claude Code](#configuring-claude-code)
- [Systemd Service Setup](#systemd-service-setup)
- [Troubleshooting](#troubleshooting)

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

### Method 1: Download Pre-built Binary (Recommended)

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

### Method 2: Build from Source

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

### Method 3: Docker

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

For production use, run routatic-proxy as a systemd service.

### Create Service File

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
ExecStart=/usr/local/bin/routatic-proxy serve
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

Replace `%USER%` with your actual username.

### Enable and Start Service

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

### Managing the Service

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

### Binary Update

```bash
# Check for updates
routatic-proxy update --check

# Update to latest version
routatic-proxy update

# Skip confirmation
routatic-proxy update --yes
```

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
