package controlplane

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/agent"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	ctxmgr "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/context"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models"
	anthropicprovider "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models/anthropic"
	openaiprovider "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models/openai"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/patch"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/policy"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/requestmeta"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/sandbox"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/secrets"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
	//nolint:staticcheck // nhooyr websocket kept for Go 1.21 compatibility.
	"nhooyr.io/websocket"
)

type Config struct {
	Addr               string
	DBPath             string
	APIToken           string
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	ProviderPolicyPath string
}

func DefaultConfig() *Config {
	return &Config{
		Addr:               ":7777",
		DBPath:             "syndicatecode.db",
		ReadTimeout:        30 * time.Second,
		WriteTimeout:       30 * time.Second,
		ProviderPolicyPath: "",
	}
}

type Server struct {
	httpServer     *http.Server
	authToken      string
	metrics        *runtimeMetrics
	providerPolicy policy.ProviderPolicy
	routeEngine    *policy.RouteEngine
	sessionMgr     *session.Manager
	turnMgr        *ctxmgr.TurnManager
	ctxManifest    *ctxmgr.ContextManifest
	eventStore     *audit.EventStore
	approvalMgr    *ApprovalManager
	toolRegistry   *tools.Registry
	toolExecutor   *tools.Executor
	bus            *streamBus
	runner         *agent.Runner
}

const (
	roleViewer   = "viewer"
	roleOperator = "operator"
)

const redactionAuditSessionID = "system:redaction"
const approvalAuditSessionID = "system:approval"
const eventStreamPollInterval = 200 * time.Millisecond
const retentionCleanupInterval = time.Minute
const maxRequestBodyBytes = 1 << 20 // 1 MiB

var sessionIDPattern = regexp.MustCompile(`/api/v1/sessions/([^/]+)`)
var sessionTurnIDPattern = regexp.MustCompile(`/api/v1/sessions/[^/]+/turns/([^/]+)`)

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

type runtimeMetrics struct {
	mu                sync.Mutex
	startedAt         time.Time
	totalRequests     int64
	statusCounts      map[int]int64
	endpointCounts    map[string]int64
	toolExecutionBySE map[string]int64
}

type busEmitter struct{ bus *streamBus }

func (b busEmitter) Emit(sessionID string, e agent.AgentEvent) {
	if b.bus == nil {
		return
	}
	b.bus.publish(sessionID, streamMessage{Type: string(e.Type), Data: e.Data})
}

func newRuntimeMetrics(now time.Time) *runtimeMetrics {
	return &runtimeMetrics{
		startedAt:         now.UTC(),
		statusCounts:      map[int]int64{},
		endpointCounts:    map[string]int64{},
		toolExecutionBySE: map[string]int64{},
	}
}

func (m *runtimeMetrics) recordRequest(path string, status int) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalRequests++
	m.statusCounts[status]++
	m.endpointCounts[path]++
}

func (m *runtimeMetrics) recordToolExecution(sideEffect tools.SideEffect) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolExecutionBySE[string(sideEffect)]++
}

func (m *runtimeMetrics) snapshot(toolsRegistered int) map[string]interface{} {
	if m == nil {
		return map[string]interface{}{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	statusCounts := make(map[string]int64, len(m.statusCounts))
	for code, count := range m.statusCounts {
		statusCounts[fmt.Sprintf("%d", code)] = count
	}

	endpointCounts := make(map[string]int64, len(m.endpointCounts))
	for path, count := range m.endpointCounts {
		endpointCounts[path] = count
	}

	toolCounts := make(map[string]int64, len(m.toolExecutionBySE))
	for sideEffect, count := range m.toolExecutionBySE {
		toolCounts[sideEffect] = count
	}

	return map[string]interface{}{
		"started_at":              m.startedAt.Format(time.RFC3339Nano),
		"uptime_seconds":          int64(time.Since(m.startedAt).Seconds()),
		"total_requests":          m.totalRequests,
		"status_counts":           statusCounts,
		"endpoint_counts":         endpointCounts,
		"tool_executions_by_side": toolCounts,
		"tools_registered":        toolsRegistered,
	}
}

type telemetryResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *telemetryResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func NewServer(ctx context.Context, cfg *Config) (*Server, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	providerPolicy := policy.DefaultProviderPolicy()
	if cfg.ProviderPolicyPath != "" {
		loadedPolicy, err := policy.LoadProviderPolicy(cfg.ProviderPolicyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load provider policy: %w", err)
		}
		providerPolicy = loadedPolicy
	} else if err := providerPolicy.Validate(); err != nil {
		return nil, fmt.Errorf("invalid default provider policy: %w", err)
	}

	dbPath := cfg.DBPath
	if dbPath == "" {
		dbPath = "syndicatecode.db"
	}
	eventStore, err := audit.NewEventStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create event store: %w", err)
	}

	sessionMgr := session.NewManager(eventStore)
	turnMgr := ctxmgr.NewTurnManagerWithPolicy(eventStore, sessionMgr, newContextRedactionPolicy(secrets.NewPolicyExecutor(nil)))
	ctxManifest := ctxmgr.NewContextManifest(eventStore)

	toolRegistry, toolExecutor, err := initializeTooling(ctx, eventStore, sessionMgr)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tooling: %w", err)
	}

	approvalMgr := NewApprovalManager(15*time.Minute, WithTransitionRecorder(newApprovalTransitionRecorder(eventStore)))

	mux := http.NewServeMux()
	server := &Server{
		httpServer: &http.Server{
			Addr:           cfg.Addr,
			Handler:        nil,
			ReadTimeout:    cfg.ReadTimeout,
			WriteTimeout:   cfg.WriteTimeout,
			MaxHeaderBytes: 1 << 16, // 64 KiB
		},
		authToken:      cfg.APIToken,
		metrics:        newRuntimeMetrics(time.Now().UTC()),
		providerPolicy: providerPolicy,
		routeEngine:    buildRouteEngine(providerPolicy),
		sessionMgr:     sessionMgr,
		turnMgr:        turnMgr,
		ctxManifest:    ctxManifest,
		eventStore:     eventStore,
		approvalMgr:    approvalMgr,
		toolRegistry:   toolRegistry,
		toolExecutor:   toolExecutor,
		bus:            newStreamBus(),
	}

	if err := server.restoreRuntimeState(ctx); err != nil {
		return nil, fmt.Errorf("failed to restore runtime state: %w", err)
	}

	server.startRetentionCleanupScheduler(ctx, retentionCleanupInterval)

	server.registerRoutes(mux)
	server.httpServer.Handler = server.withTelemetry(mux)
	server.initializeAgentRunner()

	return server, nil
}

func (s *Server) startRetentionCleanupScheduler(ctx context.Context, interval time.Duration) {
	if s.eventStore == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.runRetentionCleanupPass(ctx, time.Now().UTC()); err != nil {
					log.Printf("retention cleanup failed: %v", err)
				}
			}
		}
	}()
}

func (s *Server) runRetentionCleanupPass(ctx context.Context, now time.Time) error {
	if s.eventStore == nil {
		return nil
	}
	_, err := s.eventStore.CleanupExpired(ctx, now)
	return err
}

type auditExecutionRecorder struct {
	store *audit.EventStore
}

func (r *auditExecutionRecorder) BeforeExecute(ctx context.Context, call tools.ToolCall, _ tools.ToolDefinition) {
	payload, _ := json.Marshal(map[string]string{"tool_name": call.ToolName, "call_id": call.ID})
	_ = r.store.Append(ctx, audit.Event{
		ID:        uuid.New().String(),
		SessionID: call.SessionID,
		Timestamp: time.Now().UTC(),
		EventType: audit.EventToolInvoked,
		Actor:     requestmeta.Actor(ctx),
		Payload:   payload,
	})
}

