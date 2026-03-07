package policy

import (
	"errors"
)

type Effect string

const (
	EffectAllow Effect = "allow"
	EffectDeny  Effect = "deny"
)

type SideEffect string

const (
	SideEffectNone    SideEffect = "none"
	SideEffectRead    SideEffect = "read"
	SideEffectWrite   SideEffect = "write"
	SideEffectExecute SideEffect = "execute"
	SideEffectNetwork SideEffect = "network"
)

type Rule struct {
	Name        string
	Description string
	Effect      Effect
	Condition   func(*EvaluationContext) bool
}

type EvaluationContext struct {
	ToolName       string
	ToolSideEffect SideEffect
	Input          map[string]interface{}
	Session        string
	User           string
}

type EvaluationResult struct {
	Allowed  bool
	DeniedBy []string
	Reason   string
}

type Engine struct {
	rules []Rule
}

func NewEngine() *Engine {
	return &Engine{
		rules: make([]Rule, 0),
	}
}

func (e *Engine) AddRule(rule Rule) {
	e.rules = append(e.rules, rule)
}

func (e *Engine) Evaluate(ctx *EvaluationContext) *EvaluationResult {
	result := &EvaluationResult{
		Allowed: true,
	}

	for _, rule := range e.rules {
		if rule.Condition(ctx) {
			if rule.Effect == EffectDeny {
				result.Allowed = false
				result.DeniedBy = append(result.DeniedBy, rule.Name)
				result.Reason = rule.Description
				break
			}
		}
	}

	return result
}

var ErrPolicyDenied = errors.New("policy denied")
