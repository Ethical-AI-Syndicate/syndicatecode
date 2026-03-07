package session

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/syndicatecode/syndicatecode/internal/audit"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
	StatusTerminated Status = "terminated"
)

type Session struct {
	ID        string    `json:"session_id"`
	RepoPath  string    `json:"repo_path"`
	TrustTier string    `json:"trust_tier"`
	Status    Status    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Manager struct {
	eventStore *audit.EventStore
}

func NewManager(eventStore *audit.EventStore) *Manager {
	return &Manager{
		eventStore: eventStore,
	}
}

func (m *Manager) Create(ctx context.Context, repoPath, trustTier string) (*Session, error) {
	now := time.Now()
	session := &Session{
		ID:        uuid.New().String(),
		RepoPath:  repoPath,
		TrustTier: trustTier,
		Status:    StatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"repo_path": repoPath,
	})

	event := audit.Event{
		ID:            uuid.New().String(),
		SessionID:     session.ID,
		EventType:     "session_started",
		Actor:         "system",
		Timestamp:     now,
		TrustTier:     trustTier,
		PolicyVersion: "1.0.0",
		Payload:       payload,
	}

	if err := m.eventStore.Append(ctx, event); err != nil {
		return nil, err
	}

	return session, nil
}

func (m *Manager) Get(ctx context.Context, id string) (*Session, error) {
	events, err := m.eventStore.QueryBySession(ctx, id)
	if err != nil {
		return nil, err
	}

	if len(events) == 0 {
		return nil, ErrSessionNotFound
	}

	session := &Session{
		ID:        id,
		TrustTier: events[0].TrustTier,
		Status:    StatusActive,
		CreatedAt: events[0].Timestamp,
		UpdatedAt: events[len(events)-1].Timestamp,
	}

	for _, e := range events {
		if e.EventType == "session_terminated" {
			session.Status = StatusTerminated
		}
	}

	return session, nil
}

func (m *Manager) List(ctx context.Context) ([]*Session, error) {
	sessions := make(map[string]*Session)
	
	events, err := m.eventStore.QueryAll(ctx)
	if err != nil {
		return nil, err
	}

	for _, e := range events {
		if e.EventType != "session_started" {
			continue
		}
		
		var payload map[string]interface{}
		json.Unmarshal(e.Payload, &payload)
		repoPath, _ := payload["repo_path"].(string)
		
		if _, exists := sessions[e.SessionID]; !exists {
			sessions[e.SessionID] = &Session{
				ID:        e.SessionID,
				RepoPath:  repoPath,
				TrustTier: e.TrustTier,
				Status:    StatusActive,
				CreatedAt: e.Timestamp,
				UpdatedAt: e.Timestamp,
			}
		}
		
		if e.Timestamp.After(sessions[e.SessionID].UpdatedAt) {
			sessions[e.SessionID].UpdatedAt = e.Timestamp
		}
		
		if e.EventType == "session_terminated" {
			sessions[e.SessionID].Status = StatusTerminated
		}
	}

	result := make([]*Session, 0, len(sessions))
	for _, s := range sessions {
		result = append(result, s)
	}

	return result, nil
}

var ErrSessionNotFound = &sessionError{"session not found"}

type sessionError struct {
	msg string
}

func (e *sessionError) Error() string {
	return e.msg
}
