package runtimeclient

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/state"
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

func TestNativeRuntimeStateSemanticParityWithPowerShellReference(t *testing.T) {
	shell, err := exec.LookPath("pwsh")
	if err != nil {
		shell, err = exec.LookPath("powershell")
	}
	if err != nil {
		t.Skip("PowerShell unavailable")
	}

	repoRoot := nativeTestRepo(t)
	gitInit := exec.Command("git", "init")
	gitInit.Dir = repoRoot
	if output, err := gitInit.CombinedOutput(); err != nil {
		t.Skipf("git init unavailable for parity fixture: %v %s", err, output)
	}
	writeNativeTestFile(t, repoRoot, ".brevity/provider-health.json", `{
		"codex":{"status":"healthy","updatedAt":"2026-05-19T10:00:00Z","note":"ok"},
		"gemini":{"status":"capacity-degraded","updatedAt":"2026-05-19T10:01:00Z","note":"busy"}
	}`)
	writeNativeTestFile(t, repoRoot, ".brevity/tasks.json", `[
		{"slug":"ready-task","status":"ready-for-worker","normalizedState":"ready-for-worker","provider":"codex","profile":"default"},
		{"slug":"blocked-task","status":"blocked","normalizedState":"blocked"}
	]`)

	native := nativeState(t, repoRoot)
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("source root: %v", err)
	}
	command := exec.Command(shell, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(sourceRoot, "brevity.ps1"), "runtime", "state", "--json")
	command.Dir = repoRoot
	output, err := command.Output()
	if err != nil {
		t.Skipf("PowerShell reference runtime state unavailable for fixture: %v", err)
	}
	var reference contracts.RuntimeState
	if err := json.Unmarshal(output, &reference); err != nil {
		t.Skipf("PowerShell reference emitted unparsable JSON for fixture: %v", err)
	}

	if native.TaskCounts.Tracked != reference.TaskCounts.Tracked {
		t.Fatalf("tracked tasks native=%d reference=%d", native.TaskCounts.Tracked, reference.TaskCounts.Tracked)
	}
	if native.Providers.Summary.Total != reference.Providers.Summary.Total {
		t.Fatalf("provider total native=%d reference=%d", native.Providers.Summary.Total, reference.Providers.Summary.Total)
	}
	if native.Providers.Summary.Degraded != reference.Providers.Summary.Degraded {
		t.Fatalf("provider degraded native=%d reference=%d", native.Providers.Summary.Degraded, reference.Providers.Summary.Degraded)
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
	if len(state.Tasks) != 2 || state.Tasks[0].Slug != "blocked-task" || state.Tasks[1].Slug != "ready-task" {
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
		"{\"slug\":\"my-task\",\"runId\":\"old\",\"workerStatus\":\"failed\",\"startedAt\":\"2026-05-19T08:00:00Z\",\"finishedAt\":\"2026-05-19T08:01:00Z\",\"exitCode\":1,\"provider\":\"codex\",\"profile\":\"default\",\"logPath\":\"old.log\"}\n"+
			"{\"slug\":\"my-task\",\"runId\":\"new\",\"workerStatus\":\"succeeded\",\"startedAt\":\"2026-05-19T09:00:00Z\",\"finishedAt\":\"2026-05-19T09:01:00Z\",\"exitCode\":0,\"provider\":\"gemini\",\"profile\":\"smoke\",\"logPath\":\"new.log\"}\n")

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
	if task.RunCount != 2 || task.LatestRunFinishedAt != "2026-05-19T09:01:00Z" || task.LatestRunSource != "index" {
		t.Fatalf("run enrichment = %#v, want count, finishedAt, and source", task)
	}
}

func TestNativeTaskRunsReturnsNewestFirst(t *testing.T) {
	repoRoot := nativeTestRepo(t)
	writeNativeTestFile(t, repoRoot, ".brevity/provider-health.json", `{}`)
	writeNativeTestFile(t, repoRoot, ".brevity/tasks.json", `[{"slug":"my-task","status":"ready-for-worker"}]`)
	writeNativeTestFile(t, repoRoot, ".brevity/runs.jsonl",
		"{\"slug\":\"my-task\",\"runId\":\"old\",\"finishedAt\":\"2026-05-19T08:01:00Z\",\"workerStatus\":\"failed\",\"exitCode\":1}\n"+
			"{\"slug\":\"my-task\",\"runId\":\"new\",\"finishedAt\":\"2026-05-19T09:01:00Z\",\"workerStatus\":\"succeeded\",\"exitCode\":0}\n")

	result := nativeCommandResult(t, repoRoot, func(client NativeClient) ([]byte, error) {
		return client.TaskRunsJSON("my-task")
	})
	payload, err := contracts.ParseTaskRunsPayload(result)
	if err != nil {
		t.Fatalf("ParseTaskRunsPayload returned error: %v", err)
	}
	if payload.Count != 2 || len(payload.Runs) != 2 || payload.Runs[0].RunID != "new" || payload.Runs[1].RunID != "old" {
		t.Fatalf("payload = %#v, want newest-first runs", payload)
	}
}

