package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	ctxmgr "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/context"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/secrets"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
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
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			server.handleSessionTurns(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d. body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
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
