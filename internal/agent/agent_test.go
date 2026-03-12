package agent_test

import (
	"testing"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/agent"
)

func TestAgentConfig_Defaults_Bead_l3d_X_1(t *testing.T) {
	cfg := agent.DefaultConfig("tier1")
	if cfg.MaxDepth <= 0 {
		t.Errorf("MaxDepth must be positive, got %d", cfg.MaxDepth)
	}
	if cfg.MaxToolCalls <= 0 {
		t.Errorf("MaxToolCalls must be positive, got %d", cfg.MaxToolCalls)
	}
}

func TestAgentConfig_Tier0_IsMoreRestrictive_Bead_l3d_X_1(t *testing.T) {
	tier0 := agent.DefaultConfig("tier0")
	tier2 := agent.DefaultConfig("tier2")
	if tier0.MaxDepth >= tier2.MaxDepth {
		t.Errorf("tier0 MaxDepth %d should be less than tier2 %d", tier0.MaxDepth, tier2.MaxDepth)
	}
}
