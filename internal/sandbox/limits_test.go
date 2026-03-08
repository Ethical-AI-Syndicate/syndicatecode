package sandbox

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type sleepingExecutor struct {
	delay time.Duration
}

func (s sleepingExecutor) Run(ctx context.Context, _ string, _ []string, _ SubprocessOptions) (ExecuteResult, error) {
	select {
	case <-time.After(s.delay):
		return ExecuteResult{ExitCode: 0}, nil
	case <-ctx.Done():
		return ExecuteResult{}, ctx.Err()
	}
}

type largeOutputExecutor struct{}

func (l largeOutputExecutor) Run(_ context.Context, _ string, _ []string, _ SubprocessOptions) (ExecuteResult, error) {
	return ExecuteResult{
		ExitCode: 0,
		Stdout:   strings.Repeat("a", 64),
		Stderr:   strings.Repeat("b", 64),
	}, nil
}

type blockingExecutor struct {
	delay time.Duration
}

func (b blockingExecutor) Run(_ context.Context, _ string, _ []string, _ SubprocessOptions) (ExecuteResult, error) {
	time.Sleep(b.delay)
	return ExecuteResult{ExitCode: 0}, nil
}

func TestLimitedExecutorEnforcesTimeout(t *testing.T) {
	t.Parallel()

	base := sleepingExecutor{delay: 100 * time.Millisecond}
	limited := NewLimitedExecutor(base)

	_, err := limited.Run(context.Background(), "sleep", nil, SubprocessOptions{Timeout: 10 * time.Millisecond})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestLimitedExecutorTruncatesOutputByByteCap(t *testing.T) {
	t.Parallel()

	limited := NewLimitedExecutor(largeOutputExecutor{})

	result, err := limited.Run(context.Background(), "test", nil, SubprocessOptions{MaxOutputBytes: 16})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result.Stdout) != 16 {
		t.Fatalf("expected stdout length 16, got %d", len(result.Stdout))
	}
	if len(result.Stderr) != 16 {
		t.Fatalf("expected stderr length 16, got %d", len(result.Stderr))
	}
	if !result.StdoutTruncated || !result.StderrTruncated {
		t.Fatalf("expected truncation flags to be set")
	}
}

func TestMemoryBoundedExecutorRejectsExcessConcurrentRuns(t *testing.T) {
	t.Parallel()

	executor := NewMemoryBoundedExecutor(blockingExecutor{delay: 50 * time.Millisecond}, 1)

	var wg sync.WaitGroup
	wg.Add(2)

	var err1 error
	var err2 error

	go func() {
		defer wg.Done()
		_, err1 = executor.Run(context.Background(), "one", nil, SubprocessOptions{})
	}()

	time.Sleep(5 * time.Millisecond)
	go func() {
		defer wg.Done()
		_, err2 = executor.Run(context.Background(), "two", nil, SubprocessOptions{})
	}()

	wg.Wait()

	if err1 != nil && err2 != nil {
		t.Fatalf("expected one execution to succeed, got err1=%v err2=%v", err1, err2)
	}
	if err1 == nil && err2 == nil {
		t.Fatalf("expected one execution to be rejected by active-run cap")
	}
	if err1 != nil && !errors.Is(err1, ErrResourceLimitExceeded) {
		t.Fatalf("expected err1 to be %v, got %v", ErrResourceLimitExceeded, err1)
	}
	if err2 != nil && !errors.Is(err2, ErrResourceLimitExceeded) {
		t.Fatalf("expected err2 to be %v, got %v", ErrResourceLimitExceeded, err2)
	}
}
