package audit

import (
	"path/filepath"
	"testing"
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
