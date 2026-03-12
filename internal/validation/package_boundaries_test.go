package validation

import (
	"os"
	"os/exec"
	"slices"
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
	requiredImports := []string{"internal/context", "internal/session"}
	for _, requiredImport := range requiredImports {
		if !slices.Contains(cp.AllowedImports, requiredImport) {
			t.Fatalf("expected controlplane allowed imports to include %q", requiredImport)
		}
	}
}

func TestDefaultPackageBoundarySpecIncludesAgentAndTrust_Bead_l3d_X_2(t *testing.T) {
	t.Parallel()

	spec := DefaultPackageBoundarySpec()
	if _, ok := spec.Packages["internal/agent"]; !ok {
		t.Fatal("missing boundary for internal/agent")
	}
	if _, ok := spec.Packages["internal/trust"]; !ok {
		t.Fatal("missing boundary for internal/trust")
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

	modulePath := mustModulePathFromGoMod(t, moduleRoot)
	prefix := modulePath + "/"
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	packages := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		pkg := strings.TrimPrefix(strings.TrimSpace(line), prefix)
		packages = append(packages, pkg)
	}

	return packages
}

func mustModulePathFromGoMod(t *testing.T, moduleRoot string) string {
	t.Helper()

	goModPath := moduleRoot + "/go.mod"
	contents, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("failed to read go.mod: %v", err)
	}

	for _, line := range strings.Split(string(contents), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if strings.HasPrefix(trimmed, "module ") {
			modulePath := strings.TrimSpace(strings.TrimPrefix(trimmed, "module "))
			modulePath = strings.Trim(modulePath, "\"")
			if modulePath == "" {
				break
			}
			return modulePath
		}
	}

	t.Fatalf("failed to parse module path from go.mod at %s", goModPath)
	return ""
}
