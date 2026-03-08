package audit

import (
	"path/filepath"
	"testing"
)

func TestNewEventStoreAppliesCoreSchema(t *testing.T) {
	store, err := NewEventStore(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatalf("NewEventStore returned error: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	expectedTables := []string{
		"schema_migrations",
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

	for _, tableName := range expectedTables {
		if !tableExists(t, store, tableName) {
			t.Fatalf("expected table %q to exist", tableName)
		}
	}

	expectedIndexes := []string{
		"idx_sessions_status",
		"idx_turns_session_created",
		"idx_approvals_session_created",
		"idx_tool_invocations_turn_started",
		"idx_model_invocations_turn_started",
		"idx_context_fragments_turn_retrieved",
		"idx_patch_proposals_turn_created",
		"idx_file_mutations_turn_created",
		"idx_artifacts_turn_created",
		"idx_events_session",
		"idx_events_type",
	}

	for _, indexName := range expectedIndexes {
		if !indexExists(t, store, indexName) {
			t.Fatalf("expected index %q to exist", indexName)
		}
	}
}

func TestNewEventStoreSetsSQLiteSafetyPragmas(t *testing.T) {
	store, err := NewEventStore(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatalf("NewEventStore returned error: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	if got := pragmaValue(t, store, "foreign_keys"); got != "1" {
		t.Fatalf("expected PRAGMA foreign_keys=1, got %q", got)
	}

	if got := pragmaValue(t, store, "journal_mode"); got != "wal" {
		t.Fatalf("expected PRAGMA journal_mode=wal, got %q", got)
	}
}

func TestNewEventStoreMigrationIsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "audit.db")

	store, err := NewEventStore(dbPath)
	if err != nil {
		t.Fatalf("NewEventStore returned error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	store, err = NewEventStore(dbPath)
	if err != nil {
		t.Fatalf("NewEventStore second open returned error: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	if !tableExists(t, store, "events") {
		t.Fatalf("expected events table to exist after reopening database")
	}
}

func tableExists(t *testing.T, store *EventStore, tableName string) bool {
	t.Helper()

	var count int
	err := store.db.QueryRow(`SELECT count(1) FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&count)
	if err != nil {
		t.Fatalf("failed to check table %q: %v", tableName, err)
	}

	return count == 1
}

func indexExists(t *testing.T, store *EventStore, indexName string) bool {
	t.Helper()

	var count int
	err := store.db.QueryRow(`SELECT count(1) FROM sqlite_master WHERE type = 'index' AND name = ?`, indexName).Scan(&count)
	if err != nil {
		t.Fatalf("failed to check index %q: %v", indexName, err)
	}

	return count == 1
}

func pragmaValue(t *testing.T, store *EventStore, pragmaName string) string {
	t.Helper()

	query := "PRAGMA " + pragmaName
	var value string
	if err := store.db.QueryRow(query).Scan(&value); err != nil {
		t.Fatalf("failed to read PRAGMA %s: %v", pragmaName, err)
	}

	return value
}
