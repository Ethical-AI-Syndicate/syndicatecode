package audit

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEventStore_StoreAndLoadArtifactHashIntegrity(t *testing.T) {
	store, err := NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	defer func() { _ = store.Close() }()

	input := ArtifactInput{
		SessionID:      "s-1",
		TurnID:         "t-1",
		ArtifactType:   "tool_output",
		ContentType:    "text/plain",
		Content:        []byte("sensitive artifact payload"),
		RedactionState: "masked",
	}

	record, err := store.StoreArtifact(context.Background(), input)
	if err != nil {
		t.Fatalf("StoreArtifact returned error: %v", err)
	}

	loaded, loadedRecord, err := store.LoadArtifact(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("LoadArtifact returned error: %v", err)
	}

	if !bytes.Equal(loaded, input.Content) {
		t.Fatalf("expected loaded content to match stored content")
	}
	if loadedRecord.SHA256 == "" {
		t.Fatalf("expected artifact SHA256 to be populated")
	}
}

func TestEventStore_LoadArtifactMissingFileFailsConsistencyCheck(t *testing.T) {
	store, err := NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	defer func() { _ = store.Close() }()

	record, err := store.StoreArtifact(context.Background(), ArtifactInput{
		SessionID:      "s-1",
		TurnID:         "t-2",
		ArtifactType:   "context_fragment",
		ContentType:    "text/plain",
		Content:        []byte("hello"),
		RedactionState: "none",
	})
	if err != nil {
		t.Fatalf("StoreArtifact returned error: %v", err)
	}

	if err := os.Remove(record.URI); err != nil {
		t.Fatalf("failed to remove artifact file: %v", err)
	}

	if _, _, err := store.LoadArtifact(context.Background(), record.ID); err == nil {
		t.Fatalf("expected LoadArtifact to fail when blob file is missing")
	}
}

func TestEventStore_CleanupExpiredArtifactsRemovesRowsAndFiles(t *testing.T) {
	store, err := NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	defer func() { _ = store.Close() }()

	expiredAt := time.Now().UTC().Add(-1 * time.Minute)
	record, err := store.StoreArtifact(context.Background(), ArtifactInput{
		SessionID:      "s-1",
		TurnID:         "t-3",
		ArtifactType:   "tool_output",
		ContentType:    "application/json",
		Content:        []byte(`{"k":"v"}`),
		RedactionState: "masked",
		ExpiresAt:      &expiredAt,
	})
	if err != nil {
		t.Fatalf("StoreArtifact returned error: %v", err)
	}

	deleted, err := store.CleanupExpiredArtifacts(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("CleanupExpiredArtifacts returned error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected one expired artifact to be deleted, got %d", deleted)
	}

	if _, _, err := store.LoadArtifact(context.Background(), record.ID); err == nil {
		t.Fatalf("expected artifact to be removed after cleanup")
	}
}
