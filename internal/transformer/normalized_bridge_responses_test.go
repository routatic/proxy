package transformer

import (
	"encoding/json"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

func TestResponsesToNormalized_FunctionCallSetsToolUseStopReason(t *testing.T) {
	responsesResp := &types.ResponsesResponse{
		ID:    "resp-tool-call",
		Model: "muse-spark-1.3-contributor",
		Output: []types.ResponsesOutput{{
			Type:      "function_call",
			CallID:    "call_1",
			Name:      "Bash",
			Arguments: `{"command":"pwd"}`,
		}},
	}

	normalized := ResponsesToNormalized(responsesResp, responsesResp.Model)
	if normalized.StopReason != "tool_use" {
		t.Fatalf("normalized stop reason = %q, want tool_use", normalized.StopReason)
	}

	anthropicResp := core.DenormalizeResponse(normalized)
	if anthropicResp.StopReason != "tool_use" {
		t.Fatalf("Anthropic stop reason = %q, want tool_use", anthropicResp.StopReason)
	}
}

func TestResponsesToNormalized_TrimsWhitespaceFromToolArgumentKeys(t *testing.T) {
	responsesResp := &types.ResponsesResponse{
		ID: "resp-invalid-tool-parameters",
		Output: []types.ResponsesOutput{{
			Type:      "function_call",
			CallID:    "call_ask_user",
			Name:      "AskUserQuestion",
			Arguments: `{"questions":[{"options":[{"label":"A","description ":"answer"}]}]}`,
		}},
	}

	normalized := ResponsesToNormalized(responsesResp, responsesResp.Model)
	input := normalized.Messages[0].Blocks[0].Input

	var arguments struct {
		Questions []struct {
			Options []map[string]string `json:"options"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(input, &arguments); err != nil {
		t.Fatalf("unmarshal normalized tool arguments: %v", err)
	}
	option := arguments.Questions[0].Options[0]
	if got := option["description"]; got != "answer" {
		t.Fatalf("description = %q, want answer; normalized input = %s", got, input)
	}
	if _, ok := option["description "]; ok {
		t.Fatalf("normalized input retained whitespace-suffixed key: %s", input)
	}
}

// TestNormalizedToResponses_ToolResultOnlyMessage pins the regression that
// 400s muse-spark-1.2-contributor: a user message whose only content is a
// tool_result used to emit {"role":"user"} with no content, which the
// Responses API rejects.
func TestNormalizedToResponses_ToolResultOnlyMessage(t *testing.T) {
	req := &core.NormalizedRequest{
		Model: "muse-spark-1.2-contributor",
		Messages: []core.NormalizedMessage{{
			Role: "user",
			Blocks: []core.NormalizedContentBlock{{
				Type: "tool_result", ToolUseID: "t1",
				Content: json.RawMessage(`"42"`),
			}},
		}},
	}

	responsesReq := NormalizedToResponses(req, config.ModelConfig{ModelID: "muse-spark-1.2-contributor"})

	if len(responsesReq.Input) != 1 {
		t.Fatalf("input count = %d, want 1", len(responsesReq.Input))
	}
	item := responsesReq.Input[0]
	if item.Type != "function_call_output" {
		t.Errorf("item type = %q, want function_call_output", item.Type)
	}
	if item.CallID != "t1" {
		t.Errorf("call_id = %q, want t1", item.CallID)
	}
	if string(item.Output) != `"42"` {
		t.Errorf("output = %s, want \"42\"", item.Output)
	}
	if item.Role != "" {
		t.Errorf("role = %q, want empty for a typed item", item.Role)
	}
}

// TestNormalizedToResponses_AssistantToolUseMessage verifies tool_use blocks
// become typed function_call items instead of being folded into text.
func TestNormalizedToResponses_AssistantToolUseMessage(t *testing.T) {
	req := &core.NormalizedRequest{
		Model: "muse-spark-1.2-contributor",
		Messages: []core.NormalizedMessage{{
			Role: "assistant",
			Blocks: []core.NormalizedContentBlock{{
				Type: "tool_use", ID: "t1", Name: "get_weather",
				Input: json.RawMessage(`{"location":"SF"}`),
			}},
		}},
	}

	responsesReq := NormalizedToResponses(req, config.ModelConfig{ModelID: "muse-spark-1.2-contributor"})

	if len(responsesReq.Input) != 1 {
		t.Fatalf("input count = %d, want 1", len(responsesReq.Input))
	}
	item := responsesReq.Input[0]
	if item.Type != "function_call" {
		t.Errorf("item type = %q, want function_call", item.Type)
	}
	if item.CallID != "t1" {
		t.Errorf("call_id = %q, want t1", item.CallID)
	}
	if item.Name != "get_weather" {
		t.Errorf("name = %q, want get_weather", item.Name)
	}
	if item.Arguments != `{"location":"SF"}` {
		t.Errorf("arguments = %q, want the raw JSON", item.Arguments)
	}
}

// TestNormalizedToResponses_ThinkingOnlyMessage verifies a message whose only
// content is thinking emits no input item.
func TestNormalizedToResponses_ThinkingOnlyMessage(t *testing.T) {
	req := &core.NormalizedRequest{
		Model: "muse-spark-1.2-contributor",
		Messages: []core.NormalizedMessage{{
			Role:   "assistant",
			Blocks: []core.NormalizedContentBlock{{Type: "thinking", Thinking: "hmm"}},
		}},
	}

	responsesReq := NormalizedToResponses(req, config.ModelConfig{ModelID: "muse-spark-1.2-contributor"})

	if len(responsesReq.Input) != 0 {
		t.Fatalf("input count = %d, want 0", len(responsesReq.Input))
	}
}

// TestNormalizedToResponses_RoleMapping verifies only the roles the Responses
// API knows survive; system/tool text maps to user.
func TestNormalizedToResponses_RoleMapping(t *testing.T) {
	req := &core.NormalizedRequest{
		Model: "muse-spark-1.2-contributor",
		Messages: []core.NormalizedMessage{
			{Role: "system", Blocks: []core.NormalizedContentBlock{{Type: "text", Text: "sys"}}},
			{Role: "tool", Blocks: []core.NormalizedContentBlock{{Type: "text", Text: "tool"}}},
			{Role: "developer", Blocks: []core.NormalizedContentBlock{{Type: "text", Text: "dev"}}},
			{Role: "user", Blocks: []core.NormalizedContentBlock{{Type: "text", Text: "usr"}}},
			{Role: "assistant", Blocks: []core.NormalizedContentBlock{{Type: "text", Text: "ast"}}},
		},
	}

	responsesReq := NormalizedToResponses(req, config.ModelConfig{ModelID: "muse-spark-1.2-contributor"})

	want := []string{"user", "user", "developer", "user", "assistant"}
	if len(responsesReq.Input) != len(want) {
		t.Fatalf("input count = %d, want %d", len(responsesReq.Input), len(want))
	}
	for i, role := range want {
		if got := responsesReq.Input[i].Role; got != role {
			t.Errorf("input[%d] role = %q, want %q", i, got, role)
		}
	}
}
