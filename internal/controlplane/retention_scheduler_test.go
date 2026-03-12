package controlplane

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
)

func TestRetentionCleanupSchedulerDeletesExpiredArtifacts_Bead_l3d_10_4(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := eventStore.Close(); closeErr != nil {
			t.Fatalf("failed to close event store: %v", closeErr)
		}
	})

	expiresAt := time.Now().UTC().Add(-time.Minute)
	if err := eventStore.StoreArtifact(context.Background(), audit.ArtifactRecord{
		ID:          "expired-artifact",
		SessionID:   "",
		Kind:        "log",
		StoragePath: "/tmp/expired.log",
		SHA256:      "hash-expired",
		ExpiresAt:   &expiresAt,
		CreatedAt:   time.Now().UTC().Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("failed to store expired artifact: %v", err)
	}

	srv := &Server{eventStore: eventStore}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv.startRetentionCleanupScheduler(ctx, 10*time.Millisecond)

	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		_, getErr := eventStore.GetArtifact(context.Background(), "expired-artifact")
		events, queryErr := eventStore.QueryAll(context.Background())
		if queryErr != nil {
			t.Fatalf("failed to query events: %v", queryErr)
		}
		foundCleanup := false
		for _, event := range events {
			if event.EventType == "retention.cleanup" {
				foundCleanup = true
				break
			}
		}
		if getErr != nil && foundCleanup {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}

	t.Fatal("retention cleanup scheduler did not delete expired artifact and emit retention.cleanup event")
}
