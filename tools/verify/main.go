package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultRange = "origin/master..HEAD"
)

type Config struct {
	Mode             string
	JSON             bool
	Range            string
	Bead             string
	Title            string
	Description      string
	MRFile           string
	GenerateEvidence bool
	ClosureCheck     bool
	StrictMetadata   bool
}

type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Err      error
}

type CommandRunner interface {
	Run(command string) CommandResult
}

type ShellRunner struct{}

func (s ShellRunner) Run(command string) CommandResult {
	// #nosec G204 -- command strings are assembled internally from fixed verifier phases.
	cmd := exec.Command("bash", "-lc", command)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := CommandResult{
		ExitCode: exitCode(err),
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		Err:      err,
	}
	return result
}

type VerificationResult struct {
	SchemaVersion       string        `json:"schema_version"`
	OK                  bool          `json:"ok"`
	ReadyForPush        bool          `json:"ready_for_push"`
	ReadyForReview      bool          `json:"ready_for_review"`
	ClosureEligible     bool          `json:"closure_eligible"`
	LocalCIEquivalentOK bool          `json:"local_ci_equivalent_ok"`
	Summary             string        `json:"summary"`
	Mode                string        `json:"mode"`
	Repo                RepoInfo      `json:"repo"`
	Range               string        `json:"range"`
	Bead                string        `json:"bead,omitempty"`
	StartedAt           string        `json:"started_at"`
	FinishedAt          string        `json:"finished_at"`
	DurationMs          int64         `json:"duration_ms"`
	Phases              []PhaseResult `json:"phases"`
	Artifacts           []Artifact    `json:"artifacts"`
	Errors              []string      `json:"errors"`
	Warnings            []string      `json:"warnings"`
	CIParity            CIParity      `json:"ci_parity"`
	Limitations         []string      `json:"limitations"`
}

type RepoInfo struct {
	Root          string `json:"root"`
	Branch        string `json:"branch"`
	DefaultBranch string `json:"default_branch"`
	Dirty         bool   `json:"dirty"`
}

type PhaseResult struct {
	Name          string     `json:"name"`
	OK            bool       `json:"ok"`
	Status        string     `json:"status"`
	Required      bool       `json:"required"`
	Skipped       bool       `json:"skipped"`
	DurationMs    int64      `json:"duration_ms"`
	Command       string     `json:"command,omitempty"`
	ExitCode      *int       `json:"exit_code,omitempty"`
	StdoutSummary string     `json:"stdout_summary,omitempty"`
	StderrSummary string     `json:"stderr_summary,omitempty"`
	Artifacts     []Artifact `json:"artifacts,omitempty"`
	Notes         []string   `json:"notes,omitempty"`
}

type Artifact struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Note string `json:"note,omitempty"`
}

type CIParity struct {
	Approximate  bool     `json:"approximate"`
	CheckedLocal []string `json:"checked_local"`
	MissingLocal []string `json:"missing_local"`
	Notes        []string `json:"notes"`
}

func main() {
	cfg := parseConfig()
	if cfg.MRFile != "" && cfg.Description == "" {
		body, err := os.ReadFile(cfg.MRFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: failed to read --mr-file: %v\n", err)
			os.Exit(1)
		}
		cfg.Description = string(body)
	}

	result := RunVerification(cfg, ShellRunner{})
	if cfg.JSON {
		renderJSON(result)
	} else {
		renderText(result)
	}

	if !result.OK {
		os.Exit(1)
	}
}

func parseConfig() Config {
	var cfg Config
	flag.StringVar(&cfg.Range, "range", defaultRange, "git range for verification")
	flag.BoolVar(&cfg.JSON, "json", false, "emit machine-readable JSON output")
	flag.StringVar(&cfg.Bead, "bead", "", "canonical bead id (l3d.X)")
	flag.StringVar(&cfg.Title, "title", strings.TrimSpace(os.Getenv("CI_MERGE_REQUEST_TITLE")), "merge request title")
	flag.StringVar(&cfg.Description, "description", strings.TrimSpace(os.Getenv("CI_MERGE_REQUEST_DESCRIPTION")), "merge request description")
	flag.StringVar(&cfg.MRFile, "mr-file", "", "path to MR/PR description body file")
	flag.BoolVar(&cfg.GenerateEvidence, "generate-evidence", false, "generate bead evidence artifact")
	flag.BoolVar(&cfg.ClosureCheck, "closure-check", false, "run bead closure eligibility check")
	flag.BoolVar(&cfg.StrictMetadata, "strict-metadata", false, "treat missing metadata as failure")
	flag.Parse()
	if cfg.JSON {
		cfg.Mode = "json"
	} else {
		cfg.Mode = "text"
	}
	return cfg
}

