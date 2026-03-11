package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type EventStore struct {
	db *sql.DB
}

func NewEventStore(path string) (*EventStore, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	db.SetMaxOpenConns(1)

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
	return applyMigrations(s.db)
}

// ArtifactRecord is a stored reference to an artifact blob.
type ArtifactRecord struct {
	ID             string     `json:"id"`
	SessionID      string     `json:"session_id,omitempty"`
	TurnID         string     `json:"turn_id,omitempty"`
	Kind           string     `json:"kind"`
	StoragePath    string     `json:"storage_path"`
	SHA256         string     `json:"sha256"`
	RedactionState string     `json:"redaction_state,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type ToolInvocationRecord struct {
	ID         string
	SessionID  string
	TurnID     string
	ApprovalID string
	ToolName   string
	Success    bool
	DurationMS int64
	OutputRef  string
	CreatedAt  time.Time
}

type ModelInvocationRecord struct {
	ID            string
	SessionID     string
	TurnID        string
	Provider      string
	Model         string
	RoutingPolicy string
	PromptRef     string
	ResponseRef   string
	CreatedAt     time.Time
}

type FileMutationRecord struct {
	ID           string
	SessionID    string
	TurnID       string
	PatchID      string
	Path         string
	MutationType string
	BeforeHash   string
	AfterHash    string
	AppliedAt    time.Time
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
	payload := []byte(event.Payload)
	if len(payload) == 0 {
		payload = []byte("null")
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO events (id, session_id, turn_id, timestamp, event_type, actor, policy_version, trust_tier, payload) 
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.SessionID, event.TurnID, event.Timestamp.Format(time.RFC3339Nano),
		event.EventType, event.Actor, event.PolicyVersion, event.TrustTier, payload,
	)
	return err
}

func (s *EventStore) QueryBySession(ctx context.Context, sessionID string) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, turn_id, timestamp, event_type, actor, policy_version, trust_tier, payload
		 FROM events WHERE session_id = ? ORDER BY timestamp ASC, rowid ASC`,
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

func (s *EventStore) QuerySessionEvents(ctx context.Context) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, turn_id, timestamp, event_type, actor, policy_version, trust_tier, payload
		 FROM events WHERE event_type IN (?, ?)
		 ORDER BY timestamp ASC, rowid ASC`,
		EventSessionStarted,
		EventSessionTerminated,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	return s.scanRows(rows)
}

func (s *EventStore) QueryByTurn(ctx context.Context, turnID string) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, turn_id, timestamp, event_type, actor, policy_version, trust_tier, payload
		 FROM events WHERE turn_id = ? ORDER BY timestamp ASC, rowid ASC`,
		turnID,
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
		 FROM events ORDER BY timestamp ASC, rowid ASC`,
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

		e.Timestamp, err = time.Parse(time.RFC3339Nano, timestampStr)
		if err != nil {
			e.Timestamp, err = time.Parse(time.RFC3339, timestampStr)
			if err != nil {
				return nil, fmt.Errorf("failed to parse event timestamp %q: %w", timestampStr, err)
			}
		}
		e.Payload = payload
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *EventStore) Close() error {
	return s.db.Close()
}

func (s *EventStore) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("event store unavailable")
	}
	return s.db.PingContext(ctx)
}

func (s *EventStore) SchemaVersion() (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("event store unavailable")
	}
	return currentSchemaVersion(s.db)
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
		EventType: EventRetentionClean,
		Actor:     "system",
		Payload:   payload,
	}
	if appendErr := s.Append(ctx, event); appendErr != nil {
		return result, fmt.Errorf("append retention event: %w", appendErr)
	}

	return result, nil
}

// StoreArtifact inserts an artifact reference record.
func (s *EventStore) StoreArtifact(ctx context.Context, a ArtifactRecord) error {
	var sessionID, turnID, redactionState, expiresAt interface{}
	if a.SessionID != "" {
		sessionID = a.SessionID
	}
	if a.TurnID != "" {
		turnID = a.TurnID
	}
	if a.RedactionState != "" {
		redactionState = a.RedactionState
	}
	if a.ExpiresAt != nil {
		expiresAt = a.ExpiresAt.Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO artifacts (id, session_id, turn_id, kind, storage_path, sha256, redaction_state, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, sessionID, turnID, a.Kind, a.StoragePath, a.SHA256,
		redactionState, expiresAt, a.CreatedAt.Format(time.RFC3339),
	)
	return err
}

// GetArtifact retrieves a single artifact by ID. Returns an error if not found.
func (s *EventStore) GetArtifact(ctx context.Context, id string) (ArtifactRecord, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, COALESCE(session_id,''), COALESCE(turn_id,''), kind, storage_path,
		        COALESCE(sha256,''), COALESCE(redaction_state,''), COALESCE(expires_at,''), created_at
		 FROM artifacts WHERE id = ?`, id,
	)
	return scanArtifact(row)
}

// ListArtifactsBySession returns all artifact records for a given session.
func (s *EventStore) ListArtifactsBySession(ctx context.Context, sessionID string) ([]ArtifactRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, COALESCE(session_id,''), COALESCE(turn_id,''), kind, storage_path,
		        COALESCE(sha256,''), COALESCE(redaction_state,''), COALESCE(expires_at,''), created_at
		 FROM artifacts WHERE session_id = ? ORDER BY created_at`, sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []ArtifactRecord
	for rows.Next() {
		a, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

type artifactScanner interface {
	Scan(dest ...any) error
}

func scanArtifact(s artifactScanner) (ArtifactRecord, error) {
	var a ArtifactRecord
	var expiresAtStr, createdAtStr string
	if err := s.Scan(&a.ID, &a.SessionID, &a.TurnID, &a.Kind, &a.StoragePath,
		&a.SHA256, &a.RedactionState, &expiresAtStr, &createdAtStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ArtifactRecord{}, fmt.Errorf("artifact not found: %w", err)
		}
		return ArtifactRecord{}, fmt.Errorf("scan artifact: %w", err)
	}
	t, err := time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return ArtifactRecord{}, fmt.Errorf("parse created_at %q: %w", createdAtStr, err)
	}
	a.CreatedAt = t
	if expiresAtStr != "" {
		if t, err := time.Parse(time.RFC3339, expiresAtStr); err == nil {
			a.ExpiresAt = &t
		}
	}
	return a, nil
}

func (s *EventStore) RecordToolInvocation(ctx context.Context, r ToolInvocationRecord) error {
	success := 0
	if r.Success {
		success = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tool_invocations
		 (id, session_id, turn_id, approval_id, tool_name, success, duration_ms, output_ref, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.SessionID, r.TurnID, r.ApprovalID,
		r.ToolName, success, r.DurationMS, r.OutputRef,
		r.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *EventStore) RecordModelInvocation(ctx context.Context, r ModelInvocationRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO model_invocations
		 (id, session_id, turn_id, provider, model, routing_policy, prompt_ref, response_ref, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.SessionID, r.TurnID, r.Provider, r.Model,
		r.RoutingPolicy, r.PromptRef, r.ResponseRef,
		r.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *EventStore) RecordFileMutation(ctx context.Context, r FileMutationRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO file_mutations
		 (id, session_id, turn_id, patch_id, path, mutation_type, before_hash, after_hash, applied_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.SessionID, r.TurnID, r.PatchID,
		r.Path, r.MutationType, r.BeforeHash, r.AfterHash,
		r.AppliedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}
