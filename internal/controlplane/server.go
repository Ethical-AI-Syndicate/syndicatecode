package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	ctxmgr "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/context"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/patch"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/policy"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/sandbox"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/secrets"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

type Config struct {
	Addr               string
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	ProviderPolicyPath string
}

func DefaultConfig() *Config {
	return &Config{
		Addr:               ":7777",
		ReadTimeout:        30 * time.Second,
		WriteTimeout:       30 * time.Second,
		ProviderPolicyPath: "",
	}
}

type Server struct {
	httpServer   *http.Server
	sessionMgr   *session.Manager
	turnMgr      *ctxmgr.TurnManager
	ctxManifest  *ctxmgr.ContextManifest
	eventStore   *audit.EventStore
	approvalMgr  *ApprovalManager
	toolRegistry *tools.Registry
	toolExecutor *tools.Executor
}

const redactionAuditSessionID = "system:redaction"
const approvalAuditSessionID = "system:approval"

type redactionNotice struct {
	Path           string `json:"path"`
	Destination    string `json:"destination"`
	Action         string `json:"action"`
	Sensitivity    string `json:"sensitivity"`
	Reason         string `json:"reason"`
	Denied         bool   `json:"denied"`
	MaterialImpact bool   `json:"material_impact"`
}

type toolExecuteResponse struct {
	*tools.ToolResult
	RedactionNotices []redactionNotice `json:"redaction_notices,omitempty"`
}

func NewServer(ctx context.Context, cfg *Config) (*Server, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if cfg.ProviderPolicyPath != "" {
		if _, err := policy.LoadProviderPolicy(cfg.ProviderPolicyPath); err != nil {
			return nil, fmt.Errorf("failed to load provider policy: %w", err)
		}
	} else if err := policy.DefaultProviderPolicy().Validate(); err != nil {
		return nil, fmt.Errorf("invalid default provider policy: %w", err)
	}

	eventStore, err := audit.NewEventStore("syndicatecode.db")
	if err != nil {
		return nil, fmt.Errorf("failed to create event store: %w", err)
	}

	sessionMgr := session.NewManager(eventStore)
	turnMgr := ctxmgr.NewTurnManagerWithPolicy(eventStore, sessionMgr, newContextRedactionPolicy(secrets.NewPolicyExecutor(nil)))
	ctxManifest := ctxmgr.NewContextManifest(eventStore)

	toolRegistry, toolExecutor, err := initializeTooling(ctx, eventStore)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tooling: %w", err)
	}

	approvalMgr := NewApprovalManager(15*time.Minute, WithTransitionRecorder(newApprovalTransitionRecorder(eventStore)))

	mux := http.NewServeMux()
	server := &Server{
		httpServer: &http.Server{
			Addr:         cfg.Addr,
			Handler:      mux,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
		},
		sessionMgr:   sessionMgr,
		turnMgr:      turnMgr,
		ctxManifest:  ctxManifest,
		eventStore:   eventStore,
		approvalMgr:  approvalMgr,
		toolRegistry: toolRegistry,
		toolExecutor: toolExecutor,
	}

	if err := server.restoreRuntimeState(ctx); err != nil {
		return nil, fmt.Errorf("failed to restore runtime state: %w", err)
	}

	server.registerRoutes(mux)

	return server, nil
}

