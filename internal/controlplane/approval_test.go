package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

func TestApprovalManager_ProposeAndApproveFlow(t *testing.T) {
	mgr := NewApprovalManager(10 * time.Minute)
	call := tools.ToolCall{ToolName: "write_file", Input: map[string]interface{}{"path": "a.go"}}

	approval, err := mgr.Propose("s1", call, tools.SideEffectWrite, []string{"a.go"})
	if err != nil {
		t.Fatalf("propose failed: %v", err)
	}
	if approval.State != ApprovalStatePending {
		t.Fatalf("expected pending, got %s", approval.State)
	}
	if approval.ArgumentsHash == "" {
		t.Fatal("expected arguments hash")
	}

	approved, err := mgr.Decide(approval.ID, "approve", "")
	if err != nil {
		t.Fatalf("approve failed: %v", err)
	}
	if approved.State != ApprovalStateApproved {
		t.Fatalf("expected approved, got %s", approved.State)
	}

	if err := mgr.MarkExecuted(approval.ID); err != nil {
		t.Fatalf("mark executed failed: %v", err)
	}

	final, ok := mgr.Get(approval.ID)
	if !ok {
		t.Fatal("expected approval to exist")
	}
	if final.State != ApprovalStateExecuted {
		t.Fatalf("expected executed, got %s", final.State)
	}
}

func TestApprovalManager_Deny(t *testing.T) {
	mgr := NewApprovalManager(10 * time.Minute)
	approval, err := mgr.Propose("s1", tools.ToolCall{ToolName: "write_file", Input: map[string]interface{}{}}, tools.SideEffectWrite, nil)
	if err != nil {
		t.Fatalf("propose failed: %v", err)
	}

	denied, err := mgr.Decide(approval.ID, "deny", "unsafe")
	if err != nil {
		t.Fatalf("deny failed: %v", err)
	}
	if denied.State != ApprovalStateDenied {
		t.Fatalf("expected denied, got %s", denied.State)
	}
	if denied.DecisionReason != "unsafe" {
		t.Fatalf("expected reason to be recorded")
	}
}

func TestApprovalManager_ExpiryCancelsPending(t *testing.T) {
	mgr := NewApprovalManager(1 * time.Millisecond)
	approval, err := mgr.Propose("s1", tools.ToolCall{ToolName: "write_file", Input: map[string]interface{}{}}, tools.SideEffectWrite, nil)
	if err != nil {
		t.Fatalf("propose failed: %v", err)
	}

	time.Sleep(5 * time.Millisecond)
	pending := mgr.ListPending("")
	if len(pending) != 0 {
		t.Fatalf("expected no pending approvals after expiry, got %d", len(pending))
	}

	stored, ok := mgr.Get(approval.ID)
	if !ok {
		t.Fatal("expected approval to exist")
	}
	if stored.State != ApprovalStateCancelled {
		t.Fatalf("expected cancelled, got %s", stored.State)
	}
}

func TestApprovalManager_ExpiryEmitsTransitionCausality(t *testing.T) {
	transitions := make([]ApprovalTransition, 0)
	mgr := NewApprovalManager(1*time.Millisecond, WithTransitionRecorder(func(transition ApprovalTransition) {
		transitions = append(transitions, transition)
	}))

	approval, err := mgr.Propose("s1", tools.ToolCall{ToolName: "write_file", Input: map[string]interface{}{}}, tools.SideEffectWrite, nil)
	if err != nil {
		t.Fatalf("propose failed: %v", err)
	}

	time.Sleep(5 * time.Millisecond)
	_ = mgr.ListPending("")

	if len(transitions) < 2 {
		t.Fatalf("expected at least 2 transitions (proposed->pending and pending->cancelled), got %d", len(transitions))
	}

	last := transitions[len(transitions)-1]
	if last.ApprovalID != approval.ID {
		t.Fatalf("expected transition for approval %s, got %s", approval.ID, last.ApprovalID)
	}
	if last.FromState != ApprovalStatePending {
		t.Fatalf("expected from_state pending, got %s", last.FromState)
	}
	if last.ToState != ApprovalStateCancelled {
		t.Fatalf("expected to_state cancelled, got %s", last.ToState)
	}
	if last.Trigger != "expired" {
		t.Fatalf("expected trigger expired, got %s", last.Trigger)
	}
}

