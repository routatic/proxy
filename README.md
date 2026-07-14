# routatic-proxy

[![Go Version](https://img.shields.io/github/go-mod/go-version/routatic/proxy)](https://go.dev/)
[![License](https://img.shields.io/github/license/routatic/proxy)](./LICENSE)

[Join us on Discord](https://discord.gg/pUrfwfTFxM)

**[English](./README.md)** | [中文](./README-zh.md)

## Supported Providers

<div align="center">

[![OpenCode Go](https://img.shields.io/badge/OpenCode_Go-00C853?style=for-the-badge&logo=codeforces&logoColor=white)](https://opencode.ai/docs/go/)
[![OpenCode Zen](https://img.shields.io/badge/OpenCode_Zen-7C4DFF?style=for-the-badge&logo=codeforces&logoColor=white)](https://opencode.ai/docs/zen/)
[![AWS Bedrock](https://img.shields.io/badge/AWS_Bedrock-FF9900?style=for-the-badge&logo=amazon-aws&logoColor=white)](https://aws.amazon.com/bedrock/)
[![OpenRouter](https://img.shields.io/badge/OpenRouter-10A37F?style=for-the-badge&logo=openai&logoColor=white)](https://openrouter.ai/)
[![Anthropic](https://img.shields.io/badge/Anthropic-D4A574?style=for-the-badge&logo=anthropic&logoColor=black)](https://www.anthropic.com/)

</div>

| Provider | Description | Best For |
|----------|-------------|----------|
| **OpenCode Go** | High-performance open-source coding models with flat-rate pricing | Daily coding, complex reasoning, cost-effective workloads |
| **OpenCode Zen** | Curated, tested models with pay-as-you-go pricing | Claude/GPT/Gemini access without multiple API keys |
| **AWS Bedrock** | Enterprise-grade models on your own AWS infrastructure | Enterprises needing data sovereignty and compliance |
| **OpenRouter** | Unified API for 100+ LLMs with automatic failover | Experimenting with models from multiple providers |
| **Anthropic** | Native Claude models with anthropic-first failover mode | Claude-first workflows with OpenCode fallback |

---

A Go CLI proxy that lets you route [Claude Code](https://docs.anthropic.com/en/docs/claude-code) requests through multiple upstream providers with automatic model selection and format transformation.

`routatic-proxy` sits between Claude Code and your chosen providers, intercepting Anthropic API requests, transforming them to the appropriate format (OpenAI, Anthropic, Responses, or Gemini), and forwarding them upstream. Claude Code thinks it's talking to Anthropic — but your requests go to the models and providers you configure.

`oc-go-cc` remains available as a compatibility alias, and existing `OC_GO_CC_*` environment variables and `~/.config/oc-go-cc/config.json` files are still recognized.

---

## Why?

OpenCode Go gives you access to powerful open coding models for **$5/month** (then $10/month). OpenCode Zen provides curated, tested models with pay-as-you-go pricing. AWS Bedrock lets you run models on your own AWS infrastructure. OpenRouter gives you unified access to 100+ models. This proxy makes all of them work seamlessly with Claude Code's interface — no patches, no forks, just set two environment variables and go.

## Features

- **Multi-Provider** — Route through OpenCode Go, OpenCode Zen, AWS Bedrock, or OpenRouter from a single config
- **Transparent Proxy** — Claude Code sends Anthropic-format requests, proxy transforms to provider-native format and back
- **Model Routing** — Automatically routes to different models based on context (default, thinking, long context, background)
- **Streaming Scenario Routing** — Configurable routing for streaming requests (see [CONFIGURATION.md](CONFIGURATION.md#streaming-scenario-routing))
- **Fallback Chains** — If a model fails, automatically tries the next one in your configured chain
- **Anthropic-First Failover** — Keep Claude on Anthropic and use OpenCode only during rate limits or outages
- **Circuit Breaker** — Tracks model health and skips failing models to avoid latency spikes
- **Real-time Streaming** — Full SSE streaming with live format transformation
- **Tool Calling** — Proper Anthropic tool_use/tool_result ↔ OpenAI/Gemini function calling translation
- **Hot Reload** — Watch config file for changes and reload automatically
- **Self-Update** — Check and install the latest release with one command

See [docs/architecture.md](docs/architecture.md) for system design and request flow details.

## GUI Version

This repository provides a cross-platform GUI for `routatic-proxy`:

- **macOS** — Native Cocoa window with system tray integration (requires CGO). Download the `.dmg` from the **Releases** page.
- **Linux** — Browser-based GUI via `xdg-open` (default, no CGO required). For system tray: build with `CGO_ENABLED=1` and install `libappindicator-gtk3-devel` (Fedora) or `libayatana-appindicator3-dev` (Ubuntu/Debian).
- **Windows** — GUI not supported (CLI only).

**Dashboard tabs:** Overview (real-time metrics & model distribution), History (last 1000 requests with filters), Settings (edit config with hot-reload).

```bash
routatic-proxy ui
```

On macOS, this opens a native window. On Linux, it opens your default browser.

## Quick Start

```bash
# 1. Install
brew tap routatic/tap && brew install routatic-proxy

# 2. Initialize configuration
routatic-proxy init

# 3. Set your API key
export ROUTATIC_PROXY_API_KEY=sk-opencode-your-key-here

# 4. Start the proxy
routatic-proxy serve

# 5. Configure Claude Code
export ANTHROPIC_BASE_URL=http://127.0.0.1:3456
export ANTHROPIC_AUTH_TOKEN=unused

# 6. Run Claude Code
claude
```

See [INSTALLATION.md](INSTALLATION.md) for Homebrew, Scoop, Docker, and build-from-source options.

## CLI Commands

```
routatic-proxy serve              Start the proxy server
routatic-proxy serve -b           Start in background (detached from terminal)
routatic-proxy stop               Stop the running proxy server
routatic-proxy status             Check if the proxy is running
routatic-proxy init               Create default configuration file
routatic-proxy validate           Validate configuration file
routatic-proxy models             List all available models
routatic-proxy ui                 Launch the GUI dashboard
routatic-proxy autostart enable   Enable auto-start on login
routatic-proxy update              Update to the latest release
routatic-proxy --version          Show version
```

## Documentation

| Document | Description |
|----------|-------------|
| [docs/models.md](docs/models.md) | Model reference across all providers |
| [docs/openrouter.md](docs/openrouter.md) | OpenRouter provider setup and configuration |
| [CONFIGURATION.md](CONFIGURATION.md) | Config file reference, env vars, model routing, fallback chains |
| [MODELS.md](MODELS.md) | Complete model capabilities, costs, and routing recommendations |
| [INSTALLATION.md](INSTALLATION.md) | Homebrew, Scoop, build from source, Docker |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Development setup, architecture |
| [TROUBLESHOOTING.md](TROUBLESHOOTING.md) | Common issues and debug mode |
| [docs/architecture.md](docs/architecture.md) | System design and request flow |
| [docs/fedora-setup.md](docs/fedora-setup.md) | Fedora 44 setup (systemd, SELinux) |
| [docs/reference-api.md](docs/reference-api.md) | HTTP API reference |
| [docs/howto-add-model.md](docs/howto-add-model.md) | Adding new models (zero code changes) |
| [docs/howto-custom-routing.md](docs/howto-custom-routing.md) | Customizing scenario detection and routing |
| [docs/howto-debug-routing.md](docs/howto-debug-routing.md) | Debugging routing issues |

## Release Channels

This project uses a dual release channel system. See [RELEASE_PROCESS.md](RELEASE_PROCESS.md) for full details.

### Beta Channel (Automatic)
- **Trigger:** Every push to `main` branch
- **Version format:** `v{UPCOMING}-beta.{N}` (e.g., `v0.5.3-beta.1`), where `{N}` is a sequential counter
- **GitHub release:** Marked as prerelease
- **Use case:** Get the latest features and bug fixes immediately; ideal for testing

### Production Channel (Manual)
- **Trigger:** Manual `workflow_dispatch` on `releases` branch
- **Version format:** `vX.Y.Z` (semantic versioning)
- **GitHub release:** Marked as stable
- **Docker tags:** `vX.Y.Z`, `vX.Y`, `vX`, `latest`
- **Use case:** Stable, tested releases for production use

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, architecture overview, and how to submit pull requests.

## License

[AGPL-3.0](LICENSE)
