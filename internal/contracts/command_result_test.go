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

func TestParseCommandResultAcceptsTaskCleanupPayload(t *testing.T) {
	result, err := ParseCommandResult([]byte(`{"schema":"brevity.command-result.v1","command":"task cleanup","success":true,"severity":"info","payload":{"slug":"my-task","worktreePath":"C:\\repo\\worktrees\\active\\brevity-my-task","branch":"task/my-task","metadataRemoved":true,"branchRemoved":true,"worktreeRemoved":true,"force":true,"cleanupWarnings":["Runtime state is stale."]}}`))
	if err != nil {
		t.Fatalf("ParseCommandResult returned error: %v", err)
	}

	payload, err := ParseTaskCleanupPayload(result)
	if err != nil {
		t.Fatalf("ParseTaskCleanupPayload returned error: %v", err)
	}
	if payload.Slug != "my-task" {
		t.Fatalf("Slug = %q, want my-task", payload.Slug)
	}
	if !payload.WorktreeRemoved || !payload.BranchRemoved || !payload.MetadataRemoved {
		t.Fatalf("removal flags = worktree:%t branch:%t metadata:%t, want all true", payload.WorktreeRemoved, payload.BranchRemoved, payload.MetadataRemoved)
	}
	if len(payload.CleanupWarnings) != 1 || payload.CleanupWarnings[0] != "Runtime state is stale." {
		t.Fatalf("CleanupWarnings = %#v, want stale warning", payload.CleanupWarnings)
	}
}

func TestParseCommandResultAcceptsInspectionPayloads(t *testing.T) {
	runtimeInfo, err := ParseCommandResult([]byte(`{"schema":"brevity.command-result.v1","command":"task runtime-info","success":true,"severity":"info","payload":{"slug":"my-task","status":"ready-for-worker","normalizedState":"ready-for-worker","worktree":{"exists":true,"path":"C:\\repo\\worktrees\\active\\brevity-my-task"},"context":{"materializedFileCount":3,"missingFiles":["runtime.md"]},"execution":{"status":"succeeded","lastRunId":"run-abc"}}}`))
	if err != nil {
		t.Fatalf("ParseCommandResult runtime-info returned error: %v", err)
	}
	runtimePayload, err := ParseTaskRuntimeInfoPayload(runtimeInfo)
	if err != nil {
		t.Fatalf("ParseTaskRuntimeInfoPayload returned error: %v", err)
	}
	if runtimePayload.Context.MaterializedFileCount != 3 || len(runtimePayload.Context.MissingFiles) != 1 {
		t.Fatalf("runtime context = %#v, want 3 materialized and 1 missing", runtimePayload.Context)
	}

	runs, err := ParseCommandResult([]byte(`{"schema":"brevity.command-result.v1","command":"task runs","success":true,"severity":"info","payload":{"slug":"my-task","count":1,"runs":[{"runId":"run-abc","workerStatus":"failed","exitCode":"1","provider":"codex","profile":"default","logPath":"C:\\repo\\run-abc.log"}]}}`))
	if err != nil {
		t.Fatalf("ParseCommandResult task runs returned error: %v", err)
	}
	runsPayload, err := ParseTaskRunsPayload(runs)
	if err != nil {
		t.Fatalf("ParseTaskRunsPayload returned error: %v", err)
	}
	if len(runsPayload.Runs) != 1 || runsPayload.Runs[0].Provider != "codex" {
		t.Fatalf("runs payload = %#v, want codex run", runsPayload)
	}

	doctor, err := ParseCommandResult([]byte(`{"schema":"brevity.command-result.v1","command":"doctor","success":true,"severity":"info","warnings":[{"code":"orphaned-task-worktrees","message":"Orphaned task worktrees are present.","count":2}],"payload":{"warningCount":1,"errorCount":0,"providers":{"summary":{"total":3,"degraded":1,"unavailable":0}},"branchCounts":{"orphaned":4},"worktreeCounts":{"orphanedTaskWorktrees":2},"lock":{"exists":false,"path":"C:\\repo\\.brevity\\tasks.lock"}}}`))
	if err != nil {
		t.Fatalf("ParseCommandResult doctor returned error: %v", err)
	}
	doctorPayload, err := ParseDoctorPayload(doctor)
	if err != nil {
		t.Fatalf("ParseDoctorPayload returned error: %v", err)
	}
	if doctor.Warnings[0].Count != 2 || doctorPayload.WarningCount != 1 || doctorPayload.BranchCounts.Orphaned != 4 {
		t.Fatalf("doctor payload = %#v warnings = %#v, want counts", doctorPayload, doctor.Warnings)
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
