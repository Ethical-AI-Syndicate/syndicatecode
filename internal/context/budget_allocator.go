package context

import "sort"

type RankedFragment struct {
	Fragment ContextFragment
	Rank     int
}

type CategoryBudget struct {
	Total             int
	Control           int
	Task              int
	Instructions      int
	Evidence          int
	History           int
	CompletionReserve int
}

type AllocationSummary struct {
	TotalAllocatedTokens   int
	RemainingForCompletion int
}

func DefaultCategoryBudget(total int) CategoryBudget {
	reserve := 600
	if total < 1200 {
		reserve = total / 2
	}
	remaining := total - reserve
	if remaining < 0 {
		remaining = 0
	}

	return CategoryBudget{
		Total:             total,
		Control:           remaining * 10 / 100,
		Task:              remaining * 15 / 100,
		Instructions:      remaining * 15 / 100,
		Evidence:          remaining * 35 / 100,
		History:           remaining * 25 / 100,
		CompletionReserve: reserve,
	}
}

type BudgetAllocator struct {
	budget CategoryBudget
}

func NewBudgetAllocator(budget CategoryBudget) *BudgetAllocator {
	return &BudgetAllocator{budget: budget}
}

func (a *BudgetAllocator) AllocateFragments(fragments []RankedFragment) ([]ContextFragment, AllocationSummary) {
	ordered := append([]RankedFragment{}, fragments...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Rank == ordered[j].Rank {
			return ordered[i].Fragment.SourceRef < ordered[j].Fragment.SourceRef
		}
		return ordered[i].Rank > ordered[j].Rank
	})

	maxPromptBudget := a.budget.Total - a.budget.CompletionReserve
	if maxPromptBudget < 0 {
		maxPromptBudget = 0
	}

	usedByCategory := map[string]int{}
	totalUsed := 0
	selected := make([]ContextFragment, 0)

	for _, ranked := range ordered {
		fragment := ranked.Fragment
		category := normalizeCategory(fragment.SourceType)
		catLimit := a.categoryLimit(category)
		remainingOverall := maxPromptBudget - totalUsed
		remainingCategory := catLimit - usedByCategory[category]

		if remainingOverall <= 0 || remainingCategory <= 0 {
			continue
		}

		allowed := remainingOverall
		if remainingCategory < allowed {
			allowed = remainingCategory
		}

		if fragment.TokenCount > allowed {
			fragment.TokenCount = allowed
			fragment.Truncated = true
		}

		if fragment.TokenCount <= 0 {
			continue
		}

		selected = append(selected, fragment)
		totalUsed += fragment.TokenCount
		usedByCategory[category] += fragment.TokenCount
	}

	return selected, AllocationSummary{
		TotalAllocatedTokens:   totalUsed,
		RemainingForCompletion: a.budget.Total - totalUsed,
	}
}

func (a *BudgetAllocator) categoryLimit(category string) int {
	switch category {
	case "control":
		return a.budget.Control
	case "task":
		return a.budget.Task
	case "instructions":
		return a.budget.Instructions
	case "evidence":
		return a.budget.Evidence
	case "history":
		return a.budget.History
	default:
		return a.budget.Evidence
	}
}

func normalizeCategory(sourceType string) string {
	switch sourceType {
	case "instruction":
		return "instructions"
	case "system":
		return "control"
	case "user":
		return "task"
	case "history":
		return "history"
	default:
		return "evidence"
	}
}
