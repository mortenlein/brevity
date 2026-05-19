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

	renderStatusLine(stdout, "Provider action", result.Success)
	fmt.Fprintf(stdout, "provider: %s\n", payload.Provider)
	fmt.Fprintf(stdout, "previousStatus: %s\n", emptyAsUnknown(payload.PreviousStatus))
	fmt.Fprintf(stdout, "newStatus: %s\n", emptyAsUnknown(payload.NewStatus))
	if payload.Note != "" {
		fmt.Fprintf(stdout, "note: %s\n", payload.Note)
	}

	renderMessages(stdout, result)
	return nil
}

func emptyAsUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
