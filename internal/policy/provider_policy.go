package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
)

var allowedTrustTiers = []string{"tier0", "tier1", "tier2", "tier3"}
var allowedSensitivity = []string{"A", "B", "C", "D"}

type ProviderPolicy struct {
	Providers []ProviderRule `json:"providers"`
}

type ProviderRule struct {
	Name             string   `json:"name"`
	TrustTiers       []string `json:"trust_tiers"`
	Sensitivity      []string `json:"sensitivity"`
	Tasks            []string `json:"tasks"`
	RetentionClass   string   `json:"retention_class"`
	FallbackEligible bool     `json:"fallback_eligible"`
}

func DefaultProviderPolicy() ProviderPolicy {
	return ProviderPolicy{
		Providers: []ProviderRule{
			{
				Name:             "local-default",
				TrustTiers:       []string{"tier0", "tier1", "tier2", "tier3"},
				Sensitivity:      []string{"A", "B", "C", "D"},
				Tasks:            []string{"analysis", "codegen"},
				RetentionClass:   "ephemeral",
				FallbackEligible: false,
			},
		},
	}
}

func LoadProviderPolicy(path string) (ProviderPolicy, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return ProviderPolicy{}, fmt.Errorf("failed to read provider policy file %s: %w", path, err)
	}

	var cfg ProviderPolicy
	if err := json.Unmarshal(content, &cfg); err != nil {
		return ProviderPolicy{}, fmt.Errorf("failed to decode provider policy file %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return ProviderPolicy{}, err
	}

	return cfg.normalized(), nil
}

func (p ProviderPolicy) Validate() error {
	if len(p.Providers) == 0 {
		return fmt.Errorf("providers must contain at least one provider rule")
	}

	seen := make(map[string]struct{}, len(p.Providers))
	for idx, provider := range p.Providers {
		prefix := fmt.Sprintf("providers[%d]", idx)

		if provider.Name == "" {
			return fmt.Errorf("%s.name is required", prefix)
		}
		if _, ok := seen[provider.Name]; ok {
			return fmt.Errorf("%s.name must be unique: %s", prefix, provider.Name)
		}
		seen[provider.Name] = struct{}{}

		if len(provider.TrustTiers) == 0 {
			return fmt.Errorf("%s.trust_tiers must contain at least one tier", prefix)
		}
		for _, tier := range provider.TrustTiers {
			if !slices.Contains(allowedTrustTiers, tier) {
				return fmt.Errorf("%s.trust_tiers contains unsupported tier: %s", prefix, tier)
			}
		}

		if len(provider.Sensitivity) == 0 {
			return fmt.Errorf("%s.sensitivity must contain at least one class", prefix)
		}
		for _, class := range provider.Sensitivity {
			if !slices.Contains(allowedSensitivity, class) {
				return fmt.Errorf("%s.sensitivity contains unsupported class: %s", prefix, class)
			}
		}

		if len(provider.Tasks) == 0 {
			return fmt.Errorf("%s.tasks must contain at least one task", prefix)
		}

		if provider.RetentionClass == "" {
			return fmt.Errorf("%s.retention_class is required", prefix)
		}
	}

	return nil
}

func (p ProviderPolicy) normalized() ProviderPolicy {
	normalized := ProviderPolicy{Providers: append([]ProviderRule(nil), p.Providers...)}
	sort.Slice(normalized.Providers, func(i, j int) bool {
		return normalized.Providers[i].Name < normalized.Providers[j].Name
	})

	for i := range normalized.Providers {
		sort.Strings(normalized.Providers[i].TrustTiers)
		sort.Strings(normalized.Providers[i].Sensitivity)
		sort.Strings(normalized.Providers[i].Tasks)
	}

	return normalized
}
