package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Artifact struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

type VerificationResult struct {
	OK                  bool       `json:"ok"`
	ReadyForPush        bool       `json:"ready_for_push"`
	ReadyForReview      bool       `json:"ready_for_review"`
	ClosureEligible     bool       `json:"closure_eligible"`
	LocalCIEquivalentOK bool       `json:"local_ci_equivalent_ok"`
	Range               string     `json:"range"`
	Bead                string     `json:"bead,omitempty"`
	Artifacts           []Artifact `json:"artifacts"`
}

type GoNoGoCriterion struct {
	ID            string   `json:"id"`
	Description   string   `json:"description"`
	Status        string   `json:"status"`
	EvidenceLinks []string `json:"evidence_links"`
}

type GoNoGoReport struct {
	SchemaVersion string            `json:"schema_version"`
	GeneratedAt   string            `json:"generated_at"`
	Range         string            `json:"range"`
	Bead          string            `json:"bead,omitempty"`
	OverallStatus string            `json:"overall_status"`
	Criteria      []GoNoGoCriterion `json:"criteria"`
}

func main() {
	var (
		outputPath string
		rangeFlag  string
		beadFlag   string
	)
	flag.StringVar(&outputPath, "output", "bead-evidence/v1-go-no-go.json", "output report path")
	flag.StringVar(&rangeFlag, "range", "origin/master..HEAD", "git range to verify")
	flag.StringVar(&beadFlag, "bead", "", "optional bead id for targeted evidence")
	flag.Parse()

	verify, err := runVerifyJSON(rangeFlag, beadFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to run verify-json: %v\n", err)
		os.Exit(1)
	}

	evidenceLinks := collectEvidenceLinks(verify.Artifacts)
	report := buildReport(verify, evidenceLinks)
	if err := writeReport(outputPath, report); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to write go-no-go report: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("PASS: go-no-go report generated %s\n", outputPath)
}

func runVerifyJSON(rangeFlag, beadFlag string) (VerificationResult, error) {
	args := []string{"run", "./tools/verify", "--json", "--range", rangeFlag, "--generate-evidence"}
	if beadFlag != "" {
		args = append(args, "--bead", beadFlag)
	}
	// #nosec G204 -- command and arguments are fixed go tool invocations built from validated flags.
	cmd := exec.Command("go", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// keep parsing JSON when verify exits non-zero on optional phases
		_ = err
	}
	raw := string(out)
	jsonStart := strings.IndexByte(raw, '{')
	if jsonStart < 0 {
		return VerificationResult{}, fmt.Errorf("verify output did not contain JSON")
	}
	jsonEnd := strings.LastIndexByte(raw, '}')
	if jsonEnd < jsonStart {
		return VerificationResult{}, fmt.Errorf("verify output contained malformed JSON")
	}
	out = []byte(raw[jsonStart : jsonEnd+1])

	var parsed VerificationResult
	if err := json.Unmarshal(out, &parsed); err != nil {
		return VerificationResult{}, fmt.Errorf("decode verify output: %w", err)
	}
	return parsed, nil
}

func buildReport(verify VerificationResult, evidenceLinks []string) GoNoGoReport {
	criteria := []GoNoGoCriterion{
		{ID: "ci-equivalence", Description: "Local CI-equivalent verification passes", Status: statusForBool(verify.LocalCIEquivalentOK), EvidenceLinks: append([]string{}, evidenceLinks...)},
		{ID: "review-readiness", Description: "Changes are ready for review", Status: statusForBool(verify.ReadyForReview), EvidenceLinks: append([]string{}, evidenceLinks...)},
		{ID: "closure-eligibility", Description: "Bead closure-check is eligible", Status: statusForBool(verify.ClosureEligible), EvidenceLinks: append([]string{}, evidenceLinks...)},
	}

	overall := "pass"
	if !verify.OK || !verify.ReadyForReview || !verify.LocalCIEquivalentOK {
		overall = "fail"
	}

	return GoNoGoReport{
		SchemaVersion: "1",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Range:         verify.Range,
		Bead:          verify.Bead,
		OverallStatus: overall,
		Criteria:      criteria,
	}
}

func statusForBool(ok bool) string {
	if ok {
		return "pass"
	}
	return "fail"
}

func collectEvidenceLinks(artifacts []Artifact) []string {
	links := make([]string, 0)
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.Path) == "" {
			continue
		}
		links = append(links, filepath.ToSlash(artifact.Path))
	}
	if len(links) == 0 {
		links = append(links, "docs/ai_cli_v_1_architecture_checklist.md")
	}
	sort.Strings(links)
	return links
}

func writeReport(path string, report GoNoGoReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(encoded, '\n'), 0o600)
}
