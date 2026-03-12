package main

import "testing"

func TestBuildReportProducesPerCriterionEvidenceLinks(t *testing.T) {
	verify := VerificationResult{
		OK:                  true,
		ReadyForReview:      true,
		LocalCIEquivalentOK: true,
		ClosureEligible:     true,
		Bead:                "l3d.13.4",
		Range:               "origin/master..HEAD",
		Artifacts: []Artifact{
			{Type: "bead-evidence", Path: "bead-evidence/l3d.13.4.json"},
		},
	}

	report := buildReport(verify, []string{"bead-evidence/l3d.13.4.json"})
	if report.OverallStatus != "pass" {
		t.Fatalf("expected pass status, got %s", report.OverallStatus)
	}
	if len(report.Criteria) < 3 {
		t.Fatalf("expected multiple criteria, got %d", len(report.Criteria))
	}
	for _, criterion := range report.Criteria {
		if len(criterion.EvidenceLinks) == 0 {
			t.Fatalf("criterion %s missing evidence links", criterion.ID)
		}
	}
}

func TestBuildReportFailsWhenVerificationNotReady(t *testing.T) {
	verify := VerificationResult{OK: false, ReadyForReview: false, LocalCIEquivalentOK: false}
	report := buildReport(verify, nil)
	if report.OverallStatus != "fail" {
		t.Fatalf("expected fail status, got %s", report.OverallStatus)
	}
}
