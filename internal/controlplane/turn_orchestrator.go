package controlplane

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	ctxmgr "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/context"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/secrets"
)

func (s *Server) assembleTurnPrompt(ctx context.Context, sessionID, turnID, message string, files []string) (string, error) {
	redactionPolicy := newContextRedactionPolicy(secrets.NewPolicyExecutor(nil))
	assembler := ctxmgr.NewContextAssemblerWithPolicy(12000, redactionPolicy)

	_ = assembler.AddFragment(&ctxmgr.ContextFragment{
		SourceType:      "instruction",
		SourceRef:       "turn.message",
		Content:         message,
		TokenCount:      estimateTokenCount(message),
		Included:        true,
		InclusionReason: "user_requested",
		FreshnessState:  "fresh",
	})

	repoPath := ""
	if s.sessionMgr != nil {
		sess, err := s.sessionMgr.Get(ctx, sessionID)
		if err == nil {
			repoPath = strings.TrimSpace(sess.RepoPath)
		}
	}

	if repoPath != "" {
		for _, requested := range files {
			clean := filepath.Clean(strings.TrimSpace(requested))
			if clean == "" || filepath.IsAbs(clean) {
				continue
			}
			candidate := filepath.Join(repoPath, clean)
			if !pathWithinRepo(repoPath, candidate) {
				continue
			}
			contentBytes, err := os.ReadFile(candidate)
			if err != nil {
				continue
			}
			content := string(contentBytes)
			_ = assembler.AddFragment(&ctxmgr.ContextFragment{
				SourceType:      "file",
				SourceRef:       clean,
				Content:         content,
				TokenCount:      estimateTokenCount(content),
				Included:        true,
				InclusionReason: "user_requested",
				FreshnessState:  "fresh",
			})
		}
	}

	prompt := assembler.BuildPrompt()
	if strings.TrimSpace(prompt) == "" {
		decision := redactionPolicy.Apply("turn.message", "user_input", message, ctxmgr.DestinationModelProvider)
		prompt = decision.Content
		if strings.TrimSpace(prompt) == "" {
			prompt = "[DENIED]"
		}
	}

	if s.ctxManifest != nil {
		fragments := assembler.Fragments()
		manifest := make([]ctxmgr.ContextFragment, 0, len(fragments))
		for _, fragment := range fragments {
			if fragment == nil {
				continue
			}
			manifest = append(manifest, *fragment)
		}
		if err := s.ctxManifest.Record(ctx, sessionID, turnID, manifest); err != nil {
			return "", err
		}
	}

	return prompt, nil
}

func estimateTokenCount(content string) int {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return 1
	}
	count := len(trimmed) / 4
	if count < 1 {
		return 1
	}
	return count
}

func pathWithinRepo(repoRoot, candidate string) bool {
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return false
	}
	absCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absCandidate)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return false
	}
	return true
}
