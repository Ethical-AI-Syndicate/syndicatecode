package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type EventStore struct {
	db    *sql.DB
	dbDir string
}

func NewEventStore(path string) (*EventStore, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &EventStore{db: db, dbDir: filepath.Dir(path)}
	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return store, nil
}

func (s *EventStore) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS events (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		turn_id TEXT,
		timestamp TEXT NOT NULL,
		event_type TEXT NOT NULL,
		actor TEXT NOT NULL,
		policy_version TEXT,
		trust_tier TEXT,
		payload TEXT
	);
	
	CREATE INDEX IF NOT EXISTS idx_events_session ON events(session_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_events_type ON events(event_type, timestamp);

	CREATE TABLE IF NOT EXISTS artifacts (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		turn_id TEXT,
		artifact_type TEXT NOT NULL,
		uri TEXT NOT NULL,
		content_type TEXT,
		size_bytes INTEGER NOT NULL,
		sha256 TEXT NOT NULL,
		redaction_state TEXT NOT NULL,
		created_at TEXT NOT NULL,
		expires_at TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_artifacts_expiry ON artifacts(expires_at);
	CREATE INDEX IF NOT EXISTS idx_artifacts_session_created ON artifacts(session_id, created_at);
	`
	_, err := s.db.Exec(schema)
	return err
}

type Event struct {
	ID            string          `json:"event_id"`
	SessionID     string          `json:"session_id"`
	TurnID        string          `json:"turn_id,omitempty"`
	Timestamp     time.Time       `json:"timestamp"`
	EventType     string          `json:"event_type"`
	Actor         string          `json:"actor"`
	PolicyVersion string          `json:"policy_version,omitempty"`
	TrustTier     string          `json:"trust_tier,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

func (s *EventStore) Append(ctx context.Context, event Event) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO events (id, session_id, turn_id, timestamp, event_type, actor, policy_version, trust_tier, payload) 
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.SessionID, event.TurnID, event.Timestamp.Format(time.RFC3339),
		event.EventType, event.Actor, event.PolicyVersion, event.TrustTier, payload,
	)
	return err
}

func (s *EventStore) QueryBySession(ctx context.Context, sessionID string) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, turn_id, timestamp, event_type, actor, policy_version, trust_tier, payload 
		 FROM events WHERE session_id = ? ORDER BY timestamp ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	return s.scanRows(rows)
}

func (s *EventStore) QueryAll(ctx context.Context) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, turn_id, timestamp, event_type, actor, policy_version, trust_tier, payload 
		 FROM events ORDER BY timestamp ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	return s.scanRows(rows)
}

func (s *EventStore) scanRows(rows *sql.Rows) ([]Event, error) {
	var events []Event
	for rows.Next() {
		var e Event
		var timestampStr string
		var payload []byte

		err := rows.Scan(&e.ID, &e.SessionID, &e.TurnID, &timestampStr, &e.EventType, &e.Actor, &e.PolicyVersion, &e.TrustTier, &payload)
		if err != nil {
			return nil, err
		}

		e.Timestamp, err = time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse event timestamp %q: %w", timestampStr, err)
		}
		e.Payload = payload
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *EventStore) Close() error {
	return s.db.Close()
}
