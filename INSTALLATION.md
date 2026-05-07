# Installation

## Homebrew (macOS & Linux)

```bash
brew tap samueltuyizere/tap
brew install oc-go-cc
```

## Scoop (Windows)

```powershell
scoop bucket add oc-go-cc https://github.com/samueltuyizere/scoop-bucket
scoop install oc-go-cc
```

## Build from Source

```bash
git clone https://github.com/samueltuyizere/oc-go-cc.git
cd oc-go-cc
make build

# Binary is at bin/oc-go-cc
# Optionally install to $GOPATH/bin
make install
```

## Download a Release Binary

Download the latest release for your platform from the [Releases page](https://github.com/samueltuyizere/oc-go-cc/releases):

| Platform              | File                         |
| --------------------- | ---------------------------- |
| macOS (Apple Silicon) | `oc-go-cc_darwin-arm64`      |
| macOS (Intel)         | `oc-go-cc_darwin-amd64`      |
| Linux (x86_64)        | `oc-go-cc_linux-amd64`       |
| Linux (ARM64)         | `oc-go-cc_linux-arm64`       |
| Windows (x86_64)      | `oc-go-cc_windows-amd64.exe` |
| Windows (ARM64)       | `oc-go-cc_windows-arm64.exe` |

```bash
# macOS Apple Silicon
curl -L -o oc-go-cc https://github.com/samueltuyizere/oc-go-cc/releases/latest/download/oc-go-cc_darwin-arm64
chmod +x oc-go-cc
sudo mv oc-go-cc /usr/local/bin/

# Windows (PowerShell)
Invoke-WebRequest -Uri "https://github.com/samueltuyizere/oc-go-cc/releases/latest/download/oc-go-cc_windows-amd64.exe" -OutFile "oc-go-cc.exe"
Move-Item -Path "oc-go-cc.exe" -Destination "$env:LOCALAPPDATA\Microsoft\WindowsApps\oc-go-cc.exe"
```

## Requirements

- An [OpenCode Go](https://opencode.ai/auth) subscription and API key
- Go 1.21+ (only needed if building from source)
