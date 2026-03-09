package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCanonicalBeads_Bead_l3d_100(t *testing.T) {
	input := "feat(controlplane): enforce routing [l3d.5.2] and l3d.5.2"
	got := parseCanonicalBeads(input)
	if len(got) != 1 || got[0] != "l3d.5.2" {
		t.Fatalf("expected [l3d.5.2], got %v", got)
	}
}

func TestParseMalformedBeads_Bead_l3d_101(t *testing.T) {
	input := "fix: cover bd-12 and l3d-7-4"
	got := parseMalformedBeads(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 malformed tokens, got %v", got)
	}
}

func TestValidatePRMetadata_Bead_l3d_102(t *testing.T) {
	body := `## Bead References
- l3d.1

## Acceptance Criteria Mapping
- [x] AC-1 mapped to TestSession_Bead_l3d_1

## TDD Evidence
- [x] I wrote a failing test first
- [x] I added or updated regression tests

## Test Evidence
- internal/state/lifecycle_test.go

## Evidence Artifacts
- bead-evidence/l3d.1.json`

	issues := validatePRMetadata("feat: enforce lifecycle [l3d.1]", body)
	if len(issues) != 0 {
		t.Fatalf("expected no metadata issues, got %v", issues)
	}
}

func TestParseBeadTagsFromTestFile_Bead_l3d_103(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "sample_test.go")
	content := `package sample

func TestSessionTransition_Bead_l3d_1_3(t *testing.T) {}
func TestOtherBehavior(t *testing.T) {}
func TestPolicyGate_Bead_l3d_5_2(t *testing.T) {}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	beads, err := parseBeadTagsFromTestFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(beads) != 2 || beads[0] != "l3d.1.3" || beads[1] != "l3d.5.2" {
		t.Fatalf("unexpected tags %v", beads)
	}
}

func TestEvaluateEvidence_Bead_l3d_1(t *testing.T) {
	e := beadEvidence{
		Commits:      []commitInfo{{SHA: "abc", Subject: "feat [l3d.1]"}},
		ChangedFiles: []string{"internal/state/lifecycle.go"},
		LinkedTests:  []linkedTest{{File: "internal/state/lifecycle_test.go", TestName: "TestLifecycle_Bead_l3d_1"}},
		CIPhases: map[string]string{
			"format":      "pass",
			"lint":        "pass",
			"test":        "pass",
			"build":       "pass",
			"bead-verify": "pass",
			"security":    "pass",
		},
	}
	result := evaluateEvidence(e)
	if !result.Credible {
		t.Fatalf("expected credible evidence, got %v", result.Reasons)
	}
}

func TestValidateChangedGoFiles_NoChangedTests_Bead_l3d_1(t *testing.T) {
	issues := validateChangedGoFiles([]string{"internal/session/manager.go"}, []string{})
	if len(issues) == 0 {
		t.Fatal("expected verification issue for missing changed test files")
	}
}

func TestFindTaggedTestsForBead_SkipsSymlinkedTestFiles_Bead_l3d_1_3(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	if err := os.WriteFile("good_test.go", []byte("package main\nfunc TestX_Bead_l3d_1_3(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/definitely/missing/target_test.go", "broken_test.go"); err != nil {
		t.Fatal(err)
	}

	tests, err := findTaggedTestsForBead("l3d.1.3")
	if err != nil {
		t.Fatalf("expected no error while walking test files, got %v", err)
	}
	if len(tests) != 1 {
		t.Fatalf("expected exactly one linked test, got %d", len(tests))
	}
	if tests[0].File != "good_test.go" {
		t.Fatalf("expected good_test.go result, got %s", tests[0].File)
	}
}
