# 故障排除

[English](../../TROUBLESHOOTING.md) | **中文**

## Windows Scoop 后台模式

在 Windows 上，`routatic-proxy serve -b` 使用原生 Windows 进程 API，并保持 Scoop shim 路径不变。这意味着后台模式不需要 `nohup` 或类似 Unix 的 shell，Scoop 提供的环境变量继续工作。

## "invalid request body" 错误

这意味着代理无法解析来自 Claude Code 的请求。启用调试日志以查看原始请求：

```json
{ "logging": { "level": "debug" } }
```

或设置环境变量：

```bash
export ROUTATIC_PROXY_LOG_LEVEL=debug
```

## "all models failed" 错误

降级链中的所有模型都返回了错误。检查：

1. 你的 API key 是否有效：`routatic-proxy validate`
2. 你是否超过了[使用限制](https://opencode.ai/auth)
3. OpenCode Go 服务是否可达：`curl -H "Authorization: Bearer $ROUTATIC_PROXY_API_KEY" https://opencode.ai/zen/go/v1/models`

## 连接被拒绝

确保代理正在运行：

```bash
routatic-proxy status
```

并且 Claude Code 指向正确的地址：

```bash
echo $ANTHROPIC_BASE_URL  # 应该是 http://127.0.0.1:3456
```

## 流式传输不工作

代理实时将 OpenAI SSE 转换为 Anthropic SSE。如果流式传输出现问题：

1. 将日志级别设置为 `debug` 以查看原始 SSE 数据块
2. 检查是否有代理或防火墙正在缓冲连接
3. 先尝试非流式请求以验证模型是否工作

## `routatic-proxy update` 失败

**"install directory ... is not writable by the current user"**

更新程序需要替换二进制所在目录中的文件，而该目录属于其他用户——`/usr/local/bin` 通常属于 root。此时没有下载任何内容，现有二进制也未被修改。请使用 `sudo`（Unix）或在管理员终端中重新运行（Windows），或改用安装时的包管理器（`brew upgrade`、`scoop update`、`sudo dnf upgrade routatic-proxy`）。

**"no beta releases found"**

已选择 beta 通道但未能解析到预发布版本。用 `routatic-proxy update-channel` 确认当前通道，并在[发布页面](https://github.com/routatic/proxy/releases)确认存在预发布版本。v0.6.4 之前的版本无法匹配 `v{版本}-beta.{N}` 标签格式，因此总是报这个错——请手动安装一个新版本，见[手动安装指定的 beta 版本](INSTALLATION.md#手动安装指定的-beta-版本)。

**切回 stable 后提示 "You are already on the latest version"**

这是预期行为：beta 比当前稳定版更新，因此没有可更新的目标。切换通道不会降级。请显式重新安装稳定版，见[切回稳定版](INSTALLATION.md#切回稳定版)。

**"GitHub API returned status 403"**

未认证的 GitHub API 请求按 IP 限流。请稍后重试，或直接从[发布页面](https://github.com/routatic/proxy/releases)下载资源。

## 调试模式

要获得最大日志记录，使用调试级别运行：

```bash
ROUTATIC_PROXY_LOG_LEVEL=debug routatic-proxy serve
```

这将记录：

- 来自 Claude Code 的原始 Anthropic 请求体
- 发送到上游（OpenCode Go/Zen）的转换后请求
- 收到的上游响应
- 流式传输期间的 SSE 流事件

## 常见问题

### 代理启动但 Claude Code 无法连接

1. 检查端口是否正确：默认是 3456
2. 检查防火墙设置
3. 确保 `ANTHROPIC_BASE_URL` 正确设置

### API Key 无效

1. 在 [OpenCode 控制台](https://opencode.ai/auth) 验证你的 API key
2. 检查 key 是否正确设置在配置文件或环境变量中
3. 运行 `routatic-proxy validate` 验证配置

### 模型响应慢

1. 检查是否使用了正确的模型（某些模型比其他慢）
2. 考虑将 `fast` 场景用于流式请求
3. 检查网络延迟

### Token 计数不准确

代理使用 tiktoken (cl100k_base) 进行 token 计数。如果计数看起来不准确：

1. 这是估算值，不是精确计数
2. 不同模型可能使用不同的分词器
3. 上下文阈值检测基于此估算

## 获取帮助

如果以上方法都无法解决你的问题：

1. 查看 [GitHub Issues](https://github.com/routatic/proxy/issues)
2. 加入 [Discord](https://discord.gg/pUrfwfTFxM) 寻求帮助
3. 提交新 issue 时附上调试日志
