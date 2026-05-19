package runtimeclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
)

func TestNativeRuntimeStateParsesProviderHealth(t *testing.T) {
	repoRoot := nativeTestRepo(t)
	writeNativeTestFile(t, repoRoot, ".brevity/provider-health.json", `{
		"codex":{"status":"unknown","updatedAt":"2026-05-19T10:00:00Z","note":"ok"},
		"gemini":{"status":"capacity-degraded","updatedAt":"2026-05-19T10:01:00Z","note":"busy"},
		"copilot":{"status":"unavailable","updatedAt":"2026-05-19T10:02:00Z","note":"down"}
	}`)
	writeNativeTestFile(t, repoRoot, ".brevity/tasks.json", `[]`)

	state := nativeState(t, repoRoot)
	if state.Providers.Summary.Total != 3 {
		t.Fatalf("provider total = %d, want 3", state.Providers.Summary.Total)
	}
	if state.Providers.Summary.Degraded != 1 {
		t.Fatalf("provider degraded = %d, want 1", state.Providers.Summary.Degraded)
	}
	if state.Providers.Summary.Unavailable != 1 {
		t.Fatalf("provider unavailable = %d, want 1", state.Providers.Summary.Unavailable)
	}
	if state.Providers.Health["gemini"].Note != "busy" {
		t.Fatalf("gemini note = %q, want busy", state.Providers.Health["gemini"].Note)
	}
}

func TestNativeRuntimeStateParsesTasks(t *testing.T) {
	repoRoot := nativeTestRepo(t)
	writeNativeTestFile(t, repoRoot, ".brevity/provider-health.json", `{}`)
	writeNativeTestFile(t, repoRoot, ".brevity/tasks.json", `[
		{"slug":"ready-task","status":"ready-for-worker","normalizedState":"ready-for-worker","provider":"codex","profile":"default"},
		{"slug":"blocked-task","status":"blocked","normalizedState":"blocked"}
	]`)

	state := nativeState(t, repoRoot)
	if state.TaskCounts.Tracked != 2 {
		t.Fatalf("tracked = %d, want 2", state.TaskCounts.Tracked)
	}
	if state.TaskCounts.Runnable != 1 {
		t.Fatalf("runnable = %d, want 1", state.TaskCounts.Runnable)
	}
	if state.TaskCounts.Blocked != 1 {
		t.Fatalf("blocked = %d, want 1", state.TaskCounts.Blocked)
	}
	if len(state.Tasks) != 2 || state.Tasks[0].Slug != "ready-task" {
		t.Fatalf("tasks = %#v, want parsed tasks", state.Tasks)
	}
}

func TestNativeRuntimeStateParsesRunsJSONL(t *testing.T) {
	repoRoot := nativeTestRepo(t)
	writeNativeTestFile(t, repoRoot, ".brevity/provider-health.json", `{}`)
	writeNativeTestFile(t, repoRoot, ".brevity/tasks.json", `[
		{"slug":"my-task","status":"ready-for-worker","normalizedState":"ready-for-worker"}
	]`)
	writeNativeTestFile(t, repoRoot, ".brevity/runs.jsonl",
		"{\"slug\":\"my-task\",\"runId\":\"old\",\"workerStatus\":\"failed\",\"exitCode\":1,\"provider\":\"codex\",\"profile\":\"default\",\"logPath\":\"old.log\"}\n"+
			"{\"slug\":\"my-task\",\"runId\":\"new\",\"workerStatus\":\"succeeded\",\"exitCode\":0,\"provider\":\"gemini\",\"profile\":\"smoke\",\"logPath\":\"new.log\"}\n")

	state := nativeState(t, repoRoot)
	task := state.Tasks[0]
	if task.LatestRunID != "new" {
		t.Fatalf("LatestRunID = %q, want new", task.LatestRunID)
	}
	if task.LatestRunWorkerStatus != "succeeded" {
		t.Fatalf("LatestRunWorkerStatus = %q, want succeeded", task.LatestRunWorkerStatus)
	}
	if task.LatestRunExitCode != float64(0) {
		t.Fatalf("LatestRunExitCode = %#v, want 0", task.LatestRunExitCode)
	}
	if len(task.LatestRun) == 0 {
		t.Fatal("LatestRun is empty")
	}
}

func TestNativeRuntimeStateMissingFiles(t *testing.T) {
	repoRoot := nativeTestRepo(t)

	state := nativeState(t, repoRoot)
	if state.Schema != contracts.RuntimeStateSchema {
		t.Fatalf("schema = %q, want %q", state.Schema, contracts.RuntimeStateSchema)
	}
	if state.Providers.Summary.Total != 0 {
		t.Fatalf("provider total = %d, want 0", state.Providers.Summary.Total)
	}
	if state.TaskCounts.Tracked != 0 {
		t.Fatalf("tracked = %d, want 0", state.TaskCounts.Tracked)
	}
	if len(state.SuggestedNextActions) < 3 {
		t.Fatalf("suggested actions = %#v, want missing-file notes", state.SuggestedNextActions)
	}
}

func nativeState(t *testing.T, repoRoot string) contracts.RuntimeState {
	t.Helper()
	client := NativeClient{
		RepoRoot: repoRoot,
		Now: func() time.Time {
			return time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
		},
	}
	output, err := client.RuntimeStateJSON()
	if err != nil {
		t.Fatalf("RuntimeStateJSON returned error: %v", err)
	}
	var state contracts.RuntimeState
	if err := json.Unmarshal(output, &state); err != nil {
		t.Fatalf("unmarshal runtime state: %v", err)
	}
	return state
}

func nativeTestRepo(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, ".brevity"), 0o755); err != nil {
		t.Fatalf("mkdir .brevity: %v", err)
	}
	return repoRoot
}

func writeNativeTestFile(t *testing.T, repoRoot string, relativePath string, content string) {
	t.Helper()
	path := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
