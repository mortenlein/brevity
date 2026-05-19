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
	fmt.Fprintf(stdout, "generatedAt: %s\n", payload.GeneratedAt)
	if payload.NormalizedState != "" {
		fmt.Fprintf(stdout, "normalizedState: %s\n", payload.NormalizedState)
	}

	renderMessages(stdout, result)
	return nil
}
