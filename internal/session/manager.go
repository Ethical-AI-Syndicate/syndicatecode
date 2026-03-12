package session

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/state"
)

type Status = state.SessionState

const (
	StatusActive     Status = state.SessionStateActive
	StatusCompleted  Status = state.SessionStateCompleted
	StatusTerminated Status = state.SessionStateTerminated
)

type Session struct {
	ID        string    `json:"session_id"`
	RepoPath  string    `json:"repo_path"`
	TrustTier string    `json:"trust_tier"`
	Status    Status    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DeleteMode string

const (
	DeleteModeSoft      DeleteMode = "soft"
	DeleteModeHard      DeleteMode = "hard"
	DeleteModeTombstone DeleteMode = "tombstone"
)

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
		"repo_path":            repoPath,
		"entity_type":          "session",
		"entity_id":            session.ID,
		"previous_state":       "none",
		"next_state":           session.Status,
		"cause":                "session_create_requested",
		"transition_timestamp": now.Format(time.RFC3339Nano),
		"related_ids":          map[string]interface{}{},
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

	if isDeleted(events) {
		return nil, ErrSessionNotFound
	}

	first := firstSessionStarted(events)
	if first == nil {
		return nil, ErrSessionNotFound
	}

	session := &Session{
		ID:        id,
		TrustTier: first.TrustTier,
		Status:    StatusActive,
		CreatedAt: first.Timestamp,
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
	deleted := make(map[string]bool)

	events, err := m.eventStore.QueryAll(ctx)
	if err != nil {
		return nil, err
	}

	for _, e := range events {
		if isDeleteEvent(e.EventType) {
			deleted[e.SessionID] = true
		}

		sessionObj, exists := sessions[e.SessionID]
		switch e.EventType {
		case "session_started":
			var payload map[string]interface{}
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				continue
			}
			repoPath, _ := payload["repo_path"].(string)

			if !exists {
				sessions[e.SessionID] = &Session{
					ID:        e.SessionID,
					RepoPath:  repoPath,
					TrustTier: e.TrustTier,
					Status:    StatusActive,
					CreatedAt: e.Timestamp,
					UpdatedAt: e.Timestamp,
				}
			}
		case "session_terminated":
			if exists {
				sessionObj.Status = StatusTerminated
			}
		}

		if exists && e.Timestamp.After(sessionObj.UpdatedAt) {
			sessionObj.UpdatedAt = e.Timestamp
		}
	}

	result := make([]*Session, 0, len(sessions))
	for sessionID, s := range sessions {
		if deleted[sessionID] {
			continue
		}
		result = append(result, s)
	}

	return result, nil
}

func (m *Manager) Delete(ctx context.Context, sessionID string, mode DeleteMode, reason string) error {
	events, err := m.eventStore.QueryBySession(ctx, sessionID)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return ErrSessionNotFound
	}

	trustTier := ""
	if first := firstSessionStarted(events); first != nil {
		trustTier = first.TrustTier
	}

	now := time.Now().UTC()
	payload, err := json.Marshal(map[string]interface{}{
		"mode":       mode,
		"reason":     reason,
		"deleted_at": now.Format(time.RFC3339Nano),
	})
	if err != nil {
		return err
	}

	switch mode {
	case DeleteModeSoft:
		return m.eventStore.Append(ctx, audit.Event{
			ID:            uuid.New().String(),
			SessionID:     sessionID,
			EventType:     "session_soft_deleted",
			Actor:         "system",
			Timestamp:     now,
			TrustTier:     trustTier,
			PolicyVersion: "1.0.0",
			Payload:       payload,
		})
	case DeleteModeHard:
		return m.eventStore.DeleteBySession(ctx, sessionID)
	case DeleteModeTombstone:
		if err := m.eventStore.DeleteBySession(ctx, sessionID); err != nil {
			return err
		}
		return m.eventStore.Append(ctx, audit.Event{
			ID:            uuid.New().String(),
			SessionID:     sessionID,
			EventType:     "session_tombstoned",
			Actor:         "system",
			Timestamp:     now,
			TrustTier:     trustTier,
			PolicyVersion: "1.0.0",
			Payload:       payload,
		})
	default:
		return ErrInvalidDeleteMode
	}
}

func firstSessionStarted(events []audit.Event) *audit.Event {
	for idx := range events {
		if events[idx].EventType == "session_started" {
			return &events[idx]
		}
	}
	return nil
}

func isDeleted(events []audit.Event) bool {
	for _, event := range events {
		if isDeleteEvent(event.EventType) {
			return true
		}
	}
	return false
}

func isDeleteEvent(eventType string) bool {
	return eventType == "session_soft_deleted" || eventType == "session_tombstoned"
}

var ErrSessionNotFound = &sessionError{"session not found"}
var ErrInvalidDeleteMode = errors.New("invalid delete mode")

type sessionError struct {
	msg string
}

func (e *sessionError) Error() string {
	return e.msg
}
