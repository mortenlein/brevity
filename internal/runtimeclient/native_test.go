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

func TestParseGitWorktreePorcelain(t *testing.T) {
	output := "worktree C:/repo\nHEAD abc123\nbranch refs/heads/main\n\nworktree C:/repo/worktrees/active/task-one\nHEAD def456\nbranch refs/heads/task/one\n\n"

	records := parseGitWorktreePorcelain(output)
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	if records[0].Path != "C:/repo" || records[0].Branch != "main" || records[0].Head != "abc123" {
		t.Fatalf("first record = %#v, want parsed main worktree", records[0])
	}
	if records[1].Path != "C:/repo/worktrees/active/task-one" || records[1].Branch != "task/one" || records[1].Head != "def456" {
		t.Fatalf("second record = %#v, want parsed task worktree", records[1])
	}
}

func TestOrphanedTaskWorktreesDetectsTaskBranchesWithoutMetadata(t *testing.T) {
	worktrees := []contracts.WorktreeRecord{
		{Path: "C:/repo", Branch: "main"},
		{Path: "C:/repo/worktrees/active/lost", Branch: "task/lost"},
	}

	orphaned := orphanedTaskWorktrees(nil, worktrees)
	if len(orphaned) != 1 {
		t.Fatalf("orphaned = %#v, want one task worktree", orphaned)
	}
	if orphaned[0].Branch != "task/lost" {
		t.Fatalf("orphaned branch = %q, want task/lost", orphaned[0].Branch)
	}
}

func TestOrphanedTaskWorktreesTreatsMissingTaskMetadataAsOrphan(t *testing.T) {
	repoRoot := nativeTestRepo(t)
	writeNativeTestFile(t, repoRoot, ".brevity/provider-health.json", `{}`)
	writeNativeTestFile(t, repoRoot, ".brevity/tasks.json", `[]`)
	stateTasks, _, err := readTasks(filepath.Join(repoRoot, ".brevity", "tasks.json"))
	if err != nil {
		t.Fatalf("readTasks returned error: %v", err)
	}

	orphaned := orphanedTaskWorktrees(stateTasks, []contracts.WorktreeRecord{
		{Path: filepath.Join(repoRoot, "worktrees", "active", "missing"), Branch: "task/missing"},
	})
	if len(orphaned) != 1 {
		t.Fatalf("orphaned = %#v, want missing metadata worktree", orphaned)
	}
}

func TestOrphanedTaskWorktreesIgnoresMatchingMetadata(t *testing.T) {
	worktreePath := "C:/repo/worktrees/active/known"
	tasks := []contracts.TaskSummary{
		{Slug: "known", Branch: "task/known", Worktree: &contracts.TaskWorktree{Path: worktreePath, Branch: "task/known"}},
	}

	orphaned := orphanedTaskWorktrees(tasks, []contracts.WorktreeRecord{
		{Path: worktreePath, Branch: "task/known"},
		{Path: "C:/repo/worktrees/active/other", Branch: "task/other"},
	})
	if len(orphaned) != 1 {
		t.Fatalf("orphaned = %#v, want only unmatched task worktree", orphaned)
	}
	if orphaned[0].Branch != "task/other" {
		t.Fatalf("orphaned branch = %q, want task/other", orphaned[0].Branch)
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
