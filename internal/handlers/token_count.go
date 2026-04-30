package handlers

import (
	"encoding/json"
	"strings"

	"oc-go-cc/internal/token"
	"oc-go-cc/pkg/types"
)

func tokenMessagesFromAnthropic(messages []types.Message) []token.MessageContent {
	tokenMessages := make([]token.MessageContent, 0, len(messages))
	for _, msg := range messages {
		tokenMessages = append(tokenMessages, token.MessageContent{
			Role:    msg.Role,
			Content: extractTokenTextFromBlocks(msg.ContentBlocks()),
		})
	}
	return tokenMessages
}

func systemAndToolsTokenText(system string, tools []types.Tool) string {
	toolsText := toolsTokenText(tools)
	if system == "" {
		return toolsText
	}
	if toolsText == "" {
		return system
	}
	return system + "\n" + toolsText
}

func toolsTokenText(tools []types.Tool) string {
	if len(tools) == 0 {
		return ""
	}

	data, err := json.Marshal(tools)
	if err != nil {
		return ""
	}
	return string(data)
}

// extractTokenTextFromBlocks extracts all text-like content that contributes to
// context usage. This is intentionally broader than routing text extraction.
func extractTokenTextFromBlocks(blocks []types.ContentBlock) string {
	var content strings.Builder
	for _, block := range blocks {
		switch block.Type {
		case "text":
			content.WriteString(block.Text)
		case "tool_use":
			content.WriteString("[Tool Use: ")
			content.WriteString(block.Name)
			if len(block.Input) > 0 {
				content.WriteByte(' ')
				content.Write(block.Input)
			}
			content.WriteString("]")
		case "tool_result":
			content.WriteString(block.TextContent())
		case "thinking":
			content.WriteString(block.Thinking)
		case "image":
			content.WriteString("[Image]")
		}
	}
	return content.String()
}
