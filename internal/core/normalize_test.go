package core

import (
	"encoding/json"
	"testing"

	"github.com/routatic/proxy/pkg/types"
)

func TestNormalizeRequestPreservesOrderedBlocksAndCacheDirectives(t *testing.T) {
	cache := &types.CacheControl{Type: "ephemeral"}
	req := &types.MessageRequest{
		Model: "test",
		Messages: []types.Message{{
			Role: "user",
			Content: json.RawMessage(`[
				{"type":"text","text":"before","cache_control":{"type":"ephemeral"}},
				{"type":"custom_provider_block","payload":{"value":42}},
				{"type":"text","text":"after"}
			]`),
		}},
		Tools: []types.Tool{{
			Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`), CacheControl: cache,
		}},
	}

	normalized := NormalizeRequest(req)
	if len(normalized.Messages) != 1 || len(normalized.Messages[0].Blocks) != 3 {
		t.Fatalf("ordered blocks were not retained: %+v", normalized.Messages)
	}
	if normalized.Messages[0].Blocks[0].CacheControl == nil ||
		normalized.Messages[0].Blocks[0].CacheControl.Type != "ephemeral" {
		t.Fatalf("text cache directive was lost: %+v", normalized.Messages[0].Blocks[0])
	}
	if got := string(normalized.Messages[0].Blocks[1].Raw); got == "" ||
		normalized.Messages[0].Blocks[1].Type != "custom_provider_block" {
		t.Fatalf("unknown block was not preserved: type=%q raw=%q",
			normalized.Messages[0].Blocks[1].Type, got)
	}
	if normalized.Tools[0].CacheControl == nil {
		t.Fatal("tool cache directive was lost")
	}
}

func TestNormalizeRequestPreservesLegacyToolResultOutput(t *testing.T) {
	req := &types.MessageRequest{
		Model: "test",
		Messages: []types.Message{{
			Role: "tool",
			Content: json.RawMessage(`[
				{"type":"tool_result","tool_use_id":"call_1","output":"legacy result"}
			]`),
		}},
	}

	normalized := NormalizeRequest(req)
	if got, want := normalized.Messages[0].ToolResultsList()[0].Content, "legacy result"; got != want {
		t.Fatalf("legacy tool result content = %q, want %q", got, want)
	}
	if got, want := string(normalized.Messages[0].Blocks[0].Content), `"legacy result"`; got != want {
		t.Fatalf("normalized tool result content = %s, want %s", got, want)
	}
}
