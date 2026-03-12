package controlplane

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type lspProcessConfig struct {
	Executable string
	Args       []string
	Timeout    time.Duration
}

type lspProcessStarter func(ctx context.Context, cfg lspProcessConfig) error

type executableLookup func(name string) (string, error)

type lspProcessState struct {
	started   bool
	unhealthy bool
	lastError string
	startedAt time.Time
}

type lspProcessSupervisor struct {
	mu      sync.Mutex
	states  map[string]lspProcessState
	lookup  executableLookup
	starter lspProcessStarter
	now     func() time.Time
}

func newLSPProcessSupervisor(lookup executableLookup, starter lspProcessStarter) *lspProcessSupervisor {
	if lookup == nil {
		lookup = exec.LookPath
	}
	if starter == nil {
		starter = func(context.Context, lspProcessConfig) error { return nil }
	}
	return &lspProcessSupervisor{
		states:  make(map[string]lspProcessState),
		lookup:  lookup,
		starter: starter,
		now:     time.Now,
	}
}

func (s *lspProcessSupervisor) EnsureStarted(ctx context.Context, detected detectedLanguage) error {
	if strings.TrimSpace(detected.Executable) == "" {
		return nil
	}

	s.mu.Lock()
	state := s.states[detected.Language]
	if state.started {
		s.mu.Unlock()
		return nil
	}
	if state.unhealthy {
		s.mu.Unlock()
		return newLSPError(LSPErrorBackendUnhealthy, http.StatusServiceUnavailable, true, "lsp backend is unhealthy", map[string]string{"language": detected.Language})
	}
	s.mu.Unlock()

	executablePath, err := s.lookup(detected.Executable)
	if err != nil {
		if isMissingExecutable(err) {
			return newLSPError(LSPErrorServerUnavailable, http.StatusServiceUnavailable, false, fmt.Sprintf("lsp executable %q not found", detected.Executable), map[string]string{"language": detected.Language, "executable": detected.Executable})
		}
		return newLSPError(LSPErrorBackendUnhealthy, http.StatusServiceUnavailable, true, "failed to resolve lsp executable", map[string]string{"language": detected.Language})
	}

	cfg := lspProcessConfig{Executable: executablePath, Timeout: 5 * time.Second}
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		if startErr := s.starter(ctx, cfg); startErr == nil {
			s.mu.Lock()
			s.states[detected.Language] = lspProcessState{started: true, startedAt: s.now().UTC()}
			s.mu.Unlock()
			return nil
		} else {
			lastErr = startErr
			if isMissingExecutable(startErr) {
				return newLSPError(LSPErrorServerUnavailable, http.StatusServiceUnavailable, false, fmt.Sprintf("lsp executable %q not found", detected.Executable), map[string]string{"language": detected.Language, "executable": detected.Executable})
			}
			if errors.Is(startErr, context.DeadlineExceeded) || errors.Is(startErr, context.Canceled) {
				continue
			}
		}
	}

	s.mu.Lock()
	s.states[detected.Language] = lspProcessState{unhealthy: true, lastError: errorString(lastErr)}
	s.mu.Unlock()
	return newLSPError(LSPErrorBackendUnhealthy, http.StatusServiceUnavailable, true, "lsp backend failed to start", map[string]string{"language": detected.Language, "attempts": "2"})
}

func isMissingExecutable(err error) bool {
	return errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
