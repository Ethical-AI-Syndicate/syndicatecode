package sandbox

import (
	"context"
	"sync"
	"time"
)

type ExecutionContext struct {
	Executable            string
	Args                  []string
	WorkingDir            string
	TimeoutSeconds        int
	EnvironmentExposure   string
	NetworkPolicy         NetworkPolicy
	IsolationLevel        IsolationLevel
	SideEffectClass       SideEffectClass
	ResourceMaxOutputB    int
	ResourceMaxConcurrent int
}

type ApprovalPayload struct {
	ApprovalID       string
	ExecutionContext ExecutionContext
}

func BuildApprovalPayload(approvalID string, executionContext ExecutionContext) ApprovalPayload {
	return ApprovalPayload{
		ApprovalID:       approvalID,
		ExecutionContext: executionContext,
	}
}

type ExecutionRecord struct {
	SessionID  string
	TurnID     string
	ToolName   string
	Context    ExecutionContext
	RecordedAt time.Time
}

type Recorder interface {
	Record(ctx context.Context, record ExecutionRecord) error
	ListByTurn(turnID string) []ExecutionRecord
}

type InMemoryRecorder struct {
	mu      sync.RWMutex
	records []ExecutionRecord
}

func NewInMemoryRecorder() *InMemoryRecorder {
	return &InMemoryRecorder{records: make([]ExecutionRecord, 0)}
}

func (r *InMemoryRecorder) Record(_ context.Context, record ExecutionRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, record)
	return nil
}

func (r *InMemoryRecorder) ListByTurn(turnID string) []ExecutionRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ExecutionRecord, 0)
	for _, record := range r.records {
		if record.TurnID == turnID {
			result = append(result, record)
		}
	}
	return result
}