func RunVerification(cfg Config, runner CommandRunner) VerificationResult {
	start := time.Now().UTC()
	res := VerificationResult{
		SchemaVersion: "1",
		Mode:          cfg.Mode,
		Range:         cfg.Range,
		Bead:          cfg.Bead,
		StartedAt:     start.Format(time.RFC3339),
		Phases:        make([]PhaseResult, 0),
		Artifacts:     make([]Artifact, 0),
		Errors:        make([]string, 0),
		Warnings:      make([]string, 0),
		Limitations:   make([]string, 0),
		CIParity: CIParity{
			Approximate:  true,
			CheckedLocal: []string{"format", "lint", "test", "build", "bead-verify", "metadata-verify", "security", "evidence", "closure-check"},
			MissingLocal: []string{},
			Notes:        []string{},
		},
	}

	repo := runRepoSafetyPhase(runner)
	res.Repo = repo.info
	res.Phases = append(res.Phases, repo.phase)
	accumulateMessages(&res, repo.phase)

	formatPhase := runPhase("format", "./.gitlab/scripts/check-gofmt.sh", runner)
	res.Phases = append(res.Phases, formatPhase)
	accumulateMessages(&res, formatPhase)

	lintPhase := runPhase("lint", "golangci-lint run", runner)
	res.Phases = append(res.Phases, lintPhase)
	accumulateMessages(&res, lintPhase)

	testPhase := runPhase("test", "go test -race ./...", runner)
	res.Phases = append(res.Phases, testPhase)
	accumulateMessages(&res, testPhase)

	buildPhase := runPhase("build", "go build -o bin/ ./cmd/... && go vet ./...", runner)
	res.Phases = append(res.Phases, buildPhase)
	accumulateMessages(&res, buildPhase)

	beadVerify := runBeadVerifyPhase(cfg, runner)
	res.Phases = append(res.Phases, beadVerify)
	accumulateMessages(&res, beadVerify)

	metadata := runMetadataPhase(cfg, runner)
	res.Phases = append(res.Phases, metadata)
	accumulateMessages(&res, metadata)

	security := runSecurityPhase(runner)
	res.Phases = append(res.Phases, security)
	accumulateMessages(&res, security)
	if security.Skipped {
		res.CIParity.MissingLocal = append(res.CIParity.MissingLocal, "security")
		res.CIParity.Notes = append(res.CIParity.Notes, "security phase skipped locally when gosec unavailable")
		res.Limitations = append(res.Limitations, "gosec not available locally; security parity requires CI")
	}

	evidence := runEvidencePhase(cfg, runner, map[string]PhaseResult{
		"format":          formatPhase,
		"lint":            lintPhase,
		"test":            testPhase,
		"build":           buildPhase,
		"bead-verify":     beadVerify,
		"metadata-verify": metadata,
		"security":        security,
	})
	res.Phases = append(res.Phases, evidence)
	accumulateMessages(&res, evidence)
	res.Artifacts = append(res.Artifacts, evidence.Artifacts...)

	closure := runClosurePhase(cfg, runner)
	res.Phases = append(res.Phases, closure)
	accumulateMessages(&res, closure)

	res.LocalCIEquivalentOK = requiredCorePhasesPass(res.Phases)
	res.ClosureEligible = closure.OK && !closure.Skipped
	res.ReadyForPush = res.LocalCIEquivalentOK
	res.ReadyForReview = res.LocalCIEquivalentOK && beadVerify.OK
	res.OK = overallOK(res.Phases)
	res.CIParity.Approximate = res.LocalCIEquivalentOK && len(res.CIParity.MissingLocal) == 0

	finish := time.Now().UTC()
	res.FinishedAt = finish.Format(time.RFC3339)
	res.DurationMs = finish.Sub(start).Milliseconds()
	res.Summary = buildSummary(res)
	return res
}

