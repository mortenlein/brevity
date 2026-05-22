package actions

import (
	"fmt"
	"io"

	"github.com/mortenlein/brevity/internal/contracts"
)

func RenderTaskRunResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseTaskRunExecutionPayload(result)
	if err != nil {
		return err
	}
	applyTaskRunErrorDetails(&payload, result)

	renderStatusLine(stdout, "Task run", result.Success)
	fmt.Fprintf(stdout, "slug: %s\n", payload.Slug)
	fmt.Fprintf(stdout, "provider: %s\n", emptyAsUnknown(payload.Provider))
	fmt.Fprintf(stdout, "profile: %s\n", emptyAsUnknown(payload.Profile))
	fmt.Fprintf(stdout, "workerStatus: %s\n", emptyAsUnknown(payload.WorkerStatus))
	fmt.Fprintf(stdout, "runId: %s\n", emptyAsUnknown(payload.RunID))
	fmt.Fprintf(stdout, "startedAt: %s\n", emptyAsUnknown(payload.StartedAt))
	fmt.Fprintf(stdout, "finishedAt: %s\n", emptyAsUnknown(payload.FinishedAt))
	fmt.Fprintf(stdout, "exitCode: %s\n", formatOptional(payload.ExitCode))
	fmt.Fprintf(stdout, "logPath: %s\n", emptyAsUnknown(payload.LogPath))
	if payload.FailureType != "" {
		fmt.Fprintf(stdout, "failureType: %s\n", payload.FailureType)
	}

	renderMessages(stdout, result)
	return nil
}

func RenderTaskRunPlanResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseTaskRunPlanPayload(result)
	if err != nil {
		return err
	}
	renderStatusLine(stdout, "Task run plan", result.Success)
	fmt.Fprintf(stdout, "slug: %s\n", payload.Slug)
	fmt.Fprintf(stdout, "taskState: %s\n", emptyAsUnknown(payload.TaskState))
	fmt.Fprintf(stdout, "provider: %s\n", emptyAsUnknown(payload.Provider))
	fmt.Fprintf(stdout, "profile: %s\n", emptyAsUnknown(payload.Profile))
	if payload.Model != "" {
		fmt.Fprintf(stdout, "model: %s\n", payload.Model)
	}
	fmt.Fprintf(stdout, "worktreePath: %s\n", emptyAsUnknown(payload.WorktreePath))
	fmt.Fprintf(stdout, "promptPath: %s\n", emptyAsUnknown(payload.PromptPath))
	fmt.Fprintf(stdout, "promptStatus: %s\n", emptyAsUnknown(payload.PromptStatus))
	fmt.Fprintf(stdout, "runIdPlan: %s\n", emptyAsUnknown(payload.RunIDPlan))
	fmt.Fprintf(stdout, "logPathPlan: %s\n", emptyAsUnknown(payload.LogPathPlan))
	fmt.Fprintf(stdout, "workerCommand: %s\n", emptyAsUnknown(payload.WorkerCommand.Display))
	fmt.Fprintf(stdout, "executionKind: %s\n", emptyAsUnknown(payload.ExecutionKind))
	fmt.Fprintf(stdout, "authority: %s\n", emptyAsUnknown(payload.Authority))
	fmt.Fprintf(stdout, "noExecutionOccurred: %t\n", payload.NoExecutionOccurred)
	renderMessages(stdout, result)
	for _, blocker := range payload.Blockers {
		fmt.Fprintf(stdout, "blocker: %s\n", blocker.DisplayText())
	}
	return nil
}

func applyTaskRunErrorDetails(payload *contracts.TaskRunExecutionPayload, result contracts.CommandResult) {
	for _, commandError := range result.Errors {
		if commandError.Details == nil {
			continue
		}
		if payload.Slug == "" {
			payload.Slug = detailString(commandError.Details, "slug")
		}
		if payload.Provider == "" {
			payload.Provider = detailString(commandError.Details, "provider")
		}
	}
}
