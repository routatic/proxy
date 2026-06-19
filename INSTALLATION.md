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

## Docker

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
