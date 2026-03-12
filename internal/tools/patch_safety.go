package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gitops "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/git"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/patch"
)

type PatchSafetyConfig struct {
	CheckpointStore gitops.CheckpointStore
}

type patchMutation struct {
	Path         string
	MutationType string
	BeforeHash   string
	AfterHash    string
}

type PatchSafetyService struct {
	engine *patch.Engine
	cfg    PatchSafetyConfig
}

func NewPatchSafetyService(engine *patch.Engine, cfg PatchSafetyConfig) *PatchSafetyService {
	return &PatchSafetyService{engine: engine, cfg: cfg}
}

func (s *PatchSafetyService) Apply(ctx context.Context, patchText, sessionID, turnID string) (*patch.Result, []patchMutation, error) {
	proposal, err := s.proposalFromPatch(ctx, patchText, sessionID)
	if err != nil {
		return nil, nil, err
	}

	existingFiles := s.readExistingFiles(proposal)

	validator := patch.NewValidator(patch.EngineRepoRoot(s.engine))
	if err := validator.ValidatePreApply(proposal, existingFiles); err != nil {
		return nil, nil, err
	}

	mutationScope := make([]string, 0, len(proposal.Operations))
	for _, op := range proposal.Operations {
		mutationScope = append(mutationScope, op.TargetPath)
	}

	anchorID := ""
	var anchors *gitops.AnchorManager
	if s.cfg.CheckpointStore != nil {
		anchors = gitops.NewAnchorManager(s.cfg.CheckpointStore)
		anchor, anchorErr := anchors.CreateAnchor(ctx, gitops.AnchorRequest{
			SessionID:     sessionID,
			TurnID:        turnID,
			RepoPath:      patch.EngineRepoRoot(s.engine),
			MutationScope: mutationScope,
			EnableGitRef:  true,
		})
		if anchorErr != nil {
			return nil, nil, anchorErr
		}
		anchorID = anchor.ID
	}

	mutations := make([]patchMutation, 0, len(proposal.Operations))
	for _, op := range proposal.Operations {
		beforeHash := ""
		if current, ok := existingFiles[op.TargetPath]; ok {
			beforeHash = hashContent(current)
		}
		mutations = append(mutations, patchMutation{Path: op.TargetPath, MutationType: string(op.Type), BeforeHash: beforeHash})
	}

	result, applyErr := s.engine.Apply(patchText)
	if applyErr != nil {
		if anchors != nil && anchorID != "" {
			_ = anchors.Restore(ctx, anchorID)
		}
		return nil, nil, applyErr
	}

	for i := range mutations {
		absPath := filepath.Join(patch.EngineRepoRoot(s.engine), filepath.Clean(mutations[i].Path))
		hash, hashErr := patch.HashFile(absPath)
		if hashErr == nil {
			mutations[i].AfterHash = hash
		}
	}

	return result, mutations, nil
}

func (s *PatchSafetyService) proposalFromPatch(ctx context.Context, patchText, sessionID string) (patch.Proposal, error) {
	_ = ctx
	lines := strings.Split(strings.ReplaceAll(patchText, "\r\n", "\n"), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "*** Begin Patch" || strings.TrimSpace(lines[len(lines)-1]) != "*** End Patch" {
		return patch.Proposal{}, fmt.Errorf("invalid patch envelope")
	}

	ops := make([]patch.Operation, 0)
	for i := 1; i < len(lines)-1; i++ {
		line := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(line, "*** Add File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: "))
			ops = append(ops, patch.Operation{Type: patch.OperationTypeAdd, TargetPath: path, Content: "added"})
		case strings.HasPrefix(line, "*** Update File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: "))
			pre, err := s.currentHash(path)
			if err != nil {
				return patch.Proposal{}, err
			}
			ops = append(ops, patch.Operation{Type: patch.OperationTypeUpdate, TargetPath: path, PreimageHash: pre})
		case strings.HasPrefix(line, "*** Delete File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))
			ops = append(ops, patch.Operation{Type: patch.OperationTypeDelete, TargetPath: path})
		}
	}

	if strings.TrimSpace(sessionID) == "" {
		sessionID = "tool-session"
	}

	proposal := patch.Proposal{ID: "patch-tool", SessionID: sessionID, Operations: ops}
	if err := proposal.Validate(); err != nil {
		return patch.Proposal{}, err
	}
	return proposal, nil
}

func (s *PatchSafetyService) readExistingFiles(proposal patch.Proposal) map[string]string {
	existing := make(map[string]string)
	for _, op := range proposal.Operations {
		absPath := filepath.Join(patch.EngineRepoRoot(s.engine), filepath.Clean(op.TargetPath))
		// #nosec G304 // filepath.Clean and EngineRepoRoot provide path safety
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		existing[op.TargetPath] = string(data)
	}
	return existing
}

func (s *PatchSafetyService) currentHash(path string) (string, error) {
	absPath := filepath.Join(patch.EngineRepoRoot(s.engine), filepath.Clean(path))
	return patch.HashFile(absPath)
}

func hashContent(content string) string {
	hash, _ := patch.HashReader(strings.NewReader(content))
	return hash
}
