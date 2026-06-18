package transformer

import (
	"encoding/json"
	"fmt"
	"strings"

	"oc-go-cc/internal/config"
	"oc-go-cc/pkg/types"
)

// TransformToGemini converts an Anthropic MessageRequest to GeminiRequest.
func (t *RequestTransformer) TransformToGemini(
	anthropicReq *types.MessageRequest,
	model config.ModelConfig,
) (*types.GeminiRequest, error) {
	var contents []types.GeminiContent

	// Add system instruction via generation config (Gemini handles system separately)
	// For now, prepend system as a user message if present
	systemText := anthropicReq.SystemText()
	if systemText != "" {
		contents = append(contents, types.GeminiContent{
			Role: "user",
			Parts: []types.GeminiPart{
				{Text: "[System Instruction] " + systemText},
			},
		})
		contents = append(contents, types.GeminiContent{
			Role: "model",
			Parts: []types.GeminiPart{
				{Text: "Understood. I will follow these instructions."},
			},
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
				toolContent := block.TextContent()
				contents = append(contents, types.GeminiContent{
					Role: "user",
					Parts: []types.GeminiPart{
						{Text: fmt.Sprintf("[Tool Result for %s] %s", block.GetToolID(), toolContent)},
					},
				})
			}
		}

		if len(textParts) > 0 {
			var sb strings.Builder
			for _, p := range textParts {
				sb.WriteString(p)
			}
			text := sb.String()
			role := "user"
			if msg.Role == "assistant" {
				role = "model"
			}
			contents = append(contents, types.GeminiContent{
				Role: role,
				Parts: []types.GeminiPart{
					{Text: text},
				},
			})
		}
	}

	req := &types.GeminiRequest{
		Contents: contents,
	}

	// Set generation config
	genConfig := &types.GeminiGenerationConfig{}
	if anthropicReq.MaxTokens > 0 {
		genConfig.MaxOutputTokens = anthropicReq.MaxTokens
	}
	if model.Temperature > 0 {
		genConfig.Temperature = model.Temperature
	} else if anthropicReq.Temperature != nil {
		genConfig.Temperature = *anthropicReq.Temperature
	}
	if genConfig.MaxOutputTokens > 0 || genConfig.Temperature > 0 {
		req.GenerationConfig = genConfig
	}

	// Transform tools if present
	if len(anthropicReq.Tools) > 0 {
		req.Tools = t.transformToolsForGemini(anthropicReq.Tools)
	}

	return req, nil
}

// transformToolsForGemini converts Anthropic tools to Gemini tool format.
func (t *RequestTransformer) transformToolsForGemini(tools []types.Tool) []types.GeminiTool {
	var decls []types.GeminiFunctionDeclaration

	for _, tool := range tools {
		schema := tool.InputSchema
		if len(schema) == 0 {
			schema = []byte(`{"type":"object","properties":{}}`)
		}

		decls = append(decls, types.GeminiFunctionDeclaration{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  json.RawMessage(schema),
		})
	}

	return []types.GeminiTool{
		{FunctionDeclarations: decls},
	}
}
