package sandbox

import (
	"testing"
)

func TestSelectRunnerForTrustTier_Bead_l3d_17_1(t *testing.T) {
	tests := []struct {
		name   string
		tier   string
		wantL2 bool
	}{
		{"tier1 returns L1", "tier1", false},
		{"tier2 returns L2", "tier2", true},
		{"tier3 returns L2", "tier3", true},
		{"unknown tier defaults to L1", "unknown", false},
		{"empty tier defaults to L1", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l1 := &L1Runner{}
			l2 := &L2Runner{}

			got := selectRunnerForTrustTier(tt.tier, l1, l2)

			if tt.wantL2 {
				if got != l2 {
					t.Errorf("selectRunnerForTrustTier(%q) = L1, want L2", tt.tier)
				}
			} else {
				if got != l1 {
					t.Errorf("selectRunnerForTrustTier(%q) = L2, want L1", tt.tier)
				}
			}
		})
	}
}
