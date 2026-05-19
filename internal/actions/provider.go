package actions

import (
	"fmt"
	"io"

	"github.com/mortenlein/brevity/internal/contracts"
)

func RenderProviderResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseProviderActionPayload(result)
	if err != nil {
		return err
	}

	status := "failure"
	if result.Success {
		status = "success"
	}

	fmt.Fprintf(stdout, "Provider action: %s\n", status)
	fmt.Fprintf(stdout, "provider: %s\n", payload.Provider)
	fmt.Fprintf(stdout, "previousStatus: %s\n", emptyAsUnknown(payload.PreviousStatus))
	fmt.Fprintf(stdout, "newStatus: %s\n", emptyAsUnknown(payload.NewStatus))
	if payload.Note != "" {
		fmt.Fprintf(stdout, "note: %s\n", payload.Note)
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

func emptyAsUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
