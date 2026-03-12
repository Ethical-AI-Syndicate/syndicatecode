package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

type Runner struct {
	model    models.LanguageModel
	registry *tools.Registry
	executor *tools.Executor
	approval ApprovalGate
	emitter  EventEmitter
	cfg      Config
}

func NewRunner(model models.LanguageModel, registry *tools.Registry, executor *tools.Executor, approval ApprovalGate, emitter EventEmitter, cfg Config) *Runner {
	return &Runner{model: model, registry: registry, executor: executor, approval: approval, emitter: emitter, cfg: cfg}
}

func (r *Runner) WithConfig(cfg Config) *Runner {
	cp := *r
	cp.cfg = cfg
	return &cp
}

func (r *Runner) ModelMetadata() (provider, model string) {
	if r == nil || r.model == nil {
		return "unknown", "unknown"
	}

	model = strings.TrimSpace(r.model.ModelID())
	if model == "" {
		model = "unknown"
	}

	type providerNamer interface {
		ProviderName() string
	}
	if named, ok := r.model.(providerNamer); ok {
		provider = strings.TrimSpace(named.ProviderName())
	}
	if provider == "" {
		provider = inferProviderFromModelID(model)
	}

	return provider, model
}

func inferProviderFromModelID(model string) string {
	lower := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(lower, "claude"):
		return "anthropic"
	case strings.HasPrefix(lower, "gpt"), strings.HasPrefix(lower, "o1"), strings.HasPrefix(lower, "o3"):
		return "openai"
	case strings.HasPrefix(lower, "gemini"):
		return "google"
	default:
		return "unknown"
	}
}

func (r *Runner) RunTurn(ctx context.Context, turn AgentTurn) (<-chan AgentEvent, error) {
	ch := make(chan AgentEvent, 64)
	go func() {
		defer close(ch)
		r.loop(ctx, turn, ch)
	}()
	return ch, nil
}

type accumulatedToolUse struct {
	id        string
	name      string
	inputJSON string
}

func (r *Runner) loop(ctx context.Context, turn AgentTurn, ch chan<- AgentEvent) {
	if r.cfg.TurnTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.cfg.TurnTimeout)
		defer cancel()
	}

	messages := []models.Message{{Role: "user", Content: []models.ContentBlock{models.TextBlock{Text: turn.Message}}}}
	toolCalls := 0
	outputBytes := 0

	for depth := 0; depth < r.cfg.MaxDepth; depth++ {
		params := models.Params{Model: r.model.ModelID(), Messages: messages, MaxTokens: 4096}
		for _, def := range r.registry.List() {
			schema, _ := json.Marshal(def.InputSchema)
			params.Tools = append(params.Tools, models.Tool{Name: def.Name, Description: def.Description, InputSchema: schema})
		}

		stream, err := r.streamWithRetry(ctx, params)
		if err != nil {
			if isTurnTimeout(ctx, err) {
				r.emitTurnFailed(turn.SessionID, ch, "timeout", fmt.Sprintf("stream timed out: %v", err))
				return
			}
			r.emitTurnFailed(turn.SessionID, ch, "stream_error", fmt.Sprintf("stream error: %v", err))
			return
		}

		stopReason, toolUses, terminated := r.collectStreamEvents(ctx, turn.SessionID, stream, ch, &outputBytes)
		if terminated {
			return
		}

		if len(toolUses) == 0 || stopReason == "end_turn" {
			r.emitTurnCompleted(turn.SessionID, ch, stopReason)
			return
		}

		assistantBlocks, toolResultBlocks, terminated := r.processToolUses(ctx, turn, toolUses, ch, &toolCalls, &outputBytes)
		if terminated {
			return
		}

		messages = append(messages,
			models.Message{Role: "assistant", Content: assistantBlocks},
			models.Message{Role: "user", Content: toolResultBlocks},
		)
	}

	r.emitTurnCompleted(turn.SessionID, ch, "max_depth")
}

func (r *Runner) collectStreamEvents(ctx context.Context, sessionID string, stream <-chan models.StreamEvent, ch chan<- AgentEvent, outputBytes *int) (string, []accumulatedToolUse, bool) {
	stopReason := "end_turn"
	toolUseIdx := map[string]int{}
	toolUses := make([]accumulatedToolUse, 0)

	for {
		select {
		case <-ctx.Done():
			if isTurnTimeout(ctx, nil) {
				r.emitTurnFailed(sessionID, ch, "timeout", "turn timed out")
				return stopReason, nil, true
			}
			r.emitTurnFailed(sessionID, ch, "cancelled", fmt.Sprintf("turn cancelled: %v", ctx.Err()))
			return stopReason, nil, true
		case event, ok := <-stream:
			if !ok {
				if ctx.Err() != nil {
					if isTurnTimeout(ctx, nil) {
						r.emitTurnFailed(sessionID, ch, "timeout", "turn timed out")
						return stopReason, nil, true
					}
					r.emitTurnFailed(sessionID, ch, "cancelled", fmt.Sprintf("turn cancelled: %v", ctx.Err()))
					return stopReason, nil, true
				}
				return stopReason, toolUses, false
			}

			switch e := event.(type) {
			case models.TextDeltaEvent:
				if r.exceedsOutputCap(outputBytes, len(e.Delta)) {
					r.emitTurnCompleted(sessionID, ch, "max_output_bytes")
					return stopReason, nil, true
				}
				ae := AgentEvent{Type: EventTextDelta, Data: e.Delta}
				ch <- ae
				r.emitter.Emit(sessionID, ae)
			case models.ToolUseStartEvent:
				toolUseIdx[e.ID] = len(toolUses)
				toolUses = append(toolUses, accumulatedToolUse{id: e.ID, name: e.Name})
				ae := AgentEvent{Type: EventToolUseStart, ToolID: e.ID, ToolName: e.Name}
				ch <- ae
				r.emitter.Emit(sessionID, ae)
			case models.ToolInputDeltaEvent:
				if idx, ok := toolUseIdx[e.ID]; ok {
					toolUses[idx].inputJSON += e.Delta
				}
				ae := AgentEvent{Type: EventToolInputDelta, ToolID: e.ID, Data: e.Delta}
				ch <- ae
				r.emitter.Emit(sessionID, ae)
			case models.MessageDeltaEvent:
				stopReason = e.StopReason
			}
		}
	}
}

