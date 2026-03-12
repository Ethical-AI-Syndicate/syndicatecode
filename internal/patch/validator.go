package patch

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Validator struct {
	repoRoot string
}

func NewValidator(repoRoot string) *Validator {
	return &Validator{repoRoot: repoRoot}
}

func (v *Validator) ValidatePreApply(proposal Proposal, existingFiles map[string]string) error {
	for i, op := range proposal.Operations {
		if err := v.validateOperation(op, existingFiles); err != nil {
			return fmt.Errorf("operation %d: %w", i, err)
		}
	}
	return nil
}

func (v *Validator) validateOperation(op Operation, existingFiles map[string]string) error {
	if err := v.validatePathPolicy(op.TargetPath); err != nil {
		return err
	}

	existingContent, fileExists := existingFiles[op.TargetPath]

	switch op.Type {
	case OperationTypeAdd:
		if fileExists {
			return fmt.Errorf("cannot add file %q: already exists", op.TargetPath)
		}
	case OperationTypeUpdate:
		if !fileExists {
			return fmt.Errorf("cannot update file %q: does not exist", op.TargetPath)
		}
		if err := v.validatePreimage(op.TargetPath, op.PreimageHash, existingContent); err != nil {
			return err
		}
	case OperationTypeDelete:
		if !fileExists {
			return fmt.Errorf("cannot delete file %q: does not exist", op.TargetPath)
		}
	}

	return nil
}

func (v *Validator) validatePathPolicy(path string) error {
	if filepath.IsAbs(path) {
		return errors.New("absolute paths are not allowed")
	}

	cleanPath := filepath.Clean(path)
	if strings.HasPrefix(cleanPath, "..") {
		return errors.New("path traversal is not allowed")
	}

	return nil
}

func (v *Validator) validatePreimage(_ string, expectedHash, actualContent string) error {
	if expectedHash == "" {
		return errors.New("preimage hash is required for update operations")
	}

	actualHash := hashContent(actualContent)
	if actualHash != expectedHash {
		return fmt.Errorf("preimage mismatch: expected hash %q, file hash is %q", expectedHash, actualHash)
	}

	return nil
}

func (v *Validator) DetectConflicts(proposal Proposal) error {
	pathsByType := map[OperationType]map[string]int{
		OperationTypeAdd:    {},
		OperationTypeUpdate: {},
		OperationTypeDelete: {},
	}

	for i, op := range proposal.Operations {
		pathOps, ok := pathsByType[op.Type]
		if !ok {
			continue
		}

		if _, exists := pathOps[op.TargetPath]; exists {
			return fmt.Errorf("operation %d: duplicate target path %q", i, op.TargetPath)
		}
		pathOps[op.TargetPath] = i
	}

	addPaths := pathsByType[OperationTypeAdd]
	delPaths := pathsByType[OperationTypeDelete]
	for path := range addPaths {
		if _, exists := delPaths[path]; exists {
			return fmt.Errorf("conflict: add and delete operations for same path %q", path)
		}
	}

	return nil
}

func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}

func HashFile(path string) (string, error) {
	// #nosec G304 -- path is provided by validated proposal operations and read only for deterministic hashing.
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashContent(string(data)), nil
}

func HashReader(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return hashContent(string(data)), nil
}
