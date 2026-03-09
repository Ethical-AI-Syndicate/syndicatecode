package validation

import (
	"os"
	"testing"
)

func TestRepoBoundaryLintFirstPartyPackagesHaveZeroViolations(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	moduleRoot, err := findModuleRoot(wd)
	if err != nil {
		t.Fatalf("failed to resolve module root: %v", err)
	}

	spec := DefaultPackageBoundarySpec()
	if err := spec.ValidateCoverage(mustListFirstPartyPackages(t, moduleRoot)); err != nil {
		t.Fatalf("expected full boundary coverage, got %v", err)
	}

	violations, err := LintImportRules(moduleRoot, spec)
	if err != nil {
		t.Fatalf("expected no lint execution error, got %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("expected zero repo boundary violations, got %+v", violations)
	}
}