func initializeTooling(ctx context.Context, eventStore *audit.EventStore) (*tools.Registry, *tools.Executor, error) {
	registry := tools.NewRegistry()

	definitions := []tools.ToolDefinition{
		{
			Name:             "echo",
			Version:          "1",
			SideEffect:       tools.SideEffectNone,
			ApprovalRequired: false,
			InputSchema: map[string]tools.FieldSchema{
				"message": {Type: "string", Description: "message to echo"},
			},
			OutputSchema: map[string]tools.FieldSchema{
				"output": {Type: "string", Description: "echoed message"},
			},
			Limits: tools.ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 64 * 1024},
		},
		{
			Name:             "read_file",
			Version:          "1",
			SideEffect:       tools.SideEffectRead,
			ApprovalRequired: false,
			InputSchema: map[string]tools.FieldSchema{
				"path": {Type: "string", Description: "file path"},
			},
			OutputSchema: map[string]tools.FieldSchema{
				"content": {Type: "string", Description: "file content"},
			},
			Limits: tools.ExecutionLimits{TimeoutSeconds: 10, MaxOutputBytes: 512 * 1024},
		},
		{
			Name:             "write_file",
			Version:          "1",
			SideEffect:       tools.SideEffectWrite,
			ApprovalRequired: true,
			InputSchema: map[string]tools.FieldSchema{
				"path":    {Type: "string", Description: "file path"},
				"content": {Type: "string", Description: "file content"},
			},
			OutputSchema: map[string]tools.FieldSchema{
				"bytes_written": {Type: "integer", Description: "bytes written"},
			},
			Limits:   tools.ExecutionLimits{TimeoutSeconds: 10, MaxOutputBytes: 512 * 1024},
			Security: tools.SecurityMetadata{FilesystemScope: "repo"},
		},
		{
			Name:             "run_tests",
			Version:          "1",
			SideEffect:       tools.SideEffectExecute,
			ApprovalRequired: false,
			InputSchema: map[string]tools.FieldSchema{
				"mode": {Type: "string", Description: "reserved for future test scope"},
			},
			OutputSchema: map[string]tools.FieldSchema{
				"exit_code": {Type: "integer", Description: "command exit code"},
				"stdout":    {Type: "string", Description: "stdout"},
				"stderr":    {Type: "string", Description: "stderr"},
			},
			Limits:   tools.ExecutionLimits{TimeoutSeconds: 120, MaxOutputBytes: 1024 * 1024},
			Security: tools.SecurityMetadata{FilesystemScope: "repo"},
		},
		{
			Name:             "run_lint",
			Version:          "1",
			SideEffect:       tools.SideEffectExecute,
			ApprovalRequired: false,
			InputSchema: map[string]tools.FieldSchema{
				"mode": {Type: "string", Description: "reserved for future lint scope"},
			},
			OutputSchema: map[string]tools.FieldSchema{
				"exit_code": {Type: "integer", Description: "command exit code"},
				"stdout":    {Type: "string", Description: "stdout"},
				"stderr":    {Type: "string", Description: "stderr"},
			},
			Limits:   tools.ExecutionLimits{TimeoutSeconds: 120, MaxOutputBytes: 1024 * 1024},
			Security: tools.SecurityMetadata{FilesystemScope: "repo"},
		},
		{
			Name:             "restricted_shell",
			Version:          "1",
			SideEffect:       tools.SideEffectExecute,
			ApprovalRequired: true,
			InputSchema: map[string]tools.FieldSchema{
				"command":  {Type: "string", Description: "allowlisted symbolic command"},
				"work_dir": {Type: "string", Description: "working directory inside repo"},
			},
			OutputSchema: map[string]tools.FieldSchema{
				"exit_code": {Type: "integer", Description: "command exit code"},
				"stdout":    {Type: "string", Description: "stdout"},
				"stderr":    {Type: "string", Description: "stderr"},
			},
			Limits:   tools.ExecutionLimits{TimeoutSeconds: 120, MaxOutputBytes: 1024 * 1024},
			Security: tools.SecurityMetadata{FilesystemScope: "repo"},
		},
		{
			Name:             "apply_patch",
			Version:          "1",
			SideEffect:       tools.SideEffectWrite,
			ApprovalRequired: true,
			InputSchema: map[string]tools.FieldSchema{
				"patch": {Type: "string", Description: "patch envelope text"},
			},
			OutputSchema: map[string]tools.FieldSchema{
				"files_modified": {Type: "array", Description: "modified repository files"},
			},
			Limits:   tools.ExecutionLimits{TimeoutSeconds: 30, MaxOutputBytes: 256 * 1024},
			Security: tools.SecurityMetadata{FilesystemScope: "repo"},
		},
	}

	for _, def := range definitions {
		if err := registry.Register(def); err != nil {
			return nil, nil, fmt.Errorf("failed to register tool %s: %w", def.Name, err)
		}
	}

	if err := loadConfiguredPlugins(ctx, registry, eventStore); err != nil {
		return nil, nil, fmt.Errorf("failed to load plugins: %w", err)
	}

	executor := tools.NewExecutor(registry, nil)
	repoRoot, err := os.Getwd()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to determine working directory: %w", err)
	}

	runner := sandbox.NewRunner(sandbox.Config{
		RepoRoot: repoRoot,
		AllowedCmds: map[string]struct{}{
			"go_test_all":       {},
			"go_test_internal":  {},
			"go_test_policy":    {},
			"go_version":        {},
			"go_vet_all":        {},
			"go_fmt_all":        {},
			"golangci_lint_run": {},
		},
		DefaultTimeout: 120 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	})

	executor.RegisterHandler("echo", tools.EchoHandler())
	executor.RegisterHandler("read_file", tools.ReadFileHandler(repoRoot))
	executor.RegisterHandler("write_file", tools.WriteFileHandler(repoRoot))
	executor.RegisterHandler("run_tests", sandbox.RunTestsHandler(runner))
	executor.RegisterHandler("run_lint", sandbox.RunLintHandler(runner))
	executor.RegisterHandler("restricted_shell", sandbox.RestrictedShellHandler(runner))
	executor.RegisterHandler("apply_patch", tools.ApplyPatchHandler(patch.NewEngine(repoRoot)))

	return registry, executor, nil
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.Handle("/api/v1/sessions", schemaValidationMiddleware(
		map[string]jsonObjectSchema{http.MethodPost: sessionsCreateRequestSchema()},
		map[string]jsonObjectSchema{http.MethodPost: sessionsCreateResponseSchema()},
		http.HandlerFunc(s.handleSessions),
	))
	mux.HandleFunc("/api/v1/sessions/", s.handleSessionOrTurn)
	mux.HandleFunc("/api/v1/turns", s.handleTurns)
	mux.HandleFunc("/api/v1/turns/", s.handleTurnByID)
	mux.HandleFunc("/api/v1/approvals", s.handleApprovals)
	mux.HandleFunc("/api/v1/approvals/", s.handleApprovalByID)
	mux.HandleFunc("/api/v1/policy", s.handlePolicy)
	mux.HandleFunc("/api/v1/tools/execute", s.handleToolExecute)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		log.Printf("failed to encode health response: %v", err)
	}
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listSessions(w, r)
	case http.MethodPost:
		s.createSession(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.sessionMgr.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := json.NewEncoder(w).Encode(sessions); err != nil {
		log.Printf("failed to encode sessions: %v", err)
	}
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepoPath  string `json:"repo_path"`
		TrustTier string `json:"trust_tier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	created, err := s.sessionMgr.Create(r.Context(), req.RepoPath, req.TrustTier)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(created); err != nil {
		log.Printf("failed to encode created session: %v", err)
	}
}

func (s *Server) handleSessionOrTurn(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) >= 2 && parts[1] == "turns" {
		if len(parts) == 2 {
			s.handleSessionTurns(w, r)
		} else if len(parts) >= 3 {
			s.handleSessionTurnByID(w, r)
		}
		return
	}

	s.handleSessionByID(w, r)
}

func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 2 && parts[1] == "events" {
		s.handleSessionEvents(w, r, parts[0])
		return
	}
	if len(parts) != 1 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	sessionID := parts[0]
	session, err := s.sessionMgr.Get(r.Context(), sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := json.NewEncoder(w).Encode(session); err != nil {
		log.Printf("failed to encode session: %v", err)
	}
}

func (s *Server) handleSessionEvents(w http.ResponseWriter, r *http.Request, sessionID string) {
	if s.sessionMgr != nil {
		if _, err := s.sessionMgr.Get(r.Context(), sessionID); err != nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
	}

	events, err := s.eventStore.QueryBySession(r.Context(), sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.sessionMgr == nil && len(events) == 0 {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	eventType := strings.TrimSpace(r.URL.Query().Get("event_type"))
	if eventType != "" {
		filtered := make([]audit.Event, 0, len(events))
		for _, event := range events {
			if event.EventType == eventType {
				filtered = append(filtered, event)
			}
		}
		events = filtered
	}

	if err := json.NewEncoder(w).Encode(events); err != nil {
		log.Printf("failed to encode session events: %v", err)
	}
}

func (s *Server) handleTurns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listTurns(w, r)
	case http.MethodPost:
		s.createTurn(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listTurns(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id required", http.StatusBadRequest)
		return
	}

	turns, err := s.turnMgr.ListBySession(r.Context(), sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := json.NewEncoder(w).Encode(turns); err != nil {
		log.Printf("failed to encode turns: %v", err)
	}
}

func (s *Server) createTurn(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	turn, err := s.turnMgr.Create(r.Context(), req.SessionID, req.Message)
	if err != nil {
		if errors.Is(err, ctxmgr.ErrActiveMutableTurnConflict) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(turn); err != nil {
		log.Printf("failed to encode turn: %v", err)
	}
}

func (s *Server) handleTurnByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/turns/")

	if strings.Contains(path, "/context") {
		s.handleTurnContext(w, r)
		return
	}

	turnID := path
	turn, err := s.turnMgr.Get(r.Context(), turnID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := json.NewEncoder(w).Encode(turn); err != nil {
		log.Printf("failed to encode turn: %v", err)
	}
}

func (s *Server) handleTurnContext(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/turns/"), "/")
	if len(parts) < 2 {
		http.Error(w, "turn_id required", http.StatusBadRequest)
		return
	}
	turnID := parts[0]

	fragments, err := s.ctxManifest.Get(r.Context(), turnID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := json.NewEncoder(w).Encode(fragments); err != nil {
		log.Printf("failed to encode fragments: %v", err)
	}
}

func (s *Server) handleSessionTurns(w http.ResponseWriter, r *http.Request) {
	sessionID := extractSessionID(r.URL.Path)
	if sessionID == "" {
		http.Error(w, "session_id required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.listSessionTurns(w, r, sessionID)
	case http.MethodPost:
		s.createSessionTurn(w, r, sessionID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listSessionTurns(w http.ResponseWriter, r *http.Request, sessionID string) {
	_, err := s.sessionMgr.Get(r.Context(), sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	turns, err := s.turnMgr.ListBySession(r.Context(), sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := json.NewEncoder(w).Encode(turns); err != nil {
		log.Printf("failed to encode turns: %v", err)
	}
}

func (s *Server) createSessionTurn(w http.ResponseWriter, r *http.Request, sessionID string) {
	_, err := s.sessionMgr.Get(r.Context(), sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var req struct {
		Message string   `json:"message"`
		Files   []string `json:"files,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	turn, err := s.turnMgr.Create(r.Context(), sessionID, req.Message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(turn); err != nil {
		log.Printf("failed to encode turn: %v", err)
	}
}

