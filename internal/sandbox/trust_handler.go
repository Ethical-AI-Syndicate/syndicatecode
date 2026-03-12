package sandbox

import (
	"context"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/trust"
)

type trustTierRunner interface {
	Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error)
}

func selectRunnerForTrustTier(tier string, l1 *L1Runner, l2 *L2Runner) trustTierRunner {
	p := trust.DefaultPolicy()
	allowed := p.AllowedSideEffects(tier)
	for _, effect := range allowed {
		if effect == "execute" && (tier == "tier2" || tier == "tier3") {
			return l2
		}
	}
	return l1
}
