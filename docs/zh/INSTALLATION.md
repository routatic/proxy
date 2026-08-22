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

## Fedora / RHEL（RPM）

每个发布版本都会提供 `x86_64` 和 `aarch64` 的 RPM 包，升级和卸载都可以交给 `dnf` 处理：

```bash
VERSION=0.6.3                       # 从 Releases 页面选择版本
ARCH=$(uname -m)                    # x86_64 或 aarch64
sudo dnf install "https://github.com/routatic/proxy/releases/download/v${VERSION}/routatic-proxy-${VERSION}-1.${ARCH}.rpm"
```

该软件包将二进制文件安装到 `/usr/bin/routatic-proxy`，配置模板安装到
`/etc/routatic-proxy/config.json`（标记为 `noreplace`，升级时不会覆盖你的修改），
并附带一个可选的 systemd **用户** 单元，可通过
`systemctl --user enable --now routatic-proxy` 启用。RPM 包目前尚未进行 GPG 签名，
请使用发布页面的 `checksums.txt` 校验。注意：`routatic-proxy update` 适用于独立二进制安装；
使用 RPM 安装时，请通过 `dnf` 升级。

完整的 Fedora 指南（含 systemd 与故障排除细节）见
[docs/fedora-setup.md](../fedora-setup.md)。

## Docker

### 拉取预构建镜像

预构建的多架构镜像（linux/amd64、linux/arm64）发布在 GitHub Container Registry：

```bash
# 最新稳定版
docker pull ghcr.io/routatic/proxy:latest

# 最新 beta 版（最新的预发布构建）
docker pull ghcr.io/routatic/proxy:beta

# 特定的稳定版本
docker pull ghcr.io/routatic/proxy:v1.0.0

docker run -d --restart unless-stopped --name routatic-proxy \
  --env-file .env -p 3456:3456 ghcr.io/routatic/proxy:latest
```

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

## 更新

如果你通过 `go install` 或直接下载发布二进制安装，可以使用内置命令自行更新：

```bash
# 查看是否有新版本可用，不进行任何更改
routatic-proxy update --check

# 下载、验证校验和并原地替换正在运行的二进制文件
routatic-proxy update

# 跳过确认提示（在脚本中有用）
routatic-proxy update --yes
```

更新程序查询 [routatic/proxy 的 GitHub 发布页面](https://github.com/routatic/proxy/releases)，选择匹配你操作系统/架构的资源，在有 `checksums.txt` 时验证 SHA256，并在替换前将上一个二进制的 `.old` 备份写入运行中的可执行文件旁边。在 Windows 上，`.old` 备份会在进程退出后计划删除，因为在进程退出前运行中的可执行文件被锁定。

`dev` 版本（例如从源码编译且没有版本标签）除非传递 `--force`，否则拒绝更新。

如果你通过 **Homebrew**（`brew upgrade routatic-proxy`）或 **Scoop**（`scoop update routatic-proxy`）安装，建议使用你的包管理器——它跟踪相同的发布，并能干净地处理卸载/重新安装。
