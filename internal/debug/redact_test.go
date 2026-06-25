package debug

import (
	"strings"
	"testing"
)

func TestRedactAPIKey(t *testing.T) {
	input := `{
		"api_key": "sk-proj-abc123xyz789",
		"model": "gpt-4"
	}`

	result := RedactSensitive([]byte(input))
	resultStr := string(result)

	if strings.Contains(resultStr, "sk-proj-abc123xyz789") {
		t.Error("expected API key to be redacted")
	}

	if !strings.Contains(resultStr, "[REDACTED]") {
		t.Error("expected [REDACTED] placeholder in output")
	}
}

func TestRedactAuthorizationHeader(t *testing.T) {
	input := `{
		"headers": {
			"Authorization": "Bearer sk-secret-token-12345"
		},
		"body": "test"
	}`

	result := RedactSensitive([]byte(input))
	resultStr := string(result)

	if strings.Contains(resultStr, "sk-secret-token-12345") {
		t.Error("expected authorization token to be redacted")
	}

	if strings.Contains(resultStr, "Bearer") {
		t.Error("expected Bearer prefix to be redacted")
	}
}

func TestRedactMultipleKeys(t *testing.T) {
	input := `{
		"api_key": "first-key-123",
		"api-key": "second-key-456",
		"authorization": "Bearer third-key-789",
		"model": "gpt-4"
	}`

	result := RedactSensitive([]byte(input))
	resultStr := string(result)

	if strings.Contains(resultStr, "first-key-123") {
		t.Error("expected first API key to be redacted")
	}
	if strings.Contains(resultStr, "second-key-456") {
		t.Error("expected second API key to be redacted")
	}
	if strings.Contains(resultStr, "third-key-789") {
		t.Error("expected third API key to be redacted")
	}
	if !strings.Contains(resultStr, "gpt-4") {
		t.Error("expected non-sensitive data to remain unchanged")
	}
}

func TestRedactPreservesJSONStructure(t *testing.T) {
	input := `{
		"api_key": "secret-key",
		"model": "gpt-4",
		"messages": [
			{"role": "user", "content": "hello"}
		]
	}`

	result := RedactSensitive([]byte(input))
	resultStr := string(result)

	// Verify JSON structure is preserved
	if !strings.Contains(resultStr, `"model"`) {
		t.Error("expected model field to be preserved")
	}
	if !strings.Contains(resultStr, `"messages"`) {
		t.Error("expected messages field to be preserved")
	}
	if !strings.Contains(resultStr, `"role"`) {
		t.Error("expected nested structure to be preserved")
	}
}

func TestRedactHandlesEmptyInput(t *testing.T) {
	result := RedactSensitive([]byte{})
	if len(result) != 0 {
		t.Errorf("expected empty result for empty input, got %q", string(result))
	}
}

func TestRedactHandlesInvalidJSON(t *testing.T) {
	invalidJSON := `{"api_key": "secret", "broken` // incomplete JSON

	// Should not panic and should return original or best-effort result
	result := RedactSensitive([]byte(invalidJSON))
	if len(result) == 0 {
		t.Error("expected non-empty result even for invalid JSON")
	}
}
