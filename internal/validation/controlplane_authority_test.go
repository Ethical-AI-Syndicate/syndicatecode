package validation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateControlPlaneAuthorityRejectsBypassImport(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "internal", "context"))
	mustWriteFile(t, filepath.Join(root, "internal", "context", "bad.go"), "package context\nimport \"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/sandbox\"\n")

	violations, err := ValidateControlPlaneAuthority(root)
	if err != nil {
		t.Fatalf("expected no execution error, got %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected one violation, got %d", len(violations))
	}
	if violations[0].ImportPath != "internal/sandbox" {
		t.Fatalf("expected internal/sandbox violation, got %s", violations[0].ImportPath)
	}
}

func TestValidateControlPlaneAuthorityAllowsControlPlaneToolImports(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "internal", "controlplane"))
	mustWriteFile(t, filepath.Join(root, "internal", "controlplane", "ok.go"), "package controlplane\nimport \"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools\"\n")

	violations, err := ValidateControlPlaneAuthority(root)
	if err != nil {
		t.Fatalf("expected no execution error, got %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %d", len(violations))
	}
}

func TestValidateControlPlaneAuthorityRunsAgainstCurrentRepo(t *testing.T) {
	t.Parallel()

	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	violations, err := ValidateControlPlaneAuthority(root)
	if err != nil {
		t.Fatalf("expected validation to execute, got %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("expected no authority violations in current repo, got %d", len(violations))
	}
}