func runRepoSafetyPhase(runner CommandRunner) struct {
	phase PhaseResult
	info  RepoInfo
} {
	start := time.Now()
	rootRes := runner.Run("git rev-parse --show-toplevel")
	branchRes := runner.Run("git branch --show-current")
	remoteRes := runner.Run("git remote show origin")
	statusRes := runner.Run("git status --short")

	phase := PhaseResult{Name: "repo-safety", Required: true, Command: "git rev-parse --show-toplevel && git branch --show-current && git remote show origin && git status --short"}
	info := RepoInfo{
		Root:   strings.TrimSpace(rootRes.Stdout),
		Branch: strings.TrimSpace(branchRes.Stdout),
		Dirty:  strings.TrimSpace(statusRes.Stdout) != "",
	}
	if strings.Contains(remoteRes.Stdout, "HEAD branch:") {
		for _, line := range strings.Split(remoteRes.Stdout, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "HEAD branch:") {
				info.DefaultBranch = strings.TrimSpace(strings.TrimPrefix(line, "HEAD branch:"))
				break
			}
		}
	}
	if info.DefaultBranch == "" {
		info.DefaultBranch = "master"
	}

	phase.StdoutSummary = summarizeLines(strings.Join([]string{rootRes.Stdout, branchRes.Stdout, "default=" + info.DefaultBranch}, "\n"))
	phase.DurationMs = time.Since(start).Milliseconds()

	if rootRes.ExitCode != 0 || branchRes.ExitCode != 0 || remoteRes.ExitCode != 0 || statusRes.ExitCode != 0 {
		phase.OK = false
		phase.Status = "fail"
		ec := nonZero(rootRes.ExitCode, branchRes.ExitCode, remoteRes.ExitCode, statusRes.ExitCode)
		phase.ExitCode = &ec
		phase.StderrSummary = summarizeLines(strings.Join([]string{rootRes.Stderr, branchRes.Stderr, remoteRes.Stderr, statusRes.Stderr}, "\n"))
		return struct {
			phase PhaseResult
			info  RepoInfo
		}{phase: phase, info: info}
	}

	phase.OK = true
	phase.Status = "pass"
	zero := 0
	phase.ExitCode = &zero
	if info.Dirty {
		phase.Notes = append(phase.Notes, "working tree is dirty")
	}
	return struct {
		phase PhaseResult
		info  RepoInfo
	}{phase: phase, info: info}
}

func runPhase(name string, command string, runner CommandRunner) PhaseResult {
	start := time.Now()
	r := runner.Run(command)
	phase := PhaseResult{
		Name:          name,
		Required:      true,
		Command:       command,
		DurationMs:    time.Since(start).Milliseconds(),
		StdoutSummary: summarizeLines(r.Stdout),
		StderrSummary: summarizeLines(r.Stderr),
	}
	ec := r.ExitCode
	phase.ExitCode = &ec
	if r.ExitCode == 0 {
		phase.OK = true
		phase.Status = "pass"
	} else {
		phase.OK = false
		phase.Status = "fail"
	}
	return phase
}

func runBeadVerifyPhase(cfg Config, runner CommandRunner) PhaseResult {
	start := time.Now()
	cmd1 := "go run ./tools/beads verify-commits --range " + shellQuote(cfg.Range) + " --strict"
	cmd2 := "go run ./tools/beads verify --range " + shellQuote(cfg.Range) + " --strict"
	r1 := runner.Run(cmd1)
	r2 := runner.Run(cmd2)
	phase := PhaseResult{
		Name:          "bead-verify",
		Required:      true,
		Command:       cmd1 + " && " + cmd2,
		DurationMs:    time.Since(start).Milliseconds(),
		StdoutSummary: summarizeLines(strings.TrimSpace(r1.Stdout + "\n" + r2.Stdout)),
		StderrSummary: summarizeLines(strings.TrimSpace(r1.Stderr + "\n" + r2.Stderr)),
	}
	ec := nonZero(r1.ExitCode, r2.ExitCode)
	phase.ExitCode = &ec
	if r1.ExitCode == 0 && r2.ExitCode == 0 {
		phase.OK = true
		phase.Status = "pass"
	} else {
		phase.OK = false
		phase.Status = "fail"
	}
	return phase
}

