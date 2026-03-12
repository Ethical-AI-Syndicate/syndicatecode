package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

func TestHandleSessionByID_EventsReturnsReplayStream(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := eventStore.Close(); closeErr != nil {
			t.Fatalf("failed to close event store: %v", closeErr)
		}
	})

	sessionMgr := session.NewManager(eventStore)
	created, err := sessionMgr.Create(context.Background(), "/tmp/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if err := eventStore.Append(context.Background(), audit.Event{
		ID:        uuid.NewString(),
		SessionID: created.ID,
		Timestamp: time.Now().UTC(),
		EventType: "turn_completed",
		Actor:     "system",
		Payload:   json.RawMessage(`{"turn_id":"t1"}`),
	}); err != nil {
		t.Fatalf("failed to append replay event: %v", err)
	}

	server := &Server{
		sessionMgr: sessionMgr,
		eventStore: eventStore,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+created.ID+"/events", nil)
	rec := httptest.NewRecorder()

	server.handleSessionByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var events []audit.Event
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("failed to decode replay events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events in replay stream, got %d", len(events))
	}
	if events[0].EventType != "session_started" {
		t.Fatalf("expected first event to be session_started, got %s", events[0].EventType)
	}
	if events[1].EventType != "turn_completed" {
		t.Fatalf("expected second event to be turn_completed, got %s", events[1].EventType)
	}
}

func TestHandleSessionByID_EventsUnknownSessionReturnsNotFound(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := eventStore.Close(); closeErr != nil {
			t.Fatalf("failed to close event store: %v", closeErr)
		}
	})

	server := &Server{eventStore: eventStore}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/missing/events", nil)
	rec := httptest.NewRecorder()

	server.handleSessionByID(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleSessionByID_EventsCanBeFilteredByType(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := eventStore.Close(); closeErr != nil {
			t.Fatalf("failed to close event store: %v", closeErr)
		}
	})

	sessionMgr := session.NewManager(eventStore)
	created, err := sessionMgr.Create(context.Background(), "/tmp/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if err := eventStore.Append(context.Background(), audit.Event{
		ID:        uuid.NewString(),
		SessionID: created.ID,
		Timestamp: time.Now().UTC(),
		EventType: "mcp.call",
		Actor:     "controlplane",
		Payload:   json.RawMessage(`{"server_id":"inventory.remote"}`),
	}); err != nil {
		t.Fatalf("failed to append mcp.call event: %v", err)
	}

	if err := eventStore.Append(context.Background(), audit.Event{
		ID:        uuid.NewString(),
		SessionID: created.ID,
		Timestamp: time.Now().UTC(),
		EventType: "tool_output_redaction",
		Actor:     "system",
		Payload:   json.RawMessage(`{"notice_count":1}`),
	}); err != nil {
		t.Fatalf("failed to append tool_output_redaction event: %v", err)
	}

	server := &Server{sessionMgr: sessionMgr, eventStore: eventStore}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+created.ID+"/events?event_type=mcp.call", nil)
	rec := httptest.NewRecorder()

	server.handleSessionByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var events []audit.Event
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("failed to decode filtered events: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 filtered event, got %d", len(events))
	}
	if events[0].EventType != "mcp.call" {
		t.Fatalf("expected mcp.call event, got %s", events[0].EventType)
	}
}

func TestHandleSessionByID_EventsFilterWithNoMatchesReturnsEmptyList(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := eventStore.Close(); closeErr != nil {
			t.Fatalf("failed to close event store: %v", closeErr)
		}
	})

	sessionMgr := session.NewManager(eventStore)
	created, err := sessionMgr.Create(context.Background(), "/tmp/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	server := &Server{sessionMgr: sessionMgr, eventStore: eventStore}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+created.ID+"/events?event_type=mcp.call", nil)
	rec := httptest.NewRecorder()

	server.handleSessionByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var events []audit.Event
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("failed to decode empty filtered events response: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected empty filtered event list, got %d entries", len(events))
	}
}

func TestHandleSessionByID_EventsDeterministicOrder_Bead_l3d_2_2(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := eventStore.Close(); closeErr != nil {
			t.Fatalf("failed to close event store: %v", closeErr)
		}
	})

	sessionMgr := session.NewManager(eventStore)
	created, err := sessionMgr.Create(context.Background(), "/tmp/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	timestamp := time.Now().UTC().Add(50 * time.Millisecond).Truncate(time.Second)
	if err := eventStore.Append(context.Background(), audit.Event{
		ID:        "b-event",
		SessionID: created.ID,
		Timestamp: timestamp,
		EventType: "turn_completed",
		Actor:     "system",
		Payload:   json.RawMessage(`{"seq":2}`),
	}); err != nil {
		t.Fatalf("failed to append first event: %v", err)
	}
	if err := eventStore.Append(context.Background(), audit.Event{
		ID:        "a-event",
		SessionID: created.ID,
		Timestamp: timestamp,
		EventType: "turn_completed",
		Actor:     "system",
		Payload:   json.RawMessage(`{"seq":1}`),
	}); err != nil {
		t.Fatalf("failed to append second event: %v", err)
	}

	server := &Server{sessionMgr: sessionMgr, eventStore: eventStore}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+created.ID+"/events?event_type=turn_completed", nil)
	rec := httptest.NewRecorder()
	server.handleSessionByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var events []audit.Event
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("failed to decode events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 filtered events, got %d", len(events))
	}

	if events[0].ID != "a-event" || events[1].ID != "b-event" {
		t.Fatalf("expected deterministic ordering by (timestamp,id): [a-event b-event], got [%s %s]", events[0].ID, events[1].ID)
	}
}

