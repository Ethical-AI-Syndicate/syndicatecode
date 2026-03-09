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

func TestDefaultPackageBoundarySpecCoversCurrentFirstPartyPackages(t *testing.T) {
	t.Parallel()

	moduleRoot, err := findModuleRootForTest()
	if err != nil {
		t.Fatalf("failed to resolve module root: %v", err)
	}

	cmd := exec.Command("go", "list", "./...")
	cmd.Dir = moduleRoot
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to list packages: %v", err)
	}

	const modulePrefix = "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/"
	pkgSet := make([]string, 0)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, modulePrefix) {
			continue
		}
		rel := strings.TrimPrefix(line, modulePrefix)
		if strings.HasPrefix(rel, "internal/") || strings.HasPrefix(rel, "cmd/") {
			pkgSet = append(pkgSet, rel)
		}
	}

	spec := DefaultPackageBoundarySpec()
	if err := spec.ValidateCoverage(pkgSet); err != nil {
		t.Fatalf("expected boundary spec coverage for current first-party packages, got %v", err)
	}
}

func TestDefaultPackageBoundarySpecAllowsCurrentImports(t *testing.T) {
	t.Parallel()

	moduleRoot, err := findModuleRootForTest()
	if err != nil {
		t.Fatalf("failed to resolve module root: %v", err)
	}

	violations, err := LintImportRules(moduleRoot, DefaultPackageBoundarySpec())
	if err != nil {
		t.Fatalf("failed to lint imports: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("expected no boundary violations for current package map, got %d (first: %+v)", len(violations), violations[0])
	}
}

func findModuleRootForTest() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	current := cwd
	for {
		candidate := filepath.Join(current, "go.mod")
		if _, err := os.Stat(candidate); err == nil {
			return current, nil
		}
		next := filepath.Dir(current)
		if next == current {
			return "", os.ErrNotExist
		}
		current = next
	}
}
