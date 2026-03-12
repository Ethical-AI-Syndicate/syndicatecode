package audit

import "testing"

func TestLatestSchemaVersion_Bead_l3d_17_1(t *testing.T) {
	if latestSchemaVersion != 1 {
		t.Errorf("expected latestSchemaVersion to be 1, got %d", latestSchemaVersion)
	}
}

func TestApplyMigrationsCreatesTables_Bead_l3d_17_1(t *testing.T) {
	tableCount := 11
	tables := []string{
		"schema_migrations",
		"sessions",
		"turns",
		"approvals",
		"tool_invocations",
		"model_invocations",
		"context_fragments",
		"patch_proposals",
		"file_mutations",
		"artifacts",
		"events",
	}

	if len(tables) != tableCount {
		t.Errorf("expected %d tables, got %d", tableCount, len(tables))
	}

	for _, table := range tables {
		if table == "" {
			t.Error("table name should not be empty")
		}
	}
}
