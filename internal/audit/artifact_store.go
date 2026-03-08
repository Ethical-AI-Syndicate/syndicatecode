package audit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

type ArtifactInput struct {
	SessionID      string
	TurnID         string
	ArtifactType   string
	ContentType    string
	Content        []byte
	RedactionState string
	ExpiresAt      *time.Time
}

type ArtifactRecord struct {
	ID             string
	SessionID      string
	TurnID         string
	ArtifactType   string
	URI            string
	ContentType    string
	SizeBytes      int64
	SHA256         string
	RedactionState string
	CreatedAt      time.Time
	ExpiresAt      *time.Time
}

func (s *EventStore) StoreArtifact(ctx context.Context, input ArtifactInput) (ArtifactRecord, error) {
	if input.SessionID == "" {
		return ArtifactRecord{}, fmt.Errorf("session id is required")
	}
	if len(input.Content) == 0 {
		return ArtifactRecord{}, fmt.Errorf("artifact content is required")
	}

	id := uuid.NewString()
	hash := sha256.Sum256(input.Content)
	hashString := hex.EncodeToString(hash[:])

	artifactDir := filepath.Join(s.dbDir, "artifacts", input.SessionID)
	if err := os.MkdirAll(artifactDir, 0o750); err != nil {
		return ArtifactRecord{}, fmt.Errorf("failed to create artifact directory: %w", err)
	}

	artifactPath := filepath.Join(artifactDir, id+".blob")
	if err := os.WriteFile(artifactPath, input.Content, 0o600); err != nil {
		return ArtifactRecord{}, fmt.Errorf("failed to write artifact blob: %w", err)
	}

	createdAt := time.Now().UTC()
	var expiresAtValue interface{}
	if input.ExpiresAt != nil {
		expiresAtValue = input.ExpiresAt.UTC().Format(time.RFC3339)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO artifacts (
			id, session_id, turn_id, artifact_type, uri, content_type, size_bytes, sha256, redaction_state, created_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, input.SessionID, input.TurnID, input.ArtifactType, artifactPath, input.ContentType, len(input.Content), hashString, input.RedactionState, createdAt.Format(time.RFC3339), expiresAtValue)
	if err != nil {
		return ArtifactRecord{}, fmt.Errorf("failed to store artifact metadata: %w", err)
	}

	record := ArtifactRecord{
		ID:             id,
		SessionID:      input.SessionID,
		TurnID:         input.TurnID,
		ArtifactType:   input.ArtifactType,
		URI:            artifactPath,
		ContentType:    input.ContentType,
		SizeBytes:      int64(len(input.Content)),
		SHA256:         hashString,
		RedactionState: input.RedactionState,
		CreatedAt:      createdAt,
		ExpiresAt:      input.ExpiresAt,
	}

	return record, nil
}

func (s *EventStore) LoadArtifact(ctx context.Context, artifactID string) ([]byte, ArtifactRecord, error) {
	var record ArtifactRecord
	var createdAtStr string
	var expiresAt sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, session_id, turn_id, artifact_type, uri, content_type, size_bytes, sha256, redaction_state, created_at, expires_at
		FROM artifacts
		WHERE id = ?
	`, artifactID).Scan(
		&record.ID,
		&record.SessionID,
		&record.TurnID,
		&record.ArtifactType,
		&record.URI,
		&record.ContentType,
		&record.SizeBytes,
		&record.SHA256,
		&record.RedactionState,
		&createdAtStr,
		&expiresAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ArtifactRecord{}, fmt.Errorf("artifact %s not found", artifactID)
		}
		return nil, ArtifactRecord{}, fmt.Errorf("failed to query artifact metadata: %w", err)
	}

	createdAt, err := time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return nil, ArtifactRecord{}, fmt.Errorf("failed to parse artifact created_at: %w", err)
	}
	record.CreatedAt = createdAt

	if expiresAt.Valid {
		parsed, parseErr := time.Parse(time.RFC3339, expiresAt.String)
		if parseErr != nil {
			return nil, ArtifactRecord{}, fmt.Errorf("failed to parse artifact expires_at: %w", parseErr)
		}
		record.ExpiresAt = &parsed
	}

	content, err := os.ReadFile(record.URI)
	if err != nil {
		return nil, ArtifactRecord{}, fmt.Errorf("failed to read artifact blob: %w", err)
	}

	computed := sha256.Sum256(content)
	computedHash := hex.EncodeToString(computed[:])
	if computedHash != record.SHA256 {
		return nil, ArtifactRecord{}, fmt.Errorf("artifact hash mismatch for %s", artifactID)
	}

	return content, record, nil
}

func (s *EventStore) CleanupExpiredArtifacts(ctx context.Context, now time.Time) (int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, uri FROM artifacts WHERE expires_at IS NOT NULL AND expires_at <= ?
	`, now.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("failed to query expired artifacts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type expiredArtifact struct {
		id  string
		uri string
	}

	var expired []expiredArtifact
	for rows.Next() {
		var item expiredArtifact
		if err := rows.Scan(&item.id, &item.uri); err != nil {
			return 0, fmt.Errorf("failed to scan expired artifact row: %w", err)
		}
		expired = append(expired, item)
	}

	for _, item := range expired {
		if err := os.Remove(item.uri); err != nil && !os.IsNotExist(err) {
			return 0, fmt.Errorf("failed to remove artifact blob %s: %w", item.id, err)
		}
	}

	result, err := s.db.ExecContext(ctx, `
		DELETE FROM artifacts WHERE expires_at IS NOT NULL AND expires_at <= ?
	`, now.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired artifact metadata: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read deleted rows count: %w", err)
	}

	return int(deleted), nil
}
