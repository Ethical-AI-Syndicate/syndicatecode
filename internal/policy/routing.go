package policy

import (
	"fmt"
	"slices"
	"sort"
)

type ProviderRoute struct {
	Name               string
	TrustTiers         []string
	SensitivityClasses []string
	Tasks              []string
	Capabilities       []string
	Local              bool
	EstimatedLatencyMS int
	EstimatedCostUSD   float64
}

type RouteRequest struct {
	TrustTier            string
	SensitivityClass     string
	Task                 string
	RequiredCapabilities []string
	PreferLocal          bool
}

type RouteDecision struct {
	ProviderName string
	Ranked       []string
}

type RouteEngine struct {
	routes []ProviderRoute
}

func NewRouteEngine(routes []ProviderRoute) *RouteEngine {
	copyRoutes := append([]ProviderRoute(nil), routes...)
	return &RouteEngine{routes: copyRoutes}
}

func (e *RouteEngine) Select(req RouteRequest) (RouteDecision, error) {
	eligible := make([]ProviderRoute, 0, len(e.routes))
	for _, route := range e.routes {
		if !slices.Contains(route.TrustTiers, req.TrustTier) {
			continue
		}
		if !slices.Contains(route.SensitivityClasses, req.SensitivityClass) {
			continue
		}
		if !slices.Contains(route.Tasks, req.Task) {
			continue
		}
		if !containsAllCapabilities(route.Capabilities, req.RequiredCapabilities) {
			continue
		}
		eligible = append(eligible, route)
	}

	if len(eligible) == 0 {
		return RouteDecision{}, fmt.Errorf("no eligible routes for trust tier %s and task %s", req.TrustTier, req.Task)
	}

	sort.SliceStable(eligible, func(i, j int) bool {
		a := eligible[i]
		b := eligible[j]

		if req.PreferLocal && a.Local != b.Local {
			return a.Local
		}
		if a.EstimatedLatencyMS != b.EstimatedLatencyMS {
			return a.EstimatedLatencyMS < b.EstimatedLatencyMS
		}
		if a.EstimatedCostUSD != b.EstimatedCostUSD {
			return a.EstimatedCostUSD < b.EstimatedCostUSD
		}
		return a.Name < b.Name
	})

	ranked := make([]string, 0, len(eligible))
	for _, route := range eligible {
		ranked = append(ranked, route.Name)
	}

	return RouteDecision{
		ProviderName: eligible[0].Name,
		Ranked:       ranked,
	}, nil
}

func containsAllCapabilities(have []string, need []string) bool {
	for _, capability := range need {
		if !slices.Contains(have, capability) {
			return false
		}
	}

	return true
}
