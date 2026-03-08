package validation

import "testing"

func TestValidateUpgrade_AllowsAdditiveOptionalFieldWithoutVersionBump(t *testing.T) {
	previous := SchemaDefinition{
		ID:      "api.sessions.create.response.v1",
		Version: "v1",
		Owner:   "controlplane",
		Object: ObjectSchema{
			"session_id": {Type: FieldTypeString, Required: true},
		},
	}
	candidate := SchemaDefinition{
		ID:      previous.ID,
		Version: previous.Version,
		Owner:   previous.Owner,
		Object: ObjectSchema{
			"session_id":    {Type: FieldTypeString, Required: true},
			"request_trace": {Type: FieldTypeString, Required: false},
		},
	}

	if err := ValidateUpgrade(previous, candidate); err != nil {
		t.Fatalf("expected additive optional field to be compatible: %v", err)
	}
}

func TestValidateUpgrade_RejectsBreakingChangeWithoutVersionBump(t *testing.T) {
	previous := SchemaDefinition{
		ID:      "api.sessions.create.response.v1",
		Version: "v1",
		Owner:   "controlplane",
		Object: ObjectSchema{
			"session_id": {Type: FieldTypeString, Required: true},
		},
	}
	candidate := SchemaDefinition{
		ID:      previous.ID,
		Version: previous.Version,
		Owner:   previous.Owner,
		Object:  ObjectSchema{},
	}

	if err := ValidateUpgrade(previous, candidate); err == nil {
		t.Fatal("expected breaking change without version bump to fail")
	}
}

func TestValidateUpgrade_AllowsBreakingChangeWithVersionBump(t *testing.T) {
	previous := SchemaDefinition{
		ID:      "api.sessions.create.response.v1",
		Version: "v1",
		Owner:   "controlplane",
		Object: ObjectSchema{
			"session_id": {Type: FieldTypeString, Required: true},
		},
	}
	candidate := SchemaDefinition{
		ID:      previous.ID,
		Version: "v2",
		Owner:   previous.Owner,
		Object:  ObjectSchema{},
	}

	if err := ValidateUpgrade(previous, candidate); err != nil {
		t.Fatalf("expected version bump to allow breaking change: %v", err)
	}
}

func TestCompareSchemas_ReportsBreakingChanges(t *testing.T) {
	previous := SchemaDefinition{
		ID:      "api.sessions.create.response.v1",
		Version: "v1",
		Owner:   "controlplane",
		Object: ObjectSchema{
			"session_id": {Type: FieldTypeString, Required: true},
			"status":     {Type: FieldTypeString, Required: false},
		},
	}
	candidate := SchemaDefinition{
		ID:      previous.ID,
		Version: "v1",
		Owner:   previous.Owner,
		Object: ObjectSchema{
			"session_id": {Type: FieldTypeNumber, Required: true},
			"status":     {Type: FieldTypeString, Required: true},
			"trace":      {Type: FieldTypeString, Required: true},
		},
	}

	report := CompareSchemas(previous, candidate)
	if len(report.BreakingChanges) != 3 {
		t.Fatalf("expected 3 breaking changes, got %d", len(report.BreakingChanges))
	}
	if len(report.AdditiveChanges) != 0 {
		t.Fatalf("expected no additive changes, got %d", len(report.AdditiveChanges))
	}
}
