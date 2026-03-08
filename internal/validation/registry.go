package validation

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

type FieldType string

const (
	FieldTypeString  FieldType = "string"
	FieldTypeBoolean FieldType = "boolean"
	FieldTypeNumber  FieldType = "number"
	FieldTypeObject  FieldType = "object"
	FieldTypeArray   FieldType = "array"
)

type FieldSpec struct {
	Type     FieldType
	Required bool
}

type ObjectSchema map[string]FieldSpec

type SchemaDefinition struct {
	ID      string
	Version string
	Owner   string
	Object  ObjectSchema
}

type Registry struct {
	mu          sync.RWMutex
	definitions map[string]SchemaDefinition
}

var ErrSchemaAlreadyRegistered = errors.New("schema already registered")

const (
	ContractAPISessionCreateRequest  = "api.sessions.create.request.v1"
	ContractAPISessionCreateResponse = "api.sessions.create.response.v1"
	ContractAPITurnCreateRequest     = "api.turns.create.request.v1"
	ContractAPITurnCreateResponse    = "api.turns.create.response.v1"
	ContractAPIToolExecuteRequest    = "api.tools.execute.request.v1"
	ContractAPIToolExecuteResponse   = "api.tools.execute.response.v1"
	ContractAuditEvent               = "persistence.audit.event.v1"
	ContractContextFragment          = "context.fragment.v1"
	ContractApprovalRecord           = "approval.record.v1"
	ContractPatchProposal            = "patch.proposal.v1"
	ContractModelRoutingDecision     = "routing.model_selection.v1"
)

func NewRegistry() *Registry {
	return &Registry{definitions: make(map[string]SchemaDefinition)}
}

func (r *Registry) Register(definition SchemaDefinition) error {
	if definition.ID == "" {
		return fmt.Errorf("schema ID is required")
	}
	if definition.Version == "" {
		return fmt.Errorf("schema version is required for %s", definition.ID)
	}
	if definition.Owner == "" {
		return fmt.Errorf("schema owner is required for %s", definition.ID)
	}
	if len(definition.Object) == 0 {
		return fmt.Errorf("object schema is required for %s", definition.ID)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.definitions[definition.ID]; exists {
		return fmt.Errorf("%w: %s", ErrSchemaAlreadyRegistered, definition.ID)
	}
	r.definitions[definition.ID] = definition
	return nil
}

func (r *Registry) Get(id string) (SchemaDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	definition, ok := r.definitions[id]
	return definition, ok
}

func (r *Registry) List() []SchemaDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	definitions := make([]SchemaDefinition, 0, len(r.definitions))
	for _, definition := range r.definitions {
		definitions = append(definitions, definition)
	}

	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].ID < definitions[j].ID
	})

	return definitions
}

var defaultRegistry = mustDefaultRegistry()

func DefaultRegistry() *Registry {
	return defaultRegistry
}

func mustDefaultRegistry() *Registry {
	registry := NewRegistry()

	definitions := []SchemaDefinition{
		{
			ID:      ContractAPISessionCreateRequest,
			Version: "v1",
			Owner:   "controlplane",
			Object: ObjectSchema{
				"repo_path":  {Type: FieldTypeString, Required: true},
				"trust_tier": {Type: FieldTypeString, Required: true},
			},
		},
		{
			ID:      ContractAPISessionCreateResponse,
			Version: "v1",
			Owner:   "controlplane",
			Object: ObjectSchema{
				"session_id": {Type: FieldTypeString, Required: true},
				"repo_path":  {Type: FieldTypeString, Required: true},
				"trust_tier": {Type: FieldTypeString, Required: true},
				"status":     {Type: FieldTypeString, Required: true},
				"created_at": {Type: FieldTypeString, Required: true},
				"updated_at": {Type: FieldTypeString, Required: true},
			},
		},
		{
			ID:      ContractAPITurnCreateRequest,
			Version: "v1",
			Owner:   "controlplane",
			Object: ObjectSchema{
				"session_id": {Type: FieldTypeString, Required: true},
				"message":    {Type: FieldTypeString, Required: true},
			},
		},
		{
			ID:      ContractAPITurnCreateResponse,
			Version: "v1",
			Owner:   "controlplane",
			Object: ObjectSchema{
				"turn_id":      {Type: FieldTypeString, Required: true},
				"session_id":   {Type: FieldTypeString, Required: true},
				"user_message": {Type: FieldTypeString, Required: true},
				"created_at":   {Type: FieldTypeString, Required: true},
			},
		},
		{
			ID:      ContractAPIToolExecuteRequest,
			Version: "v1",
			Owner:   "tools",
			Object: ObjectSchema{
				"tool_name": {Type: FieldTypeString, Required: true},
				"input":     {Type: FieldTypeObject, Required: true},
			},
		},
		{
			ID:      ContractAPIToolExecuteResponse,
			Version: "v1",
			Owner:   "tools",
			Object: ObjectSchema{
				"id":      {Type: FieldTypeString, Required: true},
				"success": {Type: FieldTypeBoolean, Required: true},
				"output":  {Type: FieldTypeObject, Required: true},
			},
		},
		{
			ID:      ContractAuditEvent,
			Version: "v1",
			Owner:   "audit",
			Object: ObjectSchema{
				"event_id":   {Type: FieldTypeString, Required: true},
				"session_id": {Type: FieldTypeString, Required: true},
				"event_type": {Type: FieldTypeString, Required: true},
				"actor":      {Type: FieldTypeString, Required: true},
				"timestamp":  {Type: FieldTypeString, Required: true},
			},
		},
		{
			ID:      ContractContextFragment,
			Version: "v1",
			Owner:   "context",
			Object: ObjectSchema{
				"fragment_id":  {Type: FieldTypeString, Required: true},
				"turn_id":      {Type: FieldTypeString, Required: true},
				"source_type":  {Type: FieldTypeString, Required: true},
				"content":      {Type: FieldTypeString, Required: true},
				"retrieved_at": {Type: FieldTypeString, Required: true},
			},
		},
		{
			ID:      ContractApprovalRecord,
			Version: "v1",
			Owner:   "controlplane",
			Object: ObjectSchema{
				"approval_id": {Type: FieldTypeString, Required: true},
				"session_id":  {Type: FieldTypeString, Required: true},
				"state":       {Type: FieldTypeString, Required: true},
				"tool_name":   {Type: FieldTypeString, Required: true},
			},
		},
		{
			ID:      ContractPatchProposal,
			Version: "v1",
			Owner:   "patch",
			Object: ObjectSchema{
				"patch": {Type: FieldTypeString, Required: true},
			},
		},
		{
			ID:      ContractModelRoutingDecision,
			Version: "v1",
			Owner:   "policy",
			Object: ObjectSchema{
				"provider":   {Type: FieldTypeString, Required: true},
				"model":      {Type: FieldTypeString, Required: true},
				"trust_tier": {Type: FieldTypeString, Required: true},
			},
		},
	}

	for _, definition := range definitions {
		if err := registry.Register(definition); err != nil {
			panic(err)
		}
	}

	return registry
}
