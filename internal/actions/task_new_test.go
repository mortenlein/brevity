package actions

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mortenlein/brevity/internal/contracts"
)

func TestRenderTaskNewResult(t *testing.T) {
	result, err := contracts.ParseCommandResult([]byte(`{"schema":"brevity.command-result.v1","command":"task new","success":true,"severity":"info","warnings":[{"message":"minor"}],"suggestedNextActions":["refresh-runtime-state"],"payload":{"slug":"my-task","branch":"task/my-task","worktreePath":"C:\\repo\\worktrees\\active\\brevity-my-task","promptPath":"C:\\repo\\worktrees\\active\\brevity-my-task\\prompt.md","metadataPath":"C:\\repo\\.brevity\\tasks.json"}}`))
	if err != nil {
		t.Fatalf("ParseCommandResult returned error: %v", err)
	}

	var stdout bytes.Buffer
	if err := RenderTaskNewResult(&stdout, result); err != nil {
		t.Fatalf("RenderTaskNewResult returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"Task new: success",
		"slug: my-task",
		"branch: task/my-task",
		"worktreePath: C:\\repo\\worktrees\\active\\brevity-my-task",
		"promptPath: C:\\repo\\worktrees\\active\\brevity-my-task\\prompt.md",
		"metadataPath: C:\\repo\\.brevity\\tasks.json",
		"warning: minor",
		"- refresh-runtime-state",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}
