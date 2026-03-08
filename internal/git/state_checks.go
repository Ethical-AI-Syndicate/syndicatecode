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
	HasStagedChanges   bool
	HasUnstagedChanges bool
	HasUntrackedFiles  bool
	HasMergeMarkers    bool
	ModifiedFilePaths  []string
}

type PreEditRequest struct {
	RepoPath      string
	RequireClean  bool
	AllowBranches []string
	TargetFiles   []string
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
