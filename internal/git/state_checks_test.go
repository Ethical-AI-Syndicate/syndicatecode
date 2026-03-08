package git

import (
	"context"
	"errors"
	"testing"
)

type fakeStateInspector struct {
	state RepoState
	err   error
}

func (f *fakeStateInspector) Inspect(_ context.Context, _ string) (RepoState, error) {
	if f.err != nil {
		return RepoState{}, f.err
	}
	return f.state, nil
}

func TestPreEditCheckRejectsDirtyTreeWhenPolicyRequiresClean(t *testing.T) {
	t.Parallel()

	checker := NewPreEditChecker(&fakeStateInspector{state: RepoState{Branch: "feature/x", HasUnstagedChanges: true}})
	err := checker.Validate(context.Background(), PreEditRequest{
		RepoPath:      "/repo",
		RequireClean:  true,
		TargetFiles:   []string{"internal/context/context.go"},
		AllowBranches: []string{"feature/x"},
	})
	if !errors.Is(err, ErrUnsafeGitState) {
		t.Fatalf("expected %v, got %v", ErrUnsafeGitState, err)
	}
}

func TestPreEditCheckRejectsDisallowedBranch(t *testing.T) {
	t.Parallel()

	checker := NewPreEditChecker(&fakeStateInspector{state: RepoState{Branch: "master"}})
	err := checker.Validate(context.Background(), PreEditRequest{
		RepoPath:      "/repo",
		RequireClean:  false,
		AllowBranches: []string{"feature/x", "feature/y"},
	})
	if !errors.Is(err, ErrUnsafeGitState) {
		t.Fatalf("expected %v, got %v", ErrUnsafeGitState, err)
	}
}

func TestPreEditCheckRejectsMergeConflictMarkers(t *testing.T) {
	t.Parallel()

	checker := NewPreEditChecker(&fakeStateInspector{state: RepoState{Branch: "feature/x", HasMergeMarkers: true}})
	err := checker.Validate(context.Background(), PreEditRequest{RepoPath: "/repo", AllowBranches: []string{"feature/x"}})
	if !errors.Is(err, ErrUnsafeGitState) {
		t.Fatalf("expected %v, got %v", ErrUnsafeGitState, err)
	}
}

func TestPreEditCheckRejectsTargetFileCollision(t *testing.T) {
	t.Parallel()

	checker := NewPreEditChecker(&fakeStateInspector{state: RepoState{
		Branch:            "feature/x",
		ModifiedFilePaths: []string{"internal/context/context.go"},
	}})
	err := checker.Validate(context.Background(), PreEditRequest{
		RepoPath:      "/repo",
		AllowBranches: []string{"feature/x"},
		TargetFiles:   []string{"internal/context/context.go"},
	})
	if !errors.Is(err, ErrUnsafeGitState) {
		t.Fatalf("expected %v, got %v", ErrUnsafeGitState, err)
	}
}

func TestPreEditCheckAllowsSafeState(t *testing.T) {
	t.Parallel()

	checker := NewPreEditChecker(&fakeStateInspector{state: RepoState{Branch: "feature/x"}})
	err := checker.Validate(context.Background(), PreEditRequest{
		RepoPath:      "/repo",
		AllowBranches: []string{"feature/x"},
		TargetFiles:   []string{"internal/session/manager.go"},
		RequireClean:  true,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
