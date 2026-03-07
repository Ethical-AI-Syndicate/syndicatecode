package tools

import (
	"context"
	"fmt"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/patch"
)

func ApplyPatchHandler(engine *patch.Engine) ToolHandler {
	return func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		_ = ctx
		rawPatch, ok := input["patch"].(string)
		if !ok || rawPatch == "" {
			return nil, fmt.Errorf("patch is required")
		}

		result, err := engine.Apply(rawPatch)
		if err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"files_modified": result.ModifiedFiles,
		}, nil
	}
}
