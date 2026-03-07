package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/syndicatecode/syndicatecode/internal/audit"
	ctxmgr "github.com/syndicatecode/syndicatecode/internal/context"
	"github.com/syndicatecode/syndicatecode/internal/session"
)

type Config struct {
	Addr         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func DefaultConfig() *Config {
	return &Config{
		Addr:         ":7777",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
}

type Server struct {
	httpServer  *http.Server
	sessionMgr  *session.Manager
	turnMgr     *ctxmgr.TurnManager
	ctxManifest *ctxmgr.ContextManifest
	eventStore  *audit.EventStore
}

func NewServer(ctx context.Context, cfg *Config) (*Server, error) {
	eventStore, err := audit.NewEventStore("syndicatecode.db")
	if err != nil {
		return nil, fmt.Errorf("failed to create event store: %w", err)
	}

	sessionMgr := session.NewManager(eventStore)
	turnMgr := ctxmgr.NewTurnManager(eventStore, sessionMgr)
	ctxManifest := ctxmgr.NewContextManifest(eventStore)

	mux := http.NewServeMux()
	server := &Server{
		httpServer: &http.Server{
			Addr:         cfg.Addr,
			Handler:      mux,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
		},
		sessionMgr:  sessionMgr,
		turnMgr:     turnMgr,
		ctxManifest: ctxManifest,
		eventStore:  eventStore,
	}

	server.registerRoutes(mux)

	return server, nil
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/api/v1/sessions", s.handleSessions)
	mux.HandleFunc("/api/v1/sessions/", s.handleSessionByID)
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
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
	json.NewEncoder(w).Encode(sessions)
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
	json.NewEncoder(w).Encode(created)
}

func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/")
	session, err := s.sessionMgr.Get(r.Context(), sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(session)
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
	json.NewEncoder(w).Encode(turns)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(turn)
}

func (s *Server) handleTurnByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/turns/")

	// Check if this is a context request
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
	json.NewEncoder(w).Encode(turn)
}

func (s *Server) handleTurnContext(w http.ResponseWriter, r *http.Request) {
	// Extract turn ID from path like /api/v1/turns/{turn_id}/context
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
	json.NewEncoder(w).Encode(fragments)
}

func (s *Server) handleApprovals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	json.NewEncoder(w).Encode([]interface{}{})
}

func (s *Server) handleApprovalByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{})
}

func (s *Server) handlePolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
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
	})
}

func (s *Server) handleToolExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{})
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
