package validation

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestDefaultPackageBoundarySpecCoversAllFirstPartyPackages(t *testing.T) {
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
	packages := mustListFirstPartyPackages(t, moduleRoot)

	if err := spec.ValidateCoverage(packages); err != nil {
		t.Fatalf("expected full package boundary coverage, got %v", err)
	}
}

func mustListFirstPartyPackages(t *testing.T, moduleRoot string) []string {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}}", "./cmd/...", "./internal/...", "./pkg/...")
	cmd.Dir = moduleRoot
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to list first-party packages: %v", err)
	}

	prefix := "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/"
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	packages := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		pkg := strings.TrimPrefix(strings.TrimSpace(line), prefix)
		pkg = filepath.ToSlash(pkg)
		packages = append(packages, pkg)
	}

	return packages
}
