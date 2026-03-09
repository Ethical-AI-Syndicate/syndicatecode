package state

import (
	"errors"
	"testing"
)

func TestSessionTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		from  SessionState
		to    SessionState
		errIs error
	}{
		{name: "active to completed", from: SessionStateActive, to: SessionStateCompleted},
		{name: "active to terminated", from: SessionStateActive, to: SessionStateTerminated},
		{name: "completed to terminated denied", from: SessionStateCompleted, to: SessionStateTerminated, errIs: ErrTerminalSessionState},
		{name: "active to active denied", from: SessionStateActive, to: SessionStateActive, errIs: ErrInvalidSessionTransition},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateSessionTransition(tc.from, tc.to)
			if tc.errIs == nil && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tc.errIs != nil && !errors.Is(err, tc.errIs) {
				t.Fatalf("expected error %v, got %v", tc.errIs, err)
			}
		})
	}
}

func TestTurnTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		from  TurnState
		to    TurnState
		errIs error
	}{
		{name: "active to awaiting approval", from: TurnStateActive, to: TurnStateAwaitingApproval},
		{name: "awaiting approval to active", from: TurnStateAwaitingApproval, to: TurnStateActive},
		{name: "active to completed", from: TurnStateActive, to: TurnStateCompleted},
		{name: "completed to active denied", from: TurnStateCompleted, to: TurnStateActive, errIs: ErrTerminalTurnState},
		{name: "failed to completed denied", from: TurnStateFailed, to: TurnStateCompleted, errIs: ErrTerminalTurnState},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateTurnTransition(tc.from, tc.to)
			if tc.errIs == nil && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tc.errIs != nil && !errors.Is(err, tc.errIs) {
				t.Fatalf("expected error %v, got %v", tc.errIs, err)
			}
		})
	}
}

func TestToolInvocationTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		from  ToolInvocationState
		to    ToolInvocationState
		errIs error
	}{
		{name: "proposed to pending approval", from: ToolInvocationStateProposed, to: ToolInvocationStatePendingApproval},
		{name: "pending approval to approved", from: ToolInvocationStatePendingApproval, to: ToolInvocationStateApproved},
		{name: "approved to running", from: ToolInvocationStateApproved, to: ToolInvocationStateRunning},
		{name: "running to succeeded", from: ToolInvocationStateRunning, to: ToolInvocationStateSucceeded},
		{name: "succeeded to failed denied", from: ToolInvocationStateSucceeded, to: ToolInvocationStateFailed, errIs: ErrTerminalToolInvocationState},
		{name: "proposed to succeeded denied", from: ToolInvocationStateProposed, to: ToolInvocationStateSucceeded, errIs: ErrInvalidToolInvocationTransition},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateToolInvocationTransition(tc.from, tc.to)
			if tc.errIs == nil && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tc.errIs != nil && !errors.Is(err, tc.errIs) {
				t.Fatalf("expected error %v, got %v", tc.errIs, err)
			}
		})
	}
}

func TestApprovalTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		from  ApprovalState
		to    ApprovalState
		errIs error
	}{
		{name: "proposed to pending", from: ApprovalStateProposed, to: ApprovalStatePending},
		{name: "pending to approved", from: ApprovalStatePending, to: ApprovalStateApproved},
		{name: "pending to denied", from: ApprovalStatePending, to: ApprovalStateDenied},
		{name: "approved to executed", from: ApprovalStateApproved, to: ApprovalStateExecuted},
		{name: "denied to executed denied", from: ApprovalStateDenied, to: ApprovalStateExecuted, errIs: ErrTerminalApprovalState},
		{name: "pending to proposed denied", from: ApprovalStatePending, to: ApprovalStateProposed, errIs: ErrInvalidApprovalTransition},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateApprovalTransition(tc.from, tc.to)
			if tc.errIs == nil && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tc.errIs != nil && !errors.Is(err, tc.errIs) {
				t.Fatalf("expected error %v, got %v", tc.errIs, err)
			}
		})
	}
}

func TestEditTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		from  EditState
		to    EditState
		errIs error
	}{
		{name: "proposed to validated", from: EditStateProposed, to: EditStateValidated},
		{name: "validated to approved", from: EditStateValidated, to: EditStateApproved},
		{name: "approved to applying", from: EditStateApproved, to: EditStateApplying},
		{name: "applying to applied", from: EditStateApplying, to: EditStateApplied},
		{name: "applying to failed", from: EditStateApplying, to: EditStateFailed},
		{name: "failed to rolled back", from: EditStateFailed, to: EditStateRolledBack},
		{name: "applied to rolled back denied", from: EditStateApplied, to: EditStateRolledBack, errIs: ErrTerminalEditState},
		{name: "validated to applying denied", from: EditStateValidated, to: EditStateApplying, errIs: ErrInvalidEditTransition},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateEditTransition(tc.from, tc.to)
			if tc.errIs == nil && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tc.errIs != nil && !errors.Is(err, tc.errIs) {
				t.Fatalf("expected error %v, got %v", tc.errIs, err)
			}
		})
	}
}
