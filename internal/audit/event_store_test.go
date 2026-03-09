package audit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestEventStore_MigrateCreatesCoreTablesAndIndexes(t *testing.T) {
	store, err := NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	expectedTables := []string{
		"sessions",
		"turns",
		"approvals",
		"tool_invocations",
		"model_invocations",
		"context_fragments",
		"patch_proposals",
		"file_mutations",
		"artifacts",
		"events",
	}

	for _, table := range expectedTables {
		if !schemaObjectExists(t, store, "table", table) {
			t.Fatalf("expected table %s to exist", table)
		}
	}

	expectedIndexes := []string{
		"idx_sessions_status",
		"idx_turns_session",
		"idx_approvals_session",
		"idx_tool_invocations_session",
		"idx_model_invocations_session",
		"idx_context_fragments_turn",
		"idx_patch_proposals_turn",
		"idx_file_mutations_turn",
		"idx_artifacts_session",
		"idx_events_session",
		"idx_events_type",
	}

	for _, idx := range expectedIndexes {
		if !schemaObjectExists(t, store, "index", idx) {
			t.Fatalf("expected index %s to exist", idx)
		}
	}
}

func TestEventStore_MigrateEnforcesForeignKeysAndWAL(t *testing.T) {
	store, err := NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var foreignKeys int
	if err := store.db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("failed to check foreign key setting: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("expected foreign_keys pragma to be enabled, got %d", foreignKeys)
	}

	var journalMode string
	if err := store.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("failed to check journal mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("expected WAL journal mode, got %s", journalMode)
	}
}

func TestEventStore_CleanupExpiredDeletesArtifactsAndEmitsEvent(t *testing.T) {
	t.Parallel()

	store, err := NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	now := time.Now().UTC()

	// Insert an artifact that expired 1 hour ago.
	expiredID := "artifact-expired-1"
	_, err = store.db.ExecContext(ctx,
		`INSERT INTO artifacts (id, kind, storage_path, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		expiredID, "blob", "/tmp/old.blob",
		now.Add(-1*time.Hour).Format(time.RFC3339),
		now.Add(-2*time.Hour).Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("insert expired artifact: %v", err)
	}

	// Insert an artifact that expires in the future.
	futureID := "artifact-future-1"
	_, err = store.db.ExecContext(ctx,
		`INSERT INTO artifacts (id, kind, storage_path, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		futureID, "blob", "/tmp/new.blob",
		now.Add(1*time.Hour).Format(time.RFC3339),
		now.Add(-1*time.Hour).Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("insert future artifact: %v", err)
	}

	result, err := store.CleanupExpired(ctx, now)
	if err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}

	if result.ArtifactsDeleted != 1 {
		t.Errorf("expected 1 artifact deleted, got %d", result.ArtifactsDeleted)
	}

	// Expired artifact must be gone.
	var count int
	if err := store.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM artifacts WHERE id = ?`, expiredID,
	).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 0 {
		t.Error("expired artifact still present after cleanup")
	}

	// Future artifact must remain.
	if err := store.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM artifacts WHERE id = ?`, futureID,
	).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Error("future artifact was incorrectly deleted")
	}

	// A retention.cleanup event must have been emitted.
	events, err := store.QueryAll(ctx)
	if err != nil {
		t.Fatalf("QueryAll: %v", err)
	}
	var found bool
	for _, e := range events {
		if e.EventType == "retention.cleanup" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected retention.cleanup event, none found")
	}
}

func TestEventStore_CleanupExpiredIsIdempotent(t *testing.T) {
	t.Parallel()

	store, err := NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	result, err := store.CleanupExpired(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("first CleanupExpired: %v", err)
	}
	if result.ArtifactsDeleted != 0 {
		t.Errorf("expected 0 on empty store, got %d", result.ArtifactsDeleted)
	}

	result2, err := store.CleanupExpired(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("second CleanupExpired: %v", err)
	}
	if result2.ArtifactsDeleted != 0 {
		t.Errorf("expected 0 on second call, got %d", result2.ArtifactsDeleted)
	}
}

