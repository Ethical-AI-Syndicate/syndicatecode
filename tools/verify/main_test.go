package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRunVerification_JSONContract_Bead_l3d_10_4(t *testing.T) {
	cfg := Config{
		Mode:             "json",
		Range:            "origin/master..HEAD",
		Bead:             "l3d.10.4",
		GenerateEvidence: true,
		Title:            "feat: add verifier [l3d.10.4]",
		Description:      "## Bead References\n- l3d.10.4",
	}

	runner := &fakeRunner{results: map[string]CommandResult{
		"git rev-parse --is-inside-work-tree":        {ExitCode: 0, Stdout: "true"},
		"git branch --show-current":                  {ExitCode: 0, Stdout: "feature/l3d-10-4"},
		"git remote show origin":                     {ExitCode: 0, Stdout: "HEAD branch: master"},
		"git status --short":                         {ExitCode: 0, Stdout: ""},
		"./.gitlab/scripts/check-gofmt.sh":           {ExitCode: 0, Stdout: "gofmt check passed"},
		"golangci-lint run":                          {ExitCode: 0, Stdout: "0 issues."},
		"go test -race ./...":                        {ExitCode: 0, Stdout: "ok"},
		"go build -o bin/ ./cmd/... && go vet ./...": {ExitCode: 0, Stdout: ""},
		"go run ./tools/beads verify-commits --range origin/master..HEAD --strict":                                                   {ExitCode: 0, Stdout: "PASS"},
		"go run ./tools/beads verify --range origin/master..HEAD --strict":                                                           {ExitCode: 0, Stdout: "PASS"},
		"go run ./tools/beads verify-pr --strict --title " + shellQuote(cfg.Title) + " --description " + shellQuote(cfg.Description): {ExitCode: 0, Stdout: "PASS"},
		"gosec ./...": {ExitCode: 127, Stderr: "gosec: command not found"},
		"go run ./tools/beads generate-evidence --bead l3d.10.4 --range origin/master..HEAD --phase format=pass --phase lint=pass --phase test=pass --phase build=pass --phase bead-verify=pass --phase metadata-verify=pass --phase security=skipped": {
			ExitCode: 0,
			Stdout:   "PASS: evidence generated bead-evidence/l3d.10.4.json",
		},
	}}

	res := RunVerification(cfg, runner)
	if res.SchemaVersion != "1" {
		t.Fatalf("expected schema version 1, got %q", res.SchemaVersion)
	}
	if len(res.Phases) == 0 {
		t.Fatal("expected phases in result")
	}
	if res.OK {
		t.Fatal("expected overall failure because security is skipped")
	}

	data, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\"phases\"") {
		t.Fatalf("expected JSON phases key, got %s", string(data))
	}
}

func TestRunVerification_MetadataStrictFailure_Bead_l3d_10_4(t *testing.T) {
	cfg := Config{Range: "origin/master..HEAD", StrictMetadata: true}
	runner := &fakeRunner{results: map[string]CommandResult{
		"git rev-parse --is-inside-work-tree":        {ExitCode: 0, Stdout: "true"},
		"git branch --show-current":                  {ExitCode: 0, Stdout: "feature"},
		"git remote show origin":                     {ExitCode: 0, Stdout: "HEAD branch: master"},
		"git status --short":                         {ExitCode: 0, Stdout: ""},
		"./.gitlab/scripts/check-gofmt.sh":           {ExitCode: 0, Stdout: "ok"},
		"golangci-lint run":                          {ExitCode: 0, Stdout: "ok"},
		"go test -race ./...":                        {ExitCode: 0, Stdout: "ok"},
		"go build -o bin/ ./cmd/... && go vet ./...": {ExitCode: 0, Stdout: "ok"},
		"go run ./tools/beads verify-commits --range origin/master..HEAD --strict": {ExitCode: 0, Stdout: "ok"},
		"go run ./tools/beads verify --range origin/master..HEAD --strict":         {ExitCode: 0, Stdout: "ok"},
		"gosec ./...": {ExitCode: 0, Stdout: "ok"},
	}}

	res := RunVerification(cfg, runner)
	phase := findPhase(res.Phases, "metadata-verify")
	if phase.Status != "fail" {
		t.Fatalf("expected metadata phase fail, got %s", phase.Status)
	}
	if res.OK {
		t.Fatal("expected overall failure when strict metadata enabled")
	}
}

func TestRunVerification_AllRequiredPhasesPass_Bead_l3d_10_4(t *testing.T) {
	cfg := Config{Range: "origin/master..HEAD", Title: "feat [l3d.10.4]", Description: "## Bead References\n- l3d.10.4"}
	runner := &fakeRunner{results: map[string]CommandResult{
		"git rev-parse --show-toplevel":              {ExitCode: 0, Stdout: "/repo"},
		"git branch --show-current":                  {ExitCode: 0, Stdout: "feature/l3d-10-4"},
		"git remote show origin":                     {ExitCode: 0, Stdout: "HEAD branch: master"},
		"git status --short":                         {ExitCode: 0, Stdout: ""},
		"./.gitlab/scripts/check-gofmt.sh":           {ExitCode: 0, Stdout: "ok"},
		"golangci-lint run":                          {ExitCode: 0, Stdout: "ok"},
		"go test -race ./...":                        {ExitCode: 0, Stdout: "ok"},
		"go build -o bin/ ./cmd/... && go vet ./...": {ExitCode: 0, Stdout: "ok"},
		"go run ./tools/beads verify-commits --range origin/master..HEAD --strict":                                                   {ExitCode: 0, Stdout: "ok"},
		"go run ./tools/beads verify --range origin/master..HEAD --strict":                                                           {ExitCode: 0, Stdout: "ok"},
		"go run ./tools/beads verify-pr --strict --title " + shellQuote(cfg.Title) + " --description " + shellQuote(cfg.Description): {ExitCode: 0, Stdout: "ok"},
		"gosec ./...": {ExitCode: 0, Stdout: "ok"},
	}}

	res := RunVerification(cfg, runner)
	if !res.OK {
		t.Fatalf("expected overall pass, got errors=%v", res.Errors)
	}
	if !res.ReadyForPush {
		t.Fatal("expected ready for push")
	}
}

func findPhase(phases []PhaseResult, name string) PhaseResult {
	for _, p := range phases {
		if p.Name == name {
			return p
		}
	}
	return PhaseResult{}
}

type fakeRunner struct {
	results map[string]CommandResult
}

func (f *fakeRunner) Run(command string) CommandResult {
	if res, ok := f.results[command]; ok {
		return res
	}
	return CommandResult{ExitCode: 0, Stdout: "ok"}
}
