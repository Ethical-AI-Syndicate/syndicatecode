package audit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEventStore_RunRetentionDeletesExpiredRowsAndBlobs(t *testing.T) {
	store, err := NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	defer func() { _ = store.Close() }()

	now := time.Now().UTC()
	oldTimestamp := now.Add(-2 * time.Hour)
	newTimestamp := now.Add(-5 * time.Minute)

	if err := store.Append(context.Background(), Event{
		ID:        uuid.NewString(),
		SessionID: "s-1",
		Timestamp: oldTimestamp,
		EventType: "turn_started",
		Actor:     "user",
		Payload:   json.RawMessage(`{"old":true}`),
	}); err != nil {
		t.Fatalf("failed to append old event: %v", err)
	}

	if err := store.Append(context.Background(), Event{
		ID:        uuid.NewString(),
		SessionID: "s-1",
		Timestamp: newTimestamp,
		EventType: "turn_completed",
		Actor:     "system",
		Payload:   json.RawMessage(`{"new":true}`),
	}); err != nil {
		t.Fatalf("failed to append new event: %v", err)
	}

	blobPath, blobID, err := store.InsertArtifactBlobForTest(context.Background(), ArtifactBlobInput{
		SessionID:      "s-1",
		TurnID:         "t-1",
		Content:        []byte("sensitive artifact"),
		RedactionState: "masked",
		ExpiresAt:      now.Add(-30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("failed to insert old artifact blob: %v", err)
	}

	report, err := store.RunRetention(context.Background(), RetentionPolicy{
		Now:               now,
		EventRetention:    time.Hour,
		ArtifactTombstone: true,
	})
	if err != nil {
		t.Fatalf("RunRetention returned error: %v", err)
	}

	if report.DeletedEvents != 1 {
		t.Fatalf("expected 1 deleted event, got %d", report.DeletedEvents)
	}
	if report.CleanedArtifacts != 1 {
		t.Fatalf("expected 1 cleaned artifact, got %d", report.CleanedArtifacts)
	}

	if _, statErr := os.Stat(blobPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected blob file to be removed, stat err: %v", statErr)
	}

	meta, err := store.GetArtifactBlob(context.Background(), blobID)
	if err != nil {
		t.Fatalf("failed to query artifact metadata: %v", err)
	}
	if meta.TombstonedAt == nil {
		t.Fatalf("expected artifact metadata to be tombstoned")
	}

	events, err := store.QueryAll(context.Background())
	if err != nil {
		t.Fatalf("failed to query all events: %v", err)
	}

	foundRetentionEvent := false
	for _, event := range events {
		if event.EventType == "retention_cleanup" {
			foundRetentionEvent = true
			if string(event.Payload) == "" {
				t.Fatalf("expected retention event payload to include summary")
			}
		}
		if event.EventType == "turn_started" {
			t.Fatalf("expected expired event to be removed")
		}
	}
	if !foundRetentionEvent {
		t.Fatalf("expected retention_cleanup event to be recorded")
	}
}
