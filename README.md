# routatic-proxy

[![Go Version](https://img.shields.io/github/go-mod/go-version/routatic/proxy)](https://go.dev/)
[![License](https://img.shields.io/github/license/routatic/proxy)](./LICENSE)

[Join us on Discord](https://discord.gg/pUrfwfTFxM)

**[English](./README.md)** | [中文](./README-zh.md)

A Go CLI proxy that lets you route [Claude Code](https://docs.anthropic.com/en/docs/claude-code) requests through multiple upstream providers — [OpenCode Go](https://opencode.ai/docs/go/), [OpenCode Zen](https://opencode.ai/docs/zen/), and [AWS Bedrock](https://aws.amazon.com/bedrock/) — with automatic model selection and format transformation.

`routatic-proxy` sits between Claude Code and your chosen providers, intercepting Anthropic API requests, transforming them to the appropriate format (OpenAI, Anthropic, Responses, or Gemini), and forwarding them upstream. Claude Code thinks it's talking to Anthropic — but your requests go to the models and providers you configure.

`oc-go-cc` remains available as a compatibility alias, and existing `OC_GO_CC_*` environment variables and `~/.config/oc-go-cc/config.json` files are still recognized.

---

## macOS GUI Version

This repository provides a native macOS GUI (System Tray + Console Dashboard) for `routatic-proxy`.

### Features

- **System Tray Icon** — Control the proxy server directly from the macOS status bar (Start, Stop, Autostart, Quit)
- **Interactive Dashboard** — A beautiful native console window to view real-time request history, model usage metrics, and easily edit/save your API keys without editing JSON files
- **App DMG Installer** — Package into a standard macOS app with custom icons and launch support

### How to Run

Download the compiled `.dmg` from the **Releases** page of this repository, or run the following command directly:

```bash
# Launch with native macOS GUI
routatic-proxy ui
```

---

## Why?

OpenCode Go gives you access to powerful open coding models for **$5/month** (then $10/month). OpenCode Zen provides curated, tested models with pay-as-you-go pricing. AWS Bedrock lets you run models on your own AWS infrastructure. This proxy makes all three work seamlessly with Claude Code's interface — no patches, no forks, just set two environment variables and go.

## Features

- **Multi-Provider** — Route through OpenCode Go, OpenCode Zen, or AWS Bedrock from a single config
- **Transparent Proxy** — Claude Code sends Anthropic-format requests, proxy transforms to provider-native format and back
- **Model Routing** — Automatically routes to different models based on context (default, thinking, long context, background)
- **Streaming Scenario Routing** — Configurable routing for streaming requests; enables proper scenario selection for Claude Code multi-agent and review workflows (see [CONFIGURATION.md](CONFIGURATION.md#streaming-scenario-routing))
- **Fallback Chains** — If a model fails, automatically tries the next one in your configured chain
- **Anthropic-First Failover** — Keep Claude on Anthropic and use OpenCode only during rate limits or outages
- **Circuit Breaker** — Tracks model health and skips failing models to avoid latency spikes
- **Real-time Streaming** — Full SSE streaming with live format transformation
- **Tool Calling** — Proper Anthropic tool_use/tool_result <-> OpenAI/Gemini function calling translation
- **Token Counting** — Uses tiktoken (cl100k_base) for accurate token counting and context threshold detection
- **JSON Configuration** — Flexible config file with environment variable overrides and `${VAR}` interpolation
- **Hot Reload** — Watch config file for changes and reload automatically (off by default)
- **Background Mode** — Run as daemon detached from terminal
- **Auto-start on Login** — Launch on system startup via launchd (macOS)
- **Self-Update** — Check and install the latest release with one command

## Supported Models

### OpenCode Go Models

| Model              | Context      | Best For                                      |
| ------------------ | ------------ | --------------------------------------------- |
| **GLM-5.2**        | ~200K tokens | Critical architecture, production code review |
| **Kimi K2.7 Code** | ~256K tokens | Large code generation, 32K max output         |
| **Qwen3.7 Plus**   | ~128K tokens | General coding, better quality than Qwen3.6   |
| **Qwen3.7 Max**    | ~128K tokens | Complex coding, Qwen's best quality           |

See [MODELS.md](MODELS.md) for the complete model list including costs and routing recommendations.

### OpenCode Zen Models

Zen provides pay-as-you-go access to additional models:

- **Claude Models**: Claude Fable 5, Claude Opus 4.8/4.6/4.5/4.1, Claude Sonnet 4
- **Gemini Models**: Gemini 3.5 Flash, Gemini 3.1 Pro, Gemini 3 Flash
- **GPT Models**: GPT 5.5, GPT 5.4, GPT 5.3 Codex, and more
- **Free Tier**: Nemotron 3 Ultra Free, MiMo V2.5 Free, DeepSeek V4 Flash Free, and others

See [MODELS.md](MODELS.md#opencodes-zen) for the full Zen model list.

### Deprecated Models

The following models are deprecated and will be removed:

- GPT 5.2/5.1/5 Codex variants (replaced by GPT 5.3 Codex)
- Claude Sonnet 4 (replaced by Claude Sonnet 4.5/4.6)
- GLM 5/4.7/4.6 (replaced by GLM 5.1/5.2)
- MiniMax M2.1 (replaced by MiniMax M2.5/M2.7/M3)
- Gemini 3 Pro (replaced by Gemini 3.1 Pro)
- Kimi K2/K2 Thinking (replaced by Kimi K2.5/K2.6/K2.7 Code)

See [MODELS.md](MODELS.md#deprecated-zen-models) for the complete deprecation schedule.

## Quick Start

### 1. Install

```bash
# macOS / Linux
brew tap routatic/tap && brew install routatic-proxy

# Windows
scoop bucket add routatic https://github.com/routatic/scoop-bucket && scoop install routatic-proxy

# Docker (with Makefile)
cp .env.example .env                    # then put your API key in .env
make docker-up

# Docker (manual)
cp .env.example .env
docker build -t routatic-proxy .
docker run -d --restart unless-stopped --name routatic-proxy --env-file .env -p 3456:3456 routatic-proxy

# Docker from GitHub Container Registry
docker pull ghcr.io/routatic/proxy:latest
docker run -d --restart unless-stopped --name routatic-proxy --env-file .env -p 3456:3456 ghcr.io/routatic/proxy:latest
```

Or see [INSTALLATION.md](INSTALLATION.md) for more options.

### 2. Initialize Configuration

```bash
routatic-proxy init
```

Creates a default config at `~/.config/routatic-proxy/config.json`. Edit it to add your API key, or set the environment variable:

```bash
export ROUTATIC_PROXY_API_KEY=sk-opencode-your-key-here
```

### 3. Start the Proxy

```bash
routatic-proxy serve
```

Stop the Docker container (if using Docker):

```bash
make docker-stop
```

### 4. Configure Claude Code

For the default OpenCode-only mode:

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:3456
export ANTHROPIC_AUTH_TOKEN=unused
```

For Anthropic-first mode, enable `anthropic_first` in the proxy config and set only the base URL. Do not set an API key or auth token: Claude Code will keep using its saved Claude subscription login.

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:3456
unset ANTHROPIC_AUTH_TOKEN ANTHROPIC_API_KEY
```

Anthropic-first mode falls back on HTTP 408, 429, 5xx, and connection failures. It honors `Retry-After` and uses one real request to detect recovery, so it does not spend tokens on health checks. See [CONFIGURATION.md](CONFIGURATION.md#anthropic-first-failover).

### 5. Run Claude Code

```bash
claude
```

## CLI Commands

```
routatic-proxy serve              Start the proxy server
routatic-proxy serve -b           Start in background (detached from terminal)
routatic-proxy serve --port 8080  Start on a custom port
routatic-proxy stop               Stop the running proxy server
routatic-proxy status             Check if the proxy is running
routatic-proxy init               Create default configuration file
routatic-proxy validate           Validate configuration file
routatic-proxy models             List all available models (Go, Zen, Bedrock)
routatic-proxy autostart enable   Enable auto-start on login
routatic-proxy autostart disable  Disable auto-start on login
routatic-proxy autostart status   Check autostart status
routatic-proxy update              Update to the latest release
routatic-proxy update --check      Show if an update is available
routatic-proxy update --yes        Update without prompting
routatic-proxy --version          Show version
```

## Documentation

| Document                                                     | Description                                                     |
| ------------------------------------------------------------ | --------------------------------------------------------------- |
| [INSTALLATION.md](INSTALLATION.md)                           | Homebrew, Scoop, build from source, release binaries            |
| [CONFIGURATION.md](CONFIGURATION.md)                         | Config file reference, env vars, model routing, fallback chains |
| [MODELS.md](MODELS.md)                                       | Model capabilities, costs, and routing recommendations          |
| [CONTRIBUTING.md](CONTRIBUTING.md)                           | Development setup, architecture, how it works                   |
| [TROUBLESHOOTING.md](TROUBLESHOOTING.md)                     | Common issues and debug mode                                    |
| [docs/architecture.md](docs/architecture.md)                 | System design, request flow, module overview                    |
| [docs/reference-api.md](docs/reference-api.md)               | HTTP API reference (endpoints, streaming, errors)               |
| [docs/howto-add-model.md](docs/howto-add-model.md)           | Adding new models (zero code changes)                           |
| [docs/howto-custom-routing.md](docs/howto-custom-routing.md) | Customizing scenario detection and model selection              |
| [docs/howto-debug-routing.md](docs/howto-debug-routing.md)   | Debugging routing issues and common problems                    |

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, architecture overview, and how to submit pull requests.

## License

[AGPL-3.0](LICENSE)