func (s *Server) handleSessionTurnByID(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	sessionIDMatch := regexp.MustCompile(`/api/v1/sessions/([^/]+)`).FindStringSubmatch(path)
	if sessionIDMatch == nil {
		http.Error(w, "session_id required", http.StatusBadRequest)
		return
	}
	sessionID := sessionIDMatch[1]

	if strings.Contains(path, "/context") {
		s.handleSessionTurnContext(w, r, sessionID)
		return
	}

	turnIDMatch := regexp.MustCompile(`/api/v1/sessions/[^/]+/turns/([^/]+)`).FindStringSubmatch(path)
	if turnIDMatch == nil {
		http.Error(w, "turn_id required", http.StatusBadRequest)
		return
	}
	turnID := turnIDMatch[1]

	turn, err := s.turnMgr.Get(r.Context(), turnID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if turn.SessionID != sessionID {
		http.Error(w, "turn does not belong to session", http.StatusNotFound)
		return
	}
	if err := json.NewEncoder(w).Encode(turn); err != nil {
		log.Printf("failed to encode turn: %v", err)
	}
}

func (s *Server) handleSessionTurnContext(w http.ResponseWriter, r *http.Request, sessionID string) {
	pathParts := strings.Split(r.URL.Path, "/")
	for i, part := range pathParts {
		if part == "turns" && i+1 < len(pathParts) {
			turnID := pathParts[i+1]
			if strings.Contains(turnID, "/context") {
				turnID = strings.TrimSuffix(turnID, "/context")
			}

			turn, err := s.turnMgr.Get(r.Context(), turnID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			if turn.SessionID != sessionID {
				http.Error(w, "turn does not belong to session", http.StatusNotFound)
				return
			}

			fragments, err := s.ctxManifest.Get(r.Context(), turnID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := json.NewEncoder(w).Encode(fragments); err != nil {
				log.Printf("failed to encode fragments: %v", err)
			}
			return
		}
	}
	http.Error(w, "turn_id required", http.StatusBadRequest)
}

func extractSessionID(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/api/v1/sessions/"), "/")
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return ""
}

func (s *Server) handleApprovals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	pending := s.approvalMgr.ListPending(sessionID)
	if err := json.NewEncoder(w).Encode(pending); err != nil {
		log.Printf("failed to encode approvals: %v", err)
	}
}

func (s *Server) handleApprovalByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	approvalID := strings.TrimPrefix(r.URL.Path, "/api/v1/approvals/")
	if approvalID == "" {
		http.Error(w, "approval_id required", http.StatusBadRequest)
		return
	}

	var req struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	approval, err := s.approvalMgr.Decide(approvalID, req.Decision, req.Reason)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.appendApprovalEvent(r.Context(), "approval_decided", approval.SessionID, map[string]string{
		"approval_id": approval.ID,
		"decision":    req.Decision,
		"reason":      req.Reason,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if req.Decision == "approve" {
		result, execErr := s.toolExecutor.Execute(r.Context(), approval.Call)
		if toolDef, ok := s.toolRegistry.Get(approval.Call.ToolName); ok {
			s.recordMCPCallEvent(r.Context(), approval.Call, toolDef, result, execErr)
		}
		if execErr != nil {
			http.Error(w, fmt.Sprintf("failed to execute approved action: %v", execErr), http.StatusInternalServerError)
			return
		}
		if markErr := s.approvalMgr.MarkExecuted(approvalID); markErr != nil {
			http.Error(w, markErr.Error(), http.StatusInternalServerError)
			return
		}
		updated, ok := s.approvalMgr.Get(approvalID)
		if ok {
			approval = &updated
		}

		if err := s.appendApprovalEvent(r.Context(), "approval_executed", approval.SessionID, map[string]string{
			"approval_id": approval.ID,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := json.NewEncoder(w).Encode(approval); err != nil {
		log.Printf("failed to encode approval: %v", err)
	}
}

func (s *Server) handlePolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"version": "1.0.0",
		"trust_tiers": map[string]interface{}{
			"tier0": map[string]interface{}{
				"name":    "Untrusted External",
				"read":    true,
				"write":   "restricted",
				"shell":   "restricted",
				"network": false,
			},
			"tier1": map[string]interface{}{
				"name":    "Internal Low Risk",
				"read":    true,
				"write":   true,
				"shell":   "tests_lint",
				"network": "limited",
			},
			"tier2": map[string]interface{}{
				"name":    "Production Adjacent",
				"read":    true,
				"write":   "approval",
				"shell":   "restricted",
				"network": "limited",
			},
			"tier3": map[string]interface{}{
				"name":    "Restricted",
				"read":    true,
				"write":   "approval",
				"shell":   false,
				"network": false,
				"plugins": false,
			},
		},
	}); err != nil {
		log.Printf("failed to encode policy: %v", err)
	}
}

func (s *Server) handleToolExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req tools.ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	toolDef, found := s.toolRegistry.Get(req.ToolName)
	if !found {
		http.Error(w, "tool not found", http.StatusNotFound)
		return
	}

	if err := validateToolDataAccess(toolDef, req.Input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hidden, err := s.isToolHiddenForSession(r.Context(), req.SessionID, toolDef)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if hidden {
		http.Error(w, "tool not found", http.StatusNotFound)
		return
	}

	if err := validateMCPDestination(toolDef, req.Input); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if requiresApproval(toolDef) {
		approval, err := s.approvalMgr.Propose(req.SessionID, req, toolDef.SideEffect, toolDef.Limits.AllowedPaths)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.appendApprovalEvent(r.Context(), "approval_proposed", approval.SessionID, approval); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		if err := json.NewEncoder(w).Encode(approval); err != nil {
			log.Printf("failed to encode pending approval: %v", err)
		}
		return
	}

	result, err := s.toolExecutor.Execute(r.Context(), req)
	s.recordMCPCallEvent(r.Context(), req, toolDef, result, err)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	notices := make([]redactionNotice, 0)
	if result.Output != nil {
		result.Output, notices = applyRedactionPolicyWithNotices(secrets.NewPolicyExecutor(nil), req.ToolName, "tool_output", secrets.DestinationUI, result.Output)
		if len(notices) > 0 {
			s.recordRedactionEvent(r.Context(), req.ToolName, notices)
		}
	}

	if err := json.NewEncoder(w).Encode(toolExecuteResponse{ToolResult: result, RedactionNotices: notices}); err != nil {
		log.Printf("failed to encode tool response: %v", err)
	}
}

func newApprovalTransitionRecorder(eventStore *audit.EventStore) ApprovalTransitionRecorder {
	return func(transition ApprovalTransition) {
		if eventStore == nil {
			return
		}

		sessionID := transition.SessionID
		if sessionID == "" {
			sessionID = approvalAuditSessionID
		}

		payload, err := json.Marshal(map[string]interface{}{
			"entity_type":          "approval",
			"entity_id":            transition.ApprovalID,
			"previous_state":       transition.FromState,
			"next_state":           transition.ToState,
			"transition_timestamp": transition.Timestamp.Format(time.RFC3339Nano),
			"cause":                transition.Trigger,
			"tool_name":            transition.ToolName,
			"arguments_hash":       transition.ArgumentsHash,
			"side_effect":          transition.SideEffect,
			"decision":             transition.Decision,
			"reason":               transition.Reason,
			"related_ids": map[string]interface{}{
				"session_id": transition.SessionID,
			},
		})
		if err != nil {
			log.Printf("failed to marshal approval transition payload: %v", err)
			return
		}

		err = eventStore.Append(context.Background(), audit.Event{
			ID:            uuid.NewString(),
			SessionID:     sessionID,
			Timestamp:     transition.Timestamp,
			EventType:     "approval.transition",
			Actor:         "controlplane",
			PolicyVersion: "1.0.0",
			Payload:       payload,
		})
		if err != nil {
			log.Printf("failed to append approval.transition event: %v", err)
		}
	}
}

func requiresApproval(toolDef tools.ToolDefinition) bool {
	if toolDef.ApprovalRequired {
		return true
	}
	if toolDef.Source != tools.ToolSourcePlugin {
		return false
	}

	switch toolDef.SideEffect {
	case tools.SideEffectWrite, tools.SideEffectExecute, tools.SideEffectNetwork:
		return true
	default:
		return false
	}
}

func validateToolDataAccess(toolDef tools.ToolDefinition, input map[string]interface{}) error {
	if toolDef.Source != tools.ToolSourcePlugin {
		return nil
	}

	if full, ok := input["full_context"].(bool); ok && full {
		return fmt.Errorf("plugin tools must request scoped context, full context is not allowed")
	}

	scope := toolDef.Security.FilesystemScope
	if scope == "" {
		return fmt.Errorf("plugin tool %s missing data access scope", toolDef.Name)
	}

	path, hasPath := input["path"].(string)
	if !hasPath || path == "" {
		return nil
	}

	switch scope {
	case "none", "metadata":
		return fmt.Errorf("plugin tool %s does not allow path access for scope %s", toolDef.Name, scope)
	case "workspace_read", "workspace_write", "secrets":
		return nil
	default:
		return fmt.Errorf("plugin tool %s has invalid data access scope %s", toolDef.Name, scope)
	}
}

func validateMCPDestination(toolDef tools.ToolDefinition, input map[string]interface{}) error {
	if toolDef.MCP == nil || toolDef.MCP.Transport != "remote" {
		return nil
	}

	rawDestination, ok := input["destination"]
	if !ok {
		return fmt.Errorf("remote mcp tool %s requires destination", toolDef.Name)
	}
	destination, ok := rawDestination.(string)
	if !ok || strings.TrimSpace(destination) == "" {
		return fmt.Errorf("remote mcp tool %s requires destination", toolDef.Name)
	}

	for _, allowed := range toolDef.MCP.AllowedDestinations {
		if destination == allowed {
			return nil
		}
	}

	return fmt.Errorf("destination %s is not allowed for remote mcp tool %s", destination, toolDef.Name)
}

func (s *Server) recordMCPCallEvent(ctx context.Context, req tools.ExecuteRequest, toolDef tools.ToolDefinition, result *tools.ToolResult, execErr error) {
	if s.eventStore == nil || toolDef.MCP == nil {
		return
	}

	payload := map[string]interface{}{
		"tool_name":    req.ToolName,
		"session_id":   req.SessionID,
		"server_id":    toolDef.MCP.ServerID,
		"transport":    toolDef.MCP.Transport,
		"destination":  requestDestination(req.Input),
		"request":      req.Input,
		"response":     responseMetadata(result, execErr),
		"policy_event": "mcp_transport_v1",
	}

	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		log.Printf("failed to marshal mcp.call event payload: %v", err)
		return
	}

	err = s.eventStore.Append(ctx, audit.Event{
		ID:        uuid.NewString(),
		SessionID: req.SessionID,
		Timestamp: time.Now().UTC(),
		EventType: "mcp.call",
		Actor:     "controlplane",
		Payload:   encodedPayload,
	})
	if err != nil {
		log.Printf("failed to append mcp.call event: %v", err)
	}
}

func requestDestination(input map[string]interface{}) string {
	rawDestination, ok := input["destination"]
	if !ok {
		return ""
	}
	destination, ok := rawDestination.(string)
	if !ok {
		return ""
	}
	return destination
}

func responseMetadata(result *tools.ToolResult, execErr error) map[string]interface{} {
	response := map[string]interface{}{
		"success": result != nil && result.Success,
	}
	if result != nil {
		response["duration_ms"] = result.Duration
		response["output"] = result.Output
	}
	if execErr != nil {
		response["error"] = execErr.Error()
	}
	return response
}

func (s *Server) isToolHiddenForSession(ctx context.Context, sessionID string, toolDef tools.ToolDefinition) (bool, error) {
	if toolDef.Source != tools.ToolSourcePlugin {
		return false, nil
	}

	if s.sessionMgr == nil || sessionID == "" {
		return true, nil
	}

	sessionState, err := s.sessionMgr.Get(ctx, sessionID)
	if err != nil {
		if err == session.ErrSessionNotFound {
			return true, nil
		}
		return false, fmt.Errorf("failed to resolve session for plugin exposure policy: %w", err)
	}

	return !isPluginExposedForTrustTier(sessionState.TrustTier, toolDef.TrustLevel), nil
}

func isPluginExposedForTrustTier(sessionTrustTier, pluginTrustLevel string) bool {
	if sessionTrustTier == "tier3" {
		return false
	}

	return isTrustTierAtLeast(pluginTrustLevel, sessionTrustTier)
}

func isTrustTierAtLeast(candidate, required string) bool {
	ranking := map[string]int{
		"tier0": 0,
		"tier1": 1,
		"tier2": 2,
		"tier3": 3,
	}

	candidateRank, candidateOK := ranking[candidate]
	requiredRank, requiredOK := ranking[required]
	if !candidateOK || !requiredOK {
		return false
	}

	return candidateRank >= requiredRank
}

func applyRedactionPolicyWithNotices(policy *secrets.PolicyExecutor, path, sourceType string, destination secrets.Destination, input map[string]interface{}) (map[string]interface{}, []redactionNotice) {
	if input == nil {
		return nil, nil
	}

	output := make(map[string]interface{}, len(input))
	notices := make([]redactionNotice, 0)
	for key, value := range input {
		fieldPath := path + "." + key
		sanitized, fieldNotices := applyRedactionToValue(policy, fieldPath, sourceType, destination, value)
		output[key] = sanitized
		notices = append(notices, fieldNotices...)
	}

	return output, notices
}

func applyRedactionToValue(policy *secrets.PolicyExecutor, fieldPath, sourceType string, destination secrets.Destination, value interface{}) (interface{}, []redactionNotice) {
	switch typed := value.(type) {
	case string:
		decision := policy.Apply(fieldPath, sourceType, typed, destination)
		if decision.Action == secrets.ActionAllow {
			return decision.Content, nil
		}
		notice := redactionNotice{
			Path:           fieldPath,
			Destination:    string(destination),
			Action:         string(decision.Action),
			Sensitivity:    string(decision.Classification.Class),
			Reason:         decision.Reason,
			Denied:         decision.Denied,
			MaterialImpact: decision.Denied || decision.Content != typed,
		}
		return decision.Content, []redactionNotice{notice}
	case map[string]interface{}:
		return applyRedactionPolicyWithNotices(policy, fieldPath, sourceType, destination, typed)
	case []interface{}:
		return applyRedactionToSlice(policy, fieldPath, sourceType, destination, typed)
	default:
		return value, nil
	}
}

func applyRedactionToSlice(policy *secrets.PolicyExecutor, fieldPath, sourceType string, destination secrets.Destination, input []interface{}) ([]interface{}, []redactionNotice) {
	output := make([]interface{}, 0, len(input))
	notices := make([]redactionNotice, 0)
	for idx, item := range input {
		nestedPath := fmt.Sprintf("%s[%d]", fieldPath, idx)
		sanitized, itemNotices := applyRedactionToValue(policy, nestedPath, sourceType, destination, item)
		output = append(output, sanitized)
		notices = append(notices, itemNotices...)
	}
	return output, notices
}

func (s *Server) recordRedactionEvent(ctx context.Context, toolName string, notices []redactionNotice) {
	if s.eventStore == nil || len(notices) == 0 {
		return
	}

	payload, err := json.Marshal(map[string]interface{}{
		"tool_name":           toolName,
		"destination":         string(secrets.DestinationUI),
		"redaction_notices":   notices,
		"material_impact":     hasMaterialImpact(notices),
		"redaction_applied":   true,
		"notice_count":        len(notices),
		"policy_visibility_v": "1",
	})
	if err != nil {
		log.Printf("failed to marshal redaction event payload: %v", err)
		return
	}

	err = s.eventStore.Append(ctx, audit.Event{
		ID:            uuid.NewString(),
		SessionID:     redactionAuditSessionID,
		Timestamp:     time.Now().UTC(),
		EventType:     "tool_output_redaction",
		Actor:         "system",
		PolicyVersion: "1.0.0",
		Payload:       payload,
	})
	if err != nil {
		log.Printf("failed to append redaction event: %v", err)
	}
}

func hasMaterialImpact(notices []redactionNotice) bool {
	for _, notice := range notices {
		if notice.MaterialImpact {
			return true
		}
	}
	return false
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