func (r *Runner) processToolUses(ctx context.Context, turn AgentTurn, toolUses []accumulatedToolUse, ch chan<- AgentEvent, toolCalls *int, outputBytes *int) ([]models.ContentBlock, []models.ContentBlock, bool) {
	assistantBlocks := make([]models.ContentBlock, 0, len(toolUses))
	toolResultBlocks := make([]models.ContentBlock, 0, len(toolUses))

	for _, tu := range toolUses {
		(*toolCalls)++
		if r.cfg.MaxToolCalls > 0 && *toolCalls > r.cfg.MaxToolCalls {
			r.emitTurnCompleted(turn.SessionID, ch, "max_tool_calls")
			return nil, nil, true
		}

		input := map[string]interface{}{}
		if err := json.Unmarshal([]byte(tu.inputJSON), &input); err != nil {
			r.emitTurnFailed(turn.SessionID, ch, "invalid_tool_input", fmt.Sprintf("invalid tool input for %s: %v", tu.name, err))
			return nil, nil, true
		}

		call := tools.ToolCall{ID: tu.id, ToolName: tu.name, Input: input, SessionID: turn.SessionID}
		ar, approvalErr := r.approval.RequestApproval(ctx, call)
		if approvalErr != nil {
			r.emitTurnFailed(turn.SessionID, ch, "approval_error", fmt.Sprintf("approval error: %v", approvalErr))
			return nil, nil, true
		}
		if !ar.Approved {
			approvalID := ar.ApprovalID
			if approvalID == "" && strings.HasPrefix(ar.Reason, "approval_id:") {
				approvalID = strings.TrimPrefix(ar.Reason, "approval_id:")
			}
			ae := AgentEvent{
				Type:       EventAwaitingApproval,
				StopReason: "awaiting_approval",
				Data:       ar.Reason,
				ToolID:     tu.id,
				ToolName:   tu.name,
				ApprovalID: approvalID,
			}
			ch <- ae
			r.emitter.Emit(turn.SessionID, ae)
			r.emitTurnCompleted(turn.SessionID, ch, "awaiting_approval")
			return nil, nil, true
		}

		result, execErr := r.executor.Execute(ctx, call)
		isErr := execErr != nil
		content := ""
		if isErr {
			content = fmt.Sprintf("error: %v", execErr)
		} else {
			out, _ := json.Marshal(result.Output)
			content = string(out)
		}

		if r.exceedsOutputCap(outputBytes, len(content)) {
			r.emitTurnCompleted(turn.SessionID, ch, "max_output_bytes")
			return nil, nil, true
		}

		assistantBlocks = append(assistantBlocks, models.ToolUseBlock{ID: tu.id, Name: tu.name, Input: json.RawMessage(tu.inputJSON)})
		toolResultBlocks = append(toolResultBlocks, models.ToolResultBlock{ToolUseID: tu.id, Content: content, IsError: isErr})
	}

	return assistantBlocks, toolResultBlocks, false
}

func (r *Runner) streamWithRetry(ctx context.Context, params models.Params) (<-chan models.StreamEvent, error) {
	maxRetries := r.cfg.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	for attempt := 0; ; attempt++ {
		stream, err := r.model.Stream(ctx, params)
		if err == nil {
			return stream, nil
		}
		if !isTransientModelError(err) || attempt >= maxRetries {
			return nil, err
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
}

func isTransientModelError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	type temporary interface{ Temporary() bool }
	if te, ok := err.(temporary); ok && te.Temporary() {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{"timeout", "tempor", "rate limit", "429", "503", "unavailable", "connection reset"} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func isTurnTimeout(ctx context.Context, streamErr error) bool {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return true
	}
	return errors.Is(streamErr, context.DeadlineExceeded)
}

func (r *Runner) exceedsOutputCap(current *int, next int) bool {
	if r.cfg.MaxOutputBytes <= 0 {
		return false
	}
	*current += next
	return *current > r.cfg.MaxOutputBytes
}

func (r *Runner) emitTurnCompleted(sessionID string, ch chan<- AgentEvent, stopReason string) {
	ae := AgentEvent{Type: EventTurnCompleted, StopReason: stopReason}
	ch <- ae
	r.emitter.Emit(sessionID, ae)
}

func (r *Runner) emitTurnFailed(sessionID string, ch chan<- AgentEvent, stopReason string, message string) {
	ae := AgentEvent{Type: EventTurnFailed, StopReason: stopReason, Data: message}
	ch <- ae
	r.emitter.Emit(sessionID, ae)
}