func TestHandleApprovalsAndDecision(t *testing.T) {
	server := newApprovalTestServer(t)

	body := bytes.NewBufferString(`{"tool_name":"write_file","input":{"path":"/tmp/x.txt","content":"hello"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()
	server.handleToolExecute(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for pending approval, got %d", rec.Code)
	}

	var approval Approval
	if err := json.Unmarshal(rec.Body.Bytes(), &approval); err != nil {
		t.Fatalf("failed to decode approval: %v", err)
	}
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/approvals", nil)
	listRec := httptest.NewRecorder()
	server.handleApprovals(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for approvals list, got %d", listRec.Code)
	}

	decisionBody := bytes.NewBufferString(`{"decision":"approve"}`)
	decisionReq := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/"+approval.ID, decisionBody)
	decisionRec := httptest.NewRecorder()
	server.handleApprovalByID(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on approve, got %d", decisionRec.Code)
	}

	final, ok := server.approvalMgr.Get(approval.ID)
	if !ok {
		t.Fatal("approval not found")
	}
	if final.State != ApprovalStateExecuted {
		t.Fatalf("expected executed state, got %s", final.State)
	}
}

func TestHandleToolExecute_ApprovalTransitionEventsCarryCausalityPayload(t *testing.T) {
	server := newApprovalTestServer(t)

	body := bytes.NewBufferString(`{"session_id":"session-123","tool_name":"write_file","input":{"path":"/tmp/x.txt","content":"hello"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()
	server.handleToolExecute(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for pending approval, got %d", rec.Code)
	}

	var approval Approval
	if err := json.Unmarshal(rec.Body.Bytes(), &approval); err != nil {
		t.Fatalf("failed to decode approval: %v", err)
	}

	decisionBody := bytes.NewBufferString(`{"decision":"approve"}`)
	decisionReq := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/"+approval.ID, decisionBody)
	decisionRec := httptest.NewRecorder()
	server.handleApprovalByID(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on approve, got %d", decisionRec.Code)
	}

	events, err := server.eventStore.QueryBySession(context.Background(), approval.SessionID)
	if err != nil {
		t.Fatalf("failed querying events: %v", err)
	}

	transitionPayloads := make([]map[string]interface{}, 0)
	for _, event := range events {
		if event.EventType != "approval.transition" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("failed to decode transition payload: %v", err)
		}
		transitionPayloads = append(transitionPayloads, payload)
	}

	if len(transitionPayloads) != 3 {
		t.Fatalf("expected 3 approval.transition events, got %d", len(transitionPayloads))
	}

	if transitionPayloads[0]["entity_type"] != "approval" {
		t.Fatalf("expected entity_type approval, got %v", transitionPayloads[0]["entity_type"])
	}
	if transitionPayloads[0]["entity_id"] != approval.ID {
		t.Fatalf("expected entity_id %s, got %v", approval.ID, transitionPayloads[0]["entity_id"])
	}
	if transitionPayloads[0]["previous_state"] != "proposed" || transitionPayloads[0]["next_state"] != "pending" {
		t.Fatalf("expected proposed->pending transition, got %v->%v", transitionPayloads[0]["previous_state"], transitionPayloads[0]["next_state"])
	}
	if transitionPayloads[0]["cause"] != "tool_execute_requested" {
		t.Fatalf("expected cause tool_execute_requested, got %v", transitionPayloads[0]["cause"])
	}

	relatedIDs, ok := transitionPayloads[0]["related_ids"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected related_ids object, got %T", transitionPayloads[0]["related_ids"])
	}
	if relatedIDs["session_id"] != "session-123" {
		t.Fatalf("expected related_ids.session_id session-123, got %v", relatedIDs["session_id"])
	}

	if transitionPayloads[1]["previous_state"] != "pending" || transitionPayloads[1]["next_state"] != "approved" {
		t.Fatalf("expected pending->approved transition, got %v->%v", transitionPayloads[1]["previous_state"], transitionPayloads[1]["next_state"])
	}
	if transitionPayloads[2]["previous_state"] != "approved" || transitionPayloads[2]["next_state"] != "executed" {
		t.Fatalf("expected approved->executed transition, got %v->%v", transitionPayloads[2]["previous_state"], transitionPayloads[2]["next_state"])
	}
}

func newApprovalTestServer(t *testing.T) *Server {
	t.Helper()
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	registry := tools.NewRegistry()
	if err := registry.Register(tools.ToolDefinition{
		Name:             "echo",
		Version:          "1",
		SideEffect:       tools.SideEffectNone,
		ApprovalRequired: false,
		InputSchema:      map[string]tools.FieldSchema{"message": {Type: "string"}},
		OutputSchema:     map[string]tools.FieldSchema{"output": {Type: "string"}},
		Limits:           tools.ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 1024},
	}); err != nil {
		t.Fatalf("register echo failed: %v", err)
	}
	if err := registry.Register(tools.ToolDefinition{
		Name:             "write_file",
		Version:          "1",
		SideEffect:       tools.SideEffectWrite,
		ApprovalRequired: true,
		InputSchema:      map[string]tools.FieldSchema{"path": {Type: "string"}, "content": {Type: "string"}},
		OutputSchema:     map[string]tools.FieldSchema{"bytes_written": {Type: "integer"}},
		Limits:           tools.ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 1024, AllowedPaths: []string{"/tmp"}},
	}); err != nil {
		t.Fatalf("register write_file failed: %v", err)
	}
	if err := registry.Register(tools.ToolDefinition{
		Name:             "restricted_shell",
		Version:          "1",
		SideEffect:       tools.SideEffectExecute,
		ApprovalRequired: true,
		InputSchema:      map[string]tools.FieldSchema{"command": {Type: "string"}},
		OutputSchema:     map[string]tools.FieldSchema{"exit_code": {Type: "integer"}},
		Limits:           tools.ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 1024},
	}); err != nil {
		t.Fatalf("register restricted_shell failed: %v", err)
	}
	if err := registry.Register(tools.ToolDefinition{
		Name:             "apply_patch",
		Version:          "1",
		SideEffect:       tools.SideEffectWrite,
		ApprovalRequired: true,
		InputSchema:      map[string]tools.FieldSchema{"patch": {Type: "string"}},
		OutputSchema:     map[string]tools.FieldSchema{"files_modified": {Type: "array"}},
		Limits:           tools.ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 1024},
	}); err != nil {
		t.Fatalf("register apply_patch failed: %v", err)
	}

	executor := tools.NewExecutor(registry, nil)
	executor.RegisterHandler("echo", tools.EchoHandler())
	executor.RegisterHandler("write_file", tools.WriteFileHandler("/tmp"))
	executor.RegisterHandler("restricted_shell", func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"exit_code": 0}, nil
	})
	executor.RegisterHandler("apply_patch", func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"files_modified": []string{"a.txt"}}, nil
	})

	return &Server{
		approvalMgr:  NewApprovalManager(10*time.Minute, WithTransitionRecorder(newApprovalTransitionRecorder(eventStore))),
		toolRegistry: registry,
		toolExecutor: executor,
		eventStore:   eventStore,
		httpServer:   &http.Server{},
	}
}

