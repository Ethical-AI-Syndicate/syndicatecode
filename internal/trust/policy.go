package trust

import "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"

type Policy interface {
	MaxLoopDepth(tier string) int
	MaxToolCalls(tier string) int
	AllowedSideEffects(tier string) []tools.SideEffect
	RequiresApproval(tier string, effect tools.SideEffect) bool
	PluginsAllowed(tier string) bool
}

type defaultPolicy struct{}

func DefaultPolicy() Policy { return defaultPolicy{} }

func (defaultPolicy) MaxLoopDepth(tier string) int {
	switch tier {
	case "tier0":
		return 1
	case "tier1":
		return 3
	case "tier2":
		return 5
	default:
		return 10
	}
}

func (defaultPolicy) MaxToolCalls(tier string) int {
	switch tier {
	case "tier0":
		return 3
	case "tier1":
		return 10
	case "tier2":
		return 20
	default:
		return 50
	}
}

func (defaultPolicy) AllowedSideEffects(tier string) []tools.SideEffect {
	switch tier {
	case "tier0":
		return []tools.SideEffect{tools.SideEffectNone, tools.SideEffectRead}
	default:
		return []tools.SideEffect{tools.SideEffectNone, tools.SideEffectRead, tools.SideEffectWrite, tools.SideEffectExecute, tools.SideEffectNetwork}
	}
}

func (defaultPolicy) RequiresApproval(tier string, effect tools.SideEffect) bool {
	if tier == "tier0" {
		return effect != tools.SideEffectNone && effect != tools.SideEffectRead
	}
	if tier == "tier2" || tier == "tier3" {
		return effect == tools.SideEffectWrite || effect == tools.SideEffectExecute || effect == tools.SideEffectNetwork
	}
	return effect == tools.SideEffectExecute || effect == tools.SideEffectNetwork
}

func (defaultPolicy) PluginsAllowed(tier string) bool {
	return tier != "tier3"
}
