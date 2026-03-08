package controlplane

import (
	ctxmgr "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/context"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/secrets"
)

func contextRedactorForDestination(destination secrets.Destination) ctxmgr.RedactorFunc {
	policy := secrets.NewPolicyExecutor(nil)

	return func(sourceRef, sourceType, content string) ctxmgr.RedactionDecision {
		decision := policy.Apply(sourceRef, sourceType, content, destination)
		return ctxmgr.RedactionDecision{
			Content:             decision.Content,
			Action:              string(decision.Action),
			Denied:              decision.Denied,
			Reason:              decision.Reason,
			Sensitivity:         string(decision.Classification.Class),
			ClassificationLevel: string(decision.Classification.Level),
		}
	}
}
