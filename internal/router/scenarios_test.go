package router

import (
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
)

func TestHasComplexPattern_UserMessage(t *testing.T) {
	messages := []MessageContent{
		{Role: "user", Content: "Please refactor this code to use interfaces"},
	}
	if !hasComplexPattern(messages) {
		t.Error("Expected hasComplexPattern to detect 'refactor' in user message")
	}
}

func TestHasComplexPattern_SystemMessage(t *testing.T) {
	messages := []MessageContent{
		{Role: "system", Content: "Please architect the new service"},
	}
	if !hasComplexPattern(messages) {
		t.Error("Expected hasComplexPattern to detect 'architect' in system message")
	}
}

func TestHasComplexPattern_NoMatch(t *testing.T) {
	messages := []MessageContent{
		{Role: "user", Content: "Hello, how are you?"},
	}
	if hasComplexPattern(messages) {
		t.Error("Expected hasComplexPattern to not match simple greeting")
	}
}

func TestHasThinkingPattern_UserMessage(t *testing.T) {
	messages := []MessageContent{
		{Role: "user", Content: "Think through this problem step by step"},
	}
	if !hasThinkingPattern(messages) {
		t.Error("Expected hasThinkingPattern to detect 'think' and 'step by step' in user message")
	}
}

func TestHasThinkingPattern_SystemMessage(t *testing.T) {
	messages := []MessageContent{
		{Role: "system", Content: "You are a reasoning agent"},
	}
	if !hasThinkingPattern(messages) {
		t.Error("Expected hasThinkingPattern to detect 'reasoning' in system message")
	}
}

func TestHasThinkingPattern_AnthropicThinkingBlock(t *testing.T) {
	messages := []MessageContent{
		{Role: "user", Content: "Solve this problem antThinking(thinking block)"},
	}
	if !hasThinkingPattern(messages) {
		t.Error("Expected hasThinkingPattern to detect 'antThinking' block")
	}
}

// mockConfig returns a minimal config for testing
func mockConfig() *config.Config {
	return &config.Config{
		Models: map[string]config.ModelConfig{
			"long_context": {
				ContextThreshold: 60000,
			},
		},
	}
}

func TestDetectScenario_ComplexFromUser(t *testing.T) {
	messages := []MessageContent{
		{Role: "user", Content: "Architect a new microservice for user authentication"},
	}
	result := DetectScenario(messages, 100, mockConfig())
	if result.Scenario != ScenarioComplex {
		t.Errorf("Expected ScenarioComplex, got %s", result.Scenario)
	}
}

func TestDetectScenario_ThinkFromUser(t *testing.T) {
	messages := []MessageContent{
		{Role: "user", Content: "Analyze the tradeoffs of this design"},
	}
	result := DetectScenario(messages, 100, mockConfig())
	if result.Scenario != ScenarioThink {
		t.Errorf("Expected ScenarioThink, got %s", result.Scenario)
	}
}

func TestDetectScenario_DefaultFromSimpleUserMessage(t *testing.T) {
	messages := []MessageContent{
		{Role: "user", Content: "Hello, how are you?"},
	}
	result := DetectScenario(messages, 100, mockConfig())
	if result.Scenario != ScenarioDefault {
		t.Errorf("Expected ScenarioDefault, got %s", result.Scenario)
	}
}

func TestDetectScenario_LongContextTakesPriority(t *testing.T) {
	messages := []MessageContent{
		{Role: "user", Content: "Refactor this code"},
	}
	// Token count > 60000 should trigger long_context regardless of content
	result := DetectScenario(messages, 70000, mockConfig())
	if result.Scenario != ScenarioLongContext {
		t.Errorf("Expected ScenarioLongContext, got %s", result.Scenario)
	}
}

func TestDetectScenario_VisionSimpleRequest(t *testing.T) {
	messages := []MessageContent{
		{Role: "user", Content: "Describe this screen", HasImage: true},
	}
	result := DetectScenario(messages, 100, mockConfig())
	if result.Scenario != ScenarioVision {
		t.Errorf("Expected ScenarioVision, got %s", result.Scenario)
	}
}

func TestDetectScenario_VisionComplexRequest(t *testing.T) {
	messages := []MessageContent{
		{Role: "user", Content: "Analyze this screenshot and find the bug", HasImage: true},
	}
	result := DetectScenario(messages, 100, mockConfig())
	if result.Scenario != ScenarioVisionComplex {
		t.Errorf("Expected ScenarioVisionComplex, got %s", result.Scenario)
	}
}

func TestDetectScenario_VisionUsesLatestImageRequestComplexity(t *testing.T) {
	messages := []MessageContent{
		{Role: "user", Content: "Analyze this previous architecture"},
		{Role: "assistant", Content: "Done"},
		{Role: "user", Content: "Cosa vedi?", HasImage: true},
	}
	result := DetectScenario(messages, 100, mockConfig())
	if result.Scenario != ScenarioVision {
		t.Errorf("Expected ScenarioVision, got %s", result.Scenario)
	}
}

