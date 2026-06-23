# Plan: Update OpenCode Models Support

## Overview
Update routatic-proxy to support all current OpenCode Go and Zen models, add new models, and remove/document deprecated ones.

## Current State Analysis

### OpenCode Go Models Currently Supported:
- GLM-5, GLM-5.1 (chat completions)
- Kimi K2.5, Kimi K2.6 (chat completions)
- MiMo-V2.5, MiMo-V2.5-Pro (chat completions)
- MiniMax M2.5, M2.7, M3 (messages/Anthropic)
- Qwen3.5 Plus, Qwen3.6 Plus (messages/Anthropic)
- DeepSeek V4 Pro, V4 Flash (chat completions)

### OpenCode Zen Models Currently Supported:
- Claude Sonnet 4.5, 4.6 (messages)
- Claude Opus 4.7 (messages)
- Claude Haiku 4.5 (messages)
- DeepSeek V4 Pro, V4 Flash Free (chat completions)
- Grok Build 0.1, Big Pickle (chat completions)
- MiMo-V2.5 Free, North Mini Code Free, Nemotron 3 Ultra Free (chat completions)
- MiniMax M2.5, M2.7 (chat completions on Zen)
- GLM-5, GLM-5.1 (chat completions)
- Kimi K2.5, K2.6 (chat completions)
- Qwen models (messages on Zen)

## Changes Required

### 1. Code Changes

#### A. Update `internal/client/opencode.go`

**Add new IsAnthropicModel cases for Go provider:**
- Currently: minimax-m2.5, minimax-m2.7, minimax-m3, qwen3.5-plus, qwen3.6-plus
- Add: qwen3.7-plus, qwen3.7-max

**Update isResponsesModel for Zen:**
- Add all GPT models: gpt-5.5, gpt-5.5-pro, gpt-5.4, gpt-5.4-pro, gpt-5.4-mini, gpt-5.4-nano, gpt-5.3-codex, gpt-5.3-codex-spark, gpt-5.2, gpt-5.2-codex, gpt-5.1, gpt-5.1-codex, gpt-5.1-codex-max, gpt-5.1-codex-mini, gpt-5, gpt-5-codex, gpt-5-nano

**Update isGeminiModel for Zen:**
- Currently: gemini-3.5-flash, gemini-3.1-pro, gemini-3-flash
- Already complete

**Update isZenAnthropicModel:**
- Currently: claude-* prefix, qwen* prefix
- Already handles all Claude models correctly

#### B. Update `internal/config/model_registry.go`

**Add new model metadata entries:**
- glm-5.2: {ContextWindow: 200000, MaxOutputTokens: 8192, Vision: false, SupportsTools: true}
- kimi-k2.7-code: Already present (line 15), but verify values
- qwen3.7-plus: {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: true, SupportsTools: true}
- qwen3.7-max: {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: true, SupportsTools: true}

**Note:** kimi-k2.7-code is already in the registry but needs verification of context window (256K in docs)

#### C. Update `cmd/routatic-proxy/main.go` (default config generation)

**Add new models to default config template:**
- Add glm-5.2 entry
- Add kimi-k2.7-code entry
- Add qwen3.7-plus and qwen3.7-max entries

### 2. Documentation Updates

#### A. Update `MODELS.md`

**Add new Go models:**
- GLM-5.2: Premium model, 880 req/$12, 200K context
- Kimi K2.7 Code: Code-specialized, 1,350 req/$12, 256K context, 32K output
- Qwen3.7 Plus: 4,300 req/$12, 128K context
- Qwen3.7 Max: 950 req/$12, 128K context

**Add new Zen models:**
- Claude Fable 5: $10/$50 per 1M tokens
- Claude Opus 4.8, 4.6, 4.5, 4.1
- Claude Sonnet 4
- Claude Haiku 3.5
- GPT 5.5 series (Responses endpoint)
- GPT 5.4 series (Responses endpoint)
- GPT 5.3 Codex series (Responses endpoint)
- GPT 5.2 series (Responses endpoint)
- GPT 5.1 series (Responses endpoint)
- GPT 5 series (Responses endpoint)

**Document deprecated models:**
- GLM-5: Deprecated May 14, 2026
- GPT Codex series: Deprecated July 23, 2026
- Claude Sonnet 4: Deprecated June 15, 2026
- MiniMax M2.1: Deprecated March 15, 2026
- GLM 4.7, 4.6: Deprecated March 15, 2026
- Gemini 3 Pro: Deprecated March 9, 2026
- Kimi K2, K2 Thinking: Deprecated March 6, 2026
- Claude Haiku 3.5: Deprecated Feb 16, 2026
- Qwen3 Coder 480B: Deprecated Feb 6, 2026

#### B. Update `configs/config.example.json`

**Add new models:**
- glm-5.2 entry in models section
- kimi-k2.7-code entry
- qwen3.7-plus and qwen3.7-max entries

**Update fallbacks:**
- Add new models to appropriate fallback chains

**Add model_overrides examples:**
- Claude Fable 5
- GPT 5.5 Pro
- Gemini 3.5 Flash

#### C. Update `CLAUDE.md`

**Update Architecture section:**
- Mention new GLM-5.2 model
- Mention Kimi K2.7 Code
- Mention Qwen3.7 series
- Update endpoint classification table

#### D. Update `README.md`

**Update supported models list:**
- Add new Go models
- Add new Zen models

### 3. Test Updates

#### A. Update `internal/client/opencode_test.go`

**Add tests for:**
- ClassifyEndpoint with new GPT models
- ClassifyEndpoint with new Gemini models
- IsAnthropicModel with new Qwen models

#### B. Update `internal/router/scenarios_test.go`

**Verify routing:**
- New models are correctly routed to appropriate scenarios

## Implementation Order

1. Update `internal/config/model_registry.go` - Add metadata for new models
2. Update `internal/client/opencode.go` - Add endpoint classification for new models
3. Update `cmd/routatic-proxy/main.go` - Add new models to default config
4. Update `configs/config.example.json` - Add examples and fallbacks
5. Update `MODELS.md` - Document all models with pricing and capabilities
6. Update `CLAUDE.md` - Update architecture documentation
7. Update `README.md` - Update supported models list
8. Run tests and fix any issues

## Files to Modify

1. `internal/config/model_registry.go` - Add model metadata
2. `internal/client/opencode.go` - Update endpoint classification
3. `cmd/routatic-proxy/main.go` - Update default config template
4. `configs/config.example.json` - Add new model examples
5. `MODELS.md` - Document new and deprecated models
6. `CLAUDE.md` - Update architecture docs
7. `README.md` - Update feature list

## Deprecated Models Handling

Instead of removing deprecated models from code (which could break existing configs), we will:
1. Keep them in the code for backward compatibility
2. Document them as deprecated in MODELS.md
3. Remove them from the default config template
4. Add deprecation warnings in documentation

This ensures existing users with deprecated models in their configs don't experience breakage.

## Testing Strategy

1. Unit tests for endpoint classification
2. Unit tests for model metadata resolution
3. Integration tests for routing scenarios
4. Verify example config loads correctly

## Success Criteria

- [ ] All new OpenCode Go models are supported (GLM-5.2, Kimi K2.7 Code, Qwen3.7 Plus/Max)
- [ ] All new OpenCode Zen models are documented (Claude Fable 5, GPT series, etc.)
- [ ] Deprecated models are documented but still functional
- [ ] Default config includes new models
- [ ] Documentation is updated with pricing and capabilities
- [ ] All tests pass
