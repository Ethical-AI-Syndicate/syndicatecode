package validation

import (
	"strings"
	"testing"
)

func TestValidateImportsAllowsDeclaredInternalDependencies(t *testing.T) {
	t.Parallel()

	spec := DefaultPackageBoundarySpec()
	violations, err := spec.ValidateImports("internal/controlplane", []string{"internal/session", "internal/context"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %d", len(violations))
	}
}

func TestValidateImportsRejectsUndeclaredInternalDependency(t *testing.T) {
	t.Parallel()

	spec := DefaultPackageBoundarySpec()
	violations, err := spec.ValidateImports("internal/context", []string{"internal/tools"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Package != "internal/context" || violations[0].ImportPath != "internal/tools" {
		t.Fatalf("unexpected violation content: %+v", violations[0])
	}
	if !strings.Contains(violations[0].Message, "internal/tools") {
		t.Fatalf("expected actionable message with offending import path, got %q", violations[0].Message)
	}
}

func TestValidateImportsIgnoresStandardAndThirdPartyDependencies(t *testing.T) {
	t.Parallel()

	spec := DefaultPackageBoundarySpec()
	violations, err := spec.ValidateImports("internal/context", []string{"encoding/json", "example.com/third-party/lib"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("expected no violations for non-internal imports, got %d", len(violations))
	}
}

func TestValidateImportsRejectsUndeclaredPkgDependency(t *testing.T) {
	t.Parallel()

	spec := BoundarySpec{Packages: map[string]PackageBoundary{
		"cmd/cli": {Owner: "platform", AllowedImports: []string{}},
	}}

	violations, err := spec.ValidateImports("cmd/cli", []string{"pkg/tui"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected one violation, got %d", len(violations))
	}
	if violations[0].ImportPath != "pkg/tui" {
		t.Fatalf("expected offending import pkg/tui, got %s", violations[0].ImportPath)
	}
}
