package patch

import (
	"encoding/json"
	"testing"
)

func TestProposal_Serialization_Bead_l3d_6_1(t *testing.T) {
	tests := []struct {
		name          string
		proposal      Proposal
		expectedValid bool
	}{
		{
			name: "single file add proposal",
			proposal: Proposal{
				ID:        "proposal-1",
				SessionID: "session-1",
				Operations: []Operation{
					{
						Type:         OperationTypeAdd,
						TargetPath:   "src/main.go",
						Content:      "package main\n\nfunc main() {}\n",
						PreimageHash: "",
					},
				},
			},
			expectedValid: true,
		},
		{
			name: "single file update proposal with preimage",
			proposal: Proposal{
				ID:           "proposal-2",
				SessionID:    "session-1",
				PreimageHash: "abc123",
				Operations: []Operation{
					{
						Type:         OperationTypeUpdate,
						TargetPath:   "src/main.go",
						Content:      "package main\n\nfunc main() { println('hello') }\n",
						PreimageHash: "abc123",
					},
				},
			},
			expectedValid: true,
		},
		{
			name: "multi-file bundle proposal",
			proposal: Proposal{
				ID:          "proposal-3",
				SessionID:   "session-1",
				Description: "Add auth module",
				Operations: []Operation{
					{
						Type:         OperationTypeAdd,
						TargetPath:   "src/auth/service.go",
						Content:      "package auth\n\ntype Service struct {}\n",
						PreimageHash: "",
					},
					{
						Type:         OperationTypeAdd,
						TargetPath:   "src/auth/middleware.go",
						Content:      "package auth\n\nfunc Middleware() {}\n",
						PreimageHash: "",
					},
					{
						Type:         OperationTypeUpdate,
						TargetPath:   "src/main.go",
						Content:      "package main\n\nimport \"auth\"\n\nfunc main() {}\n",
						PreimageHash: "oldhash",
					},
				},
			},
			expectedValid: true,
		},
		{
			name: "proposal with policy metadata",
			proposal: Proposal{
				ID:               "proposal-4",
				SessionID:        "session-1",
				TrustTier:        "untrusted",
				RequiresApproval: true,
				Operations: []Operation{
					{
						Type:             OperationTypeUpdate,
						TargetPath:       "config.yaml",
						Content:          "key: value\n",
						PreimageHash:     "hash123",
						Sensitivity:      SensitivityHigh,
						RequiresApproval: true,
					},
				},
			},
			expectedValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.proposal)
			if err != nil {
				t.Fatalf("failed to marshal proposal: %v", err)
			}

			var decoded Proposal
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("failed to unmarshal proposal: %v", err)
			}

			if decoded.ID != tt.proposal.ID {
				t.Errorf("expected ID %s, got %s", tt.proposal.ID, decoded.ID)
			}
			if decoded.SessionID != tt.proposal.SessionID {
				t.Errorf("expected SessionID %s, got %s", tt.proposal.SessionID, decoded.SessionID)
			}
			if len(decoded.Operations) != len(tt.proposal.Operations) {
				t.Errorf("expected %d operations, got %d", len(tt.proposal.Operations), len(decoded.Operations))
			}

			for i, op := range decoded.Operations {
				expectedOp := tt.proposal.Operations[i]
				if op.Type != expectedOp.Type {
					t.Errorf("operation %d: expected type %s, got %s", i, expectedOp.Type, op.Type)
				}
				if op.TargetPath != expectedOp.TargetPath {
					t.Errorf("operation %d: expected path %s, got %s", i, expectedOp.TargetPath, op.TargetPath)
				}
			}
		})
	}
}

func TestProposal_Validation_Bead_l3d_6_1(t *testing.T) {
	tests := []struct {
		name          string
		proposal      Proposal
		expectError   bool
		errorContains string
	}{
		{
			name: "valid single file proposal",
			proposal: Proposal{
				ID:        "p1",
				SessionID: "s1",
				Operations: []Operation{
					{Type: OperationTypeAdd, TargetPath: "a.go", Content: "package a"},
				},
			},
			expectError: false,
		},
		{
			name: "invalid - empty operations",
			proposal: Proposal{
				ID:        "p2",
				SessionID: "s1",
			},
			expectError:   true,
			errorContains: "at least one operation",
		},
		{
			name: "invalid - empty target path",
			proposal: Proposal{
				ID:        "p3",
				SessionID: "s1",
				Operations: []Operation{
					{Type: OperationTypeAdd, TargetPath: ""},
				},
			},
			expectError:   true,
			errorContains: "target path",
		},
		{
			name: "invalid - update without preimage hash",
			proposal: Proposal{
				ID:        "p4",
				SessionID: "s1",
				Operations: []Operation{
					{Type: OperationTypeUpdate, TargetPath: "a.go", Content: "new"},
				},
			},
			expectError:   true,
			errorContains: "preimage hash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.proposal.Validate()
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorContains)
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
