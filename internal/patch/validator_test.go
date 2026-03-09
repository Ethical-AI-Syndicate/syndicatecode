package patch

import (
	"strings"
	"testing"
)

func TestPreimageValidation_Bead_l3d_6_2(t *testing.T) {
	engine := NewEngine("")

	tests := []struct {
		name          string
		proposal      Proposal
		setupFiles    map[string]string
		expectError   bool
		errorContains string
	}{
		{
			name: "preimage mismatch - file changed",
			proposal: Proposal{
				ID:        "p1",
				SessionID: "s1",
				Operations: []Operation{
					{
						Type:         OperationTypeUpdate,
						TargetPath:   "a.go",
						Content:      "new content",
						PreimageHash: "old_hash_that_doesnt_match",
					},
				},
			},
			setupFiles: map[string]string{
				"a.go": "original content",
			},
			expectError:   true,
			errorContains: "preimage",
		},
		{
			name: "preimage match - success",
			proposal: Proposal{
				ID:        "p2",
				SessionID: "s1",
				Operations: []Operation{
					{
						Type:         OperationTypeUpdate,
						TargetPath:   "a.go",
						Content:      "new content",
						PreimageHash: "cdf5a09bf85d2dfd30ed3c7cb37ec633778f921737864ffef000a8f708a982a5",
					},
				},
			},
			setupFiles: map[string]string{
				"a.go": "original content here",
			},
			expectError: false,
		},
		{
			name: "add to existing file - should fail",
			proposal: Proposal{
				ID:        "p3",
				SessionID: "s1",
				Operations: []Operation{
					{
						Type:       OperationTypeAdd,
						TargetPath: "existing.go",
						Content:    "new content",
					},
				},
			},
			setupFiles: map[string]string{
				"existing.go": "already exists",
			},
			expectError:   true,
			errorContains: "already exists",
		},
		{
			name: "delete non-existent file - should fail",
			proposal: Proposal{
				ID:        "p4",
				SessionID: "s1",
				Operations: []Operation{
					{
						Type:       OperationTypeDelete,
						TargetPath: "nonexistent.go",
					},
				},
			},
			setupFiles:    map[string]string{},
			expectError:   true,
			errorContains: "does not exist",
		},
		{
			name: "path policy violation - path traversal",
			proposal: Proposal{
				ID:        "p5",
				SessionID: "s1",
				Operations: []Operation{
					{
						Type:       OperationTypeAdd,
						TargetPath: "../../../etc/passwd",
						Content:    "malicious",
					},
				},
			},
			setupFiles:    map[string]string{},
			expectError:   true,
			errorContains: "path traversal",
		},
		{
			name: "path policy violation - absolute path",
			proposal: Proposal{
				ID:        "p6",
				SessionID: "s1",
				Operations: []Operation{
					{
						Type:       OperationTypeAdd,
						TargetPath: "/etc/passwd",
						Content:    "malicious",
					},
				},
			},
			setupFiles:    map[string]string{},
			expectError:   true,
			errorContains: "absolute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(engine.repoRoot)

			err := validator.ValidatePreApply(tt.proposal, tt.setupFiles)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorContains)
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
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

func TestConflictDetection_Bead_l3d_6_2(t *testing.T) {
	tests := []struct {
		name          string
		proposal      Proposal
		expectError   bool
		errorContains string
	}{
		{
			name: "overlapping updates to same file",
			proposal: Proposal{
				ID:        "p1",
				SessionID: "s1",
				Operations: []Operation{
					{
						Type:         OperationTypeUpdate,
						TargetPath:   "a.go",
						Content:      "content 1",
						PreimageHash: "hash1",
					},
					{
						Type:         OperationTypeUpdate,
						TargetPath:   "a.go",
						Content:      "content 2",
						PreimageHash: "hash1",
					},
				},
			},
			expectError:   true,
			errorContains: "duplicate",
		},
		{
			name: "add and delete same path",
			proposal: Proposal{
				ID:        "p2",
				SessionID: "s1",
				Operations: []Operation{
					{
						Type:       OperationTypeAdd,
						TargetPath: "a.go",
						Content:    "new",
					},
					{
						Type:       OperationTypeDelete,
						TargetPath: "a.go",
					},
				},
			},
			expectError:   true,
			errorContains: "conflict",
		},
		{
			name: "non-overlapping operations - success",
			proposal: Proposal{
				ID:        "p3",
				SessionID: "s1",
				Operations: []Operation{
					{
						Type:       OperationTypeAdd,
						TargetPath: "a.go",
						Content:    "content a",
					},
					{
						Type:       OperationTypeAdd,
						TargetPath: "b.go",
						Content:    "content b",
					},
					{
						Type:         OperationTypeUpdate,
						TargetPath:   "c.go",
						Content:      "new c",
						PreimageHash: "oldhash",
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &Validator{}

			err := validator.DetectConflicts(tt.proposal)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorContains)
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
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
