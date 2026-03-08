package context

import "testing"

func TestBuildRetrievalCandidatesPrioritizesExplicitTargetsAndDiagnostics(t *testing.T) {
	t.Parallel()

	input := RetrievalInput{
		ExplicitTargets: []string{"internal/controlplane/server.go"},
		FileMatches: []FileMatch{
			{Path: "internal/context/context.go", MatchScore: 60},
			{Path: "internal/controlplane/server.go", MatchScore: 60},
		},
		SymbolMatches: []SymbolMatch{{Path: "internal/session/manager.go", Symbol: "Create", MatchScore: 70}},
		Diagnostics:   []Diagnostic{{Path: "internal/controlplane/server.go", Severity: "error", Message: "nil pointer"}},
		GitSignals:    []GitSignal{{Path: "internal/context/context.go", RecentlyChanged: true, ChurnScore: 80}},
	}

	candidates := BuildRetrievalCandidates(input)
	if len(candidates) == 0 {
		t.Fatalf("expected candidates")
	}
	if candidates[0].Path != "internal/controlplane/server.go" {
		t.Fatalf("expected explicit target with diagnostics to rank first, got %s", candidates[0].Path)
	}
}

func TestBuildRetrievalCandidatesIsDeterministicForEqualScores(t *testing.T) {
	t.Parallel()

	input := RetrievalInput{
		FileMatches: []FileMatch{
			{Path: "b/file.go", MatchScore: 50},
			{Path: "a/file.go", MatchScore: 50},
		},
	}

	first := BuildRetrievalCandidates(input)
	second := BuildRetrievalCandidates(input)
	if len(first) != len(second) {
		t.Fatalf("expected deterministic length")
	}
	for i := range first {
		if first[i].Path != second[i].Path {
			t.Fatalf("expected deterministic ordering")
		}
	}
}

func TestBuildRetrievalCandidatesIncludesReasonTags(t *testing.T) {
	t.Parallel()

	input := RetrievalInput{
		ExplicitTargets: []string{"internal/tools/capability.go"},
		Diagnostics:     []Diagnostic{{Path: "internal/tools/capability.go", Severity: "warning", Message: "unused"}},
	}

	candidates := BuildRetrievalCandidates(input)
	if len(candidates) != 1 {
		t.Fatalf("expected one candidate, got %d", len(candidates))
	}
	if len(candidates[0].Reasons) < 2 {
		t.Fatalf("expected multiple reason tags for provenance")
	}
}
