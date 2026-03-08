package validation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLintImportRulesReportsViolationsWithFilePath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "internal", "context"))
	mustWriteFile(t, filepath.Join(root, "internal", "context", "bad.go"), "package context\nimport \"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools\"\n")

	spec := DefaultPackageBoundarySpec()
	violations, err := LintImportRules(root, spec)
	if err != nil {
		t.Fatalf("expected no lint error, got %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected one violation, got %d", len(violations))
	}
	if violations[0].FilePath == "" {
		t.Fatalf("expected file path in violation")
	}
	if violations[0].ImportPath != "internal/tools" {
		t.Fatalf("expected offending import path internal/tools, got %s", violations[0].ImportPath)
	}
}

func TestLintImportRulesIgnoresFilesOutsideBoundarySpec(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "pkg", "unknown"))
	mustWriteFile(t, filepath.Join(root, "pkg", "unknown", "ok.go"), "package unknown\nimport \"fmt\"\n")

	spec := DefaultPackageBoundarySpec()
	violations, err := LintImportRules(root, spec)
	if err != nil {
		t.Fatalf("expected no lint error, got %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("expected no violations for packages not in spec, got %d", len(violations))
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}