func (r *auditExecutionRecorder) AfterExecute(ctx context.Context, call tools.ToolCall, _ tools.ToolDefinition, result *tools.ToolResult, execErr error, duration time.Duration) {
	success := execErr == nil && result != nil && result.Success
	if recErr := r.store.RecordToolInvocation(ctx, audit.ToolInvocationRecord{
		ID:         uuid.New().String(),
		SessionID:  call.SessionID,
		ToolName:   call.ToolName,
		Success:    success,
		DurationMS: duration.Milliseconds(),
		CreatedAt:  time.Now().UTC(),
	}); recErr != nil {
		log.Printf("failed to record tool invocation: %v", recErr)
	}
	payload, _ := json.Marshal(map[string]string{"tool_name": call.ToolName, "success": fmt.Sprintf("%v", success)})
	_ = r.store.Append(ctx, audit.Event{
		ID:        uuid.New().String(),
		SessionID: call.SessionID,
		Timestamp: time.Now().UTC(),
		EventType: audit.EventToolResult,
		Actor:     requestmeta.Actor(ctx),
		Payload:   payload,
	})
}

func initializeTooling(ctx context.Context, eventStore *audit.EventStore, sessionMgr *session.Manager) (*tools.Registry, *tools.Executor, error) {
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
			Name:             "symbolic_shell",
			Version:          "1",
			Description:      "Runs a named CI command from the symbolic allowlist.",
			Source:           tools.ToolSourceCore,
			TrustLevel:       "tier1",
			SideEffect:       tools.SideEffectExecute,
			ApprovalRequired: true,
			InputSchema: map[string]tools.FieldSchema{
				"command": {Type: "string", Description: "symbolic command name", Required: true},
			},
			OutputSchema: map[string]tools.FieldSchema{
				"stdout":    {Type: "string", Description: "stdout"},
				"stderr":    {Type: "string", Description: "stderr"},
				"exit_code": {Type: "integer", Description: "exit code"},
			},
			Limits:   tools.ExecutionLimits{TimeoutSeconds: 300, MaxOutputBytes: 512 * 1024},
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
	if eventStore != nil {
		executor.SetRecorder(&auditExecutionRecorder{store: eventStore})
	}
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

	commandExecutor := sandbox.NewMemoryBoundedExecutor(
		sandbox.NewLimitedExecutor(
			sandbox.NewSymbolicCommandExecutor(sandbox.DefaultSymbolicCommands()),
		),
		2,
	)

	allowedEnv := []string{"PATH", "HOME", "TERM", "GOCACHE", "GOMODCACHE", "GOROOT", "GOPATH"}
	l1Runner := sandbox.NewL1Runner(repoRoot, allowedEnv, commandExecutor)
	l2Runner := sandbox.NewL2Runner(repoRoot, allowedEnv, commandExecutor)

	executor.RegisterHandler("echo", tools.EchoHandler())
	executor.RegisterHandler("read_file", tools.ReadFileHandler(repoRoot))
	executor.RegisterHandler("write_file", tools.WriteFileHandler(repoRoot))
	executor.RegisterHandler("run_tests", sandbox.RunTestsHandler(runner))
	executor.RegisterHandler("run_lint", sandbox.RunLintHandler(runner))
	executor.RegisterHandler("restricted_shell", sandbox.RestrictedShellByTrustHandler(
		func(handlerCtx context.Context, sessionID string) (string, error) {
			sess, err := sessionMgr.Get(handlerCtx, sessionID)
			if err != nil {
				return "", err
			}
			return sess.TrustTier, nil
		},
		l1Runner,
		l2Runner,
	))
	symExec := sandbox.NewSymbolicCommandExecutor(sandbox.DefaultSymbolicCommands())
	executor.RegisterHandler("symbolic_shell", func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		cmd, _ := input["command"].(string)
		if strings.TrimSpace(cmd) == "" {
			return nil, fmt.Errorf("command is required")
		}
		result, err := symExec.Run(ctx, cmd, nil, sandbox.SubprocessOptions{WorkingDir: repoRoot, Timeout: 300 * time.Second, MaxOutputBytes: 512 * 1024})
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"stdout":    result.Stdout,
			"stderr":    result.Stderr,
			"exit_code": result.ExitCode,
		}, nil
	})
	executor.RegisterHandler("apply_patch", tools.ApplyPatchHandler(
		patch.NewEngine(repoRoot),
		func(ctx context.Context, path, mutationType, beforeHash, afterHash string) {
			if eventStore == nil {
				return
			}
			if err := eventStore.RecordFileMutation(ctx, audit.FileMutationRecord{
				ID:           uuid.New().String(),
				Path:         path,
				MutationType: mutationType,
				BeforeHash:   beforeHash,
				AfterHash:    afterHash,
				AppliedAt:    time.Now().UTC(),
			}); err != nil {
				log.Printf("failed to record file mutation: %v", err)
			}
			payload, _ := json.Marshal(map[string]string{"path": path, "type": mutationType})
			_ = eventStore.Append(ctx, audit.Event{
				ID:        uuid.New().String(),
				Timestamp: time.Now().UTC(),
				EventType: audit.EventFileMutation,
				Payload:   payload,
			})
		},
	))

	return registry, executor, nil
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.Handle("/api/v1/health", schemaValidationMiddleware(
		nil,
		map[string]jsonObjectSchema{http.MethodGet: healthResponseSchema()},
		http.HandlerFunc(s.handleHealth),
	))
	mux.Handle("/api/v1/readiness", schemaValidationMiddleware(
		nil,
		map[string]jsonObjectSchema{http.MethodGet: readinessResponseSchema()},
		http.HandlerFunc(s.handleReadiness),
	))
	mux.Handle("/api/v1/metrics", s.withAuth(schemaValidationMiddleware(
		nil,
		map[string]jsonObjectSchema{http.MethodGet: metricsResponseSchema()},
		http.HandlerFunc(s.handleMetrics),
	)))
	mux.Handle("/api/v1/sessions", s.withAuth(schemaValidationMiddleware(
		map[string]jsonObjectSchema{http.MethodPost: sessionsCreateRequestSchema()},
		map[string]jsonObjectSchema{http.MethodPost: sessionsCreateResponseSchema()},
		http.HandlerFunc(s.handleSessions),
	)))
	mux.Handle("/api/v1/sessions/", s.withAuth(schemaValidationMiddleware(
		map[string]jsonObjectSchema{http.MethodPost: sessionTurnsCreateRequestSchema()},
		map[string]jsonObjectSchema{http.MethodPost: turnsCreateResponseSchema()},
		http.HandlerFunc(s.handleSessionOrTurn),
	)))
	mux.Handle("/api/v1/turns", s.withAuth(schemaValidationMiddleware(
		map[string]jsonObjectSchema{http.MethodPost: turnsCreateRequestSchema()},
		map[string]jsonObjectSchema{http.MethodPost: turnsCreateResponseSchema()},
		http.HandlerFunc(s.handleTurns),
	)))
	mux.Handle("/api/v1/turns/", s.withAuth(http.HandlerFunc(s.handleTurnByID)))
	mux.Handle("/api/v1/approvals", s.withAuth(http.HandlerFunc(s.handleApprovals)))
	mux.Handle("/api/v1/approvals/", s.withAuth(schemaValidationMiddleware(
		map[string]jsonObjectSchema{http.MethodPost: approvalsDecisionRequestSchema()},
		map[string]jsonObjectSchema{http.MethodPost: approvalsDecisionResponseSchema()},
		http.HandlerFunc(s.handleApprovalByID),
	)))
	mux.Handle("/api/v1/policy", s.withAuth(schemaValidationMiddleware(
		nil,
		map[string]jsonObjectSchema{http.MethodGet: policyResponseSchema()},
		http.HandlerFunc(s.handlePolicy),
	)))
	mux.Handle("/api/v1/events/stream", s.withAuth(http.HandlerFunc(s.handleEventStream)))
	mux.Handle("/api/v1/tools", s.withAuth(schemaValidationMiddleware(
		nil,
		map[string]jsonObjectSchema{http.MethodGet: toolsListResponseSchema()},
		http.HandlerFunc(s.handleTools),
	)))
	mux.Handle("/api/v1/tools/execute", s.withAuth(schemaValidationMiddlewareWithStatus(
		map[string]jsonObjectSchema{http.MethodPost: toolsExecuteRequestSchema()},
		map[string]jsonObjectSchema{http.MethodPost: toolsExecuteResponseSchema()},
		responseSchemaByStatus{http.MethodPost: {http.StatusAccepted: approvalRecordSchema()}},
		http.HandlerFunc(s.handleToolExecute),
	)))
}

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if s.toolRegistry == nil {
		writeStatusError(w, http.StatusInternalServerError, "tool registry unavailable")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"tools": s.toolRegistry.List()}); err != nil {
		log.Printf("failed to encode tools response: %v", err)
	}
}

