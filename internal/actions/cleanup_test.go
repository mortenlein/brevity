package actions

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mortenlein/brevity/internal/contracts"
)

func TestRenderTaskCleanupResultIncludesRemovalStateAndActions(t *testing.T) {
	result, err := contracts.ParseCommandResult([]byte(`{"schema":"brevity.command-result.v1","command":"task cleanup","success":true,"severity":"warning","warnings":[{"message":"Runtime state is stale."}],"suggestedNextActions":["refresh-runtime-state"],"payload":{"slug":"my-task","worktreePath":"C:\\repo\\worktrees\\active\\brevity-my-task","branch":"task/my-task","metadataRemoved":true,"branchRemoved":true,"worktreeRemoved":true,"force":true,"cleanupWarnings":["Runtime state is stale."]}}`))
	if err != nil {
		t.Fatalf("ParseCommandResult returned error: %v", err)
	}

	var output bytes.Buffer
	if err := RenderTaskCleanupResult(&output, result); err != nil {
		t.Fatalf("RenderTaskCleanupResult returned error: %v", err)
	}

	for _, want := range []string{
		"Task cleanup: success",
		"slug: my-task",
		"worktreeRemoved: true",
		"branchRemoved: true",
		"metadataRemoved: true",
		"cleanupWarning: Runtime state is stale.",
		"warning: Runtime state is stale.",
		"suggested next actions:",
		"- refresh-runtime-state",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, output.String())
		}
	}
}
