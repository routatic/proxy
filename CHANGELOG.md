# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [v0.5.0] - 2026-07-11

### Fixed
- Narrow complex scenario keywords to prevent false positives in routing (#111)
- Short-circuit fallback chain on authentication errors (401/403) to avoid unnecessary retries
- Implement provider key count logic to enhance fallback handling for auth errors
- Normalize provider strings by replacing underscores with hyphens
- Update provider retrieval to use `client.Provider` for correct model handling
- Enhance WireFormat logic to support OpenAI models and streamline fallback handling

## [v0.4.9] - 2026-07-06

### Added
- Fall back to OpenCode when Anthropic is unavailable (#107)

## [v0.4.8] - 2026-06-29

### Added
- Self-update feature for automatic binary updates

### Fixed
- Windows autostart integration (#105)

## [v0.4.7] - 2026-06-24

### Added
- macOS DMG support in release workflow with improved caching

### Changed
- Simplify target platform definition for cross-compilation

## [v0.4.6] - 2026-06-20

### Added
- Chinese documentation for installation and configuration guides
- Chinese troubleshooting guide

## [v0.4.5] - 2026-06-15

### Added
- macOS GUI with webview and system tray integration (#104)

### Fixed
- Build constraints for darwin and cgo compatibility

## [v0.4.4] - 2026-06-10

### Added
- Provider-specific API keys with comma-separated support for round-robin key rotation (#103)

## [v0.4.3] - 2026-06-05

### Fixed
- Preserve image data in normalize/denormalize bridge
- Widen and correct vision-capable model list (#99)

## [v0.4.2] - 2026-06-01

### Fixed
- Honor small `max_tokens` values in capacity filter so requests with low limits are not incorrectly routed (#100)

## [v0.4.1] - 2026-05-27

### Added
- New OpenCode models: GLM-5.2, Kimi K2.7 Code, Qwen3.7 Plus, Qwen3.7 Max (#98)

### Deprecated
- GLM-5 model (deprecated May 14, 2026); use GLM-5.1 or GLM-5.2

## [v0.4.0] - 2026-05-22

### Added
- Request/response debug capture with usage logging (#97)

## [v0.3.9] - 2026-05-17

### Fixed
- JSON-encode normalized strings in bridge
- Add k2.7-code and M3 model metadata (#89)

## [v0.3.8] - 2026-05-12

### Changed
- Update README to include Discord link and improve documentation formatting

## [v0.3.7] - 2026-05-07

### Fixed
- Race condition in heartbeat context during SSE streaming

## [v0.3.6] - 2026-05-02

### Added
- AWS Bedrock as a new provider (#85)

## [v0.3.5] - 2026-04-27

### Added
- Platform-specific PID writing for Unix and Windows for daemon mode
- Vision routing, statusline endpoint, capacity filtering, and daemon improvements (#57)

### Changed
- Remove platform-specific PID handling files (refactored approach)

## [v0.3.4] - 2026-04-22

### Fixed
- Streaming timeout handling
- Anthropic-native fallback and routing on provider-architecture branch (#84)

## [v0.3.3] - 2026-04-17

### Fixed
- Dockerfile Go version mismatch (1.24 to 1.25) (#83)

## [v0.3.2] - 2026-04-12

### Added
- Provider Abstraction + Unified Request Model + Routing Policy Engine (#82)
- Docker image publishing to GitHub Container Registry (#74)

### Changed
- Rename project from oc-go-cc to routatic-proxy (#81)

### Fixed
- SSE streaming reliability: heartbeat race, idle watchdog, fast-path, e2e tests (#79)

## [v0.3.1] - 2026-04-07

### Fixed
- Prevent DeepSeek cache invalidation from mid-conversation system-role messages (#76)

## [v0.3.0] - 2026-04-02

### Added
- Full Zen model support: Claude, GPT, Gemini, and free-tier models
- Updated model documentation for full Go + Zen provider support

## [v0.2.9] - 2026-03-28

### Added
- 4 new models: 3 Zen free-tier models + 1 Go MiniMax M3
- Qwen3.7 Plus model

### Fixed
- Qwen model routing (#73)

## [v0.2.8] - 2026-03-23

### Added
- API key rotation with round-robin across multiple keys (#72)

## [v0.2.7] - 2026-03-18

### Added
- Claude Code environment conflict check (#68)

## [v0.2.6] - 2026-03-13

### Added
- Support image multimodal content in Anthropic-to-OpenAI conversion (#71)

## [v0.2.5] - 2026-03-08

### Fixed
- Respect HTTP_PROXY/HTTPS_PROXY env vars for proxy environments

## [v0.2.4] - 2026-03-03

### Fixed
- Strip cache_control for Kimi models (#56)

## [v0.2.3] - 2026-02-26

### Added
- Model overrides for per-request model selection (#48)

## [v0.2.2] - 2026-02-21

### Added
- Qwen3.7 Max model to Anthropic endpoint routing (#63)

## [v0.2.1] - 2026-02-16

### Added
- OpenCode Zen API support with new configuration and request handling
- Enhanced model identification with Zen support
- OpenCode integration with Zen provider configuration updates

## [v0.2.0] - 2026-02-11

### Added
- Docker support (#64)

## [v0.1.9] - 2026-02-06

### Fixed
- Ensure complete SSE stream termination on EOF (#49)

## [v0.1.8] - 2026-02-01

### Fixed
- Add Thinking field and refactor thinking/effort resolution in transformer (#50)

## [v0.1.7] - 2026-01-27

### Changed
- Use `~/.cache/oc-go-cc/tiktoken` for token cache instead of `/tmp` (#41, #46)

## [v0.1.6] - 2026-01-22

### Added
- `respect_requested_model` config option to honor user-specified models (#38)

## [v0.1.5] - 2026-01-17

### Fixed
- Subtract cache tokens from input_tokens to match Anthropic API spec (#33)

## [v0.1.4] - 2026-01-12

### Fixed
- Add reasoning_content placeholder to DeepSeek text-only assistant turns (#34)

## [v0.1.3] - 2026-01-07

### Fixed
- Scoop installer post_install command to use Get-ChildItem for renaming (#35)

## [v0.1.2] - 2026-01-02

### Added
- `enable_streaming_scenario_routing` config option and updated routing logic (#29)

## [v0.1.1] - 2025-12-28

### Fixed
- Remove obsolete "budget" scenario and correct routing documentation (#8)
- Skip reasoning_effort when DeepSeek thinking mode is disabled (#26)
- Duplicate content_block_start events for streaming tool calls (#17, #18)
- Formatting (#25)

### Added
- Hot reload support for configuration file (#22)
- Detailed installation, configuration, and troubleshooting guides

### Changed
- Update license information to AGPL-3.0 in README.md

## [v0.1.0] - 2025-12-23

### Added
- Autostart functionality for macOS, Linux, and Windows (#27)
- Scoop package for Windows installation (#28)
- Chinese documentation with installation and configuration guides (#22)

## [v0.0.26] - 2025-12-18

### Fixed
- Remove obsolete "budget" scenario and correct routing documentation (#8)

## [v0.0.25] - 2025-12-13

### Fixed
- Skip reasoning_effort when DeepSeek thinking mode is disabled (#26)
- Various formatting fixes (#25)

## [v0.0.24] - 2025-12-08

### Added
- Hot reload support for configuration file (#22)

## [v0.0.23] - 2025-12-03

### Added
- Detailed installation, configuration, and troubleshooting guides
- AGPL-3.0 license information in README.md

### Fixed
- Duplicate content_block_start events for streaming tool calls (#17, #18)

## [v0.0.22] - 2025-11-28

### Added
- Platform-specific autostart functionality for macOS, Linux, and Windows (#27)

## [v0.0.21] - 2025-11-23

### Added
- FUNDING.yml with GitHub Sponsors username

## [v0.0.20] - 2025-11-18

### Added
- Scoop package for Windows (#28)

## [v0.0.19] - 2025-11-13

### Added
- CI workflow and enhanced release workflow with formatting checks (#29)

## [v0.0.18] - 2025-11-08

### Fixed
- Handle errors from os and io operations gracefully

## [v0.0.17] - 2025-11-03

### Added
- DeepSeek V4 routing and thinking support (#6)

## [v0.0.16] - 2025-10-29

### Added
- Configurable long context threshold for streaming scenarios with tests (#7)
- Enhanced DeepSeek model handling with reasoning content

## [v0.0.15] - 2025-10-24

### Added
- GNU Affero General Public License version 3

### Changed
- Remove unnecessary logging and reasoning content counting from streaming handler

## [v0.0.14] - 2025-10-19

### Added
- Windows support for background daemon mode

## [v0.0.13] - 2025-10-14

### Fixed
- Anthropic stream payloads and token usage (#9)

### Added
- Unit tests for PID handling and process status checks

## [v0.0.12] - 2025-10-09

### Added
- Enhance transformer to preserve cache control tokens with tests
- Enhance reasoning content handling with tests for response preservation
- CLAUDE.md for project guidance
- Enhance request transformer for OpenAI compatibility

## [v0.0.11] - 2025-10-04

### Changed
- Update model references to Kimi K2.6
- Add background mode support

## [v0.0.10] - 2025-09-29

### Fixed
- Update Kimi K2.6 cost efficiency rating in models guide

## [v0.0.9] - 2025-09-24

### Added
- Autostart daemon for macOS
- Background mode support

## [v0.0.8] - 2025-09-19

### Fixed
- Route complex and think scenarios from user messages (not just system prompt)

## [v0.0.7] - 2025-09-14

### Added
- Improved scenario handling logic
- 2-second heartbeat to keep Claude listening
- Rate limiter, deduplicator, and request ID generator

### Fixed
- Broken MiniMax models

## [v0.0.6] - 2025-09-09

### Added
- Budget-conscious routing

## [v0.0.5] - 2025-09-04

### Fixed
- Avoid rewriting headers while retrying a failed request

## [v0.0.4] - 2025-08-30

### Fixed
- Intel Mac support

## [v0.0.3] - 2025-08-25

### Added
- Homebrew tap release support

## [v0.0.2] - 2025-08-20

### Added
- Autostart daemon for macOS
- Config backup support

## [v0.0.1] - 2025-08-15

### Added
- Initial release of the proxy server
- Core routing of Claude Code requests to OpenCode Go
- Anthropic-to-OpenAI request/response transformation
- Basic model configuration via `config.json`
- SSE streaming support
- MiniMax model support
