package types

import (
	"encoding/json"
	"testing"
)

func TestContentBlock_MarshalJSON_TextBlock(t *testing.T) {
	block := ContentBlock{
		Type: "text",
		Text: "hello",
	}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("MarshalJSON text block: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("text block should have exactly 2 fields, got %d: %v", len(m), m)
	}
	if m["type"] != "text" {
		t.Errorf("type = %v, want text", m["type"])
	}
	if m["text"] != "hello" {
		t.Errorf("text = %v, want hello", m["text"])
	}
}

func TestContentBlock_MarshalJSON_ToolUseBlock(t *testing.T) {
	block := ContentBlock{
		Type:  "tool_use",
		ID:    "tool_123",
		Name:  "bash",
		Input: json.RawMessage(`{"command":"ls"}`),
	}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("MarshalJSON tool_use block: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(m) != 4 {
		t.Fatalf("tool_use block should have exactly 4 fields, got %d: %v", len(m), m)
	}
	if m["type"] != "tool_use" {
		t.Errorf("type = %v, want tool_use", m["type"])
	}
	if m["id"] != "tool_123" {
		t.Errorf("id = %v, want tool_123", m["id"])
	}
	if m["name"] != "bash" {
		t.Errorf("name = %v, want bash", m["name"])
	}
}

func TestContentBlock_MarshalJSON_ToolUseBlock_NilInput(t *testing.T) {
	block := ContentBlock{
		Type: "tool_use",
		ID:   "tool_456",
		Name: "read",
	}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("MarshalJSON tool_use with nil input: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	// Nil input should default to {}
	input, ok := m["input"].(map[string]interface{})
	if !ok {
		t.Errorf("input should be a map, got %T", m["input"])
	}
	if len(input) != 0 {
		t.Errorf("nil input should serialize as {}, got %v", input)
	}
}

func TestContentBlock_MarshalJSON_ToolResultBlock(t *testing.T) {
	isErr := true
	block := ContentBlock{
		Type:      "tool_result",
		ToolUseID: "tool_789",
		Content:   json.RawMessage(`"error output"`),
		IsError:   &isErr,
	}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("MarshalJSON tool_result block: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if m["type"] != "tool_result" {
		t.Errorf("type = %v, want tool_result", m["type"])
	}
	if m["tool_use_id"] != "tool_789" {
		t.Errorf("tool_use_id = %v, want tool_789", m["tool_use_id"])
	}
	if m["is_error"] != true {
		t.Errorf("is_error = %v, want true", m["is_error"])
	}
}

func TestContentBlock_MarshalJSON_ThinkingBlock(t *testing.T) {
	block := ContentBlock{
		Type:      "thinking",
		Thinking:  "I need to think about this...",
		Signature: "sig_abc",
	}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("MarshalJSON thinking block: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(m) != 3 {
		t.Fatalf("thinking block should have exactly 3 fields, got %d: %v", len(m), m)
	}
	if m["type"] != "thinking" {
		t.Errorf("type = %v, want thinking", m["type"])
	}
	if m["thinking"] != "I need to think about this..." {
		t.Errorf("thinking = %v, want correct text", m["thinking"])
	}
	if m["signature"] != "sig_abc" {
		t.Errorf("signature = %v, want sig_abc", m["signature"])
	}
}

func TestContentBlock_MarshalJSON_ImageBlock(t *testing.T) {
	block := ContentBlock{
		Type: "image",
		Source: &ImageSource{
			Type:      "base64",
			MediaType: "image/png",
			Data:      "iVBOR...",
		},
	}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("MarshalJSON image block: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("image block should have exactly 2 fields, got %d: %v", len(m), m)
	}
	if m["type"] != "image" {
		t.Errorf("type = %v, want image", m["type"])
	}
	src, ok := m["source"].(map[string]interface{})
	if !ok {
		t.Fatalf("source should be a map, got %T", m["source"])
	}
	if src["type"] != "base64" {
		t.Errorf("source.type = %v, want base64", src["type"])
	}
}

func TestContentBlock_MarshalJSON_DefaultFallback(t *testing.T) {
	block := ContentBlock{
		Type: "unknown_type",
		Text: "some text",
	}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("MarshalJSON unknown block type: %v", err)
	}
	// Unknown types should fall through to the alias marshal
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if m["type"] != "unknown_type" {
		t.Errorf("type = %v, want unknown_type", m["type"])
	}
}

