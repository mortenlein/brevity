package actions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/state"
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

func TestTaskCleanupPlanBlocksDirtyWorktree(t *testing.T) {
	repoRoot, worktreePath := tempCleanupRepo(t, "dirty")
	if err := os.WriteFile(filepath.Join(worktreePath, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	service := TaskCleanupService{Store: state.Store{RepoRoot: repoRoot}, Now: fixedCleanupNow}
	result, err := service.Plan("dirty")
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if result.Success {
		t.Fatal("plan succeeded, want dirty blocker")
	}
	plan, err := contracts.ParseTaskCleanupPlanPayload(result)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Dirty || !hasResultCode(plan.Blockers, "dirty-worktree") {
		t.Fatalf("plan did not block dirty worktree: %#v", plan)
	}
	if !plan.Destructive || !plan.RequiresForce {
		t.Fatalf("plan safety flags = destructive %t force %t", plan.Destructive, plan.RequiresForce)
	}
}

func TestTaskCleanupPlanJSONShapeIsStable(t *testing.T) {
	repoRoot, _ := tempCleanupRepo(t, "stable")
	service := TaskCleanupService{Store: state.Store{RepoRoot: repoRoot}, Now: fixedCleanupNow}
	result, err := service.Plan("stable")
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"schema":"brevity.command-result.v1"`,
		`"command":"task cleanup --plan"`,
		`"schema":"brevity.task-cleanup-plan.v1"`,
		`"slug":"stable"`,
		`"destructive":true`,
		`"requiresForce":true`,
		`"generatedAt":"2026-05-22T10:00:00Z"`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("json missing %q:\n%s", want, string(data))
		}
	}
}

func TestTaskCleanupExecutionRequiresForce(t *testing.T) {
	repoRoot, _ := tempCleanupRepo(t, "no-force")
	service := TaskCleanupService{Store: state.Store{RepoRoot: repoRoot}, Now: fixedCleanupNow}
	result, err := service.Cleanup("no-force", false)
	if err == nil {
		t.Fatal("Cleanup returned nil error")
	}
	if result.Success || !hasResultCode(result.Errors, "force-required") {
		t.Fatalf("result = %#v, want force-required error", result)
	}
}

func TestTaskCleanupExecutionRemovesWorktreeBranchAndMetadata(t *testing.T) {
	repoRoot, worktreePath := tempCleanupRepo(t, "done")
	service := TaskCleanupService{Store: state.Store{RepoRoot: repoRoot}, Now: fixedCleanupNow}
	result, err := service.Cleanup("done", true)
	if err != nil {
		t.Fatalf("Cleanup returned error: %v", err)
	}
	payload, err := contracts.ParseTaskCleanupPayload(result)
	if err != nil {
		t.Fatal(err)
	}
	if !payload.WorktreeRemoved || !payload.BranchRemoved || !payload.MetadataRemoved {
		t.Fatalf("payload = %#v", payload)
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists: %v", err)
	}
	if exec.Command("git", "-C", repoRoot, "show-ref", "--verify", "--quiet", "refs/heads/task/done").Run() == nil {
		t.Fatal("task branch still exists")
	}
	tasks, _, err := state.LoadTasks(state.Store{RepoRoot: repoRoot})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks.Items) != 0 {
		t.Fatalf("tasks = %#v, want empty after cleanup", tasks.Items)
	}
}

func tempCleanupRepo(t *testing.T, slug string) (string, string) {
	t.Helper()
	repoRoot := t.TempDir()
	runCleanupTestCommand(t, repoRoot, "git", "init")
	runCleanupTestCommand(t, repoRoot, "git", "config", "user.email", "brevity@example.test")
	runCleanupTestCommand(t, repoRoot, "git", "config", "user.name", "Brevity Test")
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCleanupTestCommand(t, repoRoot, "git", "add", "README.md")
	runCleanupTestCommand(t, repoRoot, "git", "commit", "-m", "initial")
	worktreePath := filepath.Join(repoRoot, "worktrees", "active", "brevity-"+slug)
	branch := "task/" + slug
	runCleanupTestCommand(t, repoRoot, "git", "worktree", "add", worktreePath, "-b", branch)
	brevityRoot := filepath.Join(repoRoot, ".brevity")
	if err := os.MkdirAll(brevityRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	tasks := fmt.Sprintf(`[{"slug":%q,"status":"merged","normalizedState":"merged","branch":%q,"worktreePath":%q}]`, slug, branch, worktreePath)
	if err := os.WriteFile(filepath.Join(brevityRoot, "tasks.json"), []byte(tasks+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repoRoot, worktreePath
}

func runCleanupTestCommand(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	command := exec.Command(name, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, string(output))
	}
}

func fixedCleanupNow() time.Time {
	return time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
}

func hasResultCode(messages []contracts.ResultMessage, code string) bool {
	for _, message := range messages {
		if message.Code == code {
			return true
		}
	}
	return false
}
