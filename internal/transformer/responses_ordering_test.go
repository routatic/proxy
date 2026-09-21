package transformer

import (
	"encoding/json"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

func TestTransformToResponses_PreservesContentBlockOrder(t *testing.T) {
	req := &types.MessageRequest{
		Messages: []types.Message{{
			Role: "assistant",
			Content: json.RawMessage(`[
				{"type":"text","text":"Before"},
				{"type":"tool_use","id":"call_1","name":"lookup","input":{"q":"one"}},
				{"type":"text","text":"After"}
			]`),
		}},
	}

	responsesReq, err := NewRequestTransformer().TransformToResponses(
		req,
		config.ModelConfig{ModelID: "gpt-5"},
	)
	if err != nil {
		t.Fatalf("TransformToResponses error: %v", err)
	}

	if len(responsesReq.Input) != 3 {
		t.Fatalf("input count = %d, want 3", len(responsesReq.Input))
	}
	if got := responsesInputText(t, responsesReq.Input[0]); got != "Before" {
		t.Errorf("input[0] text = %q, want Before", got)
	}
	if got := responsesReq.Input[1]; got.Type != "function_call" || got.CallID != "call_1" {
		t.Errorf("input[1] = %+v, want function_call call_1", got)
	}
	if got := responsesInputText(t, responsesReq.Input[2]); got != "After" {
		t.Errorf("input[2] text = %q, want After", got)
	}
}

func TestNormalizedToResponses_PreservesContentBlockOrder(t *testing.T) {
	req := &core.NormalizedRequest{
		Messages: []core.NormalizedMessage{{
			Role: "user",
			Blocks: []core.NormalizedContentBlock{
				{
					Type:      "tool_result",
					ToolUseID: "call_1",
					Content:   json.RawMessage(`"result"`),
				},
				{Type: "text", Text: "After result"},
			},
		}},
	}

	responsesReq := NormalizedToResponses(req, config.ModelConfig{ModelID: "gpt-5"})

	if len(responsesReq.Input) != 2 {
		t.Fatalf("input count = %d, want 2", len(responsesReq.Input))
	}
	if got := responsesReq.Input[0]; got.Type != "function_call_output" || got.CallID != "call_1" {
		t.Errorf("input[0] = %+v, want function_call_output call_1", got)
	}
	if got := responsesInputText(t, responsesReq.Input[1]); got != "After result" {
		t.Errorf("input[1] text = %q, want After result", got)
	}
}

func responsesInputText(t *testing.T, input types.ResponsesInput) string {
	t.Helper()
	var text string
	if err := json.Unmarshal(input.Content, &text); err != nil {
		t.Fatalf("unmarshal input content %s: %v", input.Content, err)
	}
	return text
}
