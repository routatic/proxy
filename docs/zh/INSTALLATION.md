# 安装指南

[English](../../INSTALLATION.md) | **中文**

## Homebrew（macOS 和 Linux）

```bash
brew tap routatic/tap
brew install routatic-proxy
```

## Scoop（Windows）

```powershell
scoop bucket add routatic https://github.com/routatic/scoop-bucket
scoop install routatic-proxy
```

## 从源码构建

```bash
git clone https://github.com/routatic/proxy.git
cd proxy
make build

# 二进制文件位于 bin/routatic-proxy
# bin/oc-go-cc 作为兼容性别名创建
# 可选：安装到 $GOPATH/bin
make install
```

## 下载发布二进制

从 [Releases 页面](https://github.com/routatic/proxy/releases) 下载适合你平台的最新版本：

| 平台 | 文件 |
|------|------|
| macOS (Apple Silicon) | `routatic-proxy_darwin-arm64` |
| macOS (Intel) | `routatic-proxy_darwin-amd64` |
| Linux (x86_64) | `routatic-proxy_linux-amd64` |
| Linux (ARM64) | `routatic-proxy_linux-arm64` |
| Windows (x86_64) | `routatic-proxy_windows-amd64.exe` |
| Windows (ARM64) | `routatic-proxy_windows-arm64.exe` |

```bash
# macOS Apple Silicon
curl -L -o routatic-proxy https://github.com/routatic/proxy/releases/latest/download/routatic-proxy_darwin-arm64
chmod +x routatic-proxy
sudo mv routatic-proxy /usr/local/bin/

# Windows (PowerShell)
Invoke-WebRequest -Uri "https://github.com/routatic/proxy/releases/latest/download/routatic-proxy_windows-amd64.exe" -OutFile "routatic-proxy.exe"
Move-Item -Path "routatic-proxy.exe" -Destination "$env:LOCALAPPDATA\Microsoft\WindowsApps\routatic-proxy.exe"
```

Homebrew 和 Scoop 安装也提供 `oc-go-cc` 作为 `routatic-proxy` 的别名。

## Docker

### 使用 Makefile 快速启动

```bash
cp .env.example .env
# 编辑 .env 并填入你的 API key
make docker-up
```

停止容器：

```bash
make docker-stop
```

### 手动构建和运行

```bash
docker build -t routatic-proxy .
docker run -d --restart unless-stopped --name routatic-proxy --env-file .env -p 3456:3456 routatic-proxy
```

### 使用自定义配置

Docker 镜像默认使用 `configs/config.json`（或 `configs/config.example.json` 作为备选）。使用卷挂载覆盖：

```bash
docker run -d --restart unless-stopped --name routatic-proxy --env-file .env -p 3456:3456 \
  -v /path/to/your/config.json:/etc/routatic-proxy/config.json:ro \
  routatic-proxy
```

## 系统要求

- [OpenCode Go](https://opencode.ai/auth) 订阅和 API key
- Go 1.21+（仅从源码构建时需要）
- Docker（仅 Docker 设置时需要）

## macOS GUI 版本

macOS 用户可以直接下载 `.dmg` 安装包：

1. 前往 [Releases 页面](https://github.com/routatic/proxy/releases)
2. 下载最新版本的 `.dmg` 文件
3. 双击安装，将应用拖入 Applications 文件夹
4. 从 Launchpad 或 Applications 文件夹启动 routatic-proxy

安装后，系统托盘图标会自动显示，点击可打开控制台面板。
