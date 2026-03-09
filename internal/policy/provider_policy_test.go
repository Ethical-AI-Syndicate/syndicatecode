package policy

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadProviderPolicyRejectsInvalidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider-policy.json")
	invalid := `{
		"providers": [
			{
				"name": "",
				"trust_tiers": ["tier9"],
				"sensitivity": ["A"],
				"tasks": [],
				"retention_class": "",
				"fallback_eligible": true
			}
		]
	}`
	if err := os.WriteFile(path, []byte(invalid), 0o600); err != nil {
		t.Fatalf("failed to write invalid policy file: %v", err)
	}

	_, err := LoadProviderPolicy(path)
	if err == nil {
		t.Fatalf("expected invalid policy to fail")
	}
	if !strings.Contains(err.Error(), "providers[0].name") {
		t.Fatalf("expected actionable field path in error, got %v", err)
	}
}

func TestLoadProviderPolicyValidConfigLoadsDeterministically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider-policy.json")
	valid := `{
		"providers": [
			{
				"name": "local-llm",
				"trust_tiers": ["tier1", "tier2"],
				"sensitivity": ["B", "C"],
				"tasks": ["codegen", "analysis"],
				"retention_class": "ephemeral",
				"fallback_eligible": false
			}
		]
	}`
	if err := os.WriteFile(path, []byte(valid), 0o600); err != nil {
		t.Fatalf("failed to write valid policy file: %v", err)
	}

	first, err := LoadProviderPolicy(path)
	if err != nil {
		t.Fatalf("expected valid policy to load, got %v", err)
	}
	second, err := LoadProviderPolicy(path)
	if err != nil {
		t.Fatalf("expected valid policy to load second time, got %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected deterministic policy loading")
	}
}

func TestLoadProviderPolicyRejectsNonJSONPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider-policy.txt")
	valid := `{
		"providers": [
			{
				"name": "local-llm",
				"trust_tiers": ["tier1"],
				"sensitivity": ["B"],
				"tasks": ["analysis"],
				"retention_class": "ephemeral",
				"fallback_eligible": false
			}
		]
	}`
	if err := os.WriteFile(path, []byte(valid), 0o600); err != nil {
		t.Fatalf("failed to write valid policy file: %v", err)
	}

	_, err := LoadProviderPolicy(path)
	if err == nil {
		t.Fatalf("expected non-json policy path to fail")
	}
	if !strings.Contains(err.Error(), "must use .json") {
		t.Fatalf("expected extension validation error, got %v", err)
	}
}