func TestDetectScenario_ReturnsToTextRoutingAfterImageTurn(t *testing.T) {
	messages := []MessageContent{
		{Role: "user", Content: "Analyze this screenshot and find the bug", HasImage: true},
		{Role: "assistant", Content: "Done"},
		{Role: "user", Content: "Refactor this code"},
	}
	result := DetectScenario(messages, 100, mockConfig())
	if result.Scenario != ScenarioComplex {
		t.Errorf("Expected ScenarioComplex, got %s", result.Scenario)
	}
}

func TestDetectScenario_ReturnsToTextRoutingWhenLatestTurnHasHistoricalImageOnly(t *testing.T) {
	messages := []MessageContent{
		{Role: "user", Content: "Cosa vedi?", HasImage: true, ImageHashes: []string{"img1"}},
		{Role: "assistant", Content: "Vedo una schermata."},
		{Role: "user", Content: "ci sei?", HasImage: true, ImageHashes: []string{"img1"}},
	}
	result := DetectScenario(messages, 100, mockConfig())
	if result.Scenario != ScenarioDefault {
		t.Errorf("Expected ScenarioDefault, got %s", result.Scenario)
	}
}

func TestDetectScenario_LatestTextVisualIntentWithoutNewImageStaysNonVision(t *testing.T) {
	// Regression test: previously the proxy routed to the vision scenario
	// whenever the latest user text contained a "visual intent" keyword
	// (image/screenshot/ui/layout/...) AND any historical message had an
	// image. That fired false positives on ordinary prose ("check the UI
	// layout", "fix the Docker image", "look at this screenshot of the
	// error") for long-running sessions that happened to have an image
	// in the conversation history, forcing every subsequent turn onto a
	// vision-capable model (and onto the long-context vision scenario
	// once tokens crossed the threshold). Vision routing should only
	// trigger when the LATEST user message actually contains a new image.
	//
	// The third user message below is identical to the previous
	// (intentionally-true) assertion except HasImage is now false. The
	// expected scenario is the text-or-complex default, not vision.
	messages := []MessageContent{
		{Role: "user", Content: "Cosa vedi?", HasImage: true, ImageHashes: []string{"img1"}},
		{Role: "assistant", Content: "Vedo una schermata."},
		{Role: "user", Content: "cosa vedi nello screenshot?", HasImage: false},
	}
	result := DetectScenario(messages, 100, mockConfig())
	if result.Scenario == ScenarioVision {
		t.Errorf("Expected non-vision scenario for text-only latest message, got %s", result.Scenario)
	}
}

func TestDetectScenario_DebugWithoutVisualIntentStaysTextDefault(t *testing.T) {
	// "debug" was removed from complexKeywords because tool_result content
	// constantly contains it, routing every turn to complex by accident.
	messages := []MessageContent{
		{Role: "user", Content: "Cosa vedi?", HasImage: true, ImageHashes: []string{"img1"}},
		{Role: "assistant", Content: "Vedo una schermata."},
		{Role: "user", Content: "debug questo codice", HasImage: false},
	}
	result := DetectScenario(messages, 100, mockConfig())
	if result.Scenario != ScenarioDefault {
		t.Errorf("Expected ScenarioDefault (debug is no longer a complex trigger), got %s", result.Scenario)
	}
}

func TestRouteForStreaming_ReturnsToFastAfterImageTurn(t *testing.T) {
	messages := []MessageContent{
		{Role: "user", Content: "Cosa vedi?", HasImage: true},
		{Role: "assistant", Content: "Done"},
		{Role: "user", Content: "Hello"},
	}
	result := RouteForStreaming(messages, 100, mockConfig())
	if result.Scenario != ScenarioFast {
		t.Errorf("Expected ScenarioFast, got %s", result.Scenario)
	}
}

func TestDetectScenario_VisionLongContextTakesPriorityOverVisionComplex(t *testing.T) {
	messages := []MessageContent{
		{Role: "user", Content: "Analyze this screenshot and refactor the code", HasImage: true},
	}
	result := DetectScenario(messages, 70000, mockConfig())
	if result.Scenario != ScenarioVisionLongContext {
		t.Errorf("Expected ScenarioVisionLongContext, got %s", result.Scenario)
	}
}

func TestRouteForStreaming_VisionComplexKeepsVisionComplexScenario(t *testing.T) {
	// "bug" was removed from complexKeywords; use a retained architectural
	// keyword ("refactor") to exercise the vision_complex path.
	messages := []MessageContent{
		{Role: "user", Content: "Refactor the code shown in this screenshot", HasImage: true},
	}
	result := RouteForStreaming(messages, 100, mockConfig())
	if result.Scenario != ScenarioVisionComplex {
		t.Errorf("Expected ScenarioVisionComplex, got %s", result.Scenario)
	}
}