func TestSessionExport_Bead_l3d_X_1(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := eventStore.Close(); closeErr != nil {
			t.Fatalf("failed to close event store: %v", closeErr)
		}
	})

	sessionMgr := session.NewManager(eventStore)
	created, err := sessionMgr.Create(context.Background(), "/tmp/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if err := eventStore.Append(context.Background(), audit.Event{
		ID:        uuid.NewString(),
		SessionID: created.ID,
		Timestamp: time.Now().UTC(),
		EventType: audit.EventToolRedaction,
		Actor:     "system",
		Payload:   json.RawMessage(`{"notice_count":1}`),
	}); err != nil {
		t.Fatalf("failed to append redaction event: %v", err)
	}

	server := &Server{sessionMgr: sessionMgr, eventStore: eventStore}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+created.ID+"/export", nil)
	rec := httptest.NewRecorder()

	server.handleSessionByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var export map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &export); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if export["schema_version"] != "1" {
		t.Errorf("expected schema_version 1, got %v", export["schema_version"])
	}
	if export["session"] == nil {
		t.Error("expected session field in export")
	}
	if export["events"] == nil {
		t.Error("expected events field in export")
	}

	redactionSummary, ok := export["redaction_summary"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected redaction_summary object, got %T", export["redaction_summary"])
	}
	if redactionSummary["reason"] == "" {
		t.Fatal("expected redaction_summary.reason to be present")
	}

	events, ok := export["events"].([]interface{})
	if !ok || len(events) == 0 {
		t.Fatalf("expected non-empty events array, got %#v", export["events"])
	}
	markerFound := false
	for _, event := range events {
		typed, eventOK := event.(map[string]interface{})
		if !eventOK {
			continue
		}
		if typed["event_type"] == "[redacted]" {
			markerFound = true
			break
		}
	}
	if !markerFound {
		t.Fatal("expected tool_output_redaction marker event in export")
	}
}

