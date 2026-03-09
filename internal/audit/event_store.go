package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type EventStore struct {
	db *sql.DB
}

func NewEventStore(path string) (*EventStore, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	store := &EventStore{db: db}
	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return store, nil
}

func (s *EventStore) migrate() error {
	schema := `
	PRAGMA foreign_keys = ON;

	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		repo_path TEXT NOT NULL,
		trust_tier TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status, updated_at);

	CREATE TABLE IF NOT EXISTS turns (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		message TEXT,
		status TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_turns_session ON turns(session_id, created_at);

	CREATE TABLE IF NOT EXISTS approvals (
		id TEXT PRIMARY KEY,
		session_id TEXT,
		turn_id TEXT,
		tool_name TEXT NOT NULL,
		state TEXT NOT NULL,
		decision_reason TEXT,
		arguments_hash TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE SET NULL,
		FOREIGN KEY(turn_id) REFERENCES turns(id) ON DELETE SET NULL
	);

	CREATE INDEX IF NOT EXISTS idx_approvals_session ON approvals(session_id, created_at);
	CREATE INDEX IF NOT EXISTS idx_approvals_state ON approvals(state, updated_at);

	CREATE TABLE IF NOT EXISTS tool_invocations (
		id TEXT PRIMARY KEY,
		session_id TEXT,
		turn_id TEXT,
		approval_id TEXT,
		tool_name TEXT NOT NULL,
		success INTEGER NOT NULL,
		duration_ms INTEGER NOT NULL,
		output_ref TEXT,
		created_at TEXT NOT NULL,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE SET NULL,
		FOREIGN KEY(turn_id) REFERENCES turns(id) ON DELETE SET NULL,
		FOREIGN KEY(approval_id) REFERENCES approvals(id) ON DELETE SET NULL
	);

	CREATE INDEX IF NOT EXISTS idx_tool_invocations_session ON tool_invocations(session_id, created_at);

	CREATE TABLE IF NOT EXISTS model_invocations (
		id TEXT PRIMARY KEY,
		session_id TEXT,
		turn_id TEXT,
		provider TEXT NOT NULL,
		model TEXT NOT NULL,
		routing_policy TEXT,
		prompt_ref TEXT,
		response_ref TEXT,
		created_at TEXT NOT NULL,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE SET NULL,
		FOREIGN KEY(turn_id) REFERENCES turns(id) ON DELETE SET NULL
	);

	CREATE INDEX IF NOT EXISTS idx_model_invocations_session ON model_invocations(session_id, created_at);

	CREATE TABLE IF NOT EXISTS context_fragments (
		id TEXT PRIMARY KEY,
		session_id TEXT,
		turn_id TEXT,
		source_type TEXT NOT NULL,
		source_ref TEXT,
		included INTEGER NOT NULL,
		exclusion_reason TEXT,
		sensitivity TEXT,
		redaction_action TEXT,
		redaction_denied INTEGER NOT NULL,
		content_ref TEXT,
		created_at TEXT NOT NULL,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE SET NULL,
		FOREIGN KEY(turn_id) REFERENCES turns(id) ON DELETE SET NULL
	);

	CREATE INDEX IF NOT EXISTS idx_context_fragments_turn ON context_fragments(turn_id, created_at);

	CREATE TABLE IF NOT EXISTS patch_proposals (
		id TEXT PRIMARY KEY,
		session_id TEXT,
		turn_id TEXT,
		proposal_ref TEXT,
		status TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE SET NULL,
		FOREIGN KEY(turn_id) REFERENCES turns(id) ON DELETE SET NULL
	);

	CREATE INDEX IF NOT EXISTS idx_patch_proposals_turn ON patch_proposals(turn_id, created_at);

	CREATE TABLE IF NOT EXISTS file_mutations (
		id TEXT PRIMARY KEY,
		session_id TEXT,
		turn_id TEXT,
		patch_id TEXT,
		path TEXT NOT NULL,
		mutation_type TEXT NOT NULL,
		before_hash TEXT,
		after_hash TEXT,
		applied_at TEXT NOT NULL,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE SET NULL,
		FOREIGN KEY(turn_id) REFERENCES turns(id) ON DELETE SET NULL,
		FOREIGN KEY(patch_id) REFERENCES patch_proposals(id) ON DELETE SET NULL
	);

	CREATE INDEX IF NOT EXISTS idx_file_mutations_turn ON file_mutations(turn_id, applied_at);

	CREATE TABLE IF NOT EXISTS artifacts (
		id TEXT PRIMARY KEY,
		session_id TEXT,
		turn_id TEXT,
		kind TEXT NOT NULL,
		storage_path TEXT NOT NULL,
		sha256 TEXT,
		redaction_state TEXT,
		expires_at TEXT,
		created_at TEXT NOT NULL,
		FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE SET NULL,
		FOREIGN KEY(turn_id) REFERENCES turns(id) ON DELETE SET NULL
	);

	CREATE INDEX IF NOT EXISTS idx_artifacts_session ON artifacts(session_id, created_at);

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

// CleanupResult reports how many rows were deleted by a retention pass.
type CleanupResult struct {
	ArtifactsDeleted int
}

// CleanupExpired deletes artifacts whose expires_at is set and before threshold,
// then appends a retention.cleanup audit event recording the counts.
func (s *EventStore) CleanupExpired(ctx context.Context, threshold time.Time) (CleanupResult, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM artifacts WHERE expires_at IS NOT NULL AND expires_at < ?`,
		threshold.Format(time.RFC3339),
	)
	if err != nil {
		return CleanupResult{}, fmt.Errorf("delete expired artifacts: %w", err)
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return CleanupResult{}, fmt.Errorf("rows affected: %w", err)
	}

	result := CleanupResult{ArtifactsDeleted: int(deleted)}

	payload, err := json.Marshal(map[string]interface{}{
		"threshold":         threshold.Format(time.RFC3339),
		"artifacts_deleted": result.ArtifactsDeleted,
	})
	if err != nil {
		return result, fmt.Errorf("marshal retention event payload: %w", err)
	}

	event := Event{
		ID:        fmt.Sprintf("ret-%d", threshold.UnixNano()),
		SessionID: "system:retention",
		Timestamp: threshold,
		EventType: "retention.cleanup",
		Actor:     "system",
		Payload:   payload,
	}
	if appendErr := s.Append(ctx, event); appendErr != nil {
		return result, fmt.Errorf("append retention event: %w", appendErr)
	}

	return result, nil
}
