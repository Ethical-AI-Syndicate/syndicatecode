package controlplane

import (
	"strings"
	"testing"

	ctxmgr "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/context"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/secrets"
)

func TestNewContextAssemblerUsesInjectedPolicy(t *testing.T) {
	assembler := ctxmgr.NewContextAssemblerWithPolicy(1000, newContextRedactionPolicy(secrets.NewPolicyExecutor(nil)))
	fragment := &ctxmgr.ContextFragment{
		SourceType: "file",
		SourceRef:  "src/keys.txt",
		Content:    "AWS key AKIA1234567890ABCDEF",
		TokenCount: 12,
	}

	_ = assembler.AddFragment(fragment)
	prompt := assembler.BuildPrompt()

	if strings.Contains(prompt, "AKIA1234567890ABCDEF") {
		t.Fatalf("expected prompt to omit raw secret after policy injection")
	}
}
