package controlplane

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProviderPolicyRejectsInvalidConfig_Bead_l3d_5_1(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "provider-policy.json")

	if err := os.WriteFile(path, []byte(`{"providers":[{"name":"","trust_tiers":[],"sensitivity":[],"tasks":[],"retention_class":"","fallback_eligible":true}]}`), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := LoadProviderPolicy(path)
	if err == nil {
		t.Fatalf("expected validation error for invalid policy config")
	}
}

func TestLoadProviderPolicyValidConfigLoadsDeterministically_Bead_l3d_5_1(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "provider-policy.json")

	content := `{
		"providers": [
			{
				"name": "remote-primary",
				"trust_tiers": ["tier1", "tier0"],
				"sensitivity": ["D", "C"],
				"tasks": ["codegen", "analysis"],
				"retention_class": "standard",
				"fallback_eligible": false
			}
		]
	}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	first, err := LoadProviderPolicy(path)
	if err != nil {
		t.Fatalf("failed to load policy first pass: %v", err)
	}
	second, err := LoadProviderPolicy(path)
	if err != nil {
		t.Fatalf("failed to load policy second pass: %v", err)
	}

	if first.Providers[0].TrustTiers[0] != second.Providers[0].TrustTiers[0] {
		t.Fatalf("expected deterministic ordering across policy loads")
	}
}
