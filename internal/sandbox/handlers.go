package sandbox

import (
	"context"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

func RunTestsHandler(runner *Runner) tools.ToolHandler {
	return func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		result, err := runner.Run(ctx, Request{Command: "go_test_policy"})
		if err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"exit_code":        result.ExitCode,
			"stdout":           result.Stdout,
			"stderr":           result.Stderr,
			"duration_ms":      result.DurationMS,
			"output_truncated": result.OutputTruncated,
		}, nil
	}
}

func RunLintHandler(runner *Runner) tools.ToolHandler {
	return func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		result, err := runner.Run(ctx, Request{Command: "golangci_lint_run"})
		if err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"exit_code":        result.ExitCode,
			"stdout":           result.Stdout,
			"stderr":           result.Stderr,
			"duration_ms":      result.DurationMS,
			"output_truncated": result.OutputTruncated,
		}, nil
	}
}

func RestrictedShellHandler(runner *Runner) tools.ToolHandler {
	return func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		cmd, _ := input["command"].(string)
		workDir, _ := input["work_dir"].(string)

		result, err := runner.Run(ctx, Request{Command: cmd, WorkDir: workDir})
		if err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"exit_code":        result.ExitCode,
			"stdout":           result.Stdout,
			"stderr":           result.Stderr,
			"duration_ms":      result.DurationMS,
			"output_truncated": result.OutputTruncated,
		}, nil
	}
}
