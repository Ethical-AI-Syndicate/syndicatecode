package context

import (
	"context"
	"testing"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
)

func TestRecordDetailedManifestPersistsFreshnessAndSensitivity(t *testing.T) {
	t.Parallel()

	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	m := NewDetailedContextManifest(eventStore)

	entries := []ManifestEntry{
		{
			Fragment:        ContextFragment{SourceType: "file", SourceRef: "internal/context/context.go", Content: "...", TokenCount: 120},
			Inclusion:       InclusionIncluded,
			InclusionReason: "explicit_target",
			Sensitivity:     SensitivityNormal,
			Freshness:       FreshnessFresh,
		},
	}

	if err := m.RecordDetailed(context.Background(), "s1", "t1", entries, nil); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	manifest, err := m.GetDetailed(context.Background(), "t1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(manifest.Entries) != 1 {
		t.Fatalf("expected one manifest entry, got %d", len(manifest.Entries))
	}
	if manifest.Entries[0].Freshness != FreshnessFresh {
		t.Fatalf("expected freshness to be preserved")
	}
	if manifest.Entries[0].Sensitivity != SensitivityNormal {
		t.Fatalf("expected sensitivity to be preserved")
	}
}

func TestRecordDetailedManifestPersistsConflicts(t *testing.T) {
	t.Parallel()

	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	m := NewDetailedContextManifest(eventStore)
	conflicts := []ManifestConflict{{
		Key:         "policy_instructions",
		Description: "repo instruction conflicts with control policy",
		Resolution:  "control_policy_wins",
	}}

	if err := m.RecordDetailed(context.Background(), "s1", "t2", nil, conflicts); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	manifest, err := m.GetDetailed(context.Background(), "t2")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(manifest.Conflicts) != 1 {
		t.Fatalf("expected one conflict, got %d", len(manifest.Conflicts))
	}
	if manifest.Conflicts[0].Resolution != "control_policy_wins" {
		t.Fatalf("expected conflict resolution to be preserved")
	}
}