func runMetadataPhase(cfg Config, runner CommandRunner) PhaseResult {
	phase := PhaseResult{Name: "metadata-verify", Required: cfg.StrictMetadata}
	if strings.TrimSpace(cfg.Title) == "" || strings.TrimSpace(cfg.Description) == "" {
		phase.Skipped = !cfg.StrictMetadata
		if cfg.StrictMetadata {
			phase.Status = "fail"
			phase.OK = false
			phase.Notes = []string{"missing --title and/or --description"}
		} else {
			phase.Status = "skipped"
			phase.OK = false
			phase.Notes = []string{"metadata not provided; phase skipped"}
		}
		return phase
	}

	start := time.Now()
	cmd := "go run ./tools/beads verify-pr --strict --title " + shellQuote(cfg.Title) + " --description " + shellQuote(cfg.Description)
	r := runner.Run(cmd)
	phase.Command = cmd
	phase.DurationMs = time.Since(start).Milliseconds()
	phase.StdoutSummary = summarizeLines(r.Stdout)
	phase.StderrSummary = summarizeLines(r.Stderr)
	ec := r.ExitCode
	phase.ExitCode = &ec
	if r.ExitCode == 0 {
		phase.OK = true
		phase.Status = "pass"
	} else {
		phase.OK = false
		phase.Status = "fail"
	}
	return phase
}

func runSecurityPhase(runner CommandRunner) PhaseResult {
	start := time.Now()
	phase := runPhase("security", "gosec ./...", runner)
	phase.DurationMs = time.Since(start).Milliseconds()
	if phase.ExitCode != nil && *phase.ExitCode == 127 {
		phase.Status = "skipped"
		phase.Skipped = true
		phase.OK = false
		phase.Notes = append(phase.Notes, "gosec not available locally")
	}
	return phase
}

func runEvidencePhase(cfg Config, runner CommandRunner, phaseStatus map[string]PhaseResult) PhaseResult {
	phase := PhaseResult{Name: "evidence", Required: false}
	if !cfg.GenerateEvidence {
		phase.Status = "skipped"
		phase.Skipped = true
		phase.Notes = []string{"enable with --generate-evidence"}
		return phase
	}
	if cfg.Bead == "" {
		phase.Status = "fail"
		phase.OK = false
		phase.Notes = []string{"--bead is required when --generate-evidence is set"}
		return phase
	}

	statuses := []string{
		"--phase format=" + phaseStatusForEvidence(phaseStatus["format"]),
		"--phase lint=" + phaseStatusForEvidence(phaseStatus["lint"]),
		"--phase test=" + phaseStatusForEvidence(phaseStatus["test"]),
		"--phase build=" + phaseStatusForEvidence(phaseStatus["build"]),
		"--phase bead-verify=" + phaseStatusForEvidence(phaseStatus["bead-verify"]),
		"--phase metadata-verify=" + phaseStatusForEvidence(phaseStatus["metadata-verify"]),
		"--phase security=" + phaseStatusForEvidence(phaseStatus["security"]),
	}
	cmd := "go run ./tools/beads generate-evidence --bead " + shellQuote(cfg.Bead) + " --range " + shellQuote(cfg.Range) + " " + strings.Join(statuses, " ")
	start := time.Now()
	r := runner.Run(cmd)
	phase.Command = cmd
	phase.DurationMs = time.Since(start).Milliseconds()
	phase.StdoutSummary = summarizeLines(r.Stdout)
	phase.StderrSummary = summarizeLines(r.Stderr)
	ec := r.ExitCode
	phase.ExitCode = &ec
	if r.ExitCode == 0 {
		phase.OK = true
		phase.Status = "pass"
		path := filepath.Join("bead-evidence", cfg.Bead+".json")
		phase.Artifacts = append(phase.Artifacts, Artifact{Type: "bead-evidence", Path: path})
	} else {
		phase.OK = false
		phase.Status = "fail"
	}
	return phase
}

