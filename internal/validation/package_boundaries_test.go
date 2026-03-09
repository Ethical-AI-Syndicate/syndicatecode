package validation

import "testing"

func TestDefaultPackageBoundarySpecIncludesCorePackages(t *testing.T) {
	t.Parallel()

	spec := DefaultPackageBoundarySpec()
	required := []string{
		"internal/controlplane",
		"internal/context",
		"internal/session",
		"internal/state",
		"internal/audit",
		"internal/tools",
		"internal/sandbox",
	}

	for _, pkg := range required {
		if _, ok := spec.Packages[pkg]; !ok {
			t.Fatalf("expected package boundary for %s", pkg)
		}
	}
}

func TestDefaultPackageBoundarySpecDefinesAllowedDependencies(t *testing.T) {
	t.Parallel()

	spec := DefaultPackageBoundarySpec()
	cp := spec.Packages["internal/controlplane"]
	if len(cp.AllowedImports) == 0 {
		t.Fatalf("expected controlplane to define allowed imports")
	}
}

func TestValidateCoverageDetectsMissingPackageBoundary(t *testing.T) {
	t.Parallel()

	spec := BoundarySpec{Packages: map[string]PackageBoundary{
		"internal/controlplane": {Owner: "controlplane", AllowedImports: []string{"internal/context"}},
	}}

	err := spec.ValidateCoverage([]string{"internal/controlplane", "internal/context"})
	if err == nil {
		t.Fatalf("expected missing package boundary error")
	}
}
