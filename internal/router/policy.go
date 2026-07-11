package router

import (
	"fmt"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
)

// EvaluationContext carries all information needed to evaluate routing policies.
// It bundles the normalized request, token count, the pool of models to choose
// from, and any prior routing decisions that may influence the outcome (e.g.,
// for debugging or dry-run evaluation).
type EvaluationContext struct {
	Request         *core.NormalizedRequest
	TokenCount      int
	AvailableModels []config.ModelConfig
	History         []RouteDecision
}

// RouteDecision records a routing decision for observability and dry-run
// inspection. Each policy that is evaluated produces one RouteDecision so
// callers can audit which policy won, which model was selected, and why.
type RouteDecision struct {
	PolicyName string
	ModelID    string
	Provider   string
	Reason     string
	Weight     int
}

// Policy evaluates a routing strategy and selects a model chain.
type Policy interface {
	// Name returns the policy identifier.
	Name() string

	// Evaluate examines the context and returns the model chain to try, the
	// decision explanation, or an error if no model matches this policy.
	Evaluate(ctx *EvaluationContext) ([]config.ModelConfig, RouteDecision, error)
}

// PolicyEngine composes multiple policies with ordered evaluation. Policies
// are evaluated in registration order; the first policy that returns a
// non-empty chain wins.
type PolicyEngine struct {
	policies []Policy
}

// NewPolicyEngine creates a policy engine with the default set of policies:
//  1. ModelOverridePolicy — check model_overrides config entries
//  2. RespectRequestModelPolicy — check respect_requested_model config
//  3. ScenarioPolicy — scenario-based routing (existing DetectScenario logic)
func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{}
}

// AddPolicy appends a policy to the evaluation chain.
func (eng *PolicyEngine) AddPolicy(p Policy) {
	eng.policies = append(eng.policies, p)
}

// Evaluate runs each policy in order and returns the first successful result.
func (eng *PolicyEngine) Evaluate(ctx *EvaluationContext) ([]config.ModelConfig, RouteDecision, error) {
	for _, p := range eng.policies {
		chain, decision, err := p.Evaluate(ctx)
		if err != nil {
			continue
		}
		if len(chain) > 0 {
			return chain, decision, nil
		}
	}
	return nil, RouteDecision{}, fmt.Errorf("no policy could route the request")
}

// EvaluateDryRun returns all policy decisions without executing. Useful for
// debugging and the dry-run endpoint.
func (eng *PolicyEngine) EvaluateDryRun(ctx *EvaluationContext) []RouteDecision {
	var decisions []RouteDecision
	for _, p := range eng.policies {
		_, decision, err := p.Evaluate(ctx)
		if err != nil {
			decisions = append(decisions, RouteDecision{
				PolicyName: p.Name(),
				Reason:     err.Error(),
			})
			continue
		}
		decisions = append(decisions, decision)
	}
	return decisions
}

// ── ModelOverridePolicy ───────────────────────────────────────────────

// ModelOverridePolicy checks whether the requested model has an entry in
// model_overrides. If so, it uses that override as the primary and appends
// the default fallback chain.
type ModelOverridePolicy struct {
	router *ModelRouter
}

// NewModelOverridePolicy creates a model override policy.
func NewModelOverridePolicy(router *ModelRouter) *ModelOverridePolicy {
	return &ModelOverridePolicy{router: router}
}

// Name returns the policy identifier.
func (p *ModelOverridePolicy) Name() string { return "model_override" }

// Evaluate checks model_overrides for the requested model.
func (p *ModelOverridePolicy) Evaluate(ctx *EvaluationContext) ([]config.ModelConfig, RouteDecision, error) {
	requestedModel := ctx.Request.Model
	if requestedModel == "" {
		return nil, RouteDecision{}, fmt.Errorf("no model in request")
	}

	result, ok := p.router.RouteWithOverride(requestedModel)
	if !ok {
		return nil, RouteDecision{}, fmt.Errorf("no override for %q", requestedModel)
	}

	return result.GetModelChain(), RouteDecision{
		PolicyName: "model_override",
		ModelID:    result.Primary.ModelID,
		Provider:   result.Primary.Provider,
		Reason:     fmt.Sprintf("matched model_override for %q", requestedModel),
	}, nil
}

// ── ScenarioPolicy ────────────────────────────────────────────────────

// ScenarioPolicy runs scenario-based routing using the existing DetectScenario
// logic. It handles both streaming and non-streaming paths.
type ScenarioPolicy struct {
	router *ModelRouter
}

// NewScenarioPolicy creates a scenario policy.
func NewScenarioPolicy(router *ModelRouter) *ScenarioPolicy {
	return &ScenarioPolicy{router: router}
}

// Name returns the policy identifier.
func (p *ScenarioPolicy) Name() string { return "scenario" }

// Evaluate runs scenario detection and returns the model chain.
func (p *ScenarioPolicy) Evaluate(ctx *EvaluationContext) ([]config.ModelConfig, RouteDecision, error) {
	// Build router messages from the normalized request.
	var messages []MessageContent
	systemText := ctx.Request.SystemPrompt
	if systemText != "" {
		messages = append(messages, MessageContent{Role: "system", Content: systemText})
	}
	for _, msg := range ctx.Request.Messages {
		messages = append(messages, MessageContent{Role: msg.Role, Content: msg.Content})
	}

	isStreaming := ctx.Request.Stream
	var result RouteResult
	var err error

	if isStreaming && !p.router.IsStreamingScenarioRoutingEnabled() {
		result, err = p.router.RouteForStreaming(messages, ctx.TokenCount, "")
	} else {
		result, err = p.router.Route(messages, ctx.TokenCount, "")
	}

	if err != nil {
		return nil, RouteDecision{}, fmt.Errorf("scenario routing failed: %w", err)
	}

	return result.GetModelChain(), RouteDecision{
		PolicyName: "scenario",
		ModelID:    result.Primary.ModelID,
		Provider:   result.Primary.Provider,
		Reason:     fmt.Sprintf("scenario=%s: %s", result.Scenario, result.Scenario),
	}, nil
}
