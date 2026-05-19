package actions

import (
	"fmt"
	"io"

	"github.com/mortenlein/brevity/internal/contracts"
)

func RenderTaskNewResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseTaskNewPayload(result)
	if err != nil {
		return err
	}
	applyTaskNewErrorDetails(&payload, result)

	status := "failure"
	if result.Success {
		status = "success"
	}

	fmt.Fprintf(stdout, "Task new: %s\n", status)
	fmt.Fprintf(stdout, "slug: %s\n", payload.Slug)
	fmt.Fprintf(stdout, "branch: %s\n", payload.Branch)
	fmt.Fprintf(stdout, "worktreePath: %s\n", payload.WorktreePath)
	fmt.Fprintf(stdout, "promptPath: %s\n", payload.PromptPath)
	fmt.Fprintf(stdout, "metadataPath: %s\n", payload.MetadataPath)

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

	return nil
}

func applyTaskNewErrorDetails(payload *contracts.TaskNewPayload, result contracts.CommandResult) {
	for _, commandError := range result.Errors {
		if commandError.Details == nil {
			continue
		}
		if payload.Slug == "" {
			payload.Slug = detailString(commandError.Details, "slug")
		}
		if payload.Branch == "" {
			payload.Branch = detailString(commandError.Details, "branch")
		}
		if payload.WorktreePath == "" {
			payload.WorktreePath = detailString(commandError.Details, "worktreePath")
		}
		if payload.PromptPath == "" {
			payload.PromptPath = detailString(commandError.Details, "promptPath")
		}
		if payload.MetadataPath == "" {
			payload.MetadataPath = detailString(commandError.Details, "metadataPath")
		}
	}
}

func detailString(details map[string]any, key string) string {
	value, ok := details[key]
	if !ok || value == nil {
		return ""
	}

	text, ok := value.(string)
	if !ok {
		return ""
	}

	return text
}
