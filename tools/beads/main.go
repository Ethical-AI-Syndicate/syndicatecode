package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	defaultRange       = "origin/master..HEAD"
	defaultEvidenceDir = "bead-evidence"
	testTagPrefix      = "Bead_l3d_"
)

var (
	canonicalBeadRE         = regexp.MustCompile(`\bl3d\.[0-9]+(?:\.[0-9]+)*\b`)
	malformedBeadRE         = regexp.MustCompile(`\b(?:bd[-.][0-9]+(?:[-.][0-9]+)*|l3d-[0-9]+(?:-[0-9]+)*)\b`)
	testNameWithTagRE       = regexp.MustCompile(`^func\s+(Test[[:alnum:]_]*` + testTagPrefix + `[0-9_]+)\s*\(`)
	goPackageFileRE         = regexp.MustCompile(`\.go$`)
	goTestFileSuffixRE      = regexp.MustCompile(`_test\.go$`)
	changedFilesForCommitFn = changedFilesForCommit
)

type options struct {
	strict      bool
	verbose     bool
	jsonOutput  bool
	beadID      string
	rangeSpec   string
	title       string
	description string
	evidenceDir string
	phaseKV     multiFlag
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

type commitInfo struct {
	SHA     string   `json:"sha"`
	Subject string   `json:"subject"`
	Beads   []string `json:"beads"`
}

type linkedTest struct {
	File     string   `json:"file"`
	Line     int      `json:"line"`
	TestName string   `json:"test_name"`
	Beads    []string `json:"beads"`
}

type evidenceSummary struct {
	Credible bool     `json:"credible_for_closure"`
	Reasons  []string `json:"reasons"`
}

type beadEvidence struct {
	BeadID             string            `json:"bead_id"`
	BeadIssueID        string            `json:"bead_issue_id"`
	BeadIssueStatus    string            `json:"bead_issue_status,omitempty"`
	GeneratedAtUTC     string            `json:"generated_at_utc"`
	GeneratorVersion   string            `json:"generator_version"`
	HeadSHA            string            `json:"head_sha"`
	Range              string            `json:"range"`
	Commits            []commitInfo      `json:"commits"`
	ChangedFiles       []string          `json:"changed_files"`
	ChangedGoFiles     []string          `json:"changed_go_files"`
	ChangedTestFiles   []string          `json:"changed_test_files"`
	LinkedTests        []linkedTest      `json:"linked_tests"`
	CIPhases           map[string]string `json:"ci_phases"`
	Summary            evidenceSummary   `json:"summary"`
	AcceptanceEvidence []string          `json:"acceptance_criteria_evidence"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	opts, err := parseOptions(cmd, os.Args[2:])
	if err != nil {
		exitErr(err)
	}

	var runErr error
	switch cmd {
	case "verify":
		runErr = runVerify(opts)
	case "verify-commits":
		runErr = runVerifyCommits(opts)
	case "verify-pr":
		runErr = runVerifyPR(opts)
	case "list-beads":
		runErr = runListBeads(opts)
	case "generate-evidence", "evidence":
		runErr = runGenerateEvidence(opts)
	case "check-closure", "closure":
		runErr = runCheckClosure(opts)
	case "show":
		runErr = runShow(opts)
	case "help", "-h", "--help":
		usage()
		return
	default:
		runErr = fmt.Errorf("unknown command %q", cmd)
	}

	if runErr != nil {
		exitErr(runErr)
	}
}

func parseOptions(cmd string, args []string) (options, error) {
	opts := options{
		rangeSpec:   defaultRange,
		evidenceDir: defaultEvidenceDir,
	}
	positionalBeadID := ""
	if (cmd == "show" || cmd == "check-closure" || cmd == "closure" || cmd == "generate-evidence" || cmd == "evidence") && len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalBeadID = strings.TrimSpace(args[0])
		args = args[1:]
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.BoolVar(&opts.strict, "strict", false, "treat warnings as errors")
	fs.BoolVar(&opts.verbose, "v", false, "verbose output")
	fs.BoolVar(&opts.jsonOutput, "json", false, "JSON output when supported")
	fs.StringVar(&opts.rangeSpec, "range", defaultRange, "git commit range")
	fs.StringVar(&opts.beadID, "bead", "", "canonical bead ID (l3d.X)")
	fs.StringVar(&opts.title, "title", "", "PR title (optional)")
	fs.StringVar(&opts.description, "description", "", "PR description (optional)")
	fs.StringVar(&opts.evidenceDir, "evidence-dir", defaultEvidenceDir, "evidence output directory")
	fs.Var(&opts.phaseKV, "phase", "CI phase status as name=pass|fail")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}

	if opts.title == "" {
		opts.title = strings.TrimSpace(os.Getenv("CI_MERGE_REQUEST_TITLE"))
	}
	if opts.description == "" {
		opts.description = strings.TrimSpace(os.Getenv("CI_MERGE_REQUEST_DESCRIPTION"))
	}
	if opts.beadID == "" {
		opts.beadID = positionalBeadID
	}
	if (cmd == "show" || cmd == "check-closure" || cmd == "closure" || cmd == "generate-evidence" || cmd == "evidence") && opts.beadID == "" && fs.NArg() > 0 {
		opts.beadID = strings.TrimSpace(fs.Arg(0))
	}

	if cmd == "generate-evidence" || cmd == "evidence" || cmd == "check-closure" || cmd == "closure" {
		if !isCanonicalBeadID(opts.beadID) {
			return opts, fmt.Errorf("--bead must be canonical l3d.X format")
		}
	}

	return opts, nil
}

func usage() {
	fmt.Println(`beads: bead-driven CI verification

Usage:
  beads <command> [options]

Commands:
  verify             Verify change traceability and test linkage
  verify-commits     Verify every commit subject in range has canonical bead ID
  verify-pr          Verify PR metadata includes bead and governance sections
  list-beads         List canonical bead IDs found in commit range
  show               Show bead issue JSON from bd
  generate-evidence  Generate evidence artifact for a bead
  check-closure      Validate bead closure credibility from evidence

Examples:
  beads verify --range origin/master..HEAD --strict
  beads verify-commits --range HEAD~5..HEAD --strict
  beads verify-pr --title "$CI_MERGE_REQUEST_TITLE" --description "$CI_MERGE_REQUEST_DESCRIPTION" --strict
  beads show l3d.1 --json
  beads generate-evidence --bead l3d.1 --range origin/master..HEAD --phase build=pass --phase test=pass
  beads check-closure --bead l3d.1`)
}

func runShow(opts options) error {
	if !isCanonicalBeadID(opts.beadID) {
		return fmt.Errorf("bead id must be canonical l3d.X format")
	}
	issueID := beadIssueID(opts.beadID)
	out, err := runCmd("bd", "show", issueID, "--json")
	if err != nil {
		return err
	}
	if opts.jsonOutput {
		fmt.Println(strings.TrimSpace(out))
		return nil
	}
	status, statusErr := beadStatusFromBD(issueID)
	if statusErr != nil {
		return statusErr
	}
	fmt.Printf("%s %s\n", issueID, status)
	return nil
}

func runVerify(opts options) error {
	if err := runVerifyCommits(opts); err != nil {
		return err
	}

	commits, err := collectCommits(opts.rangeSpec)
	if err != nil {
		return err
	}

	beads := uniqueBeadsFromCommits(commits)
	if len(beads) == 0 {
		return fmt.Errorf("no canonical bead IDs found in commit range %s", opts.rangeSpec)
	}

	changedFiles, err := changedFiles(opts.rangeSpec)
	if err != nil {
		return err
	}
	goFiles, testFiles := splitGoFiles(changedFiles)

	issues := validateChangedGoFiles(goFiles, testFiles)
	requiredBeads, reqErr := requiredBeadsForTagging(commits)
	if reqErr != nil {
		return reqErr
	}
	issues = append(issues, validateTestBeadTags(requiredBeads, testFiles)...)

	if len(issues) > 0 {
		for _, issue := range issues {
			fmt.Printf("FAIL: %s\n", issue)
		}
		if opts.strict {
			return fmt.Errorf("change verification failed")
		}
	}

	fmt.Printf("PASS: verify range=%s beads=%s changed_go=%d changed_tests=%d\n",
		opts.rangeSpec,
		strings.Join(beads, ","),
		len(goFiles),
		len(testFiles),
	)
	return nil
}

func runVerifyCommits(opts options) error {
	commits, err := collectCommits(opts.rangeSpec)
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		return fmt.Errorf("no commits found in range %s", opts.rangeSpec)
	}

	exemptSHAs := loadExemptSHAs()

	var failures []string
	for _, c := range commits {
		if isExemptCommitSubject(c.Subject) {
			continue
		}
		if isExemptSHA(c.SHA, exemptSHAs) {
			continue
		}
		beads := parseCanonicalBeads(c.Subject)
		malformed := parseMalformedBeads(c.Subject)
		if len(beads) == 0 {
			failures = append(failures, fmt.Sprintf("%s missing canonical bead: %s", shortSHA(c.SHA), c.Subject))
			continue
		}
		if len(malformed) > 0 {
			failures = append(failures, fmt.Sprintf("%s contains malformed bead token(s): %s", shortSHA(c.SHA), strings.Join(malformed, ",")))
		}
	}

	if len(failures) > 0 {
		for _, f := range failures {
			fmt.Printf("FAIL: %s\n", f)
		}
		return fmt.Errorf("commit verification failed for %d commit(s)", len(failures))
	}

	fmt.Printf("PASS: %d commit(s) include canonical bead IDs\n", len(commits))
	return nil
}

func runVerifyPR(opts options) error {
	if strings.TrimSpace(opts.title) == "" {
		return fmt.Errorf("PR title is required")
	}
	if strings.TrimSpace(opts.description) == "" {
		return fmt.Errorf("PR description is required")
	}

	issues := validatePRMetadata(opts.title, opts.description)
	if len(issues) > 0 {
		for _, issue := range issues {
			fmt.Printf("FAIL: %s\n", issue)
		}
		return fmt.Errorf("PR metadata verification failed")
	}

	fmt.Println("PASS: PR metadata includes bead references and required governance sections")
	return nil
}

func runListBeads(opts options) error {
	commits, err := collectCommits(opts.rangeSpec)
	if err != nil {
		return err
	}
	for _, bead := range uniqueBeadsFromCommits(commits) {
		fmt.Println(bead)
	}
	return nil
}

func runGenerateEvidence(opts options) error {
	issueID := beadIssueID(opts.beadID)
	issueStatus, _ := beadStatusFromBD(issueID)

	commits, err := collectCommits(opts.rangeSpec)
	if err != nil {
		return err
	}

	beadCommits := make([]commitInfo, 0)
	for _, c := range commits {
		if contains(parseCanonicalBeads(c.Subject), opts.beadID) || contains(parseCanonicalBeads(c.Subject+"\n"), opts.beadID) {
			beadCommits = append(beadCommits, c)
		}
	}

	files, err := changedFiles(opts.rangeSpec)
	if err != nil {
		return err
	}
	goFiles, testFiles := splitGoFiles(files)

	linked, err := findTaggedTestsForBead(opts.beadID)
	if err != nil {
		return err
	}

	phaseMap := map[string]string{}
	for _, kv := range opts.phaseKV {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid --phase value %q", kv)
		}
		phase := strings.TrimSpace(parts[0])
		status := strings.TrimSpace(parts[1])
		if status != "pass" && status != "fail" && status != "unknown" {
			return fmt.Errorf("phase %s has invalid status %s", phase, status)
		}
		phaseMap[phase] = status
	}

	headSHA, err := runCmd("git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}

	evidence := beadEvidence{
		BeadID:           opts.beadID,
		BeadIssueID:      issueID,
		BeadIssueStatus:  issueStatus,
		GeneratedAtUTC:   time.Now().UTC().Format(time.RFC3339),
		GeneratorVersion: "v2",
		HeadSHA:          strings.TrimSpace(headSHA),
		Range:            opts.rangeSpec,
		Commits:          beadCommits,
		ChangedFiles:     files,
		ChangedGoFiles:   goFiles,
		ChangedTestFiles: testFiles,
		LinkedTests:      linked,
		CIPhases:         phaseMap,
		AcceptanceEvidence: []string{
			"PR acceptance checklist required by template",
			"Bead-tagged tests linked by naming convention",
			"CI phase states recorded at generation time",
		},
	}
	evidence.Summary = evaluateEvidence(evidence)

	if err := os.MkdirAll(opts.evidenceDir, 0o750); err != nil {
		return err
	}
	path := filepath.Join(opts.evidenceDir, opts.beadID+".json")
	body, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return err
	}
	fmt.Printf("PASS: evidence generated %s\n", path)
	return nil
}

func runCheckClosure(opts options) error {
	issueID := beadIssueID(opts.beadID)
	issueStatus, err := beadStatusFromBD(issueID)
	if err != nil {
		return fmt.Errorf("bead not found in bd: %s", issueID)
	}
	if issueStatus == "closed" || issueStatus == "done" {
		return fmt.Errorf("bead already %s in bd: %s", issueStatus, issueID)
	}

	path := filepath.Join(opts.evidenceDir, opts.beadID+".json")
	// #nosec G304 -- path is constrained to configured evidence directory and bead filename.
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read evidence file %s: %w", path, err)
	}

	var evidence beadEvidence
	if err := json.Unmarshal(body, &evidence); err != nil {
		return err
	}

	eval := evaluateEvidence(evidence)
	if !eval.Credible {
		for _, reason := range eval.Reasons {
			fmt.Printf("FAIL: %s\n", reason)
		}
		return fmt.Errorf("bead %s is not eligible for closure", opts.beadID)
	}

	fmt.Printf("PASS: bead %s eligible for closure\n", opts.beadID)
	fmt.Printf("PASS: bd issue eligible for close transition: %s status=%s\n", issueID, issueStatus)
	return nil
}

func validatePRMetadata(title, body string) []string {
	issues := make([]string, 0)
	titleBeads := parseCanonicalBeads(title)
	bodyBeads := parseCanonicalBeads(body)
	if len(titleBeads) == 0 && len(bodyBeads) == 0 {
		issues = append(issues, "PR title or body must contain at least one canonical bead ID (l3d.X)")
	}

	requiredSections := []string{
		"## Bead References",
		"## Acceptance Criteria Mapping",
		"## TDD Evidence",
		"## Test Evidence",
		"## Evidence Artifacts",
	}
	for _, section := range requiredSections {
		if !strings.Contains(body, section) {
			issues = append(issues, fmt.Sprintf("missing PR section %q", section))
		}
	}

	if !strings.Contains(body, "- [x] I added or updated regression tests") {
		issues = append(issues, "TDD checkbox for regression tests must be checked")
	}
	if !strings.Contains(body, "- [x] I wrote a failing test first") {
		issues = append(issues, "TDD checkbox for failing test first must be checked")
	}

	if malformed := parseMalformedBeads(title + "\n" + body); len(malformed) > 0 {
		issues = append(issues, "malformed bead tokens detected: "+strings.Join(malformed, ","))
	}

	return issues
}

func validateChangedGoFiles(changedGoFiles, changedTestFiles []string) []string {
	issues := make([]string, 0)
	if len(changedGoFiles) == 0 {
		return issues
	}
	if len(changedTestFiles) == 0 {
		issues = append(issues, "changed Go source files detected but no changed *_test.go files")
	}
	for _, gf := range changedGoFiles {
		// Skip files that were deleted in the range; they no longer need a sibling.
		if _, err := os.Stat(gf); err != nil {
			continue
		}
		expected := strings.TrimSuffix(gf, ".go") + "_test.go"
		if _, err := os.Stat(expected); err != nil {
			issues = append(issues, fmt.Sprintf("missing sibling test file for %s (expected %s)", gf, expected))
		}
	}
	return issues
}

func validateTestBeadTags(beads, changedTestFiles []string) []string {
	issues := make([]string, 0)
	if len(beads) == 0 || len(changedTestFiles) == 0 {
		return issues
	}
	tagged := map[string]bool{}
	for _, f := range changedTestFiles {
		// Skip files that were deleted in the range; they carry no bead tags.
		if _, err := os.Stat(f); err != nil {
			continue
		}
		found, err := parseBeadTagsFromTestFile(f)
		if err != nil {
			issues = append(issues, fmt.Sprintf("failed reading test file %s: %v", f, err))
			continue
		}
		for _, bead := range found {
			tagged[bead] = true
		}
	}
	for _, bead := range beads {
		if !tagged[bead] {
			issues = append(issues, fmt.Sprintf("no changed tests tagged for bead %s using name suffix %s", bead, testTagForBead(bead)))
		}
	}
	return issues
}

func requiredBeadsForTagging(commits []commitInfo) ([]string, error) {
	beadSet := map[string]struct{}{}
	for _, commit := range commits {
		files, err := changedFilesForCommitFn(commit.SHA)
		if err != nil {
			return nil, err
		}
		requiresTag := false
		for _, file := range files {
			if goPackageFileRE.MatchString(file) && !goTestFileSuffixRE.MatchString(file) {
				requiresTag = true
				break
			}
		}
		if !requiresTag {
			continue
		}
		for _, bead := range commit.Beads {
			beadSet[bead] = struct{}{}
		}
	}
	beads := make([]string, 0, len(beadSet))
	for bead := range beadSet {
		beads = append(beads, bead)
	}
	sort.Strings(beads)
	return beads, nil
}

func evaluateEvidence(e beadEvidence) evidenceSummary {
	var reasons []string
	if len(e.Commits) == 0 {
		reasons = append(reasons, "no commits linked to bead in configured range")
	}
	if len(e.ChangedFiles) == 0 {
		reasons = append(reasons, "no changed files captured for bead range")
	}
	if len(e.LinkedTests) == 0 {
		reasons = append(reasons, "no bead-tagged tests found for bead")
	}

	requiredPhases := []string{"format", "lint", "test", "build", "bead-verify", "security"}
	for _, phase := range requiredPhases {
		status, ok := e.CIPhases[phase]
		if !ok {
			reasons = append(reasons, fmt.Sprintf("required CI phase %s missing", phase))
			continue
		}
		if status != "pass" {
			reasons = append(reasons, fmt.Sprintf("CI phase %s status=%s", phase, status))
		}
	}

	return evidenceSummary{Credible: len(reasons) == 0, Reasons: reasons}
}

func parseCanonicalBeads(input string) []string {
	matches := canonicalBeadRE.FindAllString(strings.ToLower(input), -1)
	return dedupeSorted(matches)
}

func parseMalformedBeads(input string) []string {
	matches := malformedBeadRE.FindAllString(strings.ToLower(input), -1)
	return dedupeSorted(matches)
}

func isExemptCommitSubject(subject string) bool {
	trimmed := strings.TrimSpace(subject)
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(trimmed, "Merge ") ||
		strings.HasPrefix(lower, "merge: ") ||
		strings.HasPrefix(lower, "merge(")
}

// loadExemptSHAs reads .bead-exempt from the repo root. Each non-empty,
// non-comment line is a SHA prefix (min 7 chars) that bead-verify will skip.
func loadExemptSHAs() []string {
	data, err := os.ReadFile(".bead-exempt")
	if err != nil {
		return nil
	}
	var out []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) >= 7 {
			out = append(out, strings.ToLower(line))
		}
	}
	return out
}

func isExemptSHA(sha string, exemptSHAs []string) bool {
	lower := strings.ToLower(sha)
	for _, prefix := range exemptSHAs {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func isCanonicalBeadID(id string) bool {
	return canonicalBeadRE.MatchString(strings.ToLower(id))
}

func testTagForBead(bead string) string {
	return strings.ReplaceAll("Bead_"+bead, ".", "_")
}

func parseBeadTagsFromTestFile(path string) ([]string, error) {
	// #nosec G304 -- path comes from git-tracked changed test files within repository root.
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	found := make([]string, 0)
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		matches := testNameWithTagRE.FindStringSubmatch(line)
		if len(matches) < 2 {
			continue
		}
		testName := matches[1]
		idx := strings.Index(testName, testTagPrefix)
		if idx < 0 {
			continue
		}
		tag := strings.TrimPrefix(testName[idx:], testTagPrefix)
		bead := "l3d." + strings.ReplaceAll(tag, "_", ".")
		found = append(found, bead)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return dedupeSorted(found), nil
}

func findTaggedTestsForBead(bead string) ([]linkedTest, error) {
	tests := make([]linkedTest, 0)
	wantedTag := testTagForBead(bead)
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == ".worktrees" {
				return filepath.SkipDir
			}
			return nil
		}
		if !goTestFileSuffixRE.MatchString(path) {
			return nil
		}
		// #nosec G304,G122 -- walk scope is repository root and reads are non-mutating for test discovery.
		body, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		lineNum := 0
		scanner := bufio.NewScanner(bytes.NewReader(body))
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			match := testNameWithTagRE.FindStringSubmatch(line)
			if len(match) < 2 {
				continue
			}
			if !strings.Contains(match[1], wantedTag) {
				continue
			}
			tests = append(tests, linkedTest{File: path, Line: lineNum, TestName: match[1], Beads: []string{bead}})
		}
		return scanner.Err()
	})
	if err != nil {
		return nil, err
	}
	return tests, nil
}

func collectCommits(rangeSpec string) ([]commitInfo, error) {
	out, err := runCmd("git", "log", "--format=%H%x1f%s", rangeSpec)
	if err != nil {
		return nil, fmt.Errorf("git log failed for range %s: %w", rangeSpec, err)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	rows := strings.Split(out, "\n")
	commits := make([]commitInfo, 0, len(rows))
	for _, row := range rows {
		parts := strings.SplitN(row, "\x1f", 2)
		if len(parts) != 2 {
			continue
		}
		commits = append(commits, commitInfo{
			SHA:     parts[0],
			Subject: parts[1],
			Beads:   parseCanonicalBeads(parts[1]),
		})
	}
	return commits, nil
}

func changedFilesForCommit(sha string) ([]string, error) {
	out, err := runCmd("git", "show", "--name-only", "--format=", sha)
	if err != nil {
		return nil, fmt.Errorf("git show failed for commit %s: %w", sha, err)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	rows := strings.Split(out, "\n")
	files := make([]string, 0, len(rows))
	for _, row := range rows {
		trimmed := strings.TrimSpace(row)
		if trimmed == "" {
			continue
		}
		files = append(files, trimmed)
	}
	return files, nil
}

func uniqueBeadsFromCommits(commits []commitInfo) []string {
	all := make([]string, 0)
	for _, c := range commits {
		all = append(all, c.Beads...)
	}
	return dedupeSorted(all)
}

func changedFiles(rangeSpec string) ([]string, error) {
	out, err := runCmd("git", "diff", "--name-only", rangeSpec)
	if err != nil {
		return nil, fmt.Errorf("git diff failed for range %s: %w", rangeSpec, err)
	}
	if strings.TrimSpace(out) == "" {
		return []string{}, nil
	}
	files := strings.Split(strings.TrimSpace(out), "\n")
	return dedupeSorted(files), nil
}

func splitGoFiles(files []string) ([]string, []string) {
	goFiles := make([]string, 0)
	testFiles := make([]string, 0)
	for _, f := range files {
		if !goPackageFileRE.MatchString(f) {
			continue
		}
		if goTestFileSuffixRE.MatchString(f) {
			testFiles = append(testFiles, f)
			continue
		}
		goFiles = append(goFiles, f)
	}
	return dedupeSorted(goFiles), dedupeSorted(testFiles)
}

func runCmd(name string, args ...string) (string, error) {
	if name != "git" && name != "bd" {
		return "", fmt.Errorf("unsupported command: %s", name)
	}
	// #nosec G204 -- command binary is allowlisted to git/bd and args are explicit call sites.
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.String(), errors.New(strings.TrimSpace(out.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

func dedupeSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func beadIssueID(bead string) string {
	repoName := "SyndicateCode"
	if remoteURL, err := runCmd("git", "remote", "get-url", "origin"); err == nil {
		remoteURL = strings.TrimSpace(remoteURL)
		if remoteURL != "" {
			candidate := strings.TrimSuffix(filepath.Base(remoteURL), ".git")
			if candidate != "" {
				repoName = candidate
			}
		}
	} else if root, rootErr := runCmd("git", "rev-parse", "--show-toplevel"); rootErr == nil {
		root = strings.TrimSpace(root)
		if root != "" {
			repoName = filepath.Base(root)
		}
	}

	if strings.EqualFold(repoName, "syndicatecode") {
		repoName = "SyndicateCode"
	}
	return fmt.Sprintf("%s-%s", repoName, bead)
}

func beadStatusFromBD(issueID string) (string, error) {
	out, err := runCmd("bd", "show", issueID, "--json")
	if err != nil {
		return "", err
	}
	var records []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &records); err != nil {
		return "", err
	}
	if len(records) == 0 {
		return "", fmt.Errorf("issue not found")
	}
	status, _ := records[0]["status"].(string)
	if strings.TrimSpace(status) == "" {
		return "", fmt.Errorf("issue status missing")
	}
	return status, nil
}

func exitErr(err error) {
	fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
	os.Exit(1)
}
