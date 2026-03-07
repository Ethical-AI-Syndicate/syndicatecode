package policy

import (
	"testing"
)

func TestPolicyEngine_Evaluate(t *testing.T) {
	engine := NewEngine()

	engine.AddRule(Rule{
		Name:        "no_network",
		Description: "Block network access",
		Effect:      EffectDeny,
		Condition: func(ctx *EvaluationContext) bool {
			return ctx.ToolSideEffect == SideEffectNetwork
		},
	})

	ctx := &EvaluationContext{
		ToolName:       "http_get",
		ToolSideEffect: SideEffectNetwork,
	}

	result := engine.Evaluate(ctx)
	if result.Allowed {
		t.Error("expected network access to be denied")
	}
}

func TestPolicyEngine_AllowRead(t *testing.T) {
	engine := NewEngine()

	ctx := &EvaluationContext{
		ToolName:       "read_file",
		ToolSideEffect: SideEffectRead,
	}

	result := engine.Evaluate(ctx)
	if !result.Allowed {
		t.Error("expected read access to be allowed")
	}
}
