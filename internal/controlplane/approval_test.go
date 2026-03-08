package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

func newApprovalTestServer(t *testing.T) *Server {
	t.Helper()

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
		approvalMgr:  NewApprovalManager(10 * time.Minute),
		toolRegistry: registry,
		toolExecutor: executor,
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
}

func TestHandleToolExecuteReturnsRedactionNotice(t *testing.T) {
	server := newApprovalTestServer(t)
	body := bytes.NewBufferString(`{"tool_name":"echo","input":{"message":"AKIA1234567890ABCDEF"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()

	server.handleToolExecute(rec, req.WithContext(context.Background()))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result tools.ToolResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode tool result: %v", err)
	}
	if result.RedactionNotice == nil {
		t.Fatalf("expected redaction notice when secret is redacted")
	}
	if !result.RedactionNotice.MaterialImpact {
		t.Fatalf("expected material impact to be true")
	}
}

func TestHandleToolExecutePersistsAuditSafeRedactionEvent(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	defer func() {
		_ = eventStore.Close()
	}()

	server := newApprovalTestServer(t)
	server.eventStore = eventStore

	body := bytes.NewBufferString(`{"tool_name":"echo","input":{"message":"AKIA1234567890ABCDEF"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()

	server.handleToolExecute(rec, req.WithContext(context.Background()))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	events, err := eventStore.QueryAll(context.Background())
	if err != nil {
		t.Fatalf("failed to query events: %v", err)
	}

	var found bool
	for _, event := range events {
		if event.EventType != "redaction_applied" {
			continue
		}
		found = true
		if strings.Contains(string(event.Payload), "AKIA1234567890ABCDEF") {
			t.Fatalf("expected redaction event payload to be audit-safe")
		}
	}

	if !found {
		t.Fatalf("expected redaction_applied event to be emitted")
	}
}
