package patch

import (
	"errors"
	"fmt"
)

type OperationType string

const (
	OperationTypeAdd    OperationType = "add"
	OperationTypeUpdate OperationType = "update"
	OperationTypeDelete OperationType = "delete"
)

type Sensitivity string

const (
	SensitivityLow      Sensitivity = "low"
	SensitivityMedium   Sensitivity = "medium"
	SensitivityHigh     Sensitivity = "high"
	SensitivityCritical Sensitivity = "critical"
)

type Operation struct {
	Type                  OperationType `json:"type"`
	TargetPath            string        `json:"target_path"`
	Content               string        `json:"content,omitempty"`
	PreimageHash          string        `json:"preimage_hash,omitempty"`
	ExpectedPostimageHash string        `json:"expected_postimage_hash,omitempty"`
	Sensitivity           Sensitivity   `json:"sensitivity,omitempty"`
	RequiresApproval      bool          `json:"requires_approval,omitempty"`
}

type Proposal struct {
	ID               string      `json:"id"`
	SessionID        string      `json:"session_id"`
	Description      string      `json:"description,omitempty"`
	TrustTier        string      `json:"trust_tier,omitempty"`
	PreimageHash     string      `json:"preimage_hash,omitempty"`
	RequiresApproval bool        `json:"requires_approval,omitempty"`
	Operations       []Operation `json:"operations"`
}

func (p *Proposal) Validate() error {
	if p.ID == "" {
		return errors.New("proposal id is required")
	}
	if p.SessionID == "" {
		return errors.New("session id is required")
	}
	if len(p.Operations) == 0 {
		return errors.New("proposal must have at least one operation")
	}

	seenPaths := make(map[string]bool)
	for i, op := range p.Operations {
		if err := op.Validate(i); err != nil {
			return err
		}
		if seenPaths[op.TargetPath] {
			return fmt.Errorf("operation %d: duplicate target path %s", i, op.TargetPath)
		}
		seenPaths[op.TargetPath] = true
	}

	return nil
}

func (o *Operation) Validate(opIndex int) error {
	if o.TargetPath == "" {
		return fmt.Errorf("operation %d: target path is required", opIndex)
	}
	if o.Type == "" {
		return fmt.Errorf("operation %d: operation type is required", opIndex)
	}
	switch o.Type {
	case OperationTypeAdd:
		if o.Content == "" {
			return fmt.Errorf("operation %d: add operation requires content", opIndex)
		}
	case OperationTypeUpdate:
		if o.PreimageHash == "" {
			return fmt.Errorf("operation %d: update operation requires preimage hash", opIndex)
		}
	case OperationTypeDelete:
		// Delete only needs target path
	default:
		return fmt.Errorf("operation %d: unknown operation type %s", opIndex, o.Type)
	}
	return nil
}
