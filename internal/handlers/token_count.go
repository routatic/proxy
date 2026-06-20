package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/routatic/proxy/internal/token"
	"github.com/routatic/proxy/pkg/types"
)

func tokenMessagesFromAnthropic(messages []types.Message) []token.MessageContent {
	tokenMessages := make([]token.MessageContent, 0, len(messages))
	for _, msg := range messages {
		blocks := msg.ContentBlocks()
		tokenMessages = append(tokenMessages, token.MessageContent{
			Role:        msg.Role,
			Content:     extractTokenTextFromBlocks(blocks),
			ExtraTokens: imageTokenEstimate(blocks),
		})
	}
	return tokenMessages
}

func systemAndToolsTokenText(system string, tools []types.Tool) (string, error) {
	toolsText, err := toolsTokenText(tools)
	if err != nil {
		return "", err
	}
	if system == "" {
		return toolsText, nil
	}
	if toolsText == "" {
		return system, nil
	}
	return system + "\n" + toolsText, nil
}

func toolsTokenText(tools []types.Tool) (string, error) {
	if len(tools) == 0 {
		return "", nil
	}

	data, err := json.Marshal(tools)
	if err != nil {
		return "", fmt.Errorf("failed to marshal tools: %w", err)
	}
	return string(data), nil
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

// imageTokenEstimate estimates extra tokens for image blocks.
// For base64 images, derives an estimate from encoded data length (file-size
// heuristic, not pixel-dimension based). For URL images where no data is
// available, returns a default estimate. This is used for routing decisions
// (scenario detection), not billing — Anthropic's actual token cost depends
// on image dimensions after resize.
func imageTokenEstimate(blocks []types.ContentBlock) int {
	total := 0
	for _, block := range blocks {
		if block.Type != "image" || block.Source == nil {
			continue
		}
		if len(block.Source.Data) > 0 {
			total += imageTokenEstimateFromBase64(len(block.Source.Data))
		} else {
			total += 1500
		}
	}
	return total
}

// imageTokenEstimateFromBase64 estimates token count from base64 image data length.
// Base64 encoding inflates size by ~4/3; raw bytes / 75 ≈ Anthropic image tokens.
func imageTokenEstimateFromBase64(base64Len int) int {
	if base64Len == 0 {
		return 1500
	}
	rawBytes := base64Len * 3 / 4
	tokens := rawBytes / 75
	if tokens < 300 {
		return 300
	}
	if tokens > 4000 {
		return 4000
	}
	return tokens
}
