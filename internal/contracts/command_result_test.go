package contracts

import (
	"strings"
	"testing"
)

func TestParseCommandResultAcceptsProviderPayload(t *testing.T) {
	result, err := ParseCommandResult([]byte(`{"schema":"brevity.command-result.v1","command":"provider set","success":true,"severity":"info","warnings":[],"errors":[],"suggestedNextActions":["refresh-runtime-state"],"payload":{"provider":"gemini","previousStatus":"unknown","newStatus":"capacity-degraded","note":"busy"}}`))
	if err != nil {
		t.Fatalf("ParseCommandResult returned error: %v", err)
	}

	if result.Command != "provider set" {
		t.Fatalf("Command = %q, want provider set", result.Command)
	}
	if !result.Success {
		t.Fatal("Success = false, want true")
	}

	payload, err := ParseProviderActionPayload(result)
	if err != nil {
		t.Fatalf("ParseProviderActionPayload returned error: %v", err)
	}
	if payload.Provider != "gemini" {
		t.Fatalf("Provider = %q, want gemini", payload.Provider)
	}
	if payload.NewStatus != "capacity-degraded" {
		t.Fatalf("NewStatus = %q, want capacity-degraded", payload.NewStatus)
	}
}

func TestParseCommandResultAcceptsTaskContextRefreshPayload(t *testing.T) {
	result, err := ParseCommandResult([]byte(`{"schema":"brevity.command-result.v1","command":"task context refresh","success":true,"severity":"info","payload":{"slug":"my-task","refreshed":true,"contextPath":"C:\\repo\\worktrees\\active\\my-task\\.brevity\\context","generatedAt":"2026-05-19T13:00:00Z","normalizedState":"ready-for-worker"}}`))
	if err != nil {
		t.Fatalf("ParseCommandResult returned error: %v", err)
	}

	payload, err := ParseTaskContextRefreshPayload(result)
	if err != nil {
		t.Fatalf("ParseTaskContextRefreshPayload returned error: %v", err)
	}
	if payload.Slug != "my-task" {
		t.Fatalf("Slug = %q, want my-task", payload.Slug)
	}
	if !payload.Refreshed {
		t.Fatal("Refreshed = false, want true")
	}
	if payload.NormalizedState != "ready-for-worker" {
		t.Fatalf("NormalizedState = %q, want ready-for-worker", payload.NormalizedState)
	}
}

func TestParseCommandResultRejectsWrongSchema(t *testing.T) {
	_, err := ParseCommandResult([]byte(`{"schema":"other","command":"provider set","success":true,"severity":"info","payload":{}}`))
	if err == nil {
		t.Fatal("ParseCommandResult returned nil error")
	}
	if !strings.Contains(err.Error(), "unsupported command result schema: other") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResultMessageParsesStringAndObject(t *testing.T) {
	result, err := ParseCommandResult([]byte(`{"schema":"brevity.command-result.v1","command":"provider set","success":false,"severity":"error","warnings":["careful"],"errors":[{"code":"bad","message":"Nope"}],"payload":{}}`))
	if err != nil {
		t.Fatalf("ParseCommandResult returned error: %v", err)
	}

	if got := result.Warnings[0].DisplayText(); got != "careful" {
		t.Fatalf("warning DisplayText = %q, want careful", got)
	}
	if got := result.Errors[0].DisplayText(); got != "bad: Nope" {
		t.Fatalf("error DisplayText = %q, want bad: Nope", got)
	}
}
