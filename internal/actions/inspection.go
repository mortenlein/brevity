package actions

import (
	"fmt"
	"io"

	"github.com/mortenlein/brevity/internal/contracts"
)

func RenderTaskRuntimeInfoResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseTaskRuntimeInfoPayload(result)
	if err != nil {
		return err
	}

	applyTaskRuntimeInfoErrorDetails(&payload, result)

	fmt.Fprintln(stdout, "Task runtime-info")
	fmt.Fprintf(stdout, "slug: %s\n", payload.Slug)
	fmt.Fprintf(stdout, "status: %s\n", emptyAsUnknown(payload.Status))
	fmt.Fprintf(stdout, "normalizedState: %s\n", emptyAsUnknown(payload.NormalizedState))
	fmt.Fprintf(stdout, "worktreeExists: %t\n", payload.Worktree.Exists)
	fmt.Fprintf(stdout, "worktreePath: %s\n", payload.Worktree.Path)
	fmt.Fprintf(stdout, "contextFileCount: %d\n", payload.Context.MaterializedFileCount)
	fmt.Fprintf(stdout, "contextMissingCount: %d\n", len(payload.Context.MissingFiles))
	fmt.Fprintf(stdout, "executionStatus: %s\n", emptyAsUnknown(payload.Execution.Status))
	fmt.Fprintf(stdout, "latestRunId: %s\n", emptyAsUnknown(payload.Execution.LastRunID))

	renderMessages(stdout, result)
	return nil
}

func RenderTaskRunsResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseTaskRunsPayload(result)
	if err != nil {
		return err
	}

	applyTaskRunsErrorDetails(&payload, result)

	fmt.Fprintln(stdout, "Task runs")
	fmt.Fprintf(stdout, "slug: %s\n", payload.Slug)
	fmt.Fprintf(stdout, "count: %d\n", payload.Count)
	if len(payload.Runs) == 0 {
		fmt.Fprintln(stdout, "recentRunIds: none")
	} else {
		fmt.Fprintln(stdout, "runs:")
		for _, run := range payload.Runs {
			fmt.Fprintf(stdout, "- runId: %s\n", emptyAsUnknown(run.RunID))
			fmt.Fprintf(stdout, "  status: %s\n", emptyAsUnknown(run.WorkerStatus))
			fmt.Fprintf(stdout, "  exitCode: %s\n", formatOptional(run.ExitCode))
			fmt.Fprintf(stdout, "  provider: %s\n", emptyAsUnknown(run.Provider))
			fmt.Fprintf(stdout, "  profile: %s\n", emptyAsUnknown(run.Profile))
			fmt.Fprintf(stdout, "  logPath: %s\n", emptyAsUnknown(run.LogPath))
		}
	}

	renderMessages(stdout, result)
	return nil
}

func RenderDoctorResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseDoctorPayload(result)
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, "Doctor")
	fmt.Fprintf(stdout, "warnings: %d\n", payload.WarningCount)
	fmt.Fprintf(stdout, "errors: %d\n", payload.ErrorCount)
	fmt.Fprintf(stdout, "providers: total=%d degraded=%d unavailable=%d\n",
		payload.Providers.Summary.Total,
		payload.Providers.Summary.Degraded,
		payload.Providers.Summary.Unavailable,
	)
	fmt.Fprintf(stdout, "orphanedWorktrees: %d\n", payload.WorktreeCounts.OrphanedTaskWorktrees)
	fmt.Fprintf(stdout, "orphanedBranches: %d\n", payload.BranchCounts.Orphaned)
	fmt.Fprintf(stdout, "lockExists: %t\n", payload.Lock.Exists)
	if payload.Lock.Path != "" {
		fmt.Fprintf(stdout, "lockPath: %s\n", payload.Lock.Path)
	}

	renderMessages(stdout, result)
	return nil
}

func renderMessages(stdout io.Writer, result contracts.CommandResult) {
	for _, warning := range result.Warnings {
		fmt.Fprintf(stdout, "warning: %s\n", warning.DisplayText())
	}
	for _, commandError := range result.Errors {
		fmt.Fprintf(stdout, "error: %s\n", commandError.DisplayText())
	}
	if len(result.SuggestedNextActions) > 0 {
		fmt.Fprintln(stdout, "suggested next actions:")
		for _, action := range result.SuggestedNextActions {
			fmt.Fprintf(stdout, "- %s\n", action)
		}
	}
}

func applyTaskRuntimeInfoErrorDetails(payload *contracts.TaskRuntimeInfoPayload, result contracts.CommandResult) {
	for _, commandError := range result.Errors {
		if commandError.Details == nil || payload.Slug != "" {
			continue
		}
		payload.Slug = detailString(commandError.Details, "slug")
	}
}

func applyTaskRunsErrorDetails(payload *contracts.TaskRunsPayload, result contracts.CommandResult) {
	for _, commandError := range result.Errors {
		if commandError.Details == nil || payload.Slug != "" {
			continue
		}
		payload.Slug = detailString(commandError.Details, "slug")
	}
}

func formatOptional(value any) string {
	if value == nil {
		return "unknown"
	}
	text := fmt.Sprint(value)
	if text == "" {
		return "unknown"
	}
	return text
}
