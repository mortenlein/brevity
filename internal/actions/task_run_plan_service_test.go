package actions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/state"
)

func TestTaskRunPlanServiceBuildsNativeEnvelope(t *testing.T) {
	store := taskRunPlanStore(t, "codex", state.StatusHealthy)
	service := TaskRunPlanService{
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC) },
		RunID: func(slug string, now time.Time) string { return "run-fixed" },
	}
	result, err := service.Plan("alpha", "codex-default")
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if !result.Success || result.Command != "task run" || result.Severity != "warning" {
		t.Fatalf("result = %#v", result)
	}
	payload, err := contracts.ParseTaskRunPlanPayload(result)
	if err != nil {
		t.Fatalf("ParseTaskRunPlanPayload returned error: %v", err)
	}
	if payload.Schema != "brevity.task-run-plan.v1" || payload.Authority != "native-go" || !payload.DryRunOnly || !payload.NoExecutionOccurred {
		t.Fatalf("payload contract fields = %#v", payload)
	}
	if payload.Provider != "codex" || payload.Profile != "codex-balanced" || payload.RunIDPlan != "run-fixed" {
		t.Fatalf("resolution fields = %#v", payload)
	}
	if payload.WorkerCommand.Command != "codex" || !contains(payload.WorkerCommand.Arguments, payload.PromptPath) {
		t.Fatalf("worker command = %#v, prompt=%q", payload.WorkerCommand, payload.PromptPath)
	}
	if len(payload.ExpectedFilesWritten) == 0 || !strings.Contains(payload.ExpectedFilesWritten[0], "runs.jsonl") {
		t.Fatalf("expected files = %#v", payload.ExpectedFilesWritten)
	}
	if _, err := os.Stat(filepath.Join(store.BrevityRoot(), "runs.jsonl")); !os.IsNotExist(err) {
		t.Fatal("Plan wrote runs.jsonl")
	}
}

func TestTaskRunPlanServiceBlocksUnavailableProviderAndMissingPrompt(t *testing.T) {
	store := taskRunPlanStore(t, "gemini", state.StatusUnavailable)
	if err := os.Remove(filepath.Join(store.RepoRoot, "worktree", "prompt.md")); err != nil {
		t.Fatal(err)
	}
	result, err := TaskRunPlanService{Store: store}.Plan("alpha", "")
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if result.Success || result.Severity != "error" {
		t.Fatalf("result = %#v", result)
	}
	payload, err := contracts.ParseTaskRunPlanPayload(result)
	if err != nil {
		t.Fatalf("ParseTaskRunPlanPayload returned error: %v", err)
	}
	if !messageCode(payload.Blockers, "provider-unavailable") || !messageCode(payload.Blockers, "prompt-missing") {
		t.Fatalf("blockers = %#v", payload.Blockers)
	}
}

func TestTaskRunPlanServiceJSONStableGeneratedFields(t *testing.T) {
	store := taskRunPlanStore(t, "codex", state.StatusHealthy)
	service := TaskRunPlanService{
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 5, 21, 12, 30, 0, 0, time.UTC) },
		RunID: func(slug string, now time.Time) string { return "run-json" },
	}
	result, err := service.Plan("alpha", "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"schema":"brevity.command-result.v1"`, `"schema":"brevity.task-run-plan.v1"`, `"generatedAt":"2026-05-21T12:30:00Z"`, `"runIdPlan":"run-json"`, `"authority":"native-go"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("JSON missing %q:\n%s", want, data)
		}
	}
}

func taskRunPlanStore(t *testing.T, provider string, status state.ProviderStatus) state.Store {
	t.Helper()
	root := t.TempDir()
	store, err := state.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(root, "worktree")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.BrevityRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	promptPath := filepath.Join(worktree, "prompt.md")
	writeActionTestFile(t, promptPath, "Do the thing.\n")
	writeActionTestFile(t, store.Path("config.json"), `{"defaultProvider":"`+provider+`","providers":{"codex":{"command":"codex","mode":"exec","sandbox":"workspace-write"},"gemini":{"command":"gemini","approvalMode":"yolo","skipTrust":true}}}`+"\n")
	writeActionTestFile(t, store.Path(state.ProviderHealthFile), `{"`+provider+`":{"status":"`+string(status)+`","note":"test"}}`+"\n")
	tasks := `[{"slug":"alpha","status":"ready-for-worker","normalizedState":"ready-for-worker","branch":"task/alpha","worktreePath":` + quoteJSON(worktree) + `,"promptPath":` + quoteJSON(promptPath) + `,"provider":"` + provider + `","promptRefreshedAt":"2026-05-21T12:00:00Z"}]`
	writeActionTestFile(t, store.Path(state.TasksFile), tasks+"\n")
	return store
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func messageCode(messages []contracts.ResultMessage, code string) bool {
	for _, message := range messages {
		if message.Code == code {
			return true
		}
	}
	return false
}
