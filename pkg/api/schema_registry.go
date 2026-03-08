package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrSchemaArtifactDrift = errors.New("schema artifact drift detected")

type SchemaRegistry struct {
	Schemas []SchemaDefinition
}

func DefaultSchemaRegistry() SchemaRegistry {
	return SchemaRegistry{
		Schemas: []SchemaDefinition{
			{
				Name:    "session",
				Version: 1,
				Fields: []SchemaField{
					{Name: "session_id", Type: "string", Required: true},
					{Name: "repo_path", Type: "string", Required: true},
					{Name: "trust_tier", Type: "string", Required: true},
				},
			},
			{
				Name:    "turn",
				Version: 1,
				Fields: []SchemaField{
					{Name: "turn_id", Type: "string", Required: true},
					{Name: "session_id", Type: "string", Required: true},
					{Name: "message", Type: "string", Required: true},
					{Name: "status", Type: "string", Required: true},
				},
			},
		},
	}
}

func (r SchemaRegistry) GenerateMachineArtifact() (string, error) {
	schemas := cloneSchemas(r.Schemas)
	sort.Slice(schemas, func(i, j int) bool {
		return schemas[i].Name < schemas[j].Name
	})
	for i := range schemas {
		sort.Slice(schemas[i].Fields, func(a, b int) bool {
			return schemas[i].Fields[a].Name < schemas[i].Fields[b].Name
		})
	}

	payload := struct {
		Schemas []SchemaDefinition `json:"schemas"`
	}{Schemas: schemas}

	bytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal schema artifact: %w", err)
	}

	return string(bytes), nil
}

func (r SchemaRegistry) GenerateDocumentationArtifact() string {
	schemas := cloneSchemas(r.Schemas)
	sort.Slice(schemas, func(i, j int) bool {
		return schemas[i].Name < schemas[j].Name
	})

	var b strings.Builder
	b.WriteString("# API Schema Registry\n\n")
	b.WriteString("This document is generated from canonical schema definitions.\n\n")

	for _, schema := range schemas {
		b.WriteString("## ")
		b.WriteString(schema.Name)
		b.WriteString("\n\n")
		_, _ = fmt.Fprintf(&b, "- Version: %d\n\n", schema.Version)
		b.WriteString("| Field | Type | Required |\n")
		b.WriteString("| --- | --- | --- |\n")

		fields := append([]SchemaField{}, schema.Fields...)
		sort.Slice(fields, func(i, j int) bool {
			return fields[i].Name < fields[j].Name
		})
		for _, field := range fields {
			b.WriteString("| ")
			b.WriteString(field.Name)
			b.WriteString(" | ")
			b.WriteString(field.Type)
			b.WriteString(" | ")
			if field.Required {
				b.WriteString("yes")
			} else {
				b.WriteString("no")
			}
			b.WriteString(" |\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

func cloneSchemas(in []SchemaDefinition) []SchemaDefinition {
	out := make([]SchemaDefinition, len(in))
	for i := range in {
		out[i] = SchemaDefinition{
			Name:    in[i].Name,
			Version: in[i].Version,
			Fields:  append([]SchemaField{}, in[i].Fields...),
		}
	}
	return out
}

func ValidateGeneratedArtifacts(registry SchemaRegistry, expectedMachine, expectedDoc string) error {
	actualMachine, err := registry.GenerateMachineArtifact()
	if err != nil {
		return err
	}
	actualDoc := registry.GenerateDocumentationArtifact()

	if actualMachine != expectedMachine {
		return fmt.Errorf("%w: machine artifact differs from generated output", ErrSchemaArtifactDrift)
	}
	if actualDoc != expectedDoc {
		return fmt.Errorf("%w: documentation artifact differs from generated output", ErrSchemaArtifactDrift)
	}

	return nil
}
