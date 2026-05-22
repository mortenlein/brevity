package actions

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/state"
)

func TestOrphanCleanupPlanJSONShapeIsStable(t *testing.T) {
	repoRoot, _ := tempOrphanCleanupRepo(t, "stable", false, false)
	service := OrphanCleanupService{Store: state.Store{RepoRoot: repoRoot}, Now: fixedCleanupNow}
	result, err := service.Plan("orphan-worktree:task-stable", false)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"command":"cleanup plan"`,
		`"schema":"brevity.orphan-cleanup-plan-set.v1"`,
		`"schema":"brevity.orphan-cleanup-plan.v1"`,
		`"candidateId":"orphan-worktree:task-stable"`,
		`"destructive":true`,
		`"requiresForce":true`,
		`"generatedAt":"2026-05-22T10:00:00Z"`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("json missing %q:\n%s", want, string(data))
		}
	}
}

func TestOrphanCleanupPlanBlocksDirtyWorktree(t *testing.T) {
	repoRoot, _ := tempOrphanCleanupRepo(t, "dirty-orphan", true, false)
	service := OrphanCleanupService{Store: state.Store{RepoRoot: repoRoot}, Now: fixedCleanupNow}
	result, err := service.Plan("orphan-worktree:task-dirty-orphan", false)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	payload, parseErr := contracts.ParseOrphanCleanupPlanSetPayload(result)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if result.Success || len(payload.Plans) != 1 || !hasResultCode(payload.Plans[0].Blockers, "dirty-worktree") || payload.Plans[0].Removable {
		t.Fatalf("result=%#v payload=%#v", result, payload)
	}
}

func TestOrphanCleanupExecutionRequiresForce(t *testing.T) {
	repoRoot, _ := tempOrphanCleanupRepo(t, "no-force-orphan", false, false)
	service := OrphanCleanupService{Store: state.Store{RepoRoot: repoRoot}, Now: fixedCleanupNow}
	result, err := service.Execute("orphan-worktree:task-no-force-orphan", false, false)
	if err == nil {
		t.Fatal("Execute returned nil error")
	}
	if result.Success || !hasResultCode(result.Errors, "force-required") {
		t.Fatalf("result=%#v, want force-required", result)
	}
}

func TestOrphanCleanupExecutionRemovesCleanWorktreeAndSafeBranch(t *testing.T) {
	repoRoot, worktreePath := tempOrphanCleanupRepo(t, "clean-orphan", false, false)
	service := OrphanCleanupService{Store: state.Store{RepoRoot: repoRoot}, Now: fixedCleanupNow}
	result, err := service.Execute("orphan-worktree:task-clean-orphan", false, true)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	payload, err := contracts.ParseOrphanCleanupExecutionPayload(result)
	if err != nil {
		t.Fatal(err)
	}
	if payload.WorktreeRemoved != 1 || payload.BranchRemoved != 1 || payload.Skipped != 0 {
		t.Fatalf("payload=%#v", payload)
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists: %v", err)
	}
	if exec.Command("git", "-C", repoRoot, "show-ref", "--verify", "--quiet", "refs/heads/task/clean-orphan").Run() == nil {
		t.Fatal("orphan branch still exists")
	}
}

func TestOrphanCleanupExecutionRemovesSafeBranchOnlyOrphan(t *testing.T) {
	repoRoot := tempBaseGitRepo(t)
	runCleanupTestCommand(t, repoRoot, "git", "branch", "task/branch-only")
	service := OrphanCleanupService{Store: state.Store{RepoRoot: repoRoot}, Now: fixedCleanupNow}
	result, err := service.Execute("orphan-branch:task-branch-only", false, true)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	payload, err := contracts.ParseOrphanCleanupExecutionPayload(result)
	if err != nil {
		t.Fatal(err)
	}
	if payload.WorktreeRemoved != 0 || payload.BranchRemoved != 1 {
		t.Fatalf("payload=%#v", payload)
	}
}

func TestOrphanCleanupExecutionBlocksUnmergedBranch(t *testing.T) {
	repoRoot, _ := tempOrphanCleanupRepo(t, "unmerged-orphan", false, true)
	service := OrphanCleanupService{Store: state.Store{RepoRoot: repoRoot}, Now: fixedCleanupNow}
	result, err := service.Execute("orphan-worktree:task-unmerged-orphan", false, true)
	if err == nil {
		t.Fatal("Execute returned nil error")
	}
	payload, parseErr := contracts.ParseOrphanCleanupExecutionPayload(result)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if result.Success || !hasResultCode(result.Errors, "unmerged-branch") || payload.WorktreeRemoved != 0 || payload.BranchRemoved != 0 {
		t.Fatalf("result=%#v payload=%#v", result, payload)
	}
}

func TestOrphanCleanupExecuteAllSkipsBlockedAndCleansSafe(t *testing.T) {
	repoRoot, safePath := tempOrphanCleanupRepo(t, "safe-all", false, false)
	dirtyPath := filepath.Join(repoRoot, "worktrees", "active", "brevity-dirty-all")
	runCleanupTestCommand(t, repoRoot, "git", "worktree", "add", dirtyPath, "-b", "task/dirty-all")
	if err := os.WriteFile(filepath.Join(dirtyPath, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := OrphanCleanupService{Store: state.Store{RepoRoot: repoRoot}, Now: fixedCleanupNow}
	result, err := service.Execute("", true, true)
	if err != nil {
		t.Fatalf("Execute all returned error: %v", err)
	}
	payload, parseErr := contracts.ParseOrphanCleanupExecutionPayload(result)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if payload.WorktreeRemoved != 1 || payload.BranchRemoved != 1 || payload.Skipped != 1 {
		t.Fatalf("payload=%#v", payload)
	}
	if _, err := os.Stat(safePath); !os.IsNotExist(err) {
		t.Fatalf("safe worktree still exists: %v", err)
	}
	if _, err := os.Stat(dirtyPath); err != nil {
		t.Fatalf("dirty worktree was removed or inaccessible: %v", err)
	}
}

func tempOrphanCleanupRepo(t *testing.T, slug string, dirty bool, unmerged bool) (string, string) {
	t.Helper()
	repoRoot := tempBaseGitRepo(t)
	worktreePath := filepath.Join(repoRoot, "worktrees", "active", "brevity-"+slug)
	runCleanupTestCommand(t, repoRoot, "git", "worktree", "add", worktreePath, "-b", "task/"+slug)
	if unmerged {
		if err := os.WriteFile(filepath.Join(worktreePath, "change.txt"), []byte("change\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runCleanupTestCommand(t, worktreePath, "git", "add", "change.txt")
		runCleanupTestCommand(t, worktreePath, "git", "commit", "-m", "task change")
	}
	if dirty {
		if err := os.WriteFile(filepath.Join(worktreePath, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return repoRoot, worktreePath
}

func tempBaseGitRepo(t *testing.T) string {
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
	if err := os.MkdirAll(filepath.Join(repoRoot, ".brevity"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".brevity", "tasks.json"), []byte("[]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repoRoot
}
