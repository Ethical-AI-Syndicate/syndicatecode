package git

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

var ErrAnchorNotFound = errors.New("rollback anchor not found")

type AnchorRequest struct {
	SessionID     string
	TurnID        string
	RepoPath      string
	BaselineHash  string
	MutationScope []string
	EnableGitRef  bool
}

type RollbackAnchor struct {
	ID            string
	SessionID     string
	TurnID        string
	RepoPath      string
	BaselineHash  string
	MutationScope []string
	CheckpointRef string
	CreatedAt     time.Time
}

type CheckpointStore interface {
	CreateCheckpoint(ctx context.Context, req AnchorRequest) (string, error)
	RestoreCheckpoint(ctx context.Context, checkpointRef string) error
}

type AnchorManager struct {
	checkpoints CheckpointStore

	mu              sync.RWMutex
	anchors         map[string]RollbackAnchor
	anchorMutations map[string][]string
}

func NewAnchorManager(checkpoints CheckpointStore) *AnchorManager {
	return &AnchorManager{
		checkpoints:     checkpoints,
		anchors:         make(map[string]RollbackAnchor),
		anchorMutations: make(map[string][]string),
	}
}

func (m *AnchorManager) CreateAnchor(ctx context.Context, req AnchorRequest) (RollbackAnchor, error) {
	anchor := RollbackAnchor{
		ID:            uuid.New().String(),
		SessionID:     req.SessionID,
		TurnID:        req.TurnID,
		RepoPath:      req.RepoPath,
		BaselineHash:  req.BaselineHash,
		MutationScope: append([]string{}, req.MutationScope...),
		CreatedAt:     time.Now().UTC(),
	}

	if req.EnableGitRef {
		checkpointRef, err := m.checkpoints.CreateCheckpoint(ctx, req)
		if err != nil {
			return RollbackAnchor{}, fmt.Errorf("failed to create checkpoint: %w", err)
		}
		anchor.CheckpointRef = checkpointRef
	}

	m.mu.Lock()
	m.anchors[anchor.ID] = anchor
	m.mu.Unlock()

	return anchor, nil
}

func (m *AnchorManager) LinkMutation(anchorID, mutationID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.anchors[anchorID]; !ok {
		return ErrAnchorNotFound
	}
	m.anchorMutations[anchorID] = append(m.anchorMutations[anchorID], mutationID)
	return nil
}

func (m *AnchorManager) MutationsForAnchor(anchorID string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string{}, m.anchorMutations[anchorID]...)
}

func (m *AnchorManager) Restore(ctx context.Context, anchorID string) error {
	m.mu.RLock()
	anchor, ok := m.anchors[anchorID]
	m.mu.RUnlock()
	if !ok {
		return ErrAnchorNotFound
	}
	if anchor.CheckpointRef == "" {
		return nil
	}
	if err := m.checkpoints.RestoreCheckpoint(ctx, anchor.CheckpointRef); err != nil {
		return fmt.Errorf("failed to restore checkpoint %s: %w", anchor.CheckpointRef, err)
	}
	return nil
}
