package context

import "sort"

type FileMatch struct {
	Path       string
	MatchScore int
}

type SymbolMatch struct {
	Path       string
	Symbol     string
	MatchScore int
}

type Diagnostic struct {
	Path     string
	Severity string
	Message  string
}

type GitSignal struct {
	Path            string
	RecentlyChanged bool
	ChurnScore      int
}

type RetrievalInput struct {
	ExplicitTargets []string
	FileMatches     []FileMatch
	SymbolMatches   []SymbolMatch
	Diagnostics     []Diagnostic
	GitSignals      []GitSignal
}

type RetrievalCandidate struct {
	Path    string
	Score   int
	Reasons []string
}

func BuildRetrievalCandidates(input RetrievalInput) []RetrievalCandidate {
	type aggregate struct {
		score   int
		reasons map[string]struct{}
	}
	byPath := make(map[string]*aggregate)

	ensure := func(path string) *aggregate {
		entry, ok := byPath[path]
		if ok {
			return entry
		}
		entry = &aggregate{reasons: make(map[string]struct{})}
		byPath[path] = entry
		return entry
	}

	for _, path := range input.ExplicitTargets {
		entry := ensure(path)
		entry.score += 1000
		entry.reasons["explicit_target"] = struct{}{}
	}

	for _, match := range input.FileMatches {
		entry := ensure(match.Path)
		entry.score += match.MatchScore
		entry.reasons["filename_match"] = struct{}{}
	}

	for _, match := range input.SymbolMatches {
		entry := ensure(match.Path)
		entry.score += match.MatchScore
		entry.reasons["symbol_match"] = struct{}{}
	}

	for _, d := range input.Diagnostics {
		entry := ensure(d.Path)
		severityBoost := 200
		if d.Severity == "error" {
			severityBoost = 400
		}
		entry.score += severityBoost
		entry.reasons["diagnostic"] = struct{}{}
	}

	for _, g := range input.GitSignals {
		entry := ensure(g.Path)
		entry.score += g.ChurnScore / 4
		if g.RecentlyChanged {
			entry.score += 50
			entry.reasons["recent_change"] = struct{}{}
		}
		entry.reasons["git_signal"] = struct{}{}
	}

	result := make([]RetrievalCandidate, 0, len(byPath))
	for path, entry := range byPath {
		reasons := make([]string, 0, len(entry.reasons))
		for reason := range entry.reasons {
			reasons = append(reasons, reason)
		}
		sort.Strings(reasons)
		result = append(result, RetrievalCandidate{Path: path, Score: entry.score, Reasons: reasons})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Score == result[j].Score {
			return result[i].Path < result[j].Path
		}
		return result[i].Score > result[j].Score
	})

	return result
}
