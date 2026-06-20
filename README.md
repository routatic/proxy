# routatic-proxy (prev OC-GO-CC)

A Go CLI proxy that lets you use your [OpenCode Go](https://opencode.ai/docs/go/) or [OpenCode Zen](https://opencode.ai/docs/zen/) subscription with [Claude Code](https://docs.anthropic.com/en/docs/claude-code).

`routatic-proxy` sits between Claude Code and OpenCode, intercepting Anthropic API requests, transforming them to the appropriate format (OpenAI, Responses, or Gemini), and forwarding them to OpenCode's endpoints. Claude Code thinks it's talking to Anthropic — but your requests go to affordable open models instead.

`oc-go-cc` remains available as a compatibility alias, and existing `OC_GO_CC_*` environment variables and `~/.config/oc-go-cc/config.json` files are still recognized.

## Why?

OpenCode Go gives you access to powerful open coding models for **$5/month** (then $10/month). OpenCode Zen provides curated, tested models with pay-as-you-go pricing. This proxy makes both work seamlessly with Claude Code's interface — no patches, no forks, just set two environment variables and go.

## Features

- **Transparent Proxy** — Claude Code sends Anthropic-format requests, proxy transforms to OpenAI/Responses/Gemini format and back
- **Dual Provider Support** — Route models through OpenCode Go or OpenCode Zen based on your needs
- **Model Routing** — Automatically routes to different models based on context (default, thinking, long context, background)
- **Streaming Scenario Routing** — Configurable routing for streaming requests; enables proper scenario selection for Claude Code multi-agent and review workflows (see [CONFIGURATION.md](CONFIGURATION.md#streaming-scenario-routing))
- **Fallback Chains** — If a model fails, automatically tries the next one in your configured chain
- **Circuit Breaker** — Tracks model health and skips failing models to avoid latency spikes
- **Real-time Streaming** — Full SSE streaming with live format transformation
- **Tool Calling** — Proper Anthropic tool_use/tool_result <-> OpenAI/Gemini function calling translation
- **Token Counting** — Uses tiktoken (cl100k_base) for accurate token counting and context threshold detection
- **JSON Configuration** — Flexible config file with environment variable overrides and `${VAR}` interpolation
- **Hot Reload** — Watch config file for changes and reload automatically (off by default)
- **Background Mode** — Run as daemon detached from terminal
- **Auto-start on Login** — Launch on system startup via launchd (macOS)

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

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:3456
export ANTHROPIC_AUTH_TOKEN=unused
```

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
routatic-proxy models             List all available models (Go + Zen)
routatic-proxy autostart enable   Enable auto-start on login
routatic-proxy autostart disable  Disable auto-start on login
routatic-proxy autostart status   Check autostart status
routatic-proxy --version          Show version
```

## Documentation

| Document                                 | Description                                                     |
| ---------------------------------------- | --------------------------------------------------------------- |
| [INSTALLATION.md](INSTALLATION.md)       | Homebrew, Scoop, build from source, release binaries            |
| [CONFIGURATION.md](CONFIGURATION.md)     | Config file reference, env vars, model routing, fallback chains |
| [MODELS.md](MODELS.md)                   | Model capabilities, costs, and routing recommendations          |
| [CONTRIBUTING.md](CONTRIBUTING.md)       | Development setup, architecture, how it works                   |
| [TROUBLESHOOTING.md](TROUBLESHOOTING.md) | Common issues and debug mode                                    |
| [docs/architecture.md](docs/architecture.md)           | System design, request flow, module overview                    |
| [docs/reference-api.md](docs/reference-api.md)         | HTTP API reference (endpoints, streaming, errors)               |
| [docs/howto-add-model.md](docs/howto-add-model.md)     | Adding new models (zero code changes)                           |
| [docs/howto-custom-routing.md](docs/howto-custom-routing.md) | Customizing scenario detection and model selection         |
| [docs/howto-debug-routing.md](docs/howto-debug-routing.md)   | Debugging routing issues and common problems               |

## License

[AGPL-3.0](LICENSE)