func TestContentBlock_MarshalJSON_NoLeakedFields(t *testing.T) {
	// A text block should NOT leak fields from other block types
	block := ContentBlock{
		Type:      "text",
		Text:      "hello",
		ID:        "should_not_appear",
		ToolUseID: "should_not_appear",
		Name:      "should_not_appear",
		Thinking:  "should_not_appear",
		Signature: "should_not_appear",
	}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	forbidden := []string{"id", "tool_use_id", "name", "thinking", "signature", "input", "content", "source"}
	for _, key := range forbidden {
		if _, ok := m[key]; ok {
			t.Errorf("text block should not contain field %q, but it was present", key)
		}
	}
}

func TestContentBlock_MarshalJSON_ToolResultNoLeakedFields(t *testing.T) {
	block := ContentBlock{
		Type:      "tool_result",
		ToolUseID: "tool_001",
		Content:   json.RawMessage(`"done"`),
	}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	forbidden := []string{"text", "id", "name", "input", "thinking", "signature", "source"}
	for _, key := range forbidden {
		if _, ok := m[key]; ok {
			t.Errorf("tool_result block should not contain field %q, but it was present", key)
		}
	}
}

func TestDelta_OmitEmptyType(t *testing.T) {
	d := Delta{
		Text: "hello",
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("Marshal Delta: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	// Type should be omitted when empty because of omitempty
	if _, ok := m["type"]; ok {
		t.Errorf("Delta with empty Type should omit type field, got: %v", m)
	}
	if m["text"] != "hello" {
		t.Errorf("text = %v, want hello", m["text"])
	}
}

func TestMessageRequest_ThinkingField(t *testing.T) {
	req := MessageRequest{
		Model:     "claude-3",
		MaxTokens: 1024,
		Messages:  []Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		Thinking:  json.RawMessage(`{"type":"enabled","budget_tokens":5000}`),
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal MessageRequest: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	thinking, ok := m["thinking"].(map[string]interface{})
	if !ok {
		t.Fatalf("thinking should be a map, got %T: %v", m["thinking"], m["thinking"])
	}
	if thinking["type"] != "enabled" {
		t.Errorf("thinking.type = %v, want enabled", thinking["type"])
	}
}

func TestMessageRequest_SystemText(t *testing.T) {
	tests := []struct {
		name   string
		system json.RawMessage
		want   string
	}{
		{
			name:   "string system",
			system: json.RawMessage(`"You are helpful"`),
			want:   "You are helpful",
		},
		{
			name:   "array system",
			system: json.RawMessage(`[{"type":"text","text":"You are helpful"},{"type":"text","text":" Be concise"}]`),
			want:   "You are helpful Be concise",
		},
		{
			name:   "empty system",
			system: nil,
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := MessageRequest{System: tt.system}
			got := req.SystemText()
			if got != tt.want {
				t.Errorf("SystemText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMessage_ContentBlocks(t *testing.T) {
	tests := []struct {
		name    string
		content json.RawMessage
		want    []ContentBlock
	}{
		{
			name:    "string content",
			content: json.RawMessage(`"hello"`),
			want:    []ContentBlock{{Type: "text", Text: "hello"}},
		},
		{
			name:    "array content",
			content: json.RawMessage(`[{"type":"text","text":"hello"},{"type":"tool_use","id":"t1","name":"bash","input":{}}]`),
			want: []ContentBlock{
				{Type: "text", Text: "hello"},
				{Type: "tool_use", ID: "t1", Name: "bash"},
			},
		},
		{
			name:    "empty content",
			content: nil,
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Message{Role: "user", Content: tt.content}
			got := m.ContentBlocks()
			if len(got) != len(tt.want) {
				t.Fatalf("ContentBlocks() length = %d, want %d", len(got), len(tt.want))
			}
			for i, block := range got {
				if block.Type != tt.want[i].Type {
					t.Errorf("block[%d].Type = %q, want %q", i, block.Type, tt.want[i].Type)
				}
				if block.Text != tt.want[i].Text {
					t.Errorf("block[%d].Text = %q, want %q", i, block.Text, tt.want[i].Text)
				}
			}
		})
	}
}

func TestContentBlock_TextContent(t *testing.T) {
	stringContent := json.RawMessage(`"output text"`)
	arrayContent := json.RawMessage(`[{"type":"text","text":"line1"},{"type":"text","text":"line2"}]`)
	outputFallback := json.RawMessage(`"fallback"`)

	tests := []struct {
		name  string
		block ContentBlock
		want  string
	}{
		{
			name:  "string content",
			block: ContentBlock{Content: stringContent},
			want:  "output text",
		},
		{
			name:  "array content",
			block: ContentBlock{Content: arrayContent},
			want:  "line1line2",
		},
		{
			name:  "output fallback",
			block: ContentBlock{Output: outputFallback},
			want:  "fallback",
		},
		{
			name:  "empty",
			block: ContentBlock{},
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.block.TextContent()
			if got != tt.want {
				t.Errorf("TextContent() = %q, want %q", got, tt.want)
			}
		})
	}
}
