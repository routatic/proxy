# Family-based model overrides

**Issue:** [#44 — Enable routing based on model choice in Claude](https://github.com/routatic/proxy/issues/44)

## Problem

`model_overrides` only matches the request `model` string exactly. Claude Code
sends versioned model IDs (e.g. `claude-opus-4-20250514`,
`claude-sonnet-4-5-20250929`), so users cannot map a Claude *family* to a target
model without cc-switch sending a hand-crafted exact string.

The issue asks to map by Claude family:

```
Opus   → GLM-5.1
Sonnet → Kimi K2.6
Haiku  → ...
```

## Solution

Add a new config block `model_family_overrides`, keyed by family keyword
(`opus`, `sonnet`, `haiku`), each value a `ModelConfig` (identical shape to
`model_overrides`). When a request's model string contains a family keyword
(case-insensitive substring), route to the mapped target model.

```json
"model_family_overrides": {
  "opus":   { "provider": "opencode-go", "model_id": "glm-5.1" },
  "sonnet": { "provider": "opencode-go", "model_id": "kimi-k2.6" },
  "haiku":  { "provider": "opencode-go", "model_id": "qwen3.7-plus" }
}
```

## Precedence

Most-specific match wins. Fully backward compatible — existing configs behave
identically because `model_family_overrides` defaults to empty.

1. exact `model_overrides[model]`
2. `model_family_overrides[<family found in model>]`  ← **new**
3. `respect_requested_model` (if enabled)
4. scenario routing

When both an exact override and a family match exist, the exact override wins.

## Changes

### `internal/config/config.go`
- Add `ModelFamilyOverrides map[string]ModelConfig` with tag
  `json:"model_family_overrides"`.

### `internal/config/loader.go`
- Default-init the map (like `ModelOverrides`).
- Validate entries: non-empty family key, non-empty `model_id`, recognized
  provider — reuse the `model_overrides` validation logic.
- Extend the `anthropic_tools_disabled` warning loop to cover the new map.

### `internal/router/model_router.go`
- Extract a shared helper `buildOverrideResult(mc ModelConfig, fallbackKey string) RouteResult`
  used by both exact and family override paths (fallbacks:
  `Fallbacks[fallbackKey]` → `Fallbacks["default"]`, `Scenario: ScenarioOverride`).
- New `RouteWithFamilyOverride(requestedModel string) (RouteResult, bool)`:
  lowercase the model, iterate family keys **longest-first** for deterministic
  matching, return on first substring match.

### `internal/handlers/messages.go`
- In `buildModelChain`, after the exact-override branch, add a family-override
  branch with the same scenario safety-net merge behavior.

### `internal/router/policy.go`
- `ModelOverridePolicy.Evaluate` tries exact override, then family override.

### `configs/config.example.json`
- Add a documented `model_family_overrides` example.

### Documentation
- `CLAUDE.md` — mention family overrides in the routing section.
- `docs/howto-custom-routing.md` — how to configure family overrides.
- `docs/reference-api.md` — config schema reference (if it documents the schema).

## Testing

- Family substring match routes to the mapped model.
- No family match falls through to existing behavior.
- Exact `model_overrides` wins over a family match for the same request.
- Longest-key-first determinism (no ambiguous overlap surprises).
- `anthropic_tools_disabled` warning emitted for family entries.
- Streaming path honors family overrides (safety-net chain merged).
- Loader validation: empty family key, empty `model_id`, invalid provider.