func TestEventStore_StoreAndGetArtifact(t *testing.T) {
	t.Parallel()

	store, err := NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// Pre-create parent rows required by FK constraints.
	if _, err := store.db.ExecContext(ctx,
		`INSERT INTO sessions (id, repo_path, trust_tier, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"sess-001", "/repo", "trusted", "active", now.Format(time.RFC3339), now.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := store.db.ExecContext(ctx,
		`INSERT INTO turns (id, session_id, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"turn-001", "sess-001", "active", now.Format(time.RFC3339), now.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("insert turn: %v", err)
	}

	artifact := ArtifactRecord{
		ID:          "art-001",
		SessionID:   "sess-001",
		TurnID:      "turn-001",
		Kind:        "log",
		StoragePath: "/artifacts/sess-001/turn-001/output.log",
		SHA256:      "abc123",
		CreatedAt:   now,
	}

	if err := store.StoreArtifact(ctx, artifact); err != nil {
		t.Fatalf("StoreArtifact: %v", err)
	}

	got, err := store.GetArtifact(ctx, "art-001")
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}

	if got.ID != artifact.ID {
		t.Errorf("ID: got %q, want %q", got.ID, artifact.ID)
	}
	if got.SHA256 != artifact.SHA256 {
		t.Errorf("SHA256: got %q, want %q", got.SHA256, artifact.SHA256)
	}
	if got.StoragePath != artifact.StoragePath {
		t.Errorf("StoragePath: got %q, want %q", got.StoragePath, artifact.StoragePath)
	}
	if got.SessionID != artifact.SessionID {
		t.Errorf("SessionID: got %q, want %q", got.SessionID, artifact.SessionID)
	}
	if got.TurnID != artifact.TurnID {
		t.Errorf("TurnID: got %q, want %q", got.TurnID, artifact.TurnID)
	}
	if got.Kind != artifact.Kind {
		t.Errorf("Kind: got %q, want %q", got.Kind, artifact.Kind)
	}
	if !got.CreatedAt.Equal(artifact.CreatedAt) {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, artifact.CreatedAt)
	}
}

func TestEventStore_GetArtifactNotFound(t *testing.T) {
	t.Parallel()

	store, err := NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	_, err = store.GetArtifact(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing artifact, got nil")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows in error chain, got: %v", err)
	}
}

func TestEventStore_ListArtifactsBySession(t *testing.T) {
	t.Parallel()

	store, err := NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// Pre-create parent sessions required by FK constraints.
	for _, sessID := range []string{"sess-x", "sess-y"} {
		if _, err := store.db.ExecContext(ctx,
			`INSERT INTO sessions (id, repo_path, trust_tier, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			sessID, "/repo", "trusted", "active", now.Format(time.RFC3339), now.Format(time.RFC3339),
		); err != nil {
			t.Fatalf("insert session %s: %v", sessID, err)
		}
	}

	for i, id := range []string{"art-a", "art-b"} {
		if err := store.StoreArtifact(ctx, ArtifactRecord{
			ID: id, SessionID: "sess-x", Kind: "log",
			StoragePath: fmt.Sprintf("/a/%d", i), SHA256: fmt.Sprintf("hash%d", i),
			CreatedAt: now,
		}); err != nil {
			t.Fatalf("StoreArtifact %s: %v", id, err)
		}
	}
	// Artifact for different session — must not appear.
	if err := store.StoreArtifact(ctx, ArtifactRecord{
		ID: "art-other", SessionID: "sess-y", Kind: "log",
		StoragePath: "/other", SHA256: "hashother", CreatedAt: now,
	}); err != nil {
		t.Fatalf("StoreArtifact other: %v", err)
	}

	artifacts, err := store.ListArtifactsBySession(ctx, "sess-x")
	if err != nil {
		t.Fatalf("ListArtifactsBySession: %v", err)
	}
	if len(artifacts) != 2 {
		t.Errorf("expected 2 artifacts, got %d", len(artifacts))
	}
}

func TestEventStore_StoreArtifactHashIntegrity(t *testing.T) {
	t.Parallel()

	store, err := NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	original := ArtifactRecord{
		ID: "art-hash", Kind: "log",
		StoragePath: "/a/b/c", SHA256: "deadbeef", CreatedAt: now,
	}
	if err := store.StoreArtifact(ctx, original); err != nil {
		t.Fatalf("StoreArtifact: %v", err)
	}

	got, err := store.GetArtifact(ctx, "art-hash")
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if got.SHA256 != original.SHA256 {
		t.Errorf("hash mismatch: stored %q, retrieved %q", original.SHA256, got.SHA256)
	}
}

func schemaObjectExists(t *testing.T, store *EventStore, objectType, name string) bool {
	t.Helper()

	var exists int
	err := store.db.QueryRow(
		"SELECT COUNT(1) FROM sqlite_master WHERE type = ? AND name = ?",
		objectType,
		name,
	).Scan(&exists)
	if err != nil {
		t.Fatalf("failed to query sqlite_master for %s %s: %v", objectType, name, err)
	}

	return exists == 1
}
