package secrets

import (
	"strings"
	"testing"
)

func TestPolicyExecutor_ApplyByDestination(t *testing.T) {
	executor := NewPolicyExecutor(NewDetector())

	tests := []struct {
		name             string
		path             string
		sourceType       string
		content          string
		destination      Destination
		expectedAction   RedactionAction
		expectedContains string
		denied           bool
	}{
		{
			name:           "class A denied for export",
			path:           "src/config.go",
			sourceType:     "file",
			content:        "AWS key AKIA1234567890ABCDEF",
			destination:    DestinationExport,
			expectedAction: ActionDeny,
			denied:         true,
		},
		{
			name:             "class A hashed for persistence",
			path:             "src/config.go",
			sourceType:       "file",
			content:          "AWS key AKIA1234567890ABCDEF",
			destination:      DestinationPersistence,
			expectedAction:   ActionHash,
			expectedContains: "sha256:",
		},
		{
			name:             "class B masked for model provider",
			path:             "src/main.go",
			sourceType:       "tool_output",
			content:          "generated token: zY8pQ1mN4vR7tX2cK9dL6sH3wB5qJ8nF",
			destination:      DestinationModelProvider,
			expectedAction:   ActionMask,
			expectedContains: "[REDACTED]",
		},
		{
			name:             "class B partially masked for ui",
			path:             "src/main.go",
			sourceType:       "tool_output",
			content:          "generated token: zY8pQ1mN4vR7tX2cK9dL6sH3wB5qJ8nF",
			destination:      DestinationUI,
			expectedAction:   ActionPartialMask,
			expectedContains: "...",
		},
		{
			name:             "class C summarized for model provider",
			path:             ".env.production",
			sourceType:       "file",
			content:          "APP_ENV=prod",
			destination:      DestinationModelProvider,
			expectedAction:   ActionSummarize,
			expectedContains: "summary:",
		},
		{
			name:           "class D allowed",
			path:           "src/app.go",
			sourceType:     "file",
			content:        "fmt.Println(\"hello\")",
			destination:    DestinationPersistence,
			expectedAction: ActionAllow,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			decision := executor.Apply(tc.path, tc.sourceType, tc.content, tc.destination)

			if decision.Action != tc.expectedAction {
				t.Fatalf("expected action %s, got %s", tc.expectedAction, decision.Action)
			}
			if decision.Denied != tc.denied {
				t.Fatalf("expected denied=%v, got %v", tc.denied, decision.Denied)
			}
			if tc.expectedContains != "" && !strings.Contains(decision.Content, tc.expectedContains) {
				t.Fatalf("expected transformed content to contain %q, got %q", tc.expectedContains, decision.Content)
			}
			if tc.expectedAction == ActionAllow && decision.Content != tc.content {
				t.Fatalf("expected allowed content to be unchanged")
			}
			if decision.Content == tc.content && tc.expectedAction != ActionAllow {
				t.Fatalf("expected content to change for action %s", tc.expectedAction)
			}
		})
	}
}

func TestPolicyExecutor_ApplyMapForUIDestination(t *testing.T) {
	executor := NewPolicyExecutor(NewDetector())
	input := map[string]interface{}{
		"safe":   "hello",
		"secret": "AKIA1234567890ABCDEF",
		"nested": map[string]interface{}{
			"token": "token=ghp_0123456789abcdefghijklmnopqrstuvwxyzAB",
		},
	}

	result := executor.ApplyMap("tool://echo", "tool_output", DestinationUI, input)

	secret, ok := result["secret"].(string)
	if !ok {
		t.Fatal("expected secret field to remain a string")
	}
	if strings.Contains(secret, "AKIA1234567890ABCDEF") {
		t.Fatalf("expected class A value to be transformed for UI destination")
	}

	nested, ok := result["nested"].(map[string]interface{})
	if !ok {
		t.Fatal("expected nested map")
	}
	token, ok := nested["token"].(string)
	if !ok {
		t.Fatal("expected nested token to remain a string")
	}
	if strings.Contains(token, "ghp_0123456789abcdefghijklmnopqrstuvwxyzAB") {
		t.Fatalf("expected nested token to be transformed")
	}
}
