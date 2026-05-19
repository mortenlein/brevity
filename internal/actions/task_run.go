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

	status := "failure"
	if result.Success {
		status = "success"
	}

	fmt.Fprintf(stdout, "Task run: %s\n", status)
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
