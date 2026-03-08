package context

import "testing"

func TestDefaultCategoryBudgetIncludesCompletionReserve(t *testing.T) {
	t.Parallel()

	budget := DefaultCategoryBudget(4000)
	if budget.CompletionReserve <= 0 {
		t.Fatalf("expected completion reserve to be allocated")
	}
	if budget.CompletionReserve != 600 {
		t.Fatalf("expected deterministic reserve of 600, got %d", budget.CompletionReserve)
	}
}

func TestAllocateFragmentsPreservesCompletionReserve(t *testing.T) {
	t.Parallel()

	budget := DefaultCategoryBudget(2000)
	allocator := NewBudgetAllocator(budget)

	fragments := []RankedFragment{
		{Fragment: ContextFragment{SourceType: "evidence", SourceRef: "a", Content: "A", TokenCount: 900}, Rank: 100},
		{Fragment: ContextFragment{SourceType: "history", SourceRef: "b", Content: "B", TokenCount: 700}, Rank: 90},
		{Fragment: ContextFragment{SourceType: "task", SourceRef: "c", Content: "C", TokenCount: 300}, Rank: 80},
	}

	selected, summary := allocator.AllocateFragments(fragments)
	if summary.RemainingForCompletion < budget.CompletionReserve {
		t.Fatalf("expected completion reserve protection, remaining=%d reserve=%d", summary.RemainingForCompletion, budget.CompletionReserve)
	}
	if len(selected) == len(fragments) {
		t.Fatalf("expected deterministic truncation under budget pressure")
	}
}

func TestAllocateFragmentsIsDeterministic(t *testing.T) {
	t.Parallel()

	budget := DefaultCategoryBudget(3000)
	allocator := NewBudgetAllocator(budget)
	fragments := []RankedFragment{
		{Fragment: ContextFragment{SourceType: "evidence", SourceRef: "f1", Content: "1", TokenCount: 600}, Rank: 50},
		{Fragment: ContextFragment{SourceType: "evidence", SourceRef: "f2", Content: "2", TokenCount: 600}, Rank: 40},
		{Fragment: ContextFragment{SourceType: "history", SourceRef: "f3", Content: "3", TokenCount: 600}, Rank: 30},
	}

	first, firstSummary := allocator.AllocateFragments(fragments)
	second, secondSummary := allocator.AllocateFragments(fragments)

	if len(first) != len(second) {
		t.Fatalf("expected deterministic selection length")
	}
	for i := range first {
		if first[i].SourceRef != second[i].SourceRef {
			t.Fatalf("expected deterministic ordering and selection")
		}
	}
	if firstSummary.TotalAllocatedTokens != secondSummary.TotalAllocatedTokens {
		t.Fatalf("expected deterministic allocation totals")
	}
}
