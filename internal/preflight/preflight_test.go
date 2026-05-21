package preflight

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mortenlein/brevity/internal/state"
)

func TestResultJSONStabilityAndSeverity(t *testing.T) {
	result := NewResult(ActionTaskRun, "alpha")
	result.ProviderExecution = true
	result.AddCheck("ok", StatusAllowed, SeverityInfo, "ok")
	result.AddCheck("warn", StatusWarn, SeverityWarn, "careful")
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if !strings.Contains(string(data), `"schema":"brevity.task-preflight.v1"`) || !strings.Contains(string(data), `"status":"warn"`) {
		t.Fatalf("unexpected json: %s", data)
	}
	result.AddCheck("block", StatusBlocked, SeverityError, "blocked")
	if result.Status != StatusBlocked || result.Severity != SeverityError {
		t.Fatalf("status=%s severity=%s, want blocked/error", result.Status, result.Severity)
	}
}

func TestRenderHumanIncludesFlags(t *testing.T) {
	result := NewResult(ActionTaskCleanup, "done")
	result.Destructive = true
	result.RequiresConfirmation = true
	result.ExpectedMutations = []string{"remove completed task worktree"}
	result.AddCheck("cleanup", StatusAllowed, SeverityInfo, "ok")
	var output bytes.Buffer
	if err := RenderHuman(&output, finish(result)); err != nil {
		t.Fatalf("RenderHuman returned error: %v", err)
	}
	text := output.String()
	for _, want := range []string{"Task mutation preflight: task-cleanup", "destructive: true", "requiresConfirmation: true", "Expected mutations"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestValidStartPreflight(t *testing.T) {
	root := fixtureRepo(t, []state.Task{fixtureTask(t, "alpha", "planned", "codex")}, healthyProviders())
	result := runFixture(t, root, ActionTaskStart, "alpha")
	if result.Status != StatusAllowed {
		t.Fatalf("status=%s blockers=%v warnings=%v", result.Status, result.Blockers, result.Warnings)
	}
}

func TestMissingTaskAndInvalidSlug(t *testing.T) {
	root := fixtureRepo(t, []state.Task{fixtureTask(t, "alpha", "planned", "codex")}, healthyProviders())
	if result := runFixture(t, root, ActionTaskStart, "missing"); result.Status != StatusBlocked {
		t.Fatalf("missing status=%s, want blocked", result.Status)
	}
	if result := runFixture(t, root, ActionTaskStart, "../bad"); result.Status != StatusBlocked {
		t.Fatalf("invalid slug status=%s, want blocked", result.Status)
	}
}

func TestProviderUnavailableAndQuotaConstrainedBlockRun(t *testing.T) {
	for name, status := range map[string]state.ProviderStatus{"unavailable": state.StatusUnavailable, "quota": state.StatusQuotaConstrained} {
		t.Run(name, func(t *testing.T) {
			root := fixtureRepo(t, []state.Task{fixtureTask(t, "alpha", "ready-for-worker", "codex")}, map[string]state.ProviderHealth{
				"codex": {Status: status},
			})
			result := runFixture(t, root, ActionTaskRun, "alpha")
			if result.Status != StatusBlocked {
				t.Fatalf("status=%s blockers=%v", result.Status, result.Blockers)
			}
		})
	}
}

func TestProviderUnknownWarnsRun(t *testing.T) {
	root := fixtureRepo(t, []state.Task{fixtureTask(t, "alpha", "ready-for-worker", "codex")}, map[string]state.ProviderHealth{
		"codex": {Status: state.StatusUnknown},
	})
	result := runFixture(t, root, ActionTaskRun, "alpha")
	if result.Status != StatusWarn {
		t.Fatalf("status=%s blockers=%v warnings=%v", result.Status, result.Blockers, result.Warnings)
	}
}

func TestMissingWorktreeBlocksCleanup(t *testing.T) {
	missing := fixtureTask(t, "missing-tree", "completed", "codex")
	missing.WorktreePath = filepath.Join(t.TempDir(), "worktrees", "active", "missing-tree")
	root := fixtureRepo(t, []state.Task{missing}, healthyProviders())
	result := runFixture(t, root, ActionTaskCleanup, "missing-tree")
	if result.Status != StatusBlocked {
		t.Fatalf("missing worktree status=%s, want blocked", result.Status)
	}
}

func TestDirtyCleanupBlocked(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "checkout", "-b", "task/dirty")
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, state.DirectoryName), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := state.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	tasks := []state.Task{{
		Slug:            "dirty",
		Status:          "completed",
		NormalizedState: "completed",
		Branch:          "task/dirty",
		WorktreePath:    root,
		Provider:        "codex",
		Profile:         "default",
	}}
	if err := store.WriteJSON(state.TasksFile, tasks); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteJSON(state.ProviderHealthFile, healthyProviders()); err != nil {
		t.Fatal(err)
	}
	result := runFixture(t, root, ActionTaskCleanup, "dirty")
	if result.Status != StatusBlocked {
		t.Fatalf("status=%s blockers=%v", result.Status, result.Blockers)
	}
}

func TestStateLockBlocks(t *testing.T) {
	root := fixtureRepo(t, []state.Task{fixtureTask(t, "alpha", "planned", "codex")}, healthyProviders())
	if err := os.WriteFile(filepath.Join(root, state.DirectoryName, "state.lock"), []byte("pid=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := runFixture(t, root, ActionTaskStart, "alpha")
	if result.Status != StatusBlocked {
		t.Fatalf("status=%s blockers=%v", result.Status, result.Blockers)
	}
}

func runFixture(t *testing.T, root string, action Action, slug string) Result {
	t.Helper()
	result, err := Run(Options{RepoRoot: root, Action: action, Slug: slug, Now: func() time.Time { return time.Now().UTC() }})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	return result
}

func fixtureRepo(t *testing.T, tasks []state.Task, health state.ProviderHealthState) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, state.DirectoryName), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := state.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteJSON(state.TasksFile, tasks); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteJSON(state.ProviderHealthFile, health); err != nil {
		t.Fatal(err)
	}
	return root
}

func fixtureTask(t *testing.T, slug string, status string, provider string) state.Task {
	t.Helper()
	root := t.TempDir()
	worktree := filepath.Join(root, "worktrees", "active", slug)
	prompt := filepath.Join(worktree, "prompt.md")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prompt, []byte("prompt"), 0o644); err != nil {
		t.Fatal(err)
	}
	return state.Task{
		Slug:            slug,
		Status:          status,
		NormalizedState: status,
		Branch:          "task/" + slug,
		WorktreePath:    worktree,
		PromptPath:      prompt,
		Provider:        provider,
		Profile:         "default",
	}
}

func healthyProviders() state.ProviderHealthState {
	return state.ProviderHealthState{"codex": {Status: state.StatusHealthy}}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}