func TestRouteForStreaming_RespectsConfiguredThreshold(t *testing.T) {
	messages := []MessageContent{
		{Role: "user", Content: "Hello"},
	}
	cfg := &config.Config{
		Models: map[string]config.ModelConfig{
			"long_context": {
				ModelID:          "deepseek-v4-flash",
				ContextThreshold: 256000,
			},
		},
	}

	// Below threshold should NOT trigger long_context
	result := RouteForStreaming(messages, 40955, cfg)
	if result.Scenario == ScenarioLongContext {
		t.Errorf("Expected NOT ScenarioLongContext for 40955 tokens with threshold 256000, got %s", result.Scenario)
	}

	// Above threshold should trigger long_context
	result = RouteForStreaming(messages, 300000, cfg)
	if result.Scenario != ScenarioLongContext {
		t.Errorf("Expected ScenarioLongContext for 300000 tokens with threshold 256000, got %s", result.Scenario)
	}
	// Scenario detection happens before a model is resolved, so the reason
	// explains the trigger and names no model. The resolved model is appended
	// by the router — see TestRoute_ReasonReportsConfiguredModel.
	if !strings.Contains(result.Reason, "256000") {
		t.Errorf("Expected reason to mention the configured threshold, got: %s", result.Reason)
	}
	if strings.Contains(result.Reason, "deepseek-v4-flash") {
		t.Errorf("Expected scenario reason not to name a model, got: %s", result.Reason)
	}
}

func TestRouteForStreaming_UsesDefaultThresholdWhenNotConfigured(t *testing.T) {
	messages := []MessageContent{
		{Role: "user", Content: "Hello"},
	}
	cfg := &config.Config{
		Models: map[string]config.ModelConfig{},
	}

	// Default threshold is 100000
	result := RouteForStreaming(messages, 90000, cfg)
	if result.Scenario == ScenarioLongContext {
		t.Errorf("Expected NOT ScenarioLongContext for 90000 tokens with default threshold, got %s", result.Scenario)
	}

	result = RouteForStreaming(messages, 110000, cfg)
	if result.Scenario != ScenarioLongContext {
		t.Errorf("Expected ScenarioLongContext for 110000 tokens with default threshold, got %s", result.Scenario)
	}
}

func TestRouteForStreaming_NilConfig(t *testing.T) {
	messages := []MessageContent{
		{Role: "user", Content: "Hello"},
	}

	// Default threshold is 100000; nil config should not panic
	result := RouteForStreaming(messages, 90000, nil)
	if result.Scenario == ScenarioLongContext {
		t.Errorf("Expected NOT ScenarioLongContext for 90000 tokens with nil config, got %s", result.Scenario)
	}

	result = RouteForStreaming(messages, 110000, nil)
	if result.Scenario != ScenarioLongContext {
		t.Errorf("Expected ScenarioLongContext for 110000 tokens with nil config, got %s", result.Scenario)
	}
	if !strings.Contains(result.Reason, "100000") {
		t.Errorf("Expected reason to mention the default threshold, got: %s", result.Reason)
	}
}

// TestDetectScenario_ReasonsNameNoModels guards against reintroducing
// hardcoded model names into scenario reasons, which is how they went stale
// before: the reason must describe the trigger only, since the model for a
// scenario comes from config and is appended by the router.
func TestDetectScenario_ReasonsNameNoModels(t *testing.T) {
	cases := []struct {
		name     string
		messages []MessageContent
		tokens   int
	}{
		{"complex", []MessageContent{{Role: "user", Content: "Architect a new service"}}, 100},
		{"think", []MessageContent{{Role: "user", Content: "Think about this"}}, 100},
		{"background", []MessageContent{{Role: "user", Content: "what is Go"}}, 100},
		{"default", []MessageContent{{Role: "user", Content: "Hello"}}, 100},
		{"long_context", []MessageContent{{Role: "user", Content: "Hello"}}, 70000},
	}

	// Substrings from model families this proxy routes to. None of them belong
	// in a scenario reason.
	banned := []string{"glm", "kimi", "qwen", "minimax", "mimo", "deepseek"}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for label, reason := range map[string]string{
				"DetectScenario":    DetectScenario(tc.messages, tc.tokens, mockConfig()).Reason,
				"RouteForStreaming": RouteForStreaming(tc.messages, tc.tokens, mockConfig()).Reason,
			} {
				if reason == "" {
					t.Fatalf("%s: expected a non-empty reason", label)
				}
				lower := strings.ToLower(reason)
				for _, model := range banned {
					if strings.Contains(lower, model) {
						t.Errorf("%s: reason names model %q (must describe the trigger only): %s", label, model, reason)
					}
				}
			}
		})
	}
}