func TestSessionExport_NotFound_Bead_l3d_X_1(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := eventStore.Close(); closeErr != nil {
			t.Fatalf("failed to close event store: %v", closeErr)
		}
	})

	server := &Server{sessionMgr: session.NewManager(eventStore), eventStore: eventStore}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/nonexistent/export", nil)
	rec := httptest.NewRecorder()

	server.handleSessionByID(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestSessionExport_AppliesRedactionPolicyToEventPayloads(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := eventStore.Close(); closeErr != nil {
			t.Fatalf("failed to close event store: %v", closeErr)
		}
	})

	sessionMgr := session.NewManager(eventStore)
	created, err := sessionMgr.Create(context.Background(), "/tmp/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if err := eventStore.Append(context.Background(), audit.Event{
		ID:        uuid.NewString(),
		SessionID: created.ID,
		Timestamp: time.Now().UTC(),
		EventType: "tool_result",
		Actor:     "system",
		Payload:   json.RawMessage(`{"token":"AKIA1234567890ABCDEF"}`),
	}); err != nil {
		t.Fatalf("failed to append event: %v", err)
	}

	server := &Server{sessionMgr: sessionMgr, eventStore: eventStore}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+created.ID+"/export", nil)
	rec := httptest.NewRecorder()

	server.handleSessionByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var export struct {
		Events  []audit.Event `json:"events"`
		Summary struct {
			Reason string `json:"reason"`
		} `json:"redaction_summary"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &export); err != nil {
		t.Fatalf("decode export: %v", err)
	}

	if len(export.Events) == 0 {
		t.Fatal("expected exported events")
	}

	var redactedPayload string
	for _, event := range export.Events {
		if event.EventType == "tool_result" {
			redactedPayload = string(event.Payload)
			break
		}
	}
	if redactedPayload == "" {
		t.Fatal("expected tool_result event in export")
	}
	if strings.Contains(redactedPayload, "AKIA1234567890ABCDEF") {
		t.Fatalf("expected secret to be redacted from payload, got %s", redactedPayload)
	}
	if !strings.Contains(redactedPayload, "[DENIED]") {
		t.Fatalf("expected denied marker in redacted payload, got %s", redactedPayload)
	}
	if export.Summary.Reason == "" {
		t.Fatal("expected redaction summary reason")
	}
}

func TestSessionExport_RedactionFlowFromToolExecutionUsesSessionScopedAudit(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := eventStore.Close(); closeErr != nil {
			t.Fatalf("failed to close event store: %v", closeErr)
		}
	})

	sessionMgr := session.NewManager(eventStore)
	created, err := sessionMgr.Create(context.Background(), "/tmp/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	registry := tools.NewRegistry()
	if err := registry.Register(tools.ToolDefinition{
		Name:             "echo",
		Version:          "1",
		SideEffect:       tools.SideEffectNone,
		ApprovalRequired: false,
		InputSchema: map[string]tools.FieldSchema{
			"message": {Type: "string"},
		},
		OutputSchema: map[string]tools.FieldSchema{
			"output": {Type: "string"},
		},
		Limits: tools.ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 1024},
	}); err != nil {
		t.Fatalf("failed to register echo tool: %v", err)
	}

	executor := tools.NewExecutor(registry, nil)
	executor.RegisterHandler("echo", tools.EchoHandler())

	server := &Server{
		sessionMgr:   sessionMgr,
		eventStore:   eventStore,
		toolRegistry: registry,
		toolExecutor: executor,
		httpServer:   &http.Server{},
	}

	executeBody := strings.NewReader(`{"tool_name":"echo","session_id":"` + created.ID + `","input":{"message":"AKIA1234567890ABCDEF"}}`)
	executeReq := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", executeBody)
	executeRec := httptest.NewRecorder()
	server.handleToolExecute(executeRec, executeReq)
	if executeRec.Code != http.StatusOK {
		t.Fatalf("expected tool execute status 200, got %d", executeRec.Code)
	}

	sessionEvents, err := eventStore.QueryBySession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("failed querying session events: %v", err)
	}
	redactionEventFound := false
	for _, event := range sessionEvents {
		if event.EventType == audit.EventToolRedaction {
			redactionEventFound = true
			break
		}
	}
	if !redactionEventFound {
		t.Fatal("expected tool_output_redaction event in originating session")
	}

	systemEvents, err := eventStore.QueryBySession(context.Background(), redactionAuditSessionID)
	if err != nil {
		t.Fatalf("failed querying system redaction session: %v", err)
	}
	for _, event := range systemEvents {
		if event.EventType == audit.EventToolRedaction {
			t.Fatal("expected no fallback redaction event when session_id is provided")
		}
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+created.ID+"/export", nil)
	exportRec := httptest.NewRecorder()
	server.handleSessionByID(exportRec, exportReq)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("expected export status 200, got %d", exportRec.Code)
	}

	var export struct {
		Events  []audit.Event `json:"events"`
		Summary struct {
			Reason string `json:"reason"`
		} `json:"redaction_summary"`
	}
	if err := json.Unmarshal(exportRec.Body.Bytes(), &export); err != nil {
		t.Fatalf("decode export: %v", err)
	}

	if export.Summary.Reason == "" {
		t.Fatal("expected non-empty redaction_summary.reason")
	}

	markerFound := false
	for _, event := range export.Events {
		if event.EventType == "[redacted]" {
			markerFound = true
			break
		}
	}
	if !markerFound {
		t.Fatal("expected [redacted] marker event in export")
	}
}

func TestSessionExport_IncludeArtifactsRequiresOperatorRole(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := eventStore.Close(); closeErr != nil {
			t.Fatalf("failed to close event store: %v", closeErr)
		}
	})

	sessionMgr := session.NewManager(eventStore)
	created, err := sessionMgr.Create(context.Background(), "/tmp/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	server := &Server{sessionMgr: sessionMgr, eventStore: eventStore}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+created.ID+"/export?include_artifacts=true", nil)
	rec := httptest.NewRecorder()

	server.handleSessionByID(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestSessionExport_IncludeArtifactsReturnsArtifactsForOperator(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := eventStore.Close(); closeErr != nil {
			t.Fatalf("failed to close event store: %v", closeErr)
		}
	})

	sessionMgr := session.NewManager(eventStore)
	created, err := sessionMgr.Create(context.Background(), "/tmp/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	server := &Server{sessionMgr: sessionMgr, eventStore: eventStore}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+created.ID+"/export?include_artifacts=true", nil)
	req = withOperatorRole(req)
	rec := httptest.NewRecorder()

	server.handleSessionByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var export map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &export); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	artifacts, hasArtifacts := export["artifacts"]
	if !hasArtifacts {
		t.Fatal("expected artifacts field to be present when requested")
	}
	if artifacts != nil {
		typed, ok := artifacts.([]interface{})
		if !ok {
			t.Fatalf("expected artifacts to be array or null, got %T", artifacts)
		}
		if len(typed) != 0 {
			t.Fatalf("expected empty artifacts array in fresh session export, got %d", len(typed))
		}
	}
}