func TestNativeTaskRunsHandlesNoRuns(t *testing.T) {
	repoRoot := nativeTestRepo(t)
	writeNativeTestFile(t, repoRoot, ".brevity/provider-health.json", `{}`)
	writeNativeTestFile(t, repoRoot, ".brevity/tasks.json", `[{"slug":"my-task","status":"ready-for-worker"}]`)

	result := nativeCommandResult(t, repoRoot, func(client NativeClient) ([]byte, error) {
		return client.TaskRunsJSON("my-task")
	})
	payload, err := contracts.ParseTaskRunsPayload(result)
	if err != nil {
		t.Fatalf("ParseTaskRunsPayload returned error: %v", err)
	}
	if payload.Count != 0 || len(payload.Runs) != 0 {
		t.Fatalf("payload = %#v, want empty runs", payload)
	}
}

func TestNativeTaskRunsMissingTaskReturnsStructuredError(t *testing.T) {
	repoRoot := nativeTestRepo(t)
	writeNativeTestFile(t, repoRoot, ".brevity/provider-health.json", `{}`)
	writeNativeTestFile(t, repoRoot, ".brevity/tasks.json", `[]`)

	result := nativeCommandResult(t, repoRoot, func(client NativeClient) ([]byte, error) {
		return client.TaskRunsJSON("missing")
	})
	if result.Success || len(result.Errors) != 1 || result.Errors[0].Code != "task-not-found" {
		t.Fatalf("result = %#v, want structured task-not-found", result)
	}
}

func TestNativeTaskRunsMalformedHistoryReturnsError(t *testing.T) {
	repoRoot := nativeTestRepo(t)
	writeNativeTestFile(t, repoRoot, ".brevity/provider-health.json", `{}`)
	writeNativeTestFile(t, repoRoot, ".brevity/tasks.json", `[{"slug":"my-task","status":"ready-for-worker"}]`)
	writeNativeTestFile(t, repoRoot, ".brevity/runs.jsonl", "{not-json}\n")

	client := NativeClient{RepoRoot: repoRoot}
	_, err := client.TaskRunsJSON("my-task")
	if err == nil {
		t.Fatal("TaskRunsJSON returned nil error")
	}
}

func TestNativeTaskRuntimeInfoReportsLatestRun(t *testing.T) {
	repoRoot := nativeTestRepo(t)
	writeNativeTestFile(t, repoRoot, ".brevity/provider-health.json", `{}`)
	writeNativeTestFile(t, repoRoot, ".brevity/tasks.json", `[{"slug":"my-task","status":"ready-for-worker","normalizedState":"ready-for-worker","provider":"codex","profile":"default"}]`)
	writeNativeTestFile(t, repoRoot, ".brevity/runs.jsonl", "{\"slug\":\"my-task\",\"runId\":\"run-1\",\"workerStatus\":\"failed\",\"startedAt\":\"2026-05-19T09:00:00Z\",\"finishedAt\":\"2026-05-19T09:01:00Z\",\"exitCode\":1,\"failureType\":\"worker-exit-failed\",\"logPath\":\"run-1.log\"}\n")

	result := nativeCommandResult(t, repoRoot, func(client NativeClient) ([]byte, error) {
		return client.TaskRuntimeInfoJSON("my-task")
	})
	payload, err := contracts.ParseTaskRuntimeInfoPayload(result)
	if err != nil {
		t.Fatalf("ParseTaskRuntimeInfoPayload returned error: %v", err)
	}
	if payload.RunCount != 1 || payload.Execution.LastRunID != "run-1" || payload.Execution.Status != "failed" || payload.LogPath != "run-1.log" {
		t.Fatalf("payload = %#v, want latest failed run", payload)
	}
}

