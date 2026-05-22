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

	renderStatusLine(stdout, "Task context refresh", result.Success)
	fmt.Fprintf(stdout, "slug: %s\n", payload.Slug)
	fmt.Fprintf(stdout, "refreshed: %t\n", payload.Refreshed)
	fmt.Fprintf(stdout, "contextPath: %s\n", payload.ContextPath)
	if payload.PromptPath != "" {
		fmt.Fprintf(stdout, "promptPath: %s\n", payload.PromptPath)
	}
	if payload.SpecPath != "" {
		fmt.Fprintf(stdout, "specPath: %s\n", payload.SpecPath)
	}
	fmt.Fprintf(stdout, "generatedAt: %s\n", payload.GeneratedAt)
	if payload.NormalizedState != "" {
		fmt.Fprintf(stdout, "normalizedState: %s\n", payload.NormalizedState)
	}
	if payload.PromptRefreshStatus != "" {
		fmt.Fprintf(stdout, "promptRefreshStatus: %s\n", payload.PromptRefreshStatus)
	}

	renderMessages(stdout, result)
	return nil
}
