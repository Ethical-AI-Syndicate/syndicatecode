package git

import (
	"context"
	"errors"
	"testing"
)

type fakeStager struct {
	status WorkingTreeStatus
	added  []string
	err    error
}

func (f *fakeStager) Status(_ context.Context, _ string) (WorkingTreeStatus, error) {
	if f.err != nil {
		return WorkingTreeStatus{}, f.err
	}
	return f.status, nil
}

func (f *fakeStager) AddPaths(_ context.Context, _ string, paths []string) error {
	if f.err != nil {
		return f.err
	}
	f.added = append([]string{}, paths...)
	return nil
}

func TestStageTaskChangesStagesOnlyApprovedPaths(t *testing.T) {
	t.Parallel()

	stager := &fakeStager{status: WorkingTreeStatus{
		Unstaged:  []string{"internal/context/context.go", "README.md"},
		Untracked: []string{"internal/git/new_file.go"},
	}}
	mgr := NewTaskScopedStagingManager(stager)

	err := mgr.StageTaskChanges(context.Background(), StageRequest{
		RepoPath:       "/repo",
		TaskID:         "task-1",
		ApprovedPaths:  []string{"internal/context/context.go", "internal/git/new_file.go"},
		ProtectedPaths: []string{"README.md"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(stager.added) != 2 {
		t.Fatalf("expected two staged paths, got %d", len(stager.added))
	}
	if stager.added[0] != "internal/context/context.go" || stager.added[1] != "internal/git/new_file.go" {
		t.Fatalf("unexpected staged paths: %v", stager.added)
	}
}

func TestStageTaskChangesRejectsMixedPurposeStaging(t *testing.T) {
	t.Parallel()

	stager := &fakeStager{status: WorkingTreeStatus{Unstaged: []string{"internal/context/context.go", "internal/session/manager.go"}}}
	mgr := NewTaskScopedStagingManager(stager)

	err := mgr.StageTaskChanges(context.Background(), StageRequest{
		RepoPath:      "/repo",
		TaskID:        "task-2",
		ApprovedPaths: []string{"internal/context/context.go"},
	})
	if !errors.Is(err, ErrMixedPurposeStaging) {
		t.Fatalf("expected %v, got %v", ErrMixedPurposeStaging, err)
	}
}

func TestStageTaskChangesPreservesUnrelatedOperatorEdits(t *testing.T) {
	t.Parallel()

	stager := &fakeStager{status: WorkingTreeStatus{Unstaged: []string{"README.md"}}}
	mgr := NewTaskScopedStagingManager(stager)

	err := mgr.StageTaskChanges(context.Background(), StageRequest{
		RepoPath:       "/repo",
		TaskID:         "task-3",
		ApprovedPaths:  []string{},
		ProtectedPaths: []string{"README.md"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(stager.added) != 0 {
		t.Fatalf("expected no staged paths when only unrelated protected edits exist")
	}
}
