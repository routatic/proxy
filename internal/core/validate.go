package core

import "fmt"

// ValidateRequest checks a NormalizedRequest for structural validity.
func ValidateRequest(req *NormalizedRequest) error {
	if req.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(req.Messages) == 0 {
		return fmt.Errorf("messages is required")
	}

	// Validate message ordering: user, assistant, tool-result alternation.
	for i, msg := range req.Messages {
		switch msg.Role {
		case "user", "assistant", "system", "tool":
			// Valid roles
		default:
			return fmt.Errorf("messages[%d]: invalid role %q", i, msg.Role)
		}

		// Tool-result messages must have a ToolCallID.
		if msg.Role == "tool" && msg.ToolCallID == "" {
			return fmt.Errorf("messages[%d]: tool-result message missing tool_call_id", i)
		}

		// Assistant messages with tool calls must have non-empty tool calls.
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for j, tc := range msg.ToolCalls {
				if tc.ID == "" {
					return fmt.Errorf("messages[%d].tool_calls[%d]: missing id", i, j)
				}
				if tc.Name == "" {
					return fmt.Errorf("messages[%d].tool_calls[%d]: missing name", i, j)
				}
			}
		}
	}

	// Validate max_tokens bounds.
	if req.MaxTokens < 0 {
		return fmt.Errorf("max_tokens must be non-negative")
	}

	return nil
}
