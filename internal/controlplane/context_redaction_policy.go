package controlplane

import (
	ctxmgr "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/context"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/secrets"
)

type contextRedactionPolicy struct {
	executor *secrets.PolicyExecutor
}

func newContextRedactionPolicy(executor *secrets.PolicyExecutor) ctxmgr.RedactionPolicy {
	if executor == nil {
		executor = secrets.NewPolicyExecutor(nil)
	}
	return contextRedactionPolicy{executor: executor}
}

func (p contextRedactionPolicy) Apply(sourceRef, sourceType, content string, destination ctxmgr.RedactionDestination) ctxmgr.RedactionDecision {
	secretDestination := secrets.DestinationPersistence
	if destination == ctxmgr.DestinationModelProvider {
		secretDestination = secrets.DestinationModelProvider
	}

	decision := p.executor.Apply(sourceRef, sourceType, content, secretDestination)

	return ctxmgr.RedactionDecision{
		Content:             decision.Content,
		Action:              string(decision.Action),
		Denied:              decision.Denied,
		Reason:              decision.Reason,
		SensitivityClass:    string(decision.Classification.Class),
		ClassificationLevel: string(decision.Classification.Level),
	}
}