func (s *Server) withTelemetry(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		tw := &telemetryResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(tw, r)

		statusCode := tw.statusCode
		s.metrics.recordRequest(r.URL.Path, statusCode)
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = "n/a"
		}
		log.Printf("event=request method=%s path=%s status=%d duration_ms=%d actor=%s role=%s request_id=%s", r.Method, r.URL.Path, statusCode, time.Since(start).Milliseconds(), requestActor(r.Context()), requestRole(r.Context()), requestID) // #nosec G706 -- structured log with controlled format string
	})
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	if strings.TrimSpace(s.authToken) == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			s.writeUnauthorized(w)
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.authToken)) != 1 {
			s.writeUnauthorized(w)
			return
		}

		role := normalizedRole(r.Header.Get("X-Syndicate-Role"))
		if role == "" {
			writeStatusError(w, http.StatusForbidden, "invalid role")
			return
		}
		actor := strings.TrimSpace(r.Header.Get("X-Syndicate-Actor"))
		if actor == "" {
			actor = "api-client"
		}

		ctx := requestmeta.WithActor(r.Context(), actor)
		ctx = requestmeta.WithRole(ctx, role)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func normalizedRole(value string) string {
	role := strings.ToLower(strings.TrimSpace(value))
	switch role {
	case "", roleOperator:
		return roleOperator
	case roleViewer:
		return roleViewer
	default:
		return ""
	}
}

func requestActor(ctx context.Context) string {
	return requestmeta.Actor(ctx)
}

func requestRole(ctx context.Context) string {
	return requestmeta.Role(ctx)
}

func requireOperatorRole(w http.ResponseWriter, r *http.Request) bool {
	if requestRole(r.Context()) != roleOperator {
		writeStatusError(w, http.StatusForbidden, "operator role required")
		return false
	}
	return true
}

func (s *Server) enforceTrustTierToolPolicy(ctx context.Context, sessionID string, toolDef tools.ToolDefinition) error {
	if requestRole(ctx) == roleViewer && toolDef.SideEffect != tools.SideEffectRead && toolDef.SideEffect != tools.SideEffectNone {
		return fmt.Errorf("policy denied: viewer role can only execute read-only tools")
	}

	if sessionID == "" || s.sessionMgr == nil {
		return nil
	}

	sess, err := s.sessionMgr.Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to resolve session trust tier: %w", err)
	}

	engine := policy.NewEngine()
	engine.AddRule(policy.Rule{
		Name:        "viewer_no_mutations",
		Description: "viewer role can only execute read-only tools",
		Effect:      policy.EffectDeny,
		Condition: func(ec *policy.EvaluationContext) bool {
			role, _ := ec.Input["role"].(string)
			if role != roleViewer {
				return false
			}
			return ec.ToolSideEffect != policy.SideEffectRead && ec.ToolSideEffect != policy.SideEffectNone
		},
	})
	engine.AddRule(policy.Rule{
		Name:        "tier0_no_mutations",
		Description: "tier0 cannot execute write/execute/network tools",
		Effect:      policy.EffectDeny,
		Condition: func(ec *policy.EvaluationContext) bool {
			tier, _ := ec.Input["trust_tier"].(string)
			if tier != "tier0" {
				return false
			}
			return ec.ToolSideEffect == policy.SideEffectWrite || ec.ToolSideEffect == policy.SideEffectExecute || ec.ToolSideEffect == policy.SideEffectNetwork
		},
	})
	engine.AddRule(policy.Rule{
		Name:        "tier3_no_plugins",
		Description: "tier3 cannot execute plugin tools",
		Effect:      policy.EffectDeny,
		Condition: func(ec *policy.EvaluationContext) bool {
			tier, _ := ec.Input["trust_tier"].(string)
			source, _ := ec.Input["tool_source"].(string)
			return tier == "tier3" && source == tools.ToolSourcePlugin
		},
	})
	engine.AddRule(policy.Rule{
		Name:        "tier3_no_exec_or_network",
		Description: "tier3 cannot execute shell or network tools",
		Effect:      policy.EffectDeny,
		Condition: func(ec *policy.EvaluationContext) bool {
			tier, _ := ec.Input["trust_tier"].(string)
			if tier != "tier3" {
				return false
			}
			return ec.ToolSideEffect == policy.SideEffectExecute || ec.ToolSideEffect == policy.SideEffectNetwork
		},
	})

	eval := engine.Evaluate(&policy.EvaluationContext{
		ToolName:       toolDef.Name,
		ToolSideEffect: mapToolSideEffect(toolDef.SideEffect),
		Input: map[string]interface{}{
			"trust_tier":  sess.TrustTier,
			"tool_source": toolDef.Source,
			"role":        requestRole(ctx),
		},
		Session: sessionID,
		User:    requestActor(ctx),
	})

	if !eval.Allowed {
		return fmt.Errorf("policy denied: %s", eval.Reason)
	}

	return nil
}

func mapToolSideEffect(effect tools.SideEffect) policy.SideEffect {
	switch effect {
	case tools.SideEffectRead:
		return policy.SideEffectRead
	case tools.SideEffectWrite:
		return policy.SideEffectWrite
	case tools.SideEffectExecute:
		return policy.SideEffectExecute
	case tools.SideEffectNetwork:
		return policy.SideEffectNetwork
	default:
		return policy.SideEffectNone
	}
}

