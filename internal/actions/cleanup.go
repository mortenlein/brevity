package actions

import (
	"fmt"
	"io"

	"github.com/mortenlein/brevity/internal/contracts"
)

func RenderTaskCleanupResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseTaskCleanupPayload(result)
	if err != nil {
		return err
	}

	status := "failure"
	if result.Success {
		status = "success"
	}

	fmt.Fprintf(stdout, "Task cleanup: %s\n", status)
	fmt.Fprintf(stdout, "slug: %s\n", payload.Slug)
	fmt.Fprintf(stdout, "worktreeRemoved: %t\n", payload.WorktreeRemoved)
	fmt.Fprintf(stdout, "branchRemoved: %t\n", payload.BranchRemoved)
	fmt.Fprintf(stdout, "metadataRemoved: %t\n", payload.MetadataRemoved)

	for _, warning := range payload.CleanupWarnings {
		fmt.Fprintf(stdout, "cleanupWarning: %s\n", warning)
	}
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
