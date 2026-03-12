package main

import "testing"

func TestGoNoGoReportAutomation_Bead_l3d_13_4(t *testing.T) {
	t.Run("per criterion evidence links", TestBuildReportProducesPerCriterionEvidenceLinks)
	t.Run("report fails when verification not ready", TestBuildReportFailsWhenVerificationNotReady)
}
