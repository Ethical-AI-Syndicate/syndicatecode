package controlplane

import (
	"strings"
	"testing"

	ctxmgr "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/context"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/secrets"
)

func TestContextRedactorForDestinationPersistenceHashesSecrets(t *testing.T) {
	t.Parallel()

	redactor := contextRedactorForDestination(secrets.DestinationPersistence)
	decision := redactor("turn.message", "user_input", "AKIA1234567890ABCDEF")

	if decision.Action != "hash" {
		t.Fatalf("expected hash action, got %s", decision.Action)
	}
	if decision.Content == "AKIA1234567890ABCDEF" {
		t.Fatalf("expected hashed content")
	}
	if decision.Sensitivity != "A" {
		t.Fatalf("expected sensitivity class A, got %s", decision.Sensitivity)
	}
}

func TestContextRedactorForDestinationModelProviderDeniesSecrets(t *testing.T) {
	t.Parallel()

	redactor := contextRedactorForDestination(secrets.DestinationModelProvider)
	decision := redactor("src/keys.txt", "file", "AKIA1234567890ABCDEF")

	if !decision.Denied {
		t.Fatalf("expected model provider destination to deny class A secret")
	}
	if decision.Action != "deny" {
		t.Fatalf("expected deny action, got %s", decision.Action)
	}
}

func TestContextAssemblerWithControlPlaneRedactorDeniesSecretPromptContent(t *testing.T) {
	t.Parallel()

	assembler := ctxmgr.NewContextAssemblerWithRedactor(1000, contextRedactorForDestination(secrets.DestinationModelProvider))
	fragment := &ctxmgr.ContextFragment{
		SourceType: "file",
		SourceRef:  "src/keys.txt",
		Content:    "AKIA1234567890ABCDEF",
		TokenCount: 10,
	}

	if err := assembler.AddFragment(fragment); err != nil {
		t.Fatalf("failed to add fragment: %v", err)
	}

	prompt := assembler.BuildPrompt()
	if prompt != "" {
		t.Fatalf("expected denied secret prompt to be empty")
	}
	if strings.Contains(prompt, "AKIA1234567890ABCDEF") {
		t.Fatalf("expected prompt output to exclude raw secret")
	}
	if !fragment.RedactionDenied {
		t.Fatalf("expected redaction denied marker")
	}
}
