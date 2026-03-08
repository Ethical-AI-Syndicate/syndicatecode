package api

import "testing"

func TestCheckCompatibilityAllowsAdditiveFieldChanges(t *testing.T) {
	t.Parallel()

	previous := SchemaDefinition{
		Name:    "session",
		Version: 1,
		Fields: []SchemaField{
			{Name: "session_id", Type: "string", Required: true},
			{Name: "repo_path", Type: "string", Required: true},
		},
	}
	next := SchemaDefinition{
		Name:    "session",
		Version: 1,
		Fields: []SchemaField{
			{Name: "session_id", Type: "string", Required: true},
			{Name: "repo_path", Type: "string", Required: true},
			{Name: "trust_tier", Type: "string", Required: false},
		},
	}

	report := CheckCompatibility(previous, next)
	if !report.Compatible {
		t.Fatalf("expected additive changes to be compatible, issues=%v", report.Issues)
	}
}

func TestCheckCompatibilityBlocksBreakingTypeChangeWithoutVersionBump(t *testing.T) {
	t.Parallel()

	previous := SchemaDefinition{
		Name:    "turn",
		Version: 2,
		Fields:  []SchemaField{{Name: "token_count", Type: "integer", Required: true}},
	}
	next := SchemaDefinition{
		Name:    "turn",
		Version: 2,
		Fields:  []SchemaField{{Name: "token_count", Type: "string", Required: true}},
	}

	report := CheckCompatibility(previous, next)
	if report.Compatible {
		t.Fatalf("expected type change without version bump to be incompatible")
	}
}

func TestCheckCompatibilityAllowsBreakingChangeWithVersionBump(t *testing.T) {
	t.Parallel()

	previous := SchemaDefinition{
		Name:    "turn",
		Version: 2,
		Fields:  []SchemaField{{Name: "token_count", Type: "integer", Required: true}},
	}
	next := SchemaDefinition{
		Name:    "turn",
		Version: 3,
		Fields:  []SchemaField{{Name: "token_count", Type: "string", Required: true}},
	}

	report := CheckCompatibility(previous, next)
	if !report.Compatible {
		t.Fatalf("expected breaking change with version bump to be allowed, issues=%v", report.Issues)
	}
}

func TestValidateMigrationPolicyRequiresPlanForBreakingChange(t *testing.T) {
	t.Parallel()

	previous := SchemaDefinition{
		Name:    "approval",
		Version: 1,
		Fields:  []SchemaField{{Name: "status", Type: "string", Required: true}},
	}
	next := SchemaDefinition{
		Name:    "approval",
		Version: 2,
		Fields:  []SchemaField{{Name: "status", Type: "integer", Required: true}},
	}

	err := ValidateMigrationPolicy(previous, next, MigrationPlan{})
	if err == nil {
		t.Fatalf("expected missing migration plan error")
	}
}

func TestValidateMigrationPolicyHonorsDeprecationWindowForNonBreakingChange(t *testing.T) {
	t.Parallel()

	previous := SchemaDefinition{
		Name:    "session",
		Version: 3,
		Fields:  []SchemaField{{Name: "mode", Type: "string", Required: true}},
	}
	next := SchemaDefinition{
		Name:    "session",
		Version: 3,
		Fields: []SchemaField{
			{Name: "mode", Type: "string", Required: true},
			{Name: "profile", Type: "string", Required: false},
		},
	}

	err := ValidateMigrationPolicy(previous, next, MigrationPlan{DeprecationWindowDays: 30})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
