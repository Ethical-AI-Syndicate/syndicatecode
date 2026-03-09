package state

import (
	"errors"
	"fmt"
)

type SessionState string

const (
	SessionStateActive     SessionState = "active"
	SessionStateCompleted  SessionState = "completed"
	SessionStateTerminated SessionState = "terminated"
)

var (
	ErrInvalidSessionTransition = errors.New("invalid session state transition")
	ErrTerminalSessionState     = errors.New("session state is terminal")
)

var sessionTransitions = map[SessionState]map[SessionState]struct{}{
	SessionStateActive: {
		SessionStateCompleted:  {},
		SessionStateTerminated: {},
	},
}

func ValidateSessionTransition(from, to SessionState) error {
	return validateTransition(from, to, sessionTransitions, []SessionState{SessionStateCompleted, SessionStateTerminated}, ErrTerminalSessionState, ErrInvalidSessionTransition, "session")
}

type TurnState string

const (
	TurnStateActive           TurnState = "active"
	TurnStateAwaitingApproval TurnState = "awaiting_approval"
	TurnStateCompleted        TurnState = "completed"
	TurnStateFailed           TurnState = "failed"
	TurnStateCancelled        TurnState = "cancelled"
)

var (
	ErrInvalidTurnTransition = errors.New("invalid turn state transition")
	ErrTerminalTurnState     = errors.New("turn state is terminal")
)

var turnTransitions = map[TurnState]map[TurnState]struct{}{
	TurnStateActive: {
		TurnStateAwaitingApproval: {},
		TurnStateCompleted:        {},
		TurnStateFailed:           {},
		TurnStateCancelled:        {},
	},
	TurnStateAwaitingApproval: {
		TurnStateActive:    {},
		TurnStateCompleted: {},
		TurnStateFailed:    {},
		TurnStateCancelled: {},
	},
}

func ValidateTurnTransition(from, to TurnState) error {
	return validateTransition(from, to, turnTransitions, []TurnState{TurnStateCompleted, TurnStateFailed, TurnStateCancelled}, ErrTerminalTurnState, ErrInvalidTurnTransition, "turn")
}

type ToolInvocationState string

const (
	ToolInvocationStateProposed        ToolInvocationState = "proposed"
	ToolInvocationStatePendingApproval ToolInvocationState = "pending_approval"
	ToolInvocationStateApproved        ToolInvocationState = "approved"
	ToolInvocationStateDenied          ToolInvocationState = "denied"
	ToolInvocationStateRunning         ToolInvocationState = "running"
	ToolInvocationStateSucceeded       ToolInvocationState = "succeeded"
	ToolInvocationStateFailed          ToolInvocationState = "failed"
	ToolInvocationStateCancelled       ToolInvocationState = "cancelled"
)

var (
	ErrInvalidToolInvocationTransition = errors.New("invalid tool invocation state transition")
	ErrTerminalToolInvocationState     = errors.New("tool invocation state is terminal")
)

var toolInvocationTransitions = map[ToolInvocationState]map[ToolInvocationState]struct{}{
	ToolInvocationStateProposed: {
		ToolInvocationStatePendingApproval: {},
		ToolInvocationStateRunning:         {},
		ToolInvocationStateCancelled:       {},
	},
	ToolInvocationStatePendingApproval: {
		ToolInvocationStateApproved:  {},
		ToolInvocationStateDenied:    {},
		ToolInvocationStateCancelled: {},
	},
	ToolInvocationStateApproved: {
		ToolInvocationStateRunning:   {},
		ToolInvocationStateCancelled: {},
	},
	ToolInvocationStateRunning: {
		ToolInvocationStateSucceeded: {},
		ToolInvocationStateFailed:    {},
		ToolInvocationStateCancelled: {},
	},
}

func ValidateToolInvocationTransition(from, to ToolInvocationState) error {
	return validateTransition(from, to, toolInvocationTransitions, []ToolInvocationState{ToolInvocationStateDenied, ToolInvocationStateSucceeded, ToolInvocationStateFailed, ToolInvocationStateCancelled}, ErrTerminalToolInvocationState, ErrInvalidToolInvocationTransition, "tool invocation")
}

type ApprovalState string

const (
	ApprovalStateProposed  ApprovalState = "proposed"
	ApprovalStatePending   ApprovalState = "pending"
	ApprovalStateApproved  ApprovalState = "approved"
	ApprovalStateDenied    ApprovalState = "denied"
	ApprovalStateExecuted  ApprovalState = "executed"
	ApprovalStateCancelled ApprovalState = "cancelled"
)

var (
	ErrInvalidApprovalTransition = errors.New("invalid approval state transition")
	ErrTerminalApprovalState     = errors.New("approval state is terminal")
)

var approvalTransitions = map[ApprovalState]map[ApprovalState]struct{}{
	ApprovalStateProposed: {
		ApprovalStatePending: {},
	},
	ApprovalStatePending: {
		ApprovalStateApproved:  {},
		ApprovalStateDenied:    {},
		ApprovalStateCancelled: {},
	},
	ApprovalStateApproved: {
		ApprovalStateExecuted:  {},
		ApprovalStateCancelled: {},
	},
}

func ValidateApprovalTransition(from, to ApprovalState) error {
	return validateTransition(from, to, approvalTransitions, []ApprovalState{ApprovalStateDenied, ApprovalStateExecuted, ApprovalStateCancelled}, ErrTerminalApprovalState, ErrInvalidApprovalTransition, "approval")
}

type EditState string

const (
	EditStateProposed   EditState = "proposed"
	EditStateValidated  EditState = "validated"
	EditStateApproved   EditState = "approved"
	EditStateApplying   EditState = "applying"
	EditStateApplied    EditState = "applied"
	EditStateFailed     EditState = "failed"
	EditStateRolledBack EditState = "rolled_back"
	EditStateRejected   EditState = "rejected"
)

var (
	ErrInvalidEditTransition = errors.New("invalid edit state transition")
	ErrTerminalEditState     = errors.New("edit state is terminal")
)

var editTransitions = map[EditState]map[EditState]struct{}{
	EditStateProposed: {
		EditStateValidated: {},
		EditStateRejected:  {},
	},
	EditStateValidated: {
		EditStateApproved: {},
		EditStateRejected: {},
	},
	EditStateApproved: {
		EditStateApplying: {},
		EditStateRejected: {},
	},
	EditStateApplying: {
		EditStateApplied: {},
		EditStateFailed:  {},
	},
	EditStateFailed: {
		EditStateRolledBack: {},
	},
}

func ValidateEditTransition(from, to EditState) error {
	return validateTransition(from, to, editTransitions, []EditState{EditStateApplied, EditStateRolledBack, EditStateRejected}, ErrTerminalEditState, ErrInvalidEditTransition, "edit")
}

func validateTransition[S comparable](
	from S,
	to S,
	allowed map[S]map[S]struct{},
	terminal []S,
	errTerminal error,
	errInvalid error,
	entity string,
) error {
	for _, terminalState := range terminal {
		if from == terminalState {
			return fmt.Errorf("%w: %v", errTerminal, from)
		}
	}

	next, ok := allowed[from]
	if !ok {
		return fmt.Errorf("%w: unknown %s state %v", errInvalid, entity, from)
	}

	if _, ok := next[to]; !ok {
		return fmt.Errorf("%w: %v -> %v", errInvalid, from, to)
	}

	return nil
}
