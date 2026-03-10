package api

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "update committed schema artifact")

func TestMain(m *testing.M) {
	flag.Parse()
	if *updateGolden {
		_, thisFile, _, _ := runtime.Caller(0)
		repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
		committed := filepath.Join(repoRoot, "docs", "schema", "registry.json")
		r := DefaultSchemaRegistry()
		artifact, err := r.GenerateMachineArtifact()
		if err != nil {
			fmt.Fprintf(os.Stderr, "GenerateMachineArtifact: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(committed, []byte(artifact+"\n"), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "updated %s\n", committed)
	}
	os.Exit(m.Run())
}

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

func TestGeneratedArtifactMatchesCommittedFile(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine source file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	committed := filepath.Join(repoRoot, "docs", "schema", "registry.json")

	data, err := os.ReadFile(committed)
	if err != nil {
		t.Fatalf("read committed artifact %s: %v — run 'make schema-generate' to create it", committed, err)
	}

	registry := DefaultSchemaRegistry()
	generated, err := registry.GenerateMachineArtifact()
	if err != nil {
		t.Fatalf("GenerateMachineArtifact: %v", err)
	}

	if strings.TrimSpace(string(data)) != strings.TrimSpace(generated) {
		t.Fatalf("committed artifact at docs/schema/registry.json is stale — run 'make schema-generate' to regenerate\n\ncommitted:\n%s\n\ngenerated:\n%s",
			string(data), generated)
	}
}

// TestSchemaDocumentationAndDriftDetection_Bead_l3d_14_4 is the bead-tagged conformance
// entry point for l3d.14.4 (generate schema documentation and SDK-facing artifacts).
func TestSchemaDocumentationAndDriftDetection_Bead_l3d_14_4(t *testing.T) {
	t.Parallel()
	t.Run("deterministic generation", TestGenerateMachineArtifactIsDeterministic)
	t.Run("includes schema versions", TestGenerateMachineArtifactIncludesSchemaVersions)
	t.Run("documentation includes field table", TestGenerateDocumentationArtifactIncludesFieldTable)
	t.Run("detects drift", TestValidateGeneratedArtifactsDetectsDrift)
	t.Run("matches committed file", TestGeneratedArtifactMatchesCommittedFile)
}