func (s *Server) writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="syndicatecode"`)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	if err := json.NewEncoder(w).Encode(ErrorEnvelope{Type: "unauthorized", Reason: "missing or invalid bearer token", Retryable: false}); err != nil {
		log.Printf("failed to encode unauthorized response: %v", err)
	}
}

func writeStatusError(w http.ResponseWriter, status int, reason string) {
	WriteError(w, status, errors.New(reason))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		log.Printf("failed to encode health response: %v", err)
	}
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	status := "ready"
	db := "ok"
	schemaVersion := 0
	schemaStatus := "ok"
	if s.eventStore == nil {
		status = "degraded"
		db = "unavailable"
		schemaStatus = "unavailable"
	} else {
		if err := s.eventStore.Ping(r.Context()); err != nil {
			status = "degraded"
			db = "unreachable"
			schemaStatus = "unavailable"
		}
		if version, err := s.eventStore.SchemaVersion(); err == nil {
			schemaVersion = version
		} else {
			status = "degraded"
			schemaStatus = "unavailable"
		}
	}
	toolsRegistered := 0
	if s.toolRegistry != nil {
		toolsRegistered = len(s.toolRegistry.List())
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":           status,
		"db":               db,
		"schema_version":   schemaVersion,
		"schema_status":    schemaStatus,
		"tools_registered": toolsRegistered,
	}); err != nil {
		log.Printf("failed to encode readiness response: %v", err)
	}
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !requireOperatorRole(w, r) {
		return
	}

	toolsRegistered := 0
	if s.toolRegistry != nil {
		toolsRegistered = len(s.toolRegistry.List())
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(s.metrics.snapshot(toolsRegistered)); err != nil {
		log.Printf("failed to encode metrics response: %v", err)
	}
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listSessions(w, r)
	case http.MethodPost:
		s.createSession(w, r)
	default:
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.sessionMgr.List(r.Context())
	if err != nil {
		writeStatusError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := json.NewEncoder(w).Encode(sessions); err != nil {
		log.Printf("failed to encode sessions: %v", err)
	}
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	if !requireOperatorRole(w, r) {
		return
	}

	var req struct {
		RepoPath  string `json:"repo_path"`
		TrustTier string `json:"trust_tier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeStatusError(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := s.sessionMgr.Create(r.Context(), req.RepoPath, req.TrustTier)
	if err != nil {
		writeStatusError(w, http.StatusInternalServerError, err.Error())
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
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 2 && parts[1] == "events" {
		s.handleSessionEvents(w, r, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "export" {
		s.handleSessionExport(w, r, parts[0])
		return
	}
	if len(parts) != 1 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	sessionID := parts[0]
	session, err := s.sessionMgr.Get(r.Context(), sessionID)
	if err != nil {
		writeStatusError(w, http.StatusNotFound, err.Error())
		return
	}
	if err := json.NewEncoder(w).Encode(session); err != nil {
		log.Printf("failed to encode session: %v", err)
	}
}

func (s *Server) handleSessionEvents(w http.ResponseWriter, r *http.Request, sessionID string) {
	if s.sessionMgr != nil {
		if _, err := s.sessionMgr.Get(r.Context(), sessionID); err != nil {
			writeStatusError(w, http.StatusNotFound, "session not found")
			return
		}
	}

	events, err := s.eventStore.QueryBySession(r.Context(), sessionID)
	if err != nil {
		writeStatusError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.sessionMgr == nil && len(events) == 0 {
		writeStatusError(w, http.StatusNotFound, "session not found")
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

	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Timestamp.Equal(events[j].Timestamp) {
			return events[i].ID < events[j].ID
		}
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	if err := json.NewEncoder(w).Encode(events); err != nil {
		log.Printf("failed to encode session events: %v", err)
	}
}

func (s *Server) handleSessionExport(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodGet {
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	includeArtifacts, err := parseIncludeArtifactsParam(r.URL.Query().Get("include_artifacts"))
	if err != nil {
		writeStatusError(w, http.StatusBadRequest, err.Error())
		return
	}
	if includeArtifacts && !requireOperatorRole(w, r) {
		return
	}

	sess, err := s.sessionMgr.Get(r.Context(), sessionID)
	if err != nil {
		writeStatusError(w, http.StatusNotFound, "session not found")
		return
	}

	events, err := s.eventStore.QueryBySession(r.Context(), sessionID)
	if err != nil {
		writeStatusError(w, http.StatusInternalServerError, err.Error())
		return
	}

	policy := secrets.NewPolicyExecutor(nil)
	redactedCount := 0
	redactionReasons := make([]string, 0)
	filtered := make([]audit.Event, 0, len(events))
	for _, e := range events {
		sanitized, eventRedactionReasons, eventRedacted, sanitizeErr := redactEventForExport(e, policy)
		if sanitizeErr != nil {
			writeStatusError(w, http.StatusInternalServerError, sanitizeErr.Error())
			return
		}
		if eventRedacted {
			redactedCount++
		}
		redactionReasons = append(redactionReasons, eventRedactionReasons...)
		filtered = append(filtered, sanitized)
	}

	redactionReason := summarizeRedactionReasons(redactionReasons)

	export := map[string]interface{}{
		"schema_version": "1",
		"exported_at":    time.Now().UTC().Format(time.RFC3339),
		"session":        sess,
		"events":         filtered,
		"redaction_summary": map[string]interface{}{
			"redacted_count": redactedCount,
			"reason":         redactionReason,
		},
	}

	if includeArtifacts {
		artifacts, artifactErr := s.eventStore.ListArtifactsBySession(r.Context(), sessionID)
		if artifactErr != nil {
			writeStatusError(w, http.StatusInternalServerError, artifactErr.Error())
			return
		}
		export["artifacts"] = artifacts
	}

	if err := json.NewEncoder(w).Encode(export); err != nil {
		log.Printf("failed to encode export: %v", err)
	}
}

func parseIncludeArtifactsParam(raw string) (bool, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return false, nil
	}

	include, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("include_artifacts must be true or false")
	}

	return include, nil
}

func redactEventForExport(event audit.Event, policy *secrets.PolicyExecutor) (audit.Event, []string, bool, error) {
	if event.EventType == audit.EventToolRedaction {
		return audit.Event{
			ID:        event.ID,
			Timestamp: event.Timestamp,
			EventType: "[redacted]",
		}, []string{"tool_output_redaction markers inserted"}, true, nil
	}

	if len(event.Payload) == 0 || string(event.Payload) == "null" {
		return event, nil, false, nil
	}

	var payload interface{}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return audit.Event{}, nil, false, fmt.Errorf("failed to decode event payload for export redaction: %w", err)
	}

	sanitizedPayload, notices := applyRedactionToValue(policy, "events.payload", "event_payload", secrets.DestinationExport, payload)
	if len(notices) == 0 {
		return event, nil, false, nil
	}

	encodedPayload, err := json.Marshal(sanitizedPayload)
	if err != nil {
		return audit.Event{}, nil, false, fmt.Errorf("failed to encode redacted event payload: %w", err)
	}

	reasons := make([]string, 0, len(notices))
	for _, notice := range notices {
		reasons = append(reasons, notice.Reason)
	}

	event.Payload = encodedPayload
	return event, reasons, true, nil
}

func summarizeRedactionReasons(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}

	unique := make(map[string]struct{}, len(reasons))
	for _, reason := range reasons {
		if strings.TrimSpace(reason) == "" {
			continue
		}
		unique[reason] = struct{}{}
	}
	if len(unique) == 0 {
		return ""
	}

	ordered := make([]string, 0, len(unique))
	for reason := range unique {
		ordered = append(ordered, reason)
	}
	sort.Strings(ordered)

	return strings.Join(ordered, "; ")
}

func (s *Server) handleTurns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listTurns(w, r)
	case http.MethodPost:
		s.createTurn(w, r)
	default:
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) listTurns(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		writeStatusError(w, http.StatusBadRequest, "session_id required")
		return
	}

	turns, err := s.turnMgr.ListBySession(r.Context(), sessionID)
	if err != nil {
		writeStatusError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := json.NewEncoder(w).Encode(turns); err != nil {
		log.Printf("failed to encode turns: %v", err)
	}
}

