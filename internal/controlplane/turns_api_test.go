package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/agent"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	ctxmgr "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/context"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/secrets"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

func TestCreateTurn_SessionScoped_Bead_l3d_2_1(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })
	sessionMgr := session.NewManager(eventStore)
	secretsPolicy := secretsRedactionPolicyForTest{executor: secrets.NewPolicyExecutor(nil)}
	turnMgr := ctxmgr.NewTurnManagerWithPolicy(eventStore, sessionMgr, secretsPolicy)

	server := &Server{
		turnMgr:     turnMgr,
		sessionMgr:  sessionMgr,
		eventStore:  eventStore,
		approvalMgr: NewApprovalManager(10 * time.Minute),
	}

	ctx := context.Background()
	sess, err := sessionMgr.Create(ctx, "test-repo", "untrusted")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	tests := []struct {
		name           string
		sessionID      string
		message        string
		files          []string
		expectedStatus int
	}{
		{
			name:           "success case creates turn",
			sessionID:      sess.ID,
			message:        "Refactor the auth service",
			files:          []string{"src/auth/service.go"},
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "validation error - empty message",
			sessionID:      sess.ID,
			message:        "",
			files:          []string{"src/auth/service.go"},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "not found - non-existent session",
			sessionID:      "non-existent-session-id",
			message:        "test",
			files:          nil,
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reqBody []byte
			if tt.message != "" || tt.files != nil {
				body := map[string]interface{}{
					"message": tt.message,
				}
				if tt.files != nil {
					body["files"] = tt.files
				}
				reqBody, _ = json.Marshal(body)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+tt.sessionID+"/turns", bytes.NewBuffer(reqBody))
			req = withOperatorRole(req.WithContext(ctx))
			w := httptest.NewRecorder()

			server.handleSessionTurns(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d. body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestCreateTurn_SessionScoped_ConflictOnActiveMutableTurn(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	secretsPolicy := secretsRedactionPolicyForTest{executor: secrets.NewPolicyExecutor(nil)}
	turnMgr := ctxmgr.NewTurnManagerWithPolicy(eventStore, sessionMgr, secretsPolicy)

	server := &Server{
		turnMgr:     turnMgr,
		sessionMgr:  sessionMgr,
		eventStore:  eventStore,
		approvalMgr: NewApprovalManager(10 * time.Minute),
	}

	ctx := context.Background()
	sess, err := sessionMgr.Create(ctx, "test-repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if _, err := turnMgr.Create(ctx, sess.ID, "first turn"); err != nil {
		t.Fatalf("failed to create initial active turn: %v", err)
	}

	reqBody, _ := json.Marshal(map[string]interface{}{"message": "second turn"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sess.ID+"/turns", bytes.NewBuffer(reqBody))
	req = withOperatorRole(req.WithContext(ctx))
	w := httptest.NewRecorder()

	server.handleSessionTurns(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d. body: %s", http.StatusConflict, w.Code, w.Body.String())
	}
}

func TestListTurns_SessionScoped_Bead_l3d_2_1(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })
	sessionMgr := session.NewManager(eventStore)
	secretsPolicy := secretsRedactionPolicyForTest{executor: secrets.NewPolicyExecutor(nil)}
	turnMgr := ctxmgr.NewTurnManagerWithPolicy(eventStore, sessionMgr, secretsPolicy)

	server := &Server{
		turnMgr:     turnMgr,
		sessionMgr:  sessionMgr,
		eventStore:  eventStore,
		approvalMgr: NewApprovalManager(10 * time.Minute),
	}

	ctx := context.Background()
	sess, err := sessionMgr.Create(ctx, "test-repo", "untrusted")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	_, err = turnMgr.Create(ctx, sess.ID, "First turn")
	if err != nil {
		t.Fatalf("failed to create turn: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sess.ID+"/turns", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleSessionTurns(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var turns []ctxmgr.Turn
	if err := json.Unmarshal(w.Body.Bytes(), &turns); err != nil {
		t.Fatalf("failed to unmarshal turns: %v", err)
	}

	if len(turns) != 1 {
		t.Errorf("expected 1 turn, got %d", len(turns))
	}
}

func TestGetTurnContext_SessionScoped_Bead_l3d_2_1(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })
	sessionMgr := session.NewManager(eventStore)
	secretsPolicy := secretsRedactionPolicyForTest{executor: secrets.NewPolicyExecutor(nil)}
	turnMgr := ctxmgr.NewTurnManagerWithPolicy(eventStore, sessionMgr, secretsPolicy)
	ctxManifest := ctxmgr.NewContextManifest(eventStore)

	server := &Server{
		turnMgr:     turnMgr,
		sessionMgr:  sessionMgr,
		eventStore:  eventStore,
		ctxManifest: ctxManifest,
		approvalMgr: NewApprovalManager(10 * time.Minute),
	}

	ctx := context.Background()
	sess, err := sessionMgr.Create(ctx, "test-repo", "untrusted")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	turn, err := turnMgr.Create(ctx, sess.ID, "Test message")
	if err != nil {
		t.Fatalf("failed to create turn: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sess.ID+"/turns/"+turn.ID+"/context", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	server.handleSessionTurnByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

type immediateModel struct{}

func (immediateModel) ModelID() string { return "test-model" }

func (immediateModel) Stream(_ context.Context, _ models.Params) (<-chan models.StreamEvent, error) {
	ch := make(chan models.StreamEvent, 2)
	ch <- models.TextDeltaEvent{Delta: "ok"}
	ch <- models.MessageDeltaEvent{StopReason: "end_turn", OutputTokens: 1}
	close(ch)
	return ch, nil
}

type approvalRequiredModel struct{}

func (approvalRequiredModel) ModelID() string { return "test-model" }

func (approvalRequiredModel) Stream(_ context.Context, _ models.Params) (<-chan models.StreamEvent, error) {
	ch := make(chan models.StreamEvent, 3)
	ch <- models.ToolUseStartEvent{ID: "tu-1", Name: "write_file"}
	ch <- models.ToolInputDeltaEvent{ID: "tu-1", Delta: `{"path":"/tmp/x.txt","content":"hello"}`}
	ch <- models.MessageDeltaEvent{StopReason: "tool_use", OutputTokens: 1}
	close(ch)
	return ch, nil
}

func TestTurnCreation_TriggersAgentRunner_Bead_l3d_X_1(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	secretsPolicy := secretsRedactionPolicyForTest{executor: secrets.NewPolicyExecutor(nil)}
	turnMgr := ctxmgr.NewTurnManagerWithPolicy(eventStore, sessionMgr, secretsPolicy)

	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, nil)
	b := newStreamBus()
	runner := agent.NewRunner(immediateModel{}, registry, executor, noopApprovalGate{}, busEmitter{bus: b}, agent.DefaultConfig("tier1"))

	server := &Server{
		turnMgr:      turnMgr,
		sessionMgr:   sessionMgr,
		eventStore:   eventStore,
		approvalMgr:  NewApprovalManager(10 * time.Minute),
		toolRegistry: registry,
		toolExecutor: executor,
		bus:          b,
		runner:       runner,
	}

	ctx := context.Background()
	sess, err := sessionMgr.Create(ctx, "test-repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	reqBody, _ := json.Marshal(map[string]interface{}{"message": "say hi"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sess.ID+"/turns", bytes.NewBuffer(reqBody))
	req = withOperatorRole(req)
	resp := httptest.NewRecorder()
	server.handleSessionTurns(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, resp.Code)
	}

	var createdTurn struct {
		ID string `json:"turn_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &createdTurn); err != nil {
		t.Fatalf("decode turn: %v", err)
	}
	if createdTurn.ID == "" {
		t.Fatal("expected non-empty turn_id")
	}

	deadline := time.Now().Add(2 * time.Second)
	foundTurnCompleted := false
	foundModelInvocationEvent := false
	for time.Now().Before(deadline) {
		events, qErr := eventStore.QueryBySession(ctx, sess.ID)
		if qErr != nil {
			t.Fatalf("query events: %v", qErr)
		}
		for _, e := range events {
			if e.EventType == string(agent.EventTurnCompleted) {
				foundTurnCompleted = true
			}
			if e.EventType == audit.EventModelInvoked {
				foundModelInvocationEvent = true
				var payload map[string]interface{}
				if err := json.Unmarshal(e.Payload, &payload); err != nil {
					t.Fatalf("decode model_invocation payload: %v", err)
				}
				if payload["session_id"] != sess.ID {
					t.Fatalf("expected payload session_id %s, got %v", sess.ID, payload["session_id"])
				}
				if payload["turn_id"] != createdTurn.ID {
					t.Fatalf("expected payload turn_id %s, got %v", createdTurn.ID, payload["turn_id"])
				}
				if payload["model"] != "test-model" {
					t.Fatalf("expected payload model test-model, got %v", payload["model"])
				}
				if payload["provider"] == nil || payload["provider"] == "" {
					t.Fatalf("expected non-empty payload provider, got %v", payload["provider"])
				}
			}
		}

		invocations, invErr := eventStore.QueryModelInvocationsBySession(ctx, sess.ID)
		if invErr != nil {
			t.Fatalf("query model invocations: %v", invErr)
		}
		if foundTurnCompleted && foundModelInvocationEvent && len(invocations) == 1 {
			invocation := invocations[0]
			if invocation.SessionID != sess.ID {
				t.Fatalf("expected invocation session_id %s, got %s", sess.ID, invocation.SessionID)
			}
			if invocation.TurnID != createdTurn.ID {
				t.Fatalf("expected invocation turn_id %s, got %s", createdTurn.ID, invocation.TurnID)
			}
			if invocation.Model != "test-model" {
				t.Fatalf("expected invocation model test-model, got %s", invocation.Model)
			}
			if invocation.Provider == "" {
				t.Fatal("expected invocation provider to be non-empty")
			}
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if !foundTurnCompleted {
		t.Fatal("expected turn_completed event emitted by agent runner")
	}
	if !foundModelInvocationEvent {
		t.Fatal("expected model_invocation audit event emitted by agent runner")
	}

	invocations, err := eventStore.QueryModelInvocationsBySession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("query model invocations: %v", err)
	}
	if len(invocations) != 1 {
		t.Fatalf("expected one model invocation row, got %d", len(invocations))
	}
}

func TestTurnCreation_AwaitingApprovalWritesLifecycleEvent_Bead_t10_5(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	secretsPolicy := secretsRedactionPolicyForTest{executor: secrets.NewPolicyExecutor(nil)}
	turnMgr := ctxmgr.NewTurnManagerWithPolicy(eventStore, sessionMgr, secretsPolicy)

	registry := tools.NewRegistry()
	if err := registry.Register(tools.ToolDefinition{
		Name:             "write_file",
		Version:          "1",
		SideEffect:       tools.SideEffectWrite,
		ApprovalRequired: true,
		InputSchema:      map[string]tools.FieldSchema{"path": {Type: "string"}, "content": {Type: "string"}},
		OutputSchema:     map[string]tools.FieldSchema{"bytes_written": {Type: "integer"}},
		Limits:           tools.ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 1024, AllowedPaths: []string{"/tmp"}},
		Security:         tools.SecurityMetadata{FilesystemScope: "repo"},
	}); err != nil {
		t.Fatalf("register write_file: %v", err)
	}
	executor := tools.NewExecutor(registry, nil)
	b := newStreamBus()

	server := &Server{
		turnMgr:      turnMgr,
		sessionMgr:   sessionMgr,
		eventStore:   eventStore,
		approvalMgr:  NewApprovalManager(10 * time.Minute),
		toolRegistry: registry,
		toolExecutor: executor,
		bus:          b,
	}
	server.runner = agent.NewRunner(approvalRequiredModel{}, registry, executor, server, busEmitter{bus: b}, agent.DefaultConfig("tier1"))

	ctx := context.Background()
	sess, err := sessionMgr.Create(ctx, "test-repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	reqBody, _ := json.Marshal(map[string]interface{}{"message": "write file"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sess.ID+"/turns", bytes.NewBuffer(reqBody))
	req = withOperatorRole(req)
	resp := httptest.NewRecorder()
	server.handleSessionTurns(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, resp.Code)
	}

	var createdTurn struct {
		ID string `json:"turn_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &createdTurn); err != nil {
		t.Fatalf("decode turn: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		events, qErr := eventStore.QueryBySession(ctx, sess.ID)
		if qErr != nil {
			t.Fatalf("query events: %v", qErr)
		}
		for _, ev := range events {
			if ev.EventType != audit.EventTurnAwaitingApproval {
				continue
			}
			if ev.TurnID != createdTurn.ID {
				t.Fatalf("expected turn_id %s, got %s", createdTurn.ID, ev.TurnID)
			}

			var payload map[string]interface{}
			if err := json.Unmarshal(ev.Payload, &payload); err != nil {
				t.Fatalf("decode lifecycle payload: %v", err)
			}
			causality, ok := payload["causality"].(map[string]interface{})
			if !ok {
				t.Fatalf("expected causality payload object, got %T", payload["causality"])
			}
			if causality["turn_id"] != createdTurn.ID {
				t.Fatalf("expected causality.turn_id %s, got %v", createdTurn.ID, causality["turn_id"])
			}
			if causality["tool_name"] != "write_file" {
				t.Fatalf("expected causality.tool_name write_file, got %v", causality["tool_name"])
			}
			approvalID, ok := causality["approval_id"].(string)
			if !ok || approvalID == "" {
				t.Fatalf("expected non-empty causality.approval_id, got %v", causality["approval_id"])
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("expected turn_awaiting_approval lifecycle event")
}

type noopApprovalGate struct{}

func (noopApprovalGate) RequestApproval(_ context.Context, _ tools.ToolCall) (agent.ApprovalResult, error) {
	return agent.ApprovalResult{Approved: true}, nil
}

type secretsRedactionPolicyForTest struct {
	executor *secrets.PolicyExecutor
}

func (p secretsRedactionPolicyForTest) Apply(sourceRef, sourceType, content string, destination ctxmgr.RedactionDestination) ctxmgr.RedactionDecision {
	secretDestination := secrets.DestinationPersistence
	if destination == ctxmgr.DestinationModelProvider {
		secretDestination = secrets.DestinationModelProvider
	}

	decision := p.executor.Apply(sourceRef, sourceType, content, secretDestination)
	return ctxmgr.RedactionDecision{
		Content:             decision.Content,
		Action:              string(decision.Action),
		Denied:              decision.Denied,
		Reason:              decision.Reason,
		SensitivityClass:    string(decision.Classification.Class),
		ClassificationLevel: string(decision.Classification.Level),
	}
}
