package api

import (
	"strings"
	"testing"
)

func TestGenerateMachineArtifactIsDeterministic(t *testing.T) {
	t.Parallel()

	registry := DefaultSchemaRegistry()
	first, err := registry.GenerateMachineArtifact()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	second, err := registry.GenerateMachineArtifact()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if first != second {
		t.Fatalf("expected deterministic artifact output")
	}
}

func TestGenerateMachineArtifactIncludesSchemaVersions(t *testing.T) {
	t.Parallel()

	registry := DefaultSchemaRegistry()
	artifact, err := registry.GenerateMachineArtifact()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(artifact, "\"name\"") || !strings.Contains(artifact, "session") {
		t.Fatalf("expected session schema in machine artifact, got: %s", artifact)
	}
	if !strings.Contains(artifact, "\"version\"") || !strings.Contains(artifact, "1") {
		t.Fatalf("expected version field in machine artifact")
	}
}

func TestGenerateDocumentationArtifactIncludesFieldTable(t *testing.T) {
	t.Parallel()

	registry := DefaultSchemaRegistry()
	doc := registry.GenerateDocumentationArtifact()

	if !strings.Contains(doc, "# API Schema Registry") {
		t.Fatalf("expected top-level heading in documentation artifact")
	}
	if !strings.Contains(doc, "| Field | Type | Required |") {
		t.Fatalf("expected field table in documentation artifact")
	}
	if !strings.Contains(doc, "## session") {
		t.Fatalf("expected session schema section in documentation artifact")
	}
}

func TestValidateGeneratedArtifactsDetectsDrift(t *testing.T) {
	t.Parallel()

	registry := DefaultSchemaRegistry()
	machine, err := registry.GenerateMachineArtifact()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	doc := registry.GenerateDocumentationArtifact()

	err = ValidateGeneratedArtifacts(registry, machine+"-drift", doc)
	if err == nil {
		t.Fatalf("expected drift detection error")
	}
}
