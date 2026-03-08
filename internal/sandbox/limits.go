package sandbox

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrResourceLimitExceeded = errors.New("resource limit exceeded")

type LimitedExecutor struct {
	base CommandExecutor
}

func NewLimitedExecutor(base CommandExecutor) *LimitedExecutor {
	return &LimitedExecutor{base: base}
}

func (l *LimitedExecutor) Run(ctx context.Context, command string, args []string, options SubprocessOptions) (ExecuteResult, error) {
	runCtx := ctx
	if options.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, options.Timeout)
		defer cancel()
	}

	result, err := l.base.Run(runCtx, command, args, options)
	if err != nil {
		return ExecuteResult{}, err
	}

	if options.MaxOutputBytes > 0 {
		result.Stdout, result.StdoutTruncated = truncateByBytes(result.Stdout, options.MaxOutputBytes)
		result.Stderr, result.StderrTruncated = truncateByBytes(result.Stderr, options.MaxOutputBytes)
	}

	return result, nil
}

func truncateByBytes(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	return value[:limit], true
}

type MemoryBoundedExecutor struct {
	base      CommandExecutor
	maxActive int

	mu     sync.Mutex
	active int
}

func NewMemoryBoundedExecutor(base CommandExecutor, maxActive int) *MemoryBoundedExecutor {
	if maxActive <= 0 {
		maxActive = 1
	}
	return &MemoryBoundedExecutor{base: base, maxActive: maxActive}
}

func (m *MemoryBoundedExecutor) Run(ctx context.Context, command string, args []string, options SubprocessOptions) (ExecuteResult, error) {
	m.mu.Lock()
	if m.active >= m.maxActive {
		m.mu.Unlock()
		return ExecuteResult{}, ErrResourceLimitExceeded
	}
	m.active++
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.active--
		m.mu.Unlock()
	}()

	return m.base.Run(ctx, command, args, options)
}

func DefaultSubprocessOptions() SubprocessOptions {
	return SubprocessOptions{
		Timeout:        30 * time.Second,
		MaxOutputBytes: 256 * 1024,
	}
}
