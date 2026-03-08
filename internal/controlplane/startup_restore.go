package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
)

func (s *Server) restoreRuntimeState(ctx context.Context) error {
	if s.eventStore == nil || s.approvalMgr == nil {
		return nil
	}

	events, err := s.eventStore.QueryAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to read events for startup restore: %w", err)
	}

	approvals := make(map[string]Approval)
	for _, event := range events {
		switch event.EventType {
		case "approval_proposed":
			var approval Approval
			if err := json.Unmarshal(event.Payload, &approval); err != nil {
				return fmt.Errorf("failed to decode approval_proposed event %s: %w", event.ID, err)
			}
			approvals[approval.ID] = approval
		case "approval_decided":
			var payload struct {
				ApprovalID string `json:"approval_id"`
				Decision   string `json:"decision"`
				Reason     string `json:"reason,omitempty"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return fmt.Errorf("failed to decode approval_decided event %s: %w", event.ID, err)
			}

			approval, ok := approvals[payload.ApprovalID]
			if !ok {
				continue
			}

			switch payload.Decision {
			case "approve":
				approval.State = ApprovalStateApproved
			case "deny":
				approval.State = ApprovalStateDenied
				approval.DecisionReason = payload.Reason
			default:
				continue
			}

			approval.UpdatedAt = event.Timestamp
			approvals[approval.ID] = approval
		case "approval_executed":
			var payload struct {
				ApprovalID string `json:"approval_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return fmt.Errorf("failed to decode approval_executed event %s: %w", event.ID, err)
			}

			approval, ok := approvals[payload.ApprovalID]
			if !ok {
				continue
			}
			approval.State = ApprovalStateExecuted
			approval.UpdatedAt = event.Timestamp
			approvals[approval.ID] = approval
		}
	}

	now := time.Now().UTC()
	restored := make([]Approval, 0, len(approvals))
	for _, approval := range approvals {
		if approval.State == ApprovalStateApproved {
			approval.State = ApprovalStateCancelled
			approval.DecisionReason = "cancelled during startup recovery before execution"
			approval.UpdatedAt = now
		}
		if approval.State == ApprovalStatePending && now.After(approval.ExpiresAt) {
			approval.State = ApprovalStateCancelled
			approval.DecisionReason = "cancelled during startup recovery because approval expired"
			approval.UpdatedAt = now
		}
		restored = append(restored, approval)
	}

	s.approvalMgr.ReplaceAll(restored)

	return nil
}

func (s *Server) appendApprovalEvent(ctx context.Context, eventType string, sessionID string, payload interface{}) error {
	if s.eventStore == nil {
		return nil
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode %s payload: %w", eventType, err)
	}

	event := audit.Event{
		ID:            fmt.Sprintf("evt-%d", time.Now().UTC().UnixNano()),
		SessionID:     sessionID,
		Timestamp:     time.Now().UTC(),
		EventType:     eventType,
		Actor:         "system",
		PolicyVersion: "1.0.0",
		Payload:       encoded,
	}

	if err := s.eventStore.Append(ctx, event); err != nil {
		return fmt.Errorf("failed to append %s event: %w", eventType, err)
	}

	return nil
}
