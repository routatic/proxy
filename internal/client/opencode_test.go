package client

import (
	"encoding/json"
	"testing"
)

func TestIsAnthropicModelOnlyRoutesNativeAnthropicModels(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		want    bool
	}{
		{
			name:    "minimax m2.5 uses anthropic endpoint",
			modelID: "minimax-m2.5",
			want:    true,
		},
		{
			name:    "minimax m2.7 uses anthropic endpoint",
			modelID: "minimax-m2.7",
			want:    true,
		},
		{
			name:    "qwen3.7-max uses anthropic endpoint",
			modelID: "qwen3.7-max",
			want:    true,
		},
		{
			name:    "qwen3.6-plus uses anthropic endpoint",
			modelID: "qwen3.6-plus",
			want:    true,
		},
		{
			name:    "qwen3.5-plus uses anthropic endpoint",
			modelID: "qwen3.5-plus",
			want:    true,
		},
		{
			name:    "deepseek pro uses openai endpoint",
			modelID: "deepseek-v4-pro",
			want:    false,
		},
		{
			name:    "deepseek flash uses openai endpoint",
			modelID: "deepseek-v4-flash",
			want:    false,
		},
		{
			name:    "glm-5.1 uses openai endpoint",
			modelID: "glm-5.1",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAnthropicModel(tt.modelID); got != tt.want {
				t.Fatalf("IsAnthropicModel(%q) = %v, want %v", tt.modelID, got, tt.want)
			}
		})
	}
}

func TestCleanAnthropicBody_DisabledThinking(t *testing.T) {
	input := json.RawMessage(`{"model":"qwen3.7-max","max_tokens":8192,"thinking":{"type":"disabled"},"messages":[{"role":"user","content":"hi"}]}`)
	cleaned := CleanAnthropicBody(input)

	var m map[string]interface{}
	if err := json.Unmarshal(cleaned, &m); err != nil {
		t.Fatalf("unmarshal cleaned body: %v", err)
	}
	if _, ok := m["thinking"]; ok {
		t.Fatal("thinking with type=disabled should have been removed")
	}
	if m["model"] != "qwen3.7-max" {
		t.Fatalf("model = %v, want qwen3.7-max", m["model"])
	}
}

func TestCleanAnthropicBody_AdaptiveThinking(t *testing.T) {
	input := json.RawMessage(`{"model":"qwen3.7-max","thinking":{"type":"adaptive"},"messages":[]}`)
	cleaned := CleanAnthropicBody(input)

	var m map[string]interface{}
	if err := json.Unmarshal(cleaned, &m); err != nil {
		t.Fatalf("unmarshal cleaned body: %v", err)
	}
	if _, ok := m["thinking"]; ok {
		t.Fatal("thinking with type=adaptive should have been removed")
	}
}

func TestCleanAnthropicBody_EnabledThinking(t *testing.T) {
	input := json.RawMessage(`{"model":"qwen3.7-max","thinking":{"type":"enabled","budget_tokens":1024},"messages":[]}`)
	cleaned := CleanAnthropicBody(input)

	var m map[string]interface{}
	if err := json.Unmarshal(cleaned, &m); err != nil {
		t.Fatalf("unmarshal cleaned body: %v", err)
	}
	thinking, ok := m["thinking"].(map[string]interface{})
	if !ok {
		t.Fatal("thinking with type=enabled should be preserved")
	}
	if thinking["type"] != "enabled" {
		t.Fatalf("thinking.type = %v, want enabled", thinking["type"])
	}
}

func TestCleanAnthropicBody_NoThinking(t *testing.T) {
	input := json.RawMessage(`{"model":"minimax-m2.5","messages":[]}`)
	cleaned := CleanAnthropicBody(input)

	var m map[string]interface{}
	if err := json.Unmarshal(cleaned, &m); err != nil {
		t.Fatalf("unmarshal cleaned body: %v", err)
	}
	if _, ok := m["thinking"]; ok {
		t.Fatal("no thinking field should remain absent")
	}
}

func TestCleanAnthropicBody_StripsCacheControl(t *testing.T) {
	input := json.RawMessage(`{"model":"qwen3.7-max","messages":[{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral"}}]}],"system":[{"type":"text","text":"sys","cache_control":{"type":"ephemeral"}}]}`)
	cleaned := CleanAnthropicBody(input)

	var m map[string]interface{}
	if err := json.Unmarshal(cleaned, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	msgs := m["messages"].([]interface{})
	msg0 := msgs[0].(map[string]interface{})
	blocks := msg0["content"].([]interface{})
	block0 := blocks[0].(map[string]interface{})
	if _, ok := block0["cache_control"]; ok {
		t.Fatal("cache_control should be stripped from message content blocks")
	}

	sys := m["system"].([]interface{})
	sys0 := sys[0].(map[string]interface{})
	if _, ok := sys0["cache_control"]; ok {
		t.Fatal("cache_control should be stripped from system blocks")
	}
}

func TestCleanAnthropicBody_StripsUnsupportedTopLevel(t *testing.T) {
	input := json.RawMessage(`{"model":"qwen3.7-max","messages":[],"context_management":{"foo":"bar"},"output_config":{"baz":"qux"},"metadata":{"k":"v"}}`)
	cleaned := CleanAnthropicBody(input)

	var m map[string]interface{}
	if err := json.Unmarshal(cleaned, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"context_management", "output_config", "metadata"} {
		if _, ok := m[key]; ok {
			t.Fatalf("%s should be stripped", key)
		}
	}
}

func TestCleanAnthropicBody_InvalidJSON(t *testing.T) {
	input := json.RawMessage(`not json`)
	cleaned := CleanAnthropicBody(input)
	if string(cleaned) != string(input) {
		t.Fatal("invalid JSON should be returned as-is")
	}
}
