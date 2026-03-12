package trust_test

import (
	"testing"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/trust"
)

func TestDefaultPolicy_Tier0_RestrictsWrite_Bead_l3d_X_1(t *testing.T) {
	p := trust.DefaultPolicy()
	if p.RequiresApproval("tier0", tools.SideEffectWrite) != true {
		t.Error("tier0 write should require approval")
	}
}

func TestDefaultPolicy_PluginsAllowed_Bead_l3d_X_1(t *testing.T) {
	p := trust.DefaultPolicy()
	if p.PluginsAllowed("tier3") {
		t.Error("tier3 should not allow plugins")
	}
	if !p.PluginsAllowed("tier1") {
		t.Error("tier1 should allow plugins")
	}
}
