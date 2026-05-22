package actions

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/state"
)

func TestTaskMergePlanBlocksDirtyWorktree(t *testing.T) {
	repo := newMergeFixture(t)
	fixtureGit(t, repo, "checkout", "-b", "task/alpha")
	writeFile(t, filepath.Join(repo, "task.txt"), "task\n")
	fixtureGit(t, repo, "add", ".")
	fixtureGit(t, repo, "commit", "-m", "task")
	fixtureGit(t, repo, "checkout", "main")
	writeTasks(t, repo, []state.Task{{Slug: "alpha", Status: "completed", NormalizedState: "completed", Branch: "task/alpha", WorktreePath: repo}})
	fixtureGit(t, repo, "add", ".brevity/tasks.json")
	fixtureGit(t, repo, "commit", "-m", "task metadata")
	writeFile(t, filepath.Join(repo, "dirty.txt"), "dirty\n")

	result, err := TaskMergeService{Store: mustStore(t, repo), Now: fixedMergeNow}.Plan("alpha")
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("Plan success = true, want false")
	}
	plan, err := contracts.ParseTaskMergePlanPayload(result)
	if err != nil {
		t.Fatalf("ParseTaskMergePlanPayload returned error: %v", err)
	}
	if !plan.Dirty {
		t.Fatalf("Dirty = false, want true")
	}
	if !hasMessageCode(plan.Blockers, "dirty-worktree") {
		t.Fatalf("blockers = %#v, want dirty-worktree", plan.Blockers)
	}
}

func TestTaskMergeSucceedsAndDoesNotCleanup(t *testing.T) {
	repo := newMergeFixture(t)
	fixtureGit(t, repo, "checkout", "-b", "task/alpha")
	writeFile(t, filepath.Join(repo, "task.txt"), "task\n")
	fixtureGit(t, repo, "add", ".")
	fixtureGit(t, repo, "commit", "-m", "task")
	fixtureGit(t, repo, "checkout", "main")
	writeTasks(t, repo, []state.Task{{Slug: "alpha", Status: "completed", NormalizedState: "completed", Branch: "task/alpha", WorktreePath: repo}})
	fixtureGit(t, repo, "add", ".brevity/tasks.json")
	fixtureGit(t, repo, "commit", "-m", "task metadata")

	result, err := TaskMergeService{Store: mustStore(t, repo), Now: fixedMergeNow}.Merge("alpha")
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Merge success = false: %#v", result.Errors)
	}
	payload, err := contracts.ParseTaskMergePayload(result)
	if err != nil {
		t.Fatalf("ParseTaskMergePayload returned error: %v", err)
	}
	if !payload.MetadataUpdated || payload.CleanupExecuted || payload.BranchRemoved || payload.WorktreeRemoved {
		t.Fatalf("cleanup/metadata flags = %#v", payload)
	}
	if _, err := os.Stat(filepath.Join(repo, "task.txt")); err != nil {
		t.Fatalf("merged file missing: %v", err)
	}
	if fixtureGitOutput(t, repo, "rev-parse", "--verify", "task/alpha") == "" {
		t.Fatalf("task branch was deleted")
	}
	tasks, _, err := state.LoadTasks(mustStore(t, repo))
	if err != nil {
		t.Fatalf("LoadTasks returned error: %v", err)
	}
	if tasks.Items[0].Status != "merged" || tasks.Items[0].NormalizedState != "merged" {
		t.Fatalf("task state = %s/%s, want merged/merged", tasks.Items[0].Status, tasks.Items[0].NormalizedState)
	}
}

func TestTaskMergeConflictDoesNotMarkMerged(t *testing.T) {
	repo := newMergeFixture(t)
	writeFile(t, filepath.Join(repo, "same.txt"), "main\n")
	fixtureGit(t, repo, "add", ".")
	fixtureGit(t, repo, "commit", "-m", "main change")
	fixtureGit(t, repo, "checkout", "-b", "task/alpha", "HEAD~1")
	writeFile(t, filepath.Join(repo, "same.txt"), "task\n")
	fixtureGit(t, repo, "add", ".")
	fixtureGit(t, repo, "commit", "-m", "task change")
	fixtureGit(t, repo, "checkout", "main")
	writeTasks(t, repo, []state.Task{{Slug: "alpha", Status: "completed", NormalizedState: "completed", Branch: "task/alpha", WorktreePath: repo}})
	fixtureGit(t, repo, "add", ".brevity/tasks.json")
	fixtureGit(t, repo, "commit", "-m", "task metadata")

	result, err := TaskMergeService{Store: mustStore(t, repo), Now: fixedMergeNow}.Merge("alpha")
	if err == nil {
		t.Fatalf("Merge returned nil error for conflict")
	}
	payload, parseErr := contracts.ParseTaskMergePayload(result)
	if parseErr != nil {
		t.Fatalf("ParseTaskMergePayload returned error: %v", parseErr)
	}
	if !payload.ConflictDetected {
		t.Fatalf("ConflictDetected = false, want true")
	}
	tasks, _, loadErr := state.LoadTasks(mustStore(t, repo))
	if loadErr != nil {
		t.Fatalf("LoadTasks returned error: %v", loadErr)
	}
	if tasks.Items[0].Status == "merged" {
		t.Fatalf("task was marked merged after conflict")
	}
}

func TestTaskMergePlanJSONStableShape(t *testing.T) {
	repo := newMergeFixture(t)
	writeTasks(t, repo, []state.Task{{Slug: "alpha", Status: "completed", NormalizedState: "completed", Branch: "missing", WorktreePath: repo}})
	fixtureGit(t, repo, "add", ".brevity/tasks.json")
	fixtureGit(t, repo, "commit", "-m", "task metadata")
	result, err := TaskMergeService{Store: mustStore(t, repo), Now: fixedMergeNow}.Plan("alpha")
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(result.Payload, &envelope); err != nil {
		t.Fatalf("payload JSON invalid: %v", err)
	}
	for _, key := range []string{"schema", "version", "slug", "sourceBranch", "targetBranch", "worktreePath", "repoRoot", "dirty", "expectedGitCommands", "expectedStateMutation", "blockers", "warnings", "destructive", "cleanupRequiredAfterMerge", "generatedAt"} {
		if _, ok := envelope[key]; !ok {
			t.Fatalf("payload missing key %s: %#v", key, envelope)
		}
	}
}

func newMergeFixture(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	fixtureGit(t, repo, "init", "-b", "main")
	fixtureGit(t, repo, "config", "user.email", "brevity@example.invalid")
	fixtureGit(t, repo, "config", "user.name", "Brevity Test")
	writeFile(t, filepath.Join(repo, "README.md"), "base\n")
	fixtureGit(t, repo, "add", ".")
	fixtureGit(t, repo, "commit", "-m", "initial")
	return repo
}

func writeTasks(t *testing.T, repo string, tasks []state.Task) {
	t.Helper()
	store := mustStore(t, repo)
	if err := os.MkdirAll(store.BrevityRoot(), 0o755); err != nil {
		t.Fatalf("MkdirAll .brevity: %v", err)
	}
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent tasks: %v", err)
	}
	if err := os.WriteFile(store.Path(state.TasksFile), append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile tasks: %v", err)
	}
}

func writeFile(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

func fixtureGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func fixtureGitOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func mustStore(t *testing.T, repo string) state.Store {
	t.Helper()
	store, err := state.NewStore(repo)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	return store
}

func fixedMergeNow() time.Time {
	return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
}

func hasMessageCode(messages []contracts.ResultMessage, code string) bool {
	for _, message := range messages {
		if message.Code == code {
			return true
		}
	}
	return false
}
