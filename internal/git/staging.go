package git

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
)

var ErrMixedPurposeStaging = errors.New("mixed-purpose staging detected")

type WorkingTreeStatus struct {
	Staged    []string
	Unstaged  []string
	Untracked []string
}

type Stager interface {
	Status(ctx context.Context, repoPath string) (WorkingTreeStatus, error)
	AddPaths(ctx context.Context, repoPath string, paths []string) error
}

type StageRequest struct {
	RepoPath       string
	TaskID         string
	ApprovedPaths  []string
	ProtectedPaths []string
}

type TaskScopedStagingManager struct {
	stager Stager
}

func NewTaskScopedStagingManager(stager Stager) *TaskScopedStagingManager {
	return &TaskScopedStagingManager{stager: stager}
}

func (m *TaskScopedStagingManager) StageTaskChanges(ctx context.Context, req StageRequest) error {
	status, err := m.stager.Status(ctx, req.RepoPath)
	if err != nil {
		return fmt.Errorf("failed to inspect working tree: %w", err)
	}

	approvedSet := make(map[string]struct{}, len(req.ApprovedPaths))
	for _, path := range req.ApprovedPaths {
		approvedSet[path] = struct{}{}
	}

	protectedSet := make(map[string]struct{}, len(req.ProtectedPaths))
	for _, path := range req.ProtectedPaths {
		protectedSet[path] = struct{}{}
	}

	candidatePaths := uniqueSorted(append(append([]string{}, status.Unstaged...), status.Untracked...))
	toStage := make([]string, 0)
	for _, path := range candidatePaths {
		if _, isApproved := approvedSet[path]; isApproved {
			toStage = append(toStage, path)
			continue
		}

		if _, isProtected := protectedSet[path]; isProtected {
			continue
		}

		return fmt.Errorf("%w: path %s is modified but not approved for task %s", ErrMixedPurposeStaging, path, req.TaskID)
	}

	if len(toStage) == 0 {
		return nil
	}

	if err := m.stager.AddPaths(ctx, req.RepoPath, toStage); err != nil {
		return fmt.Errorf("failed to stage task paths: %w", err)
	}

	return nil
}

func uniqueSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, path := range in {
		if !slices.Contains(out, path) {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}