func TestHandleToolExecuteRestrictedShellRequiresApproval(t *testing.T) {
	server := newApprovalTestServer(t)
	body := bytes.NewBufferString(`{"tool_name":"restricted_shell","input":{"command":"go_version"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()

	server.handleToolExecute(rec, req.WithContext(context.Background()))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
}

func TestHandleToolExecuteApplyPatchRequiresApproval(t *testing.T) {
	server := newApprovalTestServer(t)
	body := bytes.NewBufferString(`{"tool_name":"apply_patch","input":{"patch":"*** Begin Patch\n*** Add File: a.txt\n+hello\n*** End Patch"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()

	server.handleToolExecute(rec, req.WithContext(context.Background()))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
}

func TestHandleToolExecuteDirectExecution(t *testing.T) {
	server := newApprovalTestServer(t)
	body := bytes.NewBufferString(`{"tool_name":"echo","input":{"message":"ok"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()

	server.handleToolExecute(rec, req.WithContext(context.Background()))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleToolExecuteRedactsSecrets(t *testing.T) {
	server := newApprovalTestServer(t)
	body := bytes.NewBufferString(`{"tool_name":"echo","input":{"message":"AKIA1234567890ABCDEF"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()

	server.handleToolExecute(rec, req.WithContext(context.Background()))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if strings.Contains(rec.Body.String(), "AKIA1234567890ABCDEF") {
		t.Fatalf("expected secret to be redacted, got %s", rec.Body.String())
	}

	var response struct {
		Output           map[string]interface{} `json:"output"`
		RedactionNotices []struct {
			Action         string `json:"action"`
			Denied         bool   `json:"denied"`
			Reason         string `json:"reason"`
			MaterialImpact bool   `json:"material_impact"`
		} `json:"redaction_notices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode tool response: %v", err)
	}
	if len(response.RedactionNotices) == 0 {
		t.Fatalf("expected redaction notices to be returned")
	}
	if response.RedactionNotices[0].Action == "" {
		t.Fatalf("expected redaction notice action to be populated")
	}
	if !response.RedactionNotices[0].MaterialImpact {
		t.Fatalf("expected redaction notice to mark material impact")
	}
}

func TestHandleToolExecuteRecordsAuditSafeRedactionEvents(t *testing.T) {
	server := newApprovalTestServer(t)
	body := bytes.NewBufferString(`{"tool_name":"echo","input":{"message":"AKIA1234567890ABCDEF"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()

	server.handleToolExecute(rec, req.WithContext(context.Background()))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	events, err := server.eventStore.QueryBySession(context.Background(), redactionAuditSessionID)
	if err != nil {
		t.Fatalf("failed querying redaction events: %v", err)
	}

	found := false
	for _, event := range events {
		if event.EventType != "tool_output_redaction" {
			continue
		}
		found = true
		payload := string(event.Payload)
		if strings.Contains(payload, "AKIA1234567890ABCDEF") {
			t.Fatalf("expected audit payload to be secret-safe, got %s", payload)
		}
		if !strings.Contains(payload, "redaction_notices") {
			t.Fatalf("expected audit payload to include redaction notices, got %s", payload)
		}
	}
	if !found {
		t.Fatalf("expected tool_output_redaction event to be recorded")
	}
}
