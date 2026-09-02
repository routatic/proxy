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

如果你直接下载了发布二进制（或从源码构建），可以使用内置命令原地更新：

```bash
# 查看是否有新版本可用，不做任何更改
routatic-proxy update check

# 下载对应版本并替换正在运行的二进制文件
routatic-proxy update
```

更新程序按你所在的[发布通道](#beta-预览版)查询 [routatic/proxy 的 GitHub 发布页面](https://github.com/routatic/proxy/releases)，选择匹配你操作系统/架构的资源并替换当前二进制。它会先解析符号链接，因此通过符号链接安装时替换的是真实文件，而不是覆盖链接本身。

`update` 不会二次确认，写脚本时请自行加上确认步骤。

从源码构建的二进制会报告构建时写入的版本号。不是合法发布标签的版本（例如 `dev`）被视为早于任何发布版本，因此 `update` 会将其更新到最新的已发布版本。

### 更新失败并提示权限错误

更新程序需要写入二进制所在的目录。当该目录属于 root（最常见的是 `/usr/local/bin`）时，更新会在**下载之前**停止并明确告知：

```
install directory /usr/local/bin is not writable by the current user:
re-run with elevated privileges (sudo routatic-proxy update), or update
through the package manager you installed with ...
```

此时请使用 `sudo`（Unix）或在管理员终端中重新运行（Windows），或改用安装时所用的包管理器。发生该错误时，现有二进制不会被修改。

### 通过包管理器安装的情况

如果你使用包管理器安装，请用它更新，而不是 `routatic-proxy update`——它们跟踪相同的发布，并能干净地处理卸载/重新安装：

| 安装方式 | 更新命令 |
| -------- | -------- |
| Homebrew | `brew upgrade routatic-proxy` |
| Scoop | `scoop update routatic-proxy` |
| RPM（Fedora/RHEL） | `sudo dnf upgrade routatic-proxy` |
| Docker | `docker pull ghcr.io/routatic/proxy:latest` |

### 校验下载文件

每个发布都会附带 `checksums.txt`。`routatic-proxy update` 本身不校验，手动下载时请自行比对：

```bash
curl -LO https://github.com/routatic/proxy/releases/latest/download/checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

## Beta 预览版

每次推送到 `main` 都会自动发布 beta 构建，因此它包含尚未进入稳定版的最新功能与修复。它们在 GitHub 上标记为预发布，版本号形式为 `v{下一个补丁版本}-beta.{N}`——例如 `v0.6.4-beta.5` 是 `v0.6.4` 之前的第 5 个 beta。该计数在对应版本正式发布后重新从 `1` 开始。两个通道的构建方式详见 [RELEASE_PROCESS.md](../../RELEASE_PROCESS.md)。

Beta 发布包含与稳定版相同的产物：各平台二进制、`RoutaticProxy.dmg`、RPM 包和 Docker 镜像。它们经过同样的自动测试与构建流程，但没有人工发布审核——可能会有个别问题，遇到时欢迎[提交 issue](https://github.com/routatic/proxy/issues)。

### 将二进制安装切换到 beta 通道

```bash
# 加入 beta 通道——只影响 `update` 查看哪些发布
routatic-proxy update-channel beta

# 查看当前通道
routatic-proxy update-channel

# 安装最新的 beta
routatic-proxy update
```

该选择保存在用户配置目录下的 `update-channel.json`（Linux 为 `~/.config/routatic-proxy/`，macOS 为 `~/Library/Application Support/routatic-proxy/`，Windows 为 `%AppData%\routatic-proxy\`）。它只影响 `update` 命令，代理本身在两个通道上行为完全一致。

### 切回稳定版

```bash
routatic-proxy update-channel stable
```

这会停止后续 beta 更新，但**不会**降级当前正在运行的二进制：beta 比当前稳定版更新，因此 `update` 会正确地提示你已是最新版本。要立即回到稳定版，请显式重新安装：

```bash
# 请替换为你所在平台的资源名（见上文表格）
curl -L -o routatic-proxy https://github.com/routatic/proxy/releases/latest/download/routatic-proxy_linux-amd64
chmod +x routatic-proxy
sudo mv routatic-proxy /usr/local/bin/
```

Homebrew 和 Scoop 用户也可以直接重新安装稳定包（`brew reinstall routatic-proxy`、`scoop install routatic-proxy`）。

### 手动安装指定的 beta 版本

预发布不会出现在 `/latest/download/` 路径下，需要直接引用标签：

```bash
VERSION=v0.6.4-beta.5   # 从发布页面选择一个预发布版本
curl -L -o routatic-proxy "https://github.com/routatic/proxy/releases/download/${VERSION}/routatic-proxy_linux-amd64"
chmod +x routatic-proxy
sudo mv routatic-proxy /usr/local/bin/
```

同一个标签也适用于 DMG（`RoutaticProxy.dmg`）和 RPM 包，只是 RPM 文件名会把预发布后缀中的 `-` 写成 `.`：

```bash
sudo dnf install "https://github.com/routatic/proxy/releases/download/v0.6.4-beta.5/routatic-proxy-0.6.4.beta.5-1.$(uname -m).rpm"
```

Beta RPM 使用 RPM 原生的波浪号版本号（`0.6.4~beta.5`），排序低于 `0.6.4`。因此稳定版发布后，beta RPM 可以正常升级过去——与独立二进制不同，`dnf` 会自动帮你回到稳定版。

### 使用 Docker 运行 beta

```bash
# 始终指向最新的 beta
docker pull ghcr.io/routatic/proxy:beta

# 指定某个具体的 beta 构建
docker pull ghcr.io/routatic/proxy:v0.6.4-beta.5
```

`beta` 标签会随新预发布不断移动，需要可复现的部署请固定完整标签。

### 包管理器与 beta

Homebrew、Scoop 以及 `dnf` 路径都**只跟踪稳定版**。要运行 beta，请对独立二进制使用 `routatic-proxy update-channel beta`、手动下载，或使用 Docker 的 `beta` 标签。
