package actions

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mortenlein/brevity/internal/contracts"
)

func TestRenderTaskContextRefreshResultIncludesContextAndActions(t *testing.T) {
	result, err := contracts.ParseCommandResult([]byte(`{"schema":"brevity.command-result.v1","command":"task context refresh","success":true,"severity":"info","warnings":["minor"],"suggestedNextActions":["Refresh runtime state."],"payload":{"slug":"my-task","refreshed":true,"contextPath":"C:\\repo\\worktrees\\active\\my-task\\.brevity\\context","generatedAt":"2026-05-19T13:00:00Z","normalizedState":"ready-for-worker"}}`))
	if err != nil {
		t.Fatalf("ParseCommandResult returned error: %v", err)
	}

	var output bytes.Buffer
	if err := RenderTaskContextRefreshResult(&output, result); err != nil {
		t.Fatalf("RenderTaskContextRefreshResult returned error: %v", err)
	}

	for _, want := range []string{
		"Task context refresh: success",
		"slug: my-task",
		"refreshed: true",
		"contextPath: C:\\repo\\worktrees\\active\\my-task\\.brevity\\context",
		"generatedAt: 2026-05-19T13:00:00Z",
		"normalizedState: ready-for-worker",
		"warning: minor",
		"suggested next actions:",
		"- Refresh runtime state.",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, output.String())
		}
	}
}

func TestRenderTaskContextRefreshResultIncludesFailureErrors(t *testing.T) {
	result, err := contracts.ParseCommandResult([]byte(`{"schema":"brevity.command-result.v1","command":"task context refresh","success":false,"severity":"error","errors":[{"code":"missing-task","message":"Task not found: nope"}],"payload":{"slug":"nope","refreshed":false,"contextPath":"","generatedAt":"2026-05-19T13:00:00Z"}}`))
	if err != nil {
		t.Fatalf("ParseCommandResult returned error: %v", err)
	}

	var output bytes.Buffer
	if err := RenderTaskContextRefreshResult(&output, result); err != nil {
		t.Fatalf("RenderTaskContextRefreshResult returned error: %v", err)
	}

	for _, want := range []string{
		"Task context refresh: failure",
		"slug: nope",
		"refreshed: false",
		"generatedAt: 2026-05-19T13:00:00Z",
		"error: missing-task: Task not found: nope",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "normalizedState:") {
		t.Fatalf("output included empty normalizedState:\n%s", output.String())
	}
}