func (s *Server) createTurn(w http.ResponseWriter, r *http.Request) {
	if !requireOperatorRole(w, r) {
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	turn, err := s.turnMgr.Create(r.Context(), req.SessionID, req.Message)
	if err != nil {
		if errors.Is(err, ctxmgr.ErrActiveMutableTurnConflict) {
			writeStatusError(w, http.StatusConflict, err.Error())
			return
		}
		writeStatusError(w, http.StatusInternalServerError, err.Error())
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
		writeStatusError(w, http.StatusNotFound, err.Error())
		return
	}
	if err := json.NewEncoder(w).Encode(turn); err != nil {
		log.Printf("failed to encode turn: %v", err)
	}
}

func (s *Server) handleTurnContext(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/turns/"), "/")
	if len(parts) < 2 {
		writeStatusError(w, http.StatusBadRequest, "turn_id required")
		return
	}
	turnID := parts[0]

	fragments, err := s.ctxManifest.Get(r.Context(), turnID)
	if err != nil {
		writeStatusError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := json.NewEncoder(w).Encode(fragments); err != nil {
		log.Printf("failed to encode fragments: %v", err)
	}
}

func (s *Server) handleSessionTurns(w http.ResponseWriter, r *http.Request) {
	sessionID := extractSessionID(r.URL.Path)
	if sessionID == "" {
		writeStatusError(w, http.StatusBadRequest, "session_id required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.listSessionTurns(w, r, sessionID)
	case http.MethodPost:
		s.createSessionTurn(w, r, sessionID)
	default:
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) listSessionTurns(w http.ResponseWriter, r *http.Request, sessionID string) {
	if !s.requireSession(w, r, sessionID) {
		return
	}

	turns, err := s.turnMgr.ListBySession(r.Context(), sessionID)
	if err != nil {
		writeStatusError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := json.NewEncoder(w).Encode(turns); err != nil {
		log.Printf("failed to encode turns: %v", err)
	}
}

func (s *Server) createSessionTurn(w http.ResponseWriter, r *http.Request, sessionID string) {
	if !requireOperatorRole(w, r) {
		return
	}

	if !s.requireSession(w, r, sessionID) {
		return
	}

	var req struct {
		Message string   `json:"message"`
		Files   []string `json:"files,omitempty"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.Message == "" {
		writeStatusError(w, http.StatusBadRequest, "message is required")
		return
	}

	turn, err := s.turnMgr.Create(r.Context(), sessionID, req.Message)
	if err != nil {
		if errors.Is(err, ctxmgr.ErrActiveMutableTurnConflict) {
			writeStatusError(w, http.StatusConflict, err.Error())
			return
		}
		writeStatusError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.runner != nil {
		sess, sessErr := s.sessionMgr.Get(r.Context(), sessionID)
		if sessErr == nil {
			sessionRunner := s.runner.WithConfig(agent.DefaultConfig(sess.TrustTier))
			go s.runAgentTurn(context.WithoutCancel(r.Context()), sessionRunner, agent.AgentTurn{
				ID:        turn.ID,
				SessionID: sessionID,
				Message:   req.Message,
				TrustTier: sess.TrustTier,
				Files:     req.Files,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(turn); err != nil {
		log.Printf("failed to encode turn: %v", err)
	}
}

func (s *Server) runAgentTurn(ctx context.Context, runner *agent.Runner, turn agent.AgentTurn) {
	s.recordModelInvocation(ctx, runner, turn)

	ch, err := runner.RunTurn(ctx, turn)
	if err != nil {
		log.Printf("agent RunTurn error: %v", err)
		return
	}

	for evt := range ch {
		if s.eventStore == nil {
			continue
		}
		payload, marshalErr := json.Marshal(map[string]interface{}{
			"type":        string(evt.Type),
			"data":        evt.Data,
			"stop_reason": evt.StopReason,
			"tool_id":     evt.ToolID,
			"tool_name":   evt.ToolName,
			"approval_id": evt.ApprovalID,
		})
		if marshalErr != nil {
			continue
		}
		_ = s.eventStore.Append(ctx, audit.Event{
			ID:        uuid.NewString(),
			SessionID: turn.SessionID,
			TurnID:    turn.ID,
			Timestamp: time.Now().UTC(),
			EventType: string(evt.Type),
			Actor:     "agent",
			Payload:   payload,
		})

		if evt.Type == agent.EventAwaitingApproval {
			s.appendTurnLifecycleEventWithCausality(ctx, turn.SessionID, ctxmgr.TurnStatusAwaitingApproval, "agent_awaiting_approval", map[string]interface{}{
				"approval_id": evt.ApprovalID,
				"tool_name":   evt.ToolName,
				"turn_id":     turn.ID,
			})
		}
	}
}

func (s *Server) recordModelInvocation(ctx context.Context, runner *agent.Runner, turn agent.AgentTurn) {
	if s == nil || s.eventStore == nil || runner == nil {
		return
	}

	now := time.Now().UTC()
	provider, model := runner.ModelMetadata()
	_ = s.eventStore.EnsureSessionRecord(ctx, turn.SessionID, turn.TrustTier, now)
	_ = s.eventStore.EnsureTurnRecord(ctx, turn.ID, turn.SessionID, now)

	record := audit.ModelInvocationRecord{
		ID:        uuid.NewString(),
		SessionID: turn.SessionID,
		TurnID:    turn.ID,
		Provider:  provider,
		Model:     model,
		CreatedAt: now,
	}
	_ = s.eventStore.RecordModelInvocation(ctx, record)

	payload, err := json.Marshal(map[string]interface{}{
		"provider":   provider,
		"model":      model,
		"session_id": turn.SessionID,
		"turn_id":    turn.ID,
	})
	if err != nil {
		return
	}

	_ = s.eventStore.Append(ctx, audit.Event{
		ID:        uuid.NewString(),
		SessionID: turn.SessionID,
		TurnID:    turn.ID,
		Timestamp: now,
		EventType: audit.EventModelInvoked,
		Actor:     "agent",
		Payload:   payload,
	})
}

func (s *Server) requireSession(w http.ResponseWriter, r *http.Request, sessionID string) bool {
	if _, err := s.sessionMgr.Get(r.Context(), sessionID); err != nil {
		writeStatusError(w, http.StatusNotFound, err.Error())
		return false
	}

	return true
}

func (s *Server) handleSessionTurnByID(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	sessionIDMatch := sessionIDPattern.FindStringSubmatch(path)
	if sessionIDMatch == nil {
		writeStatusError(w, http.StatusBadRequest, "session_id required")
		return
	}
	sessionID := sessionIDMatch[1]

	if strings.Contains(path, "/context") {
		s.handleSessionTurnContext(w, r, sessionID)
		return
	}

	_, turn, ok := s.loadSessionTurn(w, r, sessionID)
	if !ok {
		return
	}
	if err := json.NewEncoder(w).Encode(turn); err != nil {
		log.Printf("failed to encode turn: %v", err)
	}
}

func (s *Server) handleSessionTurnContext(w http.ResponseWriter, r *http.Request, sessionID string) {
	turnID, _, ok := s.loadSessionTurn(w, r, sessionID)
	if !ok {
		return
	}

	fragments, err := s.ctxManifest.Get(r.Context(), turnID)
	if err != nil {
		writeStatusError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := json.NewEncoder(w).Encode(fragments); err != nil {
		log.Printf("failed to encode fragments: %v", err)
	}
}

func (s *Server) loadSessionTurn(w http.ResponseWriter, r *http.Request, sessionID string) (string, *ctxmgr.Turn, bool) {
	turnIDMatch := sessionTurnIDPattern.FindStringSubmatch(r.URL.Path)
	if turnIDMatch == nil {
		writeStatusError(w, http.StatusBadRequest, "turn_id required")
		return "", nil, false
	}
	turnID := turnIDMatch[1]

	turn, err := s.turnMgr.Get(r.Context(), turnID)
	if err != nil {
		writeStatusError(w, http.StatusNotFound, err.Error())
		return "", nil, false
	}
	if turn.SessionID != sessionID {
		writeStatusError(w, http.StatusNotFound, "turn does not belong to session")
		return "", nil, false
	}

	return turnID, turn, true
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
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
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
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !requireOperatorRole(w, r) {
		return
	}
	approvalID := strings.TrimPrefix(r.URL.Path, "/api/v1/approvals/")
	if approvalID == "" {
		writeStatusError(w, http.StatusBadRequest, "approval_id required")
		return
	}

	var req struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason,omitempty"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	approval, err := s.approvalMgr.Decide(approvalID, req.Decision, req.Reason)
	if err != nil {
		writeStatusError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.appendApprovalEvent(r.Context(), audit.EventApprovalDecided, approval.SessionID, map[string]string{
		"approval_id": approval.ID,
		"decision":    req.Decision,
		"reason":      req.Reason,
	}); err != nil {
		writeStatusError(w, http.StatusInternalServerError, err.Error())
		return
	}

	switch req.Decision {
	case "approve":
		computedHash, hashErr := hashToolCall(approval.Call)
		if hashErr != nil {
			writeStatusError(w, http.StatusInternalServerError, fmt.Sprintf("failed to validate approved action: %v", hashErr))
			return
		}
		if computedHash != approval.ArgumentsHash {
			_, _ = s.approvalMgr.Cancel(approvalID, "tool arguments mutated after approval", "arguments_mutated")
			s.appendTurnLifecycleEvent(r.Context(), approval.SessionID, ctxmgr.TurnStatusCancelled, "approval_payload_mutated")
			writeStatusError(w, http.StatusConflict, "approved action payload no longer matches recorded hash")
			return
		}

		toolDef, toolFound := s.toolRegistry.Get(approval.Call.ToolName)
		if toolFound {
			if visibilityErr := s.enforceToolVisibilityForSession(r.Context(), approval.SessionID, toolDef); visibilityErr != nil {
				_, _ = s.approvalMgr.Cancel(approvalID, fmt.Sprintf("visibility denied at execution: %v", visibilityErr), "visibility_denied")
				s.appendTurnLifecycleEvent(r.Context(), approval.SessionID, ctxmgr.TurnStatusCancelled, "approval_execution_visibility_denied")
				writeStatusError(w, http.StatusForbidden, visibilityErr.Error())
				return
			}
			if policyErr := s.enforceTrustTierToolPolicy(r.Context(), approval.SessionID, toolDef); policyErr != nil {
				_, _ = s.approvalMgr.Cancel(approvalID, fmt.Sprintf("policy denied at execution: %v", policyErr), "policy_denied")
				s.appendTurnLifecycleEvent(r.Context(), approval.SessionID, ctxmgr.TurnStatusCancelled, "approval_execution_policy_denied")
				writeStatusError(w, http.StatusForbidden, policyErr.Error())
				return
			}
		}

		result, execErr := s.toolExecutor.Execute(r.Context(), approval.Call)
		if toolFound {
			s.metrics.recordToolExecution(toolDef.SideEffect)
			s.recordMCPCallEvent(r.Context(), approval.Call, toolDef, result, execErr)
		}
		if execErr != nil {
			_, _ = s.approvalMgr.Cancel(approvalID, fmt.Sprintf("execution failed: %v", execErr), "execution_failed")
			s.appendTurnLifecycleEvent(r.Context(), approval.SessionID, ctxmgr.TurnStatusFailed, "approval_execution_failed")
			writeStatusError(w, http.StatusInternalServerError, fmt.Sprintf("failed to execute approved action: %v", execErr))
			return
		}
		if markErr := s.approvalMgr.MarkExecuted(approvalID); markErr != nil {
			writeStatusError(w, http.StatusInternalServerError, markErr.Error())
			return
		}
		updated, ok := s.approvalMgr.Get(approvalID)
		if ok {
			approval = &updated
		}

		if err := s.appendApprovalEvent(r.Context(), audit.EventApprovalExecuted, approval.SessionID, map[string]string{
			"approval_id": approval.ID,
		}); err != nil {
			writeStatusError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.appendTurnLifecycleEvent(r.Context(), approval.SessionID, ctxmgr.TurnStatusCompleted, "approval_executed")
	case "deny":
		s.appendTurnLifecycleEvent(r.Context(), approval.SessionID, ctxmgr.TurnStatusCancelled, "approval_denied")
	}

	if err := json.NewEncoder(w).Encode(approval); err != nil {
		log.Printf("failed to encode approval: %v", err)
	}
}

func (s *Server) handlePolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	response := map[string]interface{}{
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
		"provider_policy": s.providerPolicy,
	}

	trustTier := strings.TrimSpace(r.URL.Query().Get("trust_tier"))
	sensitivity := strings.TrimSpace(r.URL.Query().Get("sensitivity"))
	task := strings.TrimSpace(r.URL.Query().Get("task"))
	if trustTier != "" && sensitivity != "" && task != "" && s.routeEngine != nil {
		decision, err := s.routeEngine.Select(policy.RouteRequest{
			TrustTier:        trustTier,
			SensitivityClass: sensitivity,
			Task:             task,
			PreferLocal:      true,
		})
		if err != nil {
			writeStatusError(w, http.StatusBadRequest, err.Error())
			return
		}
		response["route_decision"] = decision
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("failed to encode policy: %v", err)
	}
}

func buildRouteEngine(providerPolicy policy.ProviderPolicy) *policy.RouteEngine {
	routes := make([]policy.ProviderRoute, 0, len(providerPolicy.Providers))
	for idx, provider := range providerPolicy.Providers {
		routes = append(routes, policy.ProviderRoute{
			Name:               provider.Name,
			TrustTiers:         append([]string(nil), provider.TrustTiers...),
			SensitivityClasses: append([]string(nil), provider.Sensitivity...),
			Tasks:              append([]string(nil), provider.Tasks...),
			Capabilities:       []string{"text"},
			Local:              strings.HasPrefix(provider.Name, "local"),
			EstimatedLatencyMS: 50 + idx*5,
			EstimatedCostUSD:   float64(idx) * 0.001,
		})
	}

	return policy.NewRouteEngine(routes)
}

func (s *Server) initializeAgentRunner() {
	if s == nil || s.toolRegistry == nil || s.toolExecutor == nil || s.approvalMgr == nil {
		return
	}

	providerRegistry := models.NewRegistry()
	if key := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); key != "" {
		providerRegistry.Register("anthropic", anthropicprovider.NewProvider(key))
	}
	if key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); key != "" {
		providerRegistry.Register("openai", openaiprovider.NewProvider(key))
	}

	var modelInstance models.LanguageModel
	if resolved, err := providerRegistry.Resolve("anthropic", "claude-sonnet-4-6"); err == nil {
		modelInstance = resolved
	} else if resolved, err := providerRegistry.Resolve("openai", "gpt-4o"); err == nil {
		modelInstance = resolved
	}

	if modelInstance == nil {
		return
	}

	if s.bus == nil {
		s.bus = newStreamBus()
	}
	s.runner = agent.NewRunner(modelInstance, s.toolRegistry, s.toolExecutor, s, busEmitter{bus: s.bus}, agent.DefaultConfig("tier1"))
}

func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	cursorParam := r.URL.Query().Get("cursor")
	cursor, err := parseEventCursor(cursorParam)
	if err != nil {
		writeStatusError(w, http.StatusBadRequest, fmt.Sprintf("invalid cursor: %v", err))
		return
	}

	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		writeStatusError(w, http.StatusBadRequest, "session_id query parameter is required")
		return
	}
	if s.sessionMgr != nil {
		if _, err := s.sessionMgr.Get(r.Context(), sessionID); err != nil {
			writeStatusError(w, http.StatusNotFound, "session not found")
			return
		}
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	//nolint:staticcheck // nhooyr websocket kept for Go 1.21 compatibility.
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Printf("failed to accept event stream websocket: %v", err)
		writeStatusError(w, http.StatusBadRequest, "websocket upgrade failed")
		return
	}

	go func() {
		defer cancel()
		for {
			//nolint:staticcheck // nhooyr websocket kept for Go 1.21 compatibility.
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
		}
	}()

	if s.bus == nil {
		s.bus = newStreamBus()
	}
	busC, busUnsub := s.bus.subscribe(sessionID)
	defer busUnsub()

	ticker := time.NewTicker(eventStreamPollInterval)
	defer ticker.Stop()

	for {
		cursor, err = s.streamNewEvents(ctx, conn, sessionID, cursor)
		if err != nil {
			log.Printf("failed to stream events: %v", err)
			//nolint:staticcheck // nhooyr websocket kept for Go 1.21 compatibility.
			_ = conn.Close(websocket.StatusInternalError, "stream error")
			return
		}

		select {
		case msg, ok := <-busC:
			if !ok {
				return
			}
			payload, marshalErr := json.Marshal(msg)
			if marshalErr != nil {
				continue
			}
			//nolint:staticcheck // nhooyr websocket kept for Go 1.21 compatibility.
			if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
				_ = conn.Close(websocket.StatusInternalError, "stream error")
				return
			}
		case <-ctx.Done():
			//nolint:staticcheck // nhooyr websocket kept for Go 1.21 compatibility.
			_ = conn.Close(websocket.StatusNormalClosure, "")
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) RequestApproval(ctx context.Context, call tools.ToolCall) (agent.ApprovalResult, error) {
	if s == nil || s.toolRegistry == nil || s.approvalMgr == nil {
		return agent.ApprovalResult{Approved: true}, nil
	}

	toolDef, ok := s.toolRegistry.Get(call.ToolName)
	if !ok || !toolDef.ApprovalRequired {
		return agent.ApprovalResult{Approved: true}, nil
	}
	if err := s.enforceToolVisibilityForSession(ctx, call.SessionID, toolDef); err != nil {
		return agent.ApprovalResult{}, err
	}
	if err := s.enforceTrustTierToolPolicy(ctx, call.SessionID, toolDef); err != nil {
		return agent.ApprovalResult{}, err
	}

	execCtxMap := buildExecutionContext(call, toolDef)
	execCtxBytes, _ := json.Marshal(execCtxMap)
	approval, err := s.approvalMgr.Propose(call.SessionID, call, toolDef.SideEffect, toolDef.Limits.AllowedPaths, execCtxBytes)
	if err != nil {
		return agent.ApprovalResult{}, err
	}
	if err := s.appendApprovalEvent(ctx, audit.EventApprovalProposed, approval.SessionID, approval); err != nil {
		return agent.ApprovalResult{}, err
	}
	return agent.ApprovalResult{Approved: false, Reason: "approval_required", ApprovalID: approval.ID}, nil
}

type eventStreamCursor struct {
	timestamp time.Time
	eventID   string
	coarse    bool
}

func parseEventCursor(value string) (eventStreamCursor, error) {
	if value == "" {
		return eventStreamCursor{}, nil
	}
	parts := strings.Split(value, "|")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return eventStreamCursor{}, fmt.Errorf("timestamp missing")
	}

	ts, err := time.Parse(time.RFC3339, parts[0])
	if err != nil {
		return eventStreamCursor{}, fmt.Errorf("timestamp parse error: %w", err)
	}

	cursor := eventStreamCursor{timestamp: ts.UTC()}
	cursor.coarse = !strings.Contains(parts[0], ".")
	if len(parts) > 1 {
		cursor.eventID = parts[1]
	}
	return cursor, nil
}

//nolint:staticcheck // nhooyr websocket kept for Go 1.21 compatibility.
func (s *Server) streamNewEvents(ctx context.Context, conn *websocket.Conn, sessionID string, cursor eventStreamCursor) (eventStreamCursor, error) {
	events, err := s.eventsSince(ctx, sessionID, cursor.timestamp)
	if err != nil {
		return cursor, err
	}
	foundCursor := cursor.eventID == ""

	for _, ev := range events {
		if !foundCursor {
			if cursor.eventID != "" && cursor.coarse && ev.Timestamp.Truncate(time.Second).Equal(cursor.timestamp) {
				if ev.ID == cursor.eventID {
					foundCursor = true
				}
				continue
			}

			switch {
			case ev.Timestamp.Before(cursor.timestamp):
				continue
			case ev.Timestamp.After(cursor.timestamp):
				foundCursor = true
			case ev.ID == cursor.eventID:
				foundCursor = true
				continue
			default:
				continue
			}
		}

		payload, err := json.Marshal(ev)
		if err != nil {
			return cursor, err
		}
		//nolint:staticcheck // nhooyr websocket kept for Go 1.21 compatibility.
		if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
			return cursor, err
		}

		cursor.timestamp = ev.Timestamp
		cursor.eventID = ev.ID
	}

	return cursor, nil
}

func (s *Server) eventsSince(ctx context.Context, sessionID string, since time.Time) ([]audit.Event, error) {
	if s.eventStore == nil {
		return nil, fmt.Errorf("event store unavailable")
	}

	var (
		events []audit.Event
		err    error
	)
	if sessionID == "" {
		events, err = s.eventStore.QueryAll(ctx)
	} else {
		events, err = s.eventStore.QueryBySession(ctx, sessionID)
	}
	if err != nil {
		return nil, err
	}

	filtered := make([]audit.Event, 0, len(events))
	for _, ev := range events {
		if ev.Timestamp.Before(since) {
			continue
		}
		filtered = append(filtered, ev)
	}
	return filtered, nil
}

func (s *Server) handleToolExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	req, ok := decodeToolExecuteRequest(w, r)
	if !ok {
		return
	}

	toolDef, ok := s.validateToolExecutionRequest(r.Context(), w, req)
	if !ok {
		return
	}

	if err := s.enforceTrustTierToolPolicy(r.Context(), req.SessionID, toolDef); err != nil {
		writeStatusError(w, http.StatusForbidden, err.Error())
		return
	}

	if requiresApproval(toolDef) {
		s.proposeToolApproval(w, r, req, toolDef)
		return
	}

	s.executeToolRequest(w, r, req, toolDef, req.SessionID)
}

func decodeToolExecuteRequest(w http.ResponseWriter, r *http.Request) (tools.ExecuteRequest, bool) {
	var body struct {
		ToolName  string                 `json:"tool_name"`
		Input     map[string]interface{} `json:"input"`
		SessionID string                 `json:"session_id,omitempty"`
		Tool      string                 `json:"tool"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if !decodeJSONBody(w, r, &body) {
		return tools.ExecuteRequest{}, false
	}

	req := tools.ExecuteRequest{
		ToolName:  strings.TrimSpace(body.ToolName),
		Input:     body.Input,
		SessionID: strings.TrimSpace(body.SessionID),
	}

	if req.ToolName == "" {
		req.ToolName = strings.TrimSpace(body.Tool)
	}
	if req.Input == nil {
		req.Input = body.Arguments
	}

	if req.ToolName == "" {
		writeStatusError(w, http.StatusBadRequest, "tool_name (or legacy tool) is required")
		return tools.ExecuteRequest{}, false
	}
	if req.Input == nil {
		writeStatusError(w, http.StatusBadRequest, "input (or legacy arguments) is required")
		return tools.ExecuteRequest{}, false
	}

	return req, true
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeStatusError(w, http.StatusBadRequest, err.Error())
		return false
	}

	return true
}

func (s *Server) validateToolExecutionRequest(ctx context.Context, w http.ResponseWriter, req tools.ExecuteRequest) (tools.ToolDefinition, bool) {
	toolDef, found := s.toolRegistry.Get(req.ToolName)
	if !found {
		writeStatusError(w, http.StatusNotFound, "tool not found")
		return tools.ToolDefinition{}, false
	}

	if err := validateToolDataAccess(toolDef, req.Input); err != nil {
		writeStatusError(w, http.StatusBadRequest, err.Error())
		return tools.ToolDefinition{}, false
	}

	hidden, err := s.isToolHiddenForSession(ctx, req.SessionID, toolDef)
	if err != nil {
		writeStatusError(w, http.StatusInternalServerError, err.Error())
		return tools.ToolDefinition{}, false
	}
	if hidden {
		writeStatusError(w, http.StatusNotFound, "tool not found")
		return tools.ToolDefinition{}, false
	}

	if err := validateMCPDestination(toolDef, req.Input); err != nil {
		writeStatusError(w, http.StatusForbidden, err.Error())
		return tools.ToolDefinition{}, false
	}

	return toolDef, true
}

func (s *Server) proposeToolApproval(w http.ResponseWriter, r *http.Request, req tools.ExecuteRequest, toolDef tools.ToolDefinition) {
	var execCtxBytes json.RawMessage
	execCtxMap := buildExecutionContext(req, toolDef)
	if b, err := json.Marshal(execCtxMap); err == nil {
		execCtxBytes = b
	} else {
		log.Printf("failed to marshal execution context: %v", err)
	}

	approval, err := s.approvalMgr.Propose(req.SessionID, req, toolDef.SideEffect, toolDef.Limits.AllowedPaths, execCtxBytes)
	if err != nil {
		writeStatusError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.appendApprovalEvent(r.Context(), audit.EventApprovalProposed, approval.SessionID, approval); err != nil {
		writeStatusError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.appendTurnLifecycleEvent(r.Context(), approval.SessionID, ctxmgr.TurnStatusAwaitingApproval, "approval_proposed")

	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(approval); err != nil {
		log.Printf("failed to encode pending approval: %v", err)
	}
}

func (s *Server) executeToolRequest(w http.ResponseWriter, r *http.Request, req tools.ExecuteRequest, toolDef tools.ToolDefinition, sessionID string) {
	s.metrics.recordToolExecution(toolDef.SideEffect)
	result, err := s.toolExecutor.Execute(r.Context(), req)
	s.recordMCPCallEvent(r.Context(), req, toolDef, result, err)
	if err != nil {
		s.appendTurnLifecycleEvent(r.Context(), sessionID, ctxmgr.TurnStatusFailed, "tool_execution_failed")
		writeStatusError(w, http.StatusBadRequest, err.Error())
		return
	}

	notices := make([]redactionNotice, 0)
	if result.Output != nil {
		result.Output, notices = applyRedactionPolicyWithNotices(secrets.NewPolicyExecutor(nil), req.ToolName, "tool_output", secrets.DestinationUI, result.Output)
		if len(notices) > 0 {
			s.recordRedactionEvent(r.Context(), sessionID, req.ToolName, notices)
		}
	}

	if err := json.NewEncoder(w).Encode(toolExecuteResponse{ToolResult: result, RedactionNotices: notices}); err != nil {
		log.Printf("failed to encode tool response: %v", err)
	}

	if sessionID != "" {
		s.appendTurnLifecycleEvent(r.Context(), sessionID, ctxmgr.TurnStatusCompleted, "tool_execution_completed")
	}
}

func turnEventTypeForStatus(status ctxmgr.TurnStatus) string {
	switch status {
	case ctxmgr.TurnStatusAwaitingApproval:
		return audit.EventTurnAwaitingApproval
	case ctxmgr.TurnStatusCompleted:
		return audit.EventTurnCompleted
	case ctxmgr.TurnStatusFailed:
		return audit.EventTurnFailed
	case ctxmgr.TurnStatusCancelled:
		return audit.EventTurnCancelled
	default:
		return ""
	}
}

func (s *Server) appendTurnLifecycleEvent(ctx context.Context, sessionID string, status ctxmgr.TurnStatus, cause string) {
	s.appendTurnLifecycleEventWithCausality(ctx, sessionID, status, cause, nil)
}

func (s *Server) appendTurnLifecycleEventWithCausality(ctx context.Context, sessionID string, status ctxmgr.TurnStatus, cause string, causality map[string]interface{}) {
	if s.eventStore == nil || s.turnMgr == nil || sessionID == "" {
		return
	}

	turnID, err := s.resolveActiveTurnID(ctx, sessionID)
	if err != nil || turnID == "" {
		return
	}

	eventType := turnEventTypeForStatus(status)
	if eventType == "" {
		return
	}

	payloadMap := map[string]interface{}{
		"entity_type":          "turn",
		"entity_id":            turnID,
		"next_state":           string(status),
		"cause":                cause,
		"transition_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"related_ids": map[string]interface{}{
			"session_id": sessionID,
			"turn_id":    turnID,
		},
	}
	if len(causality) > 0 {
		payloadMap["causality"] = causality
	}

	payload, err := json.Marshal(payloadMap)
	if err != nil {
		return
	}

	_ = s.eventStore.Append(ctx, audit.Event{
		ID:            uuid.NewString(),
		SessionID:     sessionID,
		TurnID:        turnID,
		Timestamp:     time.Now().UTC(),
		EventType:     eventType,
		Actor:         requestActor(ctx),
		PolicyVersion: "1.0.0",
		Payload:       payload,
	})
}

func (s *Server) resolveActiveTurnID(ctx context.Context, sessionID string) (string, error) {
	turns, err := s.turnMgr.ListBySession(ctx, sessionID)
	if err != nil {
		return "", err
	}
	var chosen *ctxmgr.Turn
	for _, turn := range turns {
		if turn == nil {
			continue
		}
		if turn.Status != ctxmgr.TurnStatusActive && turn.Status != ctxmgr.TurnStatusAwaitingApproval {
			continue
		}
		if chosen == nil || turn.UpdatedAt.After(chosen.UpdatedAt) {
			chosen = turn
		}
	}
	if chosen == nil {
		return "", nil
	}
	return chosen.ID, nil
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
			EventType:     audit.EventApprovalTransition,
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
		EventType: audit.EventMCPCall,
		Actor:     requestActor(ctx),
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
	enforceTrustVisibility := toolDef.Source == tools.ToolSourcePlugin || strings.TrimSpace(toolDef.TrustLevel) != ""
	if !enforceTrustVisibility {
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

func (s *Server) enforceToolVisibilityForSession(ctx context.Context, sessionID string, toolDef tools.ToolDefinition) error {
	hidden, err := s.isToolHiddenForSession(ctx, sessionID, toolDef)
	if err != nil {
		return err
	}
	if hidden {
		return fmt.Errorf("tool not exposed for session trust tier")
	}
	return nil
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

func (s *Server) recordRedactionEvent(ctx context.Context, sessionID, toolName string, notices []redactionNotice) {
	if s.eventStore == nil || len(notices) == 0 {
		return
	}

	targetSessionID := strings.TrimSpace(sessionID)
	if targetSessionID == "" {
		targetSessionID = redactionAuditSessionID
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
		SessionID:     targetSessionID,
		Timestamp:     time.Now().UTC(),
		EventType:     audit.EventToolRedaction,
		Actor:         requestActor(ctx),
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

func buildExecutionContext(req tools.ExecuteRequest, def tools.ToolDefinition) map[string]interface{} {
	return map[string]interface{}{
		"tool_name":        def.Name,
		"side_effect":      string(def.SideEffect),
		"filesystem_scope": def.Security.FilesystemScope,
		"network_access":   def.Security.NetworkAccess,
		"timeout_seconds":  def.Limits.TimeoutSeconds,
		"max_output_bytes": def.Limits.MaxOutputBytes,
		"working_dir":      def.Limits.WorkingDir,
		"session_id":       req.SessionID,
	}
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
