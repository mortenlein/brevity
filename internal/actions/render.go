package actions

import (
	"fmt"
	"io"

	"github.com/mortenlein/brevity/internal/contracts"
)

func renderStatusLine(stdout io.Writer, label string, success bool) {
	status := "failure"
	if success {
		status = "success"
	}

	fmt.Fprintf(stdout, "%s: %s\n", label, status)
}

func renderSeverityLine(stdout io.Writer, severity string) {
	if severity == "" {
		return
	}

	fmt.Fprintf(stdout, "severity: %s\n", severity)
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