func TestNativeTaskRuntimeInfoReportsStaleIncompleteRun(t *testing.T) {
	repoRoot := nativeTestRepo(t)
	writeNativeTestFile(t, repoRoot, ".brevity/provider-health.json", `{}`)
	writeNativeTestFile(t, repoRoot, ".brevity/tasks.json", `[{"slug":"my-task","status":"ready-for-worker"}]`)
	writeNativeTestFile(t, repoRoot, ".brevity/runs.jsonl", "{\"slug\":\"my-task\",\"runId\":\"stale\",\"workerStatus\":\"running\",\"startedAt\":\"2026-05-19T09:00:00Z\"}\n")

	result := nativeCommandResult(t, repoRoot, func(client NativeClient) ([]byte, error) {
		return client.TaskRuntimeInfoJSON("my-task")
	})
	payload, err := contracts.ParseTaskRuntimeInfoPayload(result)
	if err != nil {
		t.Fatalf("ParseTaskRuntimeInfoPayload returned error: %v", err)
	}
	if !payload.Stale || !payload.Incomplete || payload.Execution.Status != "stale" {
		t.Fatalf("payload = %#v, want stale incomplete latest run", payload)
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

func TestParseGitBranchOutput(t *testing.T) {
	output := "main\n task/alpha \n* task/beta\n\nmain\n"

	branches := parseGitBranchOutput(output)
	if len(branches) != 3 {
		t.Fatalf("branches = %#v, want three unique branches", branches)
	}
	if branches[0] != "main" || branches[1] != "task/alpha" || branches[2] != "task/beta" {
		t.Fatalf("branches = %#v, want sorted parsed branch names", branches)
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
	store, err := state.NewStore(repoRoot)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	taskStore, _, err := state.LoadTasks(store)
	if err != nil {
		t.Fatalf("LoadTasks returned error: %v", err)
	}
	stateTasks := taskStore.ToContracts()

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

func TestOrphanedTaskBranchesExcludesCheckedOutBranches(t *testing.T) {
	worktrees := []contracts.WorktreeRecord{
		{Path: "C:/repo/worktrees/active/checked-out", Branch: "task/checked-out"},
	}
	branches := []string{"main", "task/checked-out", "task/lost"}

	orphaned := orphanedTaskBranches(nil, worktrees, branches)
	if len(orphaned) != 1 {
		t.Fatalf("orphaned = %#v, want one branch", orphaned)
	}
	if orphaned[0].Branch != "task/lost" {
		t.Fatalf("orphaned branch = %q, want task/lost", orphaned[0].Branch)
	}
}

func TestOrphanedTaskBranchesExcludesMatchingTaskMetadata(t *testing.T) {
	tasks := []contracts.TaskSummary{
		{Slug: "known", Branch: "task/known"},
		{Slug: "nested", Worktree: &contracts.TaskWorktree{Branch: "task/nested"}},
	}
	branches := []string{"task/known", "task/nested", "task/lost", "feature/other"}

	orphaned := orphanedTaskBranches(tasks, nil, branches)
	if len(orphaned) != 1 {
		t.Fatalf("orphaned = %#v, want one branch", orphaned)
	}
	if orphaned[0].Branch != "task/lost" {
		t.Fatalf("orphaned branch = %q, want task/lost", orphaned[0].Branch)
	}
}

func TestCleanupCandidatesForOrphanedTaskBranches(t *testing.T) {
	candidates := cleanupCandidatesForOrphanedTaskBranches([]contracts.WorktreeRecord{
		{Branch: "task/lost"},
	})
	if len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want one candidate", candidates)
	}
	candidate := candidates[0]
	if candidate.ID != "orphan-branch:task-lost" {
		t.Fatalf("candidate ID = %q, want orphan-branch:task-lost", candidate.ID)
	}
	if candidate.Category != "destructive-if-removed" || candidate.Severity != "warning" {
		t.Fatalf("candidate classification = %s/%s, want warning/destructive-if-removed", candidate.Severity, candidate.Category)
	}
	if candidate.DestructiveIfUnmerged == nil || !*candidate.DestructiveIfUnmerged {
		t.Fatalf("DestructiveIfUnmerged = %#v, want true", candidate.DestructiveIfUnmerged)
	}
	if len(candidate.SuggestedCommands) != 1 || candidate.SuggestedCommands[0] != "git branch -D task/lost" {
		t.Fatalf("SuggestedCommands = %#v, want branch deletion guidance", candidate.SuggestedCommands)
	}
}

func TestCleanupSummaryIncludesOrphanedTaskBranches(t *testing.T) {
	worktreeCandidates := cleanupCandidatesForOrphanedTaskWorktrees([]contracts.WorktreeRecord{
		{Path: "C:/repo/worktrees/active/lost", Branch: "task/lost-worktree"},
	})
	branchCandidates := cleanupCandidatesForOrphanedTaskBranches([]contracts.WorktreeRecord{
		{Branch: "task/lost-branch"},
	})

	summary := cleanupSummary(worktreeCandidates, branchCandidates)
	if summary.TotalCandidates != 2 {
		t.Fatalf("TotalCandidates = %d, want 2", summary.TotalCandidates)
	}
	if summary.OrphanedTaskWorktreeCount != 1 || summary.OrphanedTaskBranchCount != 1 {
		t.Fatalf("orphan counts = worktrees %d branches %d, want 1/1", summary.OrphanedTaskWorktreeCount, summary.OrphanedTaskBranchCount)
	}
	if summary.ByCategory["destructive-if-removed"] != 1 {
		t.Fatalf("ByCategory = %#v, want one destructive-if-removed", summary.ByCategory)
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

func nativeCommandResult(t *testing.T, repoRoot string, call func(NativeClient) ([]byte, error)) contracts.CommandResult {
	t.Helper()
	client := NativeClient{
		RepoRoot: repoRoot,
		Now: func() time.Time {
			return time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
		},
	}
	output, err := call(client)
	if err != nil {
		t.Fatalf("native command returned error: %v", err)
	}
	result, err := contracts.ParseCommandResult(output)
	if err != nil {
		t.Fatalf("ParseCommandResult returned error: %v", err)
	}
	return result
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