func runClosurePhase(cfg Config, runner CommandRunner) PhaseResult {
	phase := PhaseResult{Name: "closure-check", Required: false}
	if !cfg.ClosureCheck {
		phase.Status = "skipped"
		phase.Skipped = true
		phase.Notes = []string{"enable with --closure-check"}
		return phase
	}
	if cfg.Bead == "" {
		phase.Status = "fail"
		phase.OK = false
		phase.Notes = []string{"--bead is required when --closure-check is set"}
		return phase
	}
	cmd := "go run ./tools/beads check-closure --bead " + shellQuote(cfg.Bead)
	start := time.Now()
	r := runner.Run(cmd)
	phase.Command = cmd
	phase.DurationMs = time.Since(start).Milliseconds()
	phase.StdoutSummary = summarizeLines(r.Stdout)
	phase.StderrSummary = summarizeLines(r.Stderr)
	ec := r.ExitCode
	phase.ExitCode = &ec
	if r.ExitCode == 0 {
		phase.OK = true
		phase.Status = "pass"
	} else {
		phase.OK = false
		phase.Status = "fail"
	}
	return phase
}

func renderText(result VerificationResult) {
	fmt.Printf("verify: %s\n", result.Summary)
	fmt.Printf("repo=%s branch=%s default=%s range=%s\n", result.Repo.Root, result.Repo.Branch, result.Repo.DefaultBranch, result.Range)
	if result.Bead != "" {
		fmt.Printf("bead=%s\n", result.Bead)
	}
	for _, phase := range result.Phases {
		fmt.Printf("[%s] %s", strings.ToUpper(phase.Status), phase.Name)
		if phase.Command != "" {
			fmt.Printf(" :: %s", phase.Command)
		}
		fmt.Println()
		if phase.StdoutSummary != "" {
			fmt.Printf("  out: %s\n", phase.StdoutSummary)
		}
		if phase.StderrSummary != "" {
			fmt.Printf("  err: %s\n", phase.StderrSummary)
		}
		for _, note := range phase.Notes {
			fmt.Printf("  note: %s\n", note)
		}
	}
	if len(result.Artifacts) > 0 {
		fmt.Println("artifacts:")
		for _, a := range result.Artifacts {
			fmt.Printf("- %s (%s)\n", a.Path, a.Type)
		}
	}
	if len(result.Errors) > 0 {
		fmt.Println("errors:")
		for _, e := range result.Errors {
			fmt.Printf("- %s\n", e)
		}
	}
	if len(result.Warnings) > 0 {
		fmt.Println("warnings:")
		for _, w := range result.Warnings {
			fmt.Printf("- %s\n", w)
		}
	}
}

func renderJSON(result VerificationResult) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(result)
}

func summarizeLines(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) > 6 {
		lines = append(lines[:6], "...truncated")
	}
	joined := strings.Join(lines, " | ")
	if len(joined) > 600 {
		return joined[:600] + "..."
	}
	return joined
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func phaseStatusForEvidence(phase PhaseResult) string {
	if phase.Skipped {
		return "unknown"
	}
	if phase.OK {
		return "pass"
	}
	if phase.Status == "" {
		return "unknown"
	}
	if phase.Status == "fail" {
		return "fail"
	}
	return "unknown"
}

func accumulateMessages(result *VerificationResult, phase PhaseResult) {
	if phase.Status == "fail" && phase.Required {
		result.Errors = append(result.Errors, phase.Name+" failed")
	}
	if phase.Status == "skipped" {
		result.Warnings = append(result.Warnings, phase.Name+" skipped")
	}
	if phase.Name == "repo-safety" && result.Repo.Dirty {
		result.Warnings = append(result.Warnings, "working tree is dirty")
	}
}

func requiredCorePhasesPass(phases []PhaseResult) bool {
	required := map[string]bool{
		"repo-safety":     false,
		"format":          true,
		"lint":            true,
		"test":            true,
		"build":           true,
		"bead-verify":     true,
		"metadata-verify": false,
		"security":        true,
	}
	for _, p := range phases {
		if required[p.Name] && !p.OK {
			return false
		}
	}
	return true
}

func overallOK(phases []PhaseResult) bool {
	for _, p := range phases {
		if p.Required && !p.OK {
			return false
		}
	}
	return true
}

func buildSummary(result VerificationResult) string {
	passed := 0
	failed := 0
	skipped := 0
	for _, p := range result.Phases {
		switch p.Status {
		case "pass":
			passed++
		case "fail":
			failed++
		case "skipped":
			skipped++
		}
	}
	status := "PASS"
	if !result.OK {
		status = "FAIL"
	}
	return fmt.Sprintf("%s phases(pass=%d fail=%d skipped=%d)", status, passed, failed, skipped)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return 1
}

func nonZero(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}
