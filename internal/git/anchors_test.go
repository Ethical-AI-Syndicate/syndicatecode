package git

import (
	"context"
	"errors"
	"testing"
)

type fakeCheckpointStore struct {
	createCalled  bool
	restoreCalled bool
	lastRef       string
	createRef     string
	err           error
}

func (f *fakeCheckpointStore) CreateCheckpoint(_ context.Context, _ AnchorRequest) (string, error) {
	f.createCalled = true
	if f.err != nil {
		return "", f.err
	}
	if f.createRef == "" {
		f.createRef = "cp-1"
	}
	return f.createRef, nil
}

func (f *fakeCheckpointStore) RestoreCheckpoint(_ context.Context, checkpointRef string) error {
	f.restoreCalled = true
	f.lastRef = checkpointRef
	return f.err
}

func TestCreateAnchorIncludesOptionalCheckpointReference(t *testing.T) {
	t.Parallel()

	store := &fakeCheckpointStore{createRef: "checkpoint-42"}
	mgr := NewAnchorManager(store)

	anchor, err := mgr.CreateAnchor(context.Background(), AnchorRequest{
		SessionID:     "s1",
		TurnID:        "t1",
		RepoPath:      "/repo",
		EnableGitRef:  true,
		BaselineHash:  "abc123",
		MutationScope: []string{"internal/context/context.go"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if anchor.CheckpointRef != "checkpoint-42" {
		t.Fatalf("expected checkpoint reference to be attached")
	}
	if !store.createCalled {
		t.Fatalf("expected checkpoint creation to be invoked")
	}
}

func TestCreateAnchorWithInternalOnlyStrategySkipsCheckpoint(t *testing.T) {
	t.Parallel()

	store := &fakeCheckpointStore{}
	mgr := NewAnchorManager(store)

	anchor, err := mgr.CreateAnchor(context.Background(), AnchorRequest{
		SessionID:     "s1",
		TurnID:        "t1",
		RepoPath:      "/repo",
		EnableGitRef:  false,
		BaselineHash:  "abc123",
		MutationScope: []string{"internal/tools/capability.go"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if anchor.CheckpointRef != "" {
		t.Fatalf("expected no checkpoint reference")
	}
	if store.createCalled {
		t.Fatalf("did not expect checkpoint creation")
	}
}

func TestLinkMutationAssociatesAnchorToMutationEvent(t *testing.T) {
	t.Parallel()

	mgr := NewAnchorManager(&fakeCheckpointStore{})
	anchor, err := mgr.CreateAnchor(context.Background(), AnchorRequest{SessionID: "s1", TurnID: "t1", RepoPath: "/repo", BaselineHash: "abc123"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if err := mgr.LinkMutation(anchor.ID, "mutation-9"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	links := mgr.MutationsForAnchor(anchor.ID)
	if len(links) != 1 || links[0] != "mutation-9" {
		t.Fatalf("expected mutation linkage to be recorded")
	}
}

func TestRestoreUsesRecordedCheckpointReference(t *testing.T) {
	t.Parallel()

	store := &fakeCheckpointStore{createRef: "cp-restore"}
	mgr := NewAnchorManager(store)
	anchor, err := mgr.CreateAnchor(context.Background(), AnchorRequest{SessionID: "s1", TurnID: "t1", RepoPath: "/repo", BaselineHash: "abc123", EnableGitRef: true})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if err := mgr.Restore(context.Background(), anchor.ID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !store.restoreCalled || store.lastRef != "cp-restore" {
		t.Fatalf("expected restore to use checkpoint reference")
	}
}

func TestRestoreFailsForUnknownAnchor(t *testing.T) {
	t.Parallel()

	mgr := NewAnchorManager(&fakeCheckpointStore{})
	err := mgr.Restore(context.Background(), "missing")
	if !errors.Is(err, ErrAnchorNotFound) {
		t.Fatalf("expected %v, got %v", ErrAnchorNotFound, err)
	}
}
