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

	renderStatusLine(stdout, "Task cleanup", result.Success)
	fmt.Fprintf(stdout, "slug: %s\n", payload.Slug)
	fmt.Fprintf(stdout, "worktreeRemoved: %t\n", payload.WorktreeRemoved)
	fmt.Fprintf(stdout, "branchRemoved: %t\n", payload.BranchRemoved)
	fmt.Fprintf(stdout, "metadataRemoved: %t\n", payload.MetadataRemoved)

	for _, warning := range payload.CleanupWarnings {
		fmt.Fprintf(stdout, "cleanupWarning: %s\n", warning)
	}

	renderMessages(stdout, result)
	return nil
}
