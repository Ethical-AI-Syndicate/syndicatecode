package tools

import (
	"context"
	"testing"
)

func TestNewPatchSafetyService_Bead_l3d_17_1(t *testing.T) {
	cfg := PatchSafetyConfig{}
	service := NewPatchSafetyService(nil, cfg)
	if service == nil {
		t.Error("NewPatchSafetyService() returned nil")
	}
}

func TestPatchSafetyServiceProposalFromPatch_Bead_l3d_17_1(t *testing.T) {
	service := &PatchSafetyService{}

	tests := []struct {
		name      string
		patchText string
		sessionID string
		wantErr   bool
	}{
		{
			name:      "invalid patch envelope - missing start",
			patchText: "*** Update File: test.txt\n*** End Patch",
			sessionID: "test-session",
			wantErr:   true,
		},
		{
			name:      "invalid patch envelope - missing end",
			patchText: "*** Begin Patch\n*** Update File: test.txt",
			sessionID: "test-session",
			wantErr:   true,
		},
		{
			name:      "add file operation",
			patchText: "*** Begin Patch\n*** Add File: new.txt\n*** End Patch",
			sessionID: "test-session",
			wantErr:   false,
		},
		{
			name:      "delete file operation",
			patchText: "*** Begin Patch\n*** Delete File: old.txt\n*** End Patch",
			sessionID: "test-session",
			wantErr:   false,
		},
		{
			name:      "empty session ID uses default",
			patchText: "*** Begin Patch\n*** Delete File: old.txt\n*** End Patch",
			sessionID: "",
			wantErr:   false,
		},
		{
			name:      "multiple operations",
			patchText: "*** Begin Patch\n*** Add File: new.txt\n*** Delete File: old.txt\n*** End Patch",
			sessionID: "test-session",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.proposalFromPatch(context.TODO(), tt.patchText, tt.sessionID)
			if (err != nil) != tt.wantErr {
				t.Errorf("proposalFromPatch() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHashContent_Bead_l3d_17_1(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"empty string", ""},
		{"simple content", "hello world"},
		{"multiline content", "line1\nline2\nline3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := hashContent(tt.content)
			if hash == "" {
				t.Error("hashContent() should not return empty hash")
			}
		})
	}
}
