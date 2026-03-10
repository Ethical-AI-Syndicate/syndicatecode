package policy

import "testing"

func TestRouteEngineSelectPrefersLocalWhenEligible_Bead_l3d_5_2(t *testing.T) {
	engine := NewRouteEngine([]ProviderRoute{
		{
			Name:               "remote-fast",
			TrustTiers:         []string{"tier1", "tier2"},
			SensitivityClasses: []string{"B", "C"},
			Tasks:              []string{"analysis"},
			Capabilities:       []string{"tool_use"},
			Local:              false,
			EstimatedLatencyMS: 120,
			EstimatedCostUSD:   0.03,
		},
		{
			Name:               "local-safe",
			TrustTiers:         []string{"tier1", "tier2"},
			SensitivityClasses: []string{"B", "C"},
			Tasks:              []string{"analysis"},
			Capabilities:       []string{"tool_use"},
			Local:              true,
			EstimatedLatencyMS: 180,
			EstimatedCostUSD:   0.00,
		},
	})

	decision, err := engine.Select(RouteRequest{
		TrustTier:            "tier2",
		SensitivityClass:     "B",
		Task:                 "analysis",
		RequiredCapabilities: []string{"tool_use"},
		PreferLocal:          true,
	})
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if decision.ProviderName != "local-safe" {
		t.Fatalf("expected local-safe, got %s", decision.ProviderName)
	}
}

func TestRouteEngineSelectFiltersIneligibleProviders_Bead_l3d_5_2(t *testing.T) {
	engine := NewRouteEngine([]ProviderRoute{
		{
			Name:               "missing-capability",
			TrustTiers:         []string{"tier1", "tier2"},
			SensitivityClasses: []string{"B", "C"},
			Tasks:              []string{"analysis"},
			Capabilities:       []string{"chat"},
			Local:              true,
			EstimatedLatencyMS: 70,
			EstimatedCostUSD:   0.01,
		},
		{
			Name:               "eligible",
			TrustTiers:         []string{"tier2"},
			SensitivityClasses: []string{"B"},
			Tasks:              []string{"analysis"},
			Capabilities:       []string{"tool_use", "json_mode"},
			Local:              false,
			EstimatedLatencyMS: 90,
			EstimatedCostUSD:   0.02,
		},
	})

	decision, err := engine.Select(RouteRequest{
		TrustTier:            "tier2",
		SensitivityClass:     "B",
		Task:                 "analysis",
		RequiredCapabilities: []string{"tool_use"},
	})
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if decision.ProviderName != "eligible" {
		t.Fatalf("expected eligible, got %s", decision.ProviderName)
	}
}

func TestRouteEngineSelectIsDeterministicOnTie_Bead_l3d_5_2(t *testing.T) {
	engine := NewRouteEngine([]ProviderRoute{
		{
			Name:               "provider-b",
			TrustTiers:         []string{"tier1"},
			SensitivityClasses: []string{"C"},
			Tasks:              []string{"codegen"},
			Capabilities:       []string{"tool_use"},
			Local:              false,
			EstimatedLatencyMS: 80,
			EstimatedCostUSD:   0.01,
		},
		{
			Name:               "provider-a",
			TrustTiers:         []string{"tier1"},
			SensitivityClasses: []string{"C"},
			Tasks:              []string{"codegen"},
			Capabilities:       []string{"tool_use"},
			Local:              false,
			EstimatedLatencyMS: 80,
			EstimatedCostUSD:   0.01,
		},
	})

	decision, err := engine.Select(RouteRequest{
		TrustTier:            "tier1",
		SensitivityClass:     "C",
		Task:                 "codegen",
		RequiredCapabilities: []string{"tool_use"},
	})
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if decision.ProviderName != "provider-a" {
		t.Fatalf("expected deterministic tie-break to provider-a, got %s", decision.ProviderName)
	}
}
