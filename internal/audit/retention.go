package audit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

type ArtifactBlobInput struct {
	SessionID      string
	TurnID         string
	Content        []byte
	RedactionState string
	ExpiresAt      time.Time
}

type ArtifactBlobMetadata struct {
	ID             string
	SessionID      string
	TurnID         string
	URI            string
	SizeBytes      int64
	SHA256         string
	RedactionState string
	CreatedAt      time.Time
	ExpiresAt      *time.Time
	TombstonedAt   *time.Time
}

type RetentionPolicy struct {
	Now               time.Time
	EventRetention    time.Duration
	ArtifactTombstone bool
}

type RetentionReport struct {
	DeletedEvents    int64
	CleanedArtifacts int64
}

func (s *EventStore) InsertArtifactBlobForTest(ctx context.Context, input ArtifactBlobInput) (string, string, error) {
	id := uuid.NewString()
	artifactDir := filepath.Join(s.dbDir, "artifacts", input.SessionID)
	if err := os.MkdirAll(artifactDir, 0o750); err != nil {
		return "", "", fmt.Errorf("failed to create artifact directory: %w", err)
	}

	path := filepath.Join(artifactDir, id+".blob")
	if err := os.WriteFile(path, input.Content, 0o600); err != nil {
		return "", "", fmt.Errorf("failed to write artifact blob: %w", err)
	}

	sum := sha256.Sum256(input.Content)
	hash := hex.EncodeToString(sum[:])
	now := time.Now().UTC()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO artifact_blobs (id, session_id, turn_id, uri, size_bytes, sha256, redaction_state, created_at, expires_at, tombstoned_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
	`, id, input.SessionID, input.TurnID, path, len(input.Content), hash, input.RedactionState, now.Format(time.RFC3339), input.ExpiresAt.UTC().Format(time.RFC3339))
	if err != nil {
		return "", "", fmt.Errorf("failed to insert artifact metadata: %w", err)
	}

	return path, id, nil
}

func (s *EventStore) GetArtifactBlob(ctx context.Context, id string) (ArtifactBlobMetadata, error) {
	var meta ArtifactBlobMetadata
	var createdAtStr string
	var expiresAt sql.NullString
	var tombstonedAt sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, session_id, turn_id, uri, size_bytes, sha256, redaction_state, created_at, expires_at, tombstoned_at
		FROM artifact_blobs
		WHERE id = ?
	`, id).Scan(
		&meta.ID,
		&meta.SessionID,
		&meta.TurnID,
		&meta.URI,
		&meta.SizeBytes,
		&meta.SHA256,
		&meta.RedactionState,
		&createdAtStr,
		&expiresAt,
		&tombstonedAt,
	)
	if err != nil {
		return ArtifactBlobMetadata{}, err
	}

	createdAt, err := time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return ArtifactBlobMetadata{}, fmt.Errorf("failed to parse created_at: %w", err)
	}
	meta.CreatedAt = createdAt

	if expiresAt.Valid {
		parsed, parseErr := time.Parse(time.RFC3339, expiresAt.String)
		if parseErr != nil {
			return ArtifactBlobMetadata{}, fmt.Errorf("failed to parse expires_at: %w", parseErr)
		}
		meta.ExpiresAt = &parsed
	}

	if tombstonedAt.Valid {
		parsed, parseErr := time.Parse(time.RFC3339, tombstonedAt.String)
		if parseErr != nil {
			return ArtifactBlobMetadata{}, fmt.Errorf("failed to parse tombstoned_at: %w", parseErr)
		}
		meta.TombstonedAt = &parsed
	}

	return meta, nil
}

func (s *EventStore) RunRetention(ctx context.Context, policy RetentionPolicy) (RetentionReport, error) {
	if policy.Now.IsZero() {
		policy.Now = time.Now().UTC()
	}

	var report RetentionReport

	if policy.EventRetention > 0 {
		cutoff := policy.Now.UTC().Add(-policy.EventRetention)
		result, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE timestamp < ?`, cutoff.Format(time.RFC3339))
		if err != nil {
			return report, fmt.Errorf("failed to delete expired events: %w", err)
		}
		deleted, err := result.RowsAffected()
		if err != nil {
			return report, fmt.Errorf("failed to read deleted events count: %w", err)
		}
		report.DeletedEvents = deleted
	}

	type expiredBlob struct {
		id  string
		uri string
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, uri FROM artifact_blobs
		WHERE expires_at IS NOT NULL AND expires_at <= ? AND tombstoned_at IS NULL
	`, policy.Now.UTC().Format(time.RFC3339))
	if err != nil {
		return report, fmt.Errorf("failed to query expired artifacts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var expired []expiredBlob
	for rows.Next() {
		var item expiredBlob
		if err := rows.Scan(&item.id, &item.uri); err != nil {
			return report, fmt.Errorf("failed to scan expired artifact row: %w", err)
		}
		expired = append(expired, item)
	}

	for _, item := range expired {
		if err := os.Remove(item.uri); err != nil && !os.IsNotExist(err) {
			return report, fmt.Errorf("failed to remove expired artifact blob %s: %w", item.id, err)
		}

		if policy.ArtifactTombstone {
			_, err := s.db.ExecContext(ctx, `
				UPDATE artifact_blobs SET tombstoned_at = ?, uri = '[tombstoned]', size_bytes = 0
				WHERE id = ?
			`, policy.Now.UTC().Format(time.RFC3339), item.id)
			if err != nil {
				return report, fmt.Errorf("failed to tombstone artifact %s: %w", item.id, err)
			}
		} else {
			_, err := s.db.ExecContext(ctx, `DELETE FROM artifact_blobs WHERE id = ?`, item.id)
			if err != nil {
				return report, fmt.Errorf("failed to delete artifact %s: %w", item.id, err)
			}
		}

		report.CleanedArtifacts++
	}

	payload, err := json.Marshal(map[string]interface{}{
		"deleted_events":     report.DeletedEvents,
		"cleaned_artifacts":  report.CleanedArtifacts,
		"artifact_tombstone": policy.ArtifactTombstone,
	})
	if err != nil {
		return report, fmt.Errorf("failed to marshal retention payload: %w", err)
	}

	if err := s.Append(ctx, Event{
		ID:        uuid.NewString(),
		SessionID: "system",
		Timestamp: policy.Now.UTC(),
		EventType: "retention_cleanup",
		Actor:     "system",
		Payload:   payload,
	}); err != nil {
		return report, fmt.Errorf("failed to append retention cleanup event: %w", err)
	}

	return report, nil
}
