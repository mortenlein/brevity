package actions

import (
	"fmt"
	"io"

	"github.com/mortenlein/brevity/internal/contracts"
)

func RenderTaskContextRefreshResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseTaskContextRefreshPayload(result)
	if err != nil {
		return err
	}

	status := "failure"
	if result.Success {
		status = "success"
	}

	fmt.Fprintf(stdout, "Task context refresh: %s\n", status)
	fmt.Fprintf(stdout, "slug: %s\n", payload.Slug)
	fmt.Fprintf(stdout, "refreshed: %t\n", payload.Refreshed)
	fmt.Fprintf(stdout, "contextPath: %s\n", payload.ContextPath)
	fmt.Fprintf(stdout, "generatedAt: %s\n", payload.GeneratedAt)
	if payload.NormalizedState != "" {
		fmt.Fprintf(stdout, "normalizedState: %s\n", payload.NormalizedState)
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
