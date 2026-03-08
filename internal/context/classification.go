package context

import "strings"

type TaskClass string

const (
	TaskClassAnalysis TaskClass = "analysis"
	TaskClassEdit     TaskClass = "edit"
	TaskClassTest     TaskClass = "test"
	TaskClassDebug    TaskClass = "debug"
)

type RetrievalRadius string

const (
	RetrievalRadiusTargeted RetrievalRadius = "targeted"
	RetrievalRadiusSession  RetrievalRadius = "session"
	RetrievalRadiusRepoWide RetrievalRadius = "repo_wide"
)

type RetrievalProfile struct {
	RetrievalRadius  RetrievalRadius
	SourcePriorities []string
	TokenBudget      map[string]int
}

func ClassifyTaskClass(message string) TaskClass {
	normalized := strings.ToLower(strings.TrimSpace(message))

	if containsAny(normalized, []string{"fix", "update", "refactor", "implement", "add "}) {
		return TaskClassEdit
	}
	if containsAny(normalized, []string{"test", "spec", "coverage", "assert"}) {
		return TaskClassTest
	}
	if containsAny(normalized, []string{"debug", "panic", "error", "trace", "failing"}) {
		return TaskClassDebug
	}

	return TaskClassAnalysis
}

func RetrievalProfileForTaskClass(class TaskClass) RetrievalProfile {
	baseBudget := map[string]int{
		"control":            400,
		"task":               700,
		"instructions":       600,
		"evidence":           900,
		"history":            800,
		"completion_reserve": 600,
	}

	switch class {
	case TaskClassEdit:
		baseBudget["evidence"] = 1200
		return RetrievalProfile{
			RetrievalRadius:  RetrievalRadiusRepoWide,
			SourcePriorities: []string{"explicit_targets", "diagnostics", "symbol_refs", "repo_search", "history"},
			TokenBudget:      baseBudget,
		}
	case TaskClassTest:
		baseBudget["evidence"] = 1100
		baseBudget["history"] = 600
		return RetrievalProfile{
			RetrievalRadius:  RetrievalRadiusSession,
			SourcePriorities: []string{"test_files", "diagnostics", "explicit_targets", "repo_search", "history"},
			TokenBudget:      baseBudget,
		}
	case TaskClassDebug:
		baseBudget["evidence"] = 1300
		baseBudget["history"] = 700
		return RetrievalProfile{
			RetrievalRadius:  RetrievalRadiusRepoWide,
			SourcePriorities: []string{"diagnostics", "stack_traces", "explicit_targets", "repo_search", "history"},
			TokenBudget:      baseBudget,
		}
	case TaskClassAnalysis:
		return RetrievalProfile{
			RetrievalRadius:  RetrievalRadiusSession,
			SourcePriorities: []string{"instructions", "explicit_targets", "history", "repo_search"},
			TokenBudget:      baseBudget,
		}
	default:
		return RetrievalProfile{
			RetrievalRadius:  RetrievalRadiusTargeted,
			SourcePriorities: []string{"instructions", "explicit_targets", "history"},
			TokenBudget:      baseBudget,
		}
	}
}

func containsAny(value string, parts []string) bool {
	for _, part := range parts {
		if strings.Contains(value, part) {
			return true
		}
	}
	return false
}
