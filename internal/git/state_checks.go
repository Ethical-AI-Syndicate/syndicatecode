package git

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

var ErrUnsafeGitState = errors.New("unsafe git state for edit")

type RepoState struct {
	Branch             string
	IsWorktree         bool
	HasStagedChanges   bool
	HasUnstagedChanges bool
	HasUntrackedFiles  bool
	HasMergeMarkers    bool
	ModifiedFilePaths  []string
}

type PreEditRequest struct {
	RepoPath             string
	RequireClean         bool
	AllowBranches        []string
	TargetFiles          []string
	TrustTier            string
	ProtectedBranches    []string
	RequireWorktreeFor   []string
	RequireTaskBranchFor []string
	TaskBranchPrefixes   []string
}

type StateInspector interface {
	Inspect(ctx context.Context, repoPath string) (RepoState, error)
}

type PreEditChecker struct {
	inspector StateInspector
}

func NewPreEditChecker(inspector StateInspector) *PreEditChecker {
	return &PreEditChecker{inspector: inspector}
}

func (c *PreEditChecker) Validate(ctx context.Context, req PreEditRequest) error {
	state, err := c.inspector.Inspect(ctx, req.RepoPath)
	if err != nil {
		return fmt.Errorf("failed to inspect git state: %w", err)
	}

	if len(req.AllowBranches) > 0 && !slices.Contains(req.AllowBranches, state.Branch) {
		return fmt.Errorf("%w: branch %s is not allowed", ErrUnsafeGitState, state.Branch)
	}

	if slices.Contains(req.ProtectedBranches, state.Branch) && (req.TrustTier == "tier2" || req.TrustTier == "tier3") {
		return fmt.Errorf("%w: protected branch %s is not allowed for trust tier %s", ErrUnsafeGitState, state.Branch, req.TrustTier)
	}

	if slices.Contains(req.RequireWorktreeFor, req.TrustTier) && !state.IsWorktree {
		return fmt.Errorf("%w: trust tier %s requires worktree mode", ErrUnsafeGitState, req.TrustTier)
	}

	if slices.Contains(req.RequireTaskBranchFor, req.TrustTier) && !branchMatchesAnyPrefix(state.Branch, req.TaskBranchPrefixes) {
		return fmt.Errorf("%w: trust tier %s requires task branch prefix", ErrUnsafeGitState, req.TrustTier)
	}

	if req.RequireClean && (state.HasStagedChanges || state.HasUnstagedChanges || state.HasUntrackedFiles) {
		return fmt.Errorf("%w: working tree is not clean", ErrUnsafeGitState)
	}

	if state.HasMergeMarkers {
		return fmt.Errorf("%w: merge conflict markers present", ErrUnsafeGitState)
	}

	for _, target := range req.TargetFiles {
		if slices.Contains(state.ModifiedFilePaths, target) {
			return fmt.Errorf("%w: target file already modified: %s", ErrUnsafeGitState, target)
		}
	}

	return nil
}

func branchMatchesAnyPrefix(branch string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return false
	}

	for _, prefix := range prefixes {
		if prefix != "" && len(branch) >= len(prefix) && branch[:len(prefix)] == prefix {
			return true
		}
	}

	return false
}
