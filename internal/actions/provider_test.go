package actions

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mortenlein/brevity/internal/contracts"
)

func TestRenderProviderResultIncludesProviderStatusNoteAndActions(t *testing.T) {
	result, err := contracts.ParseCommandResult([]byte(`{"schema":"brevity.command-result.v1","command":"provider set","success":true,"severity":"info","warnings":["minor"],"errors":[],"suggestedNextActions":["refresh-runtime-state"],"payload":{"provider":"gemini","previousStatus":"unknown","newStatus":"capacity-degraded","note":"busy"}}`))
	if err != nil {
		t.Fatalf("ParseCommandResult returned error: %v", err)
	}

	var output bytes.Buffer
	if err := RenderProviderResult(&output, result); err != nil {
		t.Fatalf("RenderProviderResult returned error: %v", err)
	}

	for _, want := range []string{
		"Provider action: success",
		"provider: gemini",
		"previousStatus: unknown",
		"newStatus: capacity-degraded",
		"note: busy",
		"warning: minor",
		"suggested next actions:",
		"- refresh-runtime-state",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, output.String())
		}
	}
}

func TestRenderProviderResultIncludesFailureErrors(t *testing.T) {
	result, err := contracts.ParseCommandResult([]byte(`{"schema":"brevity.command-result.v1","command":"provider reset","success":false,"severity":"error","errors":[{"code":"invalid-provider","message":"Invalid provider: nope"}],"payload":{"provider":"nope","previousStatus":"","newStatus":""}}`))
	if err != nil {
		t.Fatalf("ParseCommandResult returned error: %v", err)
	}

	var output bytes.Buffer
	if err := RenderProviderResult(&output, result); err != nil {
		t.Fatalf("RenderProviderResult returned error: %v", err)
	}

	for _, want := range []string{
		"Provider action: failure",
		"provider: nope",
		"previousStatus: unknown",
		"newStatus: unknown",
		"error: invalid-provider: Invalid provider: nope",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, output.String())
		}
	}
}
