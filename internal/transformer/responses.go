package transformer

import (
	"encoding/json"
	"fmt"
	"strings"

	"oc-go-cc/internal/config"
	"oc-go-cc/pkg/types"
)

// TransformToResponses converts an Anthropic MessageRequest to OpenAI ResponsesRequest.
func (t *RequestTransformer) TransformToResponses(
	anthropicReq *types.MessageRequest,
	model config.ModelConfig,
) (*types.ResponsesRequest, error) {
	var input []types.ResponsesInput

	// Add system message if present
	systemText := anthropicReq.SystemText()
	if systemText != "" {
		content, _ := json.Marshal(systemText)
		input = append(input, types.ResponsesInput{
			Role:    "developer",
			Content: content,
		})
	}

	// Transform messages
	for _, msg := range anthropicReq.Messages {
		blocks := msg.ContentBlocks()
		var textParts []string

		for _, block := range blocks {
			switch block.Type {
			case "text":
				textParts = append(textParts, block.Text)
			case "image":
				textParts = append(textParts, "[Image]")
			case "tool_use":
				textParts = append(textParts, fmt.Sprintf("[Tool: %s(%s)]", block.Name, string(block.Input)))
			case "tool_result":
				// For Responses API, tool results are separate items
				toolContent := block.TextContent()
				content, _ := json.Marshal(toolContent)
				input = append(input, types.ResponsesInput{
					Role:    "tool",
					Content: content,
				})
			}
		}

		if len(textParts) > 0 {
			var sb strings.Builder
			for _, p := range textParts {
				sb.WriteString(p)
			}
			text := sb.String()
			content, _ := json.Marshal(text)
			input = append(input, types.ResponsesInput{
				Role:    msg.Role,
				Content: content,
			})
		}
	}

	req := &types.ResponsesRequest{
		Model:  model.ModelID,
		Input:  input,
		Stream: anthropicReq.Stream != nil && *anthropicReq.Stream,
	}

	// Transform tools if present
	if len(anthropicReq.Tools) > 0 {
		req.Tools = t.transformToolsForResponses(anthropicReq.Tools)
	}

	// Add reasoning if model supports it
	if model.ReasoningEffort != "" {
		req.Reasoning = &types.ResponsesReasoning{
			Effort: model.ReasoningEffort,
		}
	}

	return req, nil
}

// transformToolsForResponses converts Anthropic tools to Responses tool format.
func (t *RequestTransformer) transformToolsForResponses(tools []types.Tool) []types.ResponsesTool {
	var result []types.ResponsesTool

	for _, tool := range tools {
		schema := tool.InputSchema
		if len(schema) == 0 {
			schema = []byte(`{"type":"object","properties":{}}`)
		}

		result = append(result, types.ResponsesTool{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  json.RawMessage(schema),
		})
	}

	return result
}
