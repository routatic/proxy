package transformer

import (
	"encoding/json"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

func TestTransformRequestPreservesCacheControlOnNonTextBlocks(t *testing.T) {
	req := &types.MessageRequest{
		Model:     "deepseek-v4-pro",
		MaxTokens: 100,
		Messages: []types.Message{
			{
				Role: "user",
				Content: json.RawMessage(`[
					{"type":"image","source":{"type":"base64","media_type":"image/png","data":"abc"},"cache_control":{"type":"ephemeral"}},
					{"type":"tool_result","tool_use_id":"toolu_1","content":"done","cache_control":{"type":"ephemeral"}}
				]`),
			},
			{
				Role:    "assistant",
				Content: json.RawMessage(`[{"type":"thinking","thinking":"reason","cache_control":{"type":"ephemeral"}}]`),
			},
		},
	}

	out, err := NewRequestTransformer().TransformRequest(req, config.ModelConfig{
		ModelID: "deepseek-v4-pro",
		Vision:  true,
	})
	if err != nil {
		t.Fatalf("TransformRequest: %v", err)
	}

	if len(out.Messages) < 3 {
		t.Fatalf("got %d messages, want image, tool, and assistant messages", len(out.Messages))
	}
	if out.Messages[0].CacheControl == nil {
		t.Fatal("image cache directive was not preserved on the user message")
	}
	if out.Messages[1].CacheControl == nil {
		t.Fatal("tool-result cache directive was not preserved")
	}
	if out.Messages[2].CacheControl == nil {
		t.Fatal("thinking cache directive was not preserved on the assistant message")
	}
}
