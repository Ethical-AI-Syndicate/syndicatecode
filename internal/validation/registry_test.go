package validation

import "testing"

func TestDefaultRegistry_ContainsCanonicalContracts(t *testing.T) {
	registry := DefaultRegistry()
	required := []string{
		ContractAPISessionCreateRequest,
		ContractAPISessionCreateResponse,
		ContractAPITurnCreateRequest,
		ContractAPITurnCreateResponse,
		ContractAPIToolExecuteRequest,
		ContractAPIToolExecuteResponse,
		ContractAuditEvent,
		ContractContextFragment,
		ContractApprovalRecord,
		ContractPatchProposal,
		ContractModelRoutingDecision,
	}

	for _, id := range required {
		schema, ok := registry.Get(id)
		if !ok {
			t.Fatalf("expected contract schema %s to be registered", id)
		}
		if schema.Version == "" {
			t.Fatalf("expected version for schema %s", id)
		}
		if schema.Owner == "" {
			t.Fatalf("expected owner for schema %s", id)
		}
		if len(schema.Object) == 0 {
			t.Fatalf("expected object schema fields for contract %s", id)
		}
	}
}

func TestRegistry_RegisterRejectsDuplicateSchemaID(t *testing.T) {
	registry := NewRegistry()
	definition := SchemaDefinition{
		ID:      "api.test.request.v1",
		Version: "v1",
		Owner:   "controlplane",
		Object: ObjectSchema{
			"field": {Type: FieldTypeString, Required: true},
		},
	}

	if err := registry.Register(definition); err != nil {
		t.Fatalf("failed to register schema: %v", err)
	}
	if err := registry.Register(definition); err == nil {
		t.Fatal("expected duplicate schema registration to fail")
	}
}
