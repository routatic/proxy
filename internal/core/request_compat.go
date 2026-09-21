package core

import (
	"fmt"

	"github.com/routatic/proxy/internal/config"
)

// ValidateRequestCompatibility applies provider capability rules to ordered
// normalized blocks. Provider implementations call this after resolving their
// wire format and model capabilities.
func ValidateRequestCompatibility(req *NormalizedRequest, model config.ModelConfig, caps ProviderCapabilities, wire WireFormat) error {
	if req == nil {
		return &CompatibilityError{Provider: model.Provider, ModelID: model.ModelID, Reason: "request is nil"}
	}
	for _, block := range req.SystemBlocks {
		if err := validateBlock(block, model, caps, wire); err != nil {
			return err
		}
	}
	for _, msg := range req.Messages {
		for _, block := range msg.Blocks {
			if err := validateBlock(block, model, caps, wire); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateBlock(block NormalizedContentBlock, model config.ModelConfig, caps ProviderCapabilities, wire WireFormat) error {
	switch block.Type {
	case "", "text", "tool_use", "tool_result", "thinking", "image":
	default:
		if wire != WireFormatAnthropic {
			return &CompatibilityError{
				Provider: model.Provider,
				ModelID:  model.ModelID,
				Reason:   fmt.Sprintf("content block type %q is only supported by the Anthropic wire format", block.Type),
			}
		}
	}
	switch block.Type {
	case "tool_use", "tool_result":
		if !caps.SupportsTools {
			return &CompatibilityError{Provider: model.Provider, ModelID: model.ModelID, Reason: "tools are not supported"}
		}
	case "thinking":
		if !caps.SupportsThinking {
			return &CompatibilityError{Provider: model.Provider, ModelID: model.ModelID, Reason: "thinking blocks are not supported"}
		}
	case "image":
		if !caps.SupportsImageInput {
			return &CompatibilityError{Provider: model.Provider, ModelID: model.ModelID, Reason: "image input is not supported"}
		}
	}
	return nil
}
