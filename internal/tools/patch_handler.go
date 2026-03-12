package tools

import (
	"context"
	"fmt"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/patch"
)

// MutationRecorder is called for each successfully applied patch operation.
type MutationRecorder func(ctx context.Context, path, mutationType, beforeHash, afterHash string)

func ApplyPatchHandler(engine *patch.Engine, recorder MutationRecorder) ToolHandler {
	return func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		rawPatch, ok := input["patch"].(string)
		if !ok || rawPatch == "" {
			return nil, fmt.Errorf("patch is required")
		}

		result, err := engine.Apply(rawPatch)
		if err != nil {
			return nil, err
		}

		if recorder != nil {
			for _, path := range result.ModifiedFiles {
				recorder(ctx, path, "modified", "", "")
			}
		}

		return map[string]interface{}{
			"files_modified": result.ModifiedFiles,
		}, nil
	}
}
