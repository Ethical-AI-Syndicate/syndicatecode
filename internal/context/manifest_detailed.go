package context

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
)

type InclusionState string

const (
	InclusionIncluded InclusionState = "included"
	InclusionExcluded InclusionState = "excluded"
)

type SensitivityClass string

const (
	SensitivityNormal      SensitivityClass = "normal"
	SensitivityRestricted  SensitivityClass = "restricted"
	SensitivitySecretClass SensitivityClass = "secret_candidate"
)

type FreshnessState string

const (
	FreshnessFresh   FreshnessState = "fresh"
	FreshnessStale   FreshnessState = "stale"
	FreshnessUnknown FreshnessState = "unknown"
)

type ManifestEntry struct {
	Fragment        ContextFragment  `json:"fragment"`
	Inclusion       InclusionState   `json:"inclusion"`
	InclusionReason string           `json:"inclusion_reason"`
	Sensitivity     SensitivityClass `json:"sensitivity"`
	Freshness       FreshnessState   `json:"freshness"`
}

type ManifestConflict struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Resolution  string `json:"resolution"`
}

type DetailedManifest struct {
	TurnID    string             `json:"turn_id"`
	Entries   []ManifestEntry    `json:"entries"`
	Conflicts []ManifestConflict `json:"conflicts"`
}

type DetailedContextManifest struct {
	eventStore *audit.EventStore
}

func NewDetailedContextManifest(eventStore *audit.EventStore) *DetailedContextManifest {
	return &DetailedContextManifest{eventStore: eventStore}
}

func (m *DetailedContextManifest) RecordDetailed(ctx context.Context, sessionID, turnID string, entries []ManifestEntry, conflicts []ManifestConflict) error {
	now := time.Now().UTC()

	for _, entry := range entries {
		payload, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("failed to marshal manifest entry: %w", err)
		}
		event := audit.Event{
			ID:            uuid.New().String(),
			SessionID:     sessionID,
			TurnID:        turnID,
			EventType:     "context_manifest_entry",
			Actor:         "system",
			Timestamp:     now,
			PolicyVersion: "1.0.0",
			Payload:       payload,
		}
		if err := m.eventStore.Append(ctx, event); err != nil {
			return fmt.Errorf("failed to append manifest entry event: %w", err)
		}
	}

	for _, conflict := range conflicts {
		payload, err := json.Marshal(conflict)
		if err != nil {
			return fmt.Errorf("failed to marshal manifest conflict: %w", err)
		}
		event := audit.Event{
			ID:            uuid.New().String(),
			SessionID:     sessionID,
			TurnID:        turnID,
			EventType:     "context_manifest_conflict",
			Actor:         "system",
			Timestamp:     now,
			PolicyVersion: "1.0.0",
			Payload:       payload,
		}
		if err := m.eventStore.Append(ctx, event); err != nil {
			return fmt.Errorf("failed to append manifest conflict event: %w", err)
		}
	}

	return nil
}

func (m *DetailedContextManifest) GetDetailed(ctx context.Context, turnID string) (DetailedManifest, error) {
	allEvents, err := m.eventStore.QueryAll(ctx)
	if err != nil {
		return DetailedManifest{}, fmt.Errorf("failed to query events: %w", err)
	}

	manifest := DetailedManifest{TurnID: turnID, Entries: make([]ManifestEntry, 0), Conflicts: make([]ManifestConflict, 0)}
	for _, event := range allEvents {
		if event.TurnID != turnID {
			continue
		}
		switch event.EventType {
		case "context_manifest_entry":
			var entry ManifestEntry
			if err := json.Unmarshal(event.Payload, &entry); err != nil {
				return DetailedManifest{}, fmt.Errorf("failed to decode manifest entry: %w", err)
			}
			manifest.Entries = append(manifest.Entries, entry)
		case "context_manifest_conflict":
			var conflict ManifestConflict
			if err := json.Unmarshal(event.Payload, &conflict); err != nil {
				return DetailedManifest{}, fmt.Errorf("failed to decode manifest conflict: %w", err)
			}
			manifest.Conflicts = append(manifest.Conflicts, conflict)
		}
	}

	sort.Slice(manifest.Entries, func(i, j int) bool {
		return manifest.Entries[i].Fragment.SourceRef < manifest.Entries[j].Fragment.SourceRef
	})
	sort.Slice(manifest.Conflicts, func(i, j int) bool {
		return manifest.Conflicts[i].Key < manifest.Conflicts[j].Key
	})

	return manifest, nil
}
