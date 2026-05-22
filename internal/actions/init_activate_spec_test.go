package actions

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/state"
)

func TestInitRepairActivateSpecFixture(t *testing.T) {
	repo := newGitFixture(t)
	store := state.Store{RepoRoot: repo}

	initResult, err := InitService{Store: store}.Run()
	if err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	if !initResult.Success {
		t.Fatalf("init success=false: %+v", initResult.Errors)
	}
	config, missing, err := state.LoadConfig(store)
	if err != nil || missing {
		t.Fatalf("LoadConfig err=%v missing=%v", err, missing)
	}
	if config.DefaultProvider != "gemini" {
		t.Fatalf("default provider = %q, want gemini", config.DefaultProvider)
	}

	config.DefaultProvider = "codex"
	config.WorktreesRoot = filepath.Join(filepath.Dir(repo), "custom-worktrees")
	if err := store.WriteJSON(state.ConfigFile, config); err != nil {
		t.Fatal(err)
	}
	repairResult, err := InitService{Store: store, Repair: true}.Run()
	if err != nil {
		t.Fatalf("repair returned error: %v", err)
	}
	repaired, _, err := state.LoadConfig(store)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.DefaultProvider != "codex" {
		t.Fatalf("repair overwrote custom default provider: %q", repaired.DefaultProvider)
	}
	if repaired.WorktreesRoot != config.WorktreesRoot {
		t.Fatalf("repair overwrote custom worktreesRoot: %q", repaired.WorktreesRoot)
	}
	if !repairResult.Success {
		t.Fatalf("repair success=false: %+v", repairResult.Errors)
	}

	specPath := filepath.Join(repaired.VaultPath, "tasks", "fixture-slug.md")
	if err := os.WriteFile(specPath, []byte("# Fixture\n\nDo work.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	activateResult, err := TaskActivateService{Store: store}.Activate("fixture-slug")
	if err != nil {
		t.Fatalf("activate returned error: %v", err)
	}
	payload, err := contracts.ParseTaskActivatePayload(activateResult)
	if err != nil {
		t.Fatal(err)
	}
	if !payload.NoProviderExecution || !payload.NoWorkerExecution {
		t.Fatalf("activate execution boundary flags missing: %+v", payload)
	}
	if _, err := os.Stat(payload.PromptPath); err != nil {
		t.Fatalf("prompt not materialized: %v", err)
	}

	specResult, err := TaskSpecService{Store: store}.Show("fixture-slug")
	if err != nil {
		t.Fatalf("spec returned error: %v", err)
	}
	specPayload, err := contracts.ParseTaskSpecPayload(specResult)
	if err != nil {
		t.Fatal(err)
	}
	if !specPayload.NoMutation || specPayload.Content == "" || !specPayload.TaskExists {
		t.Fatalf("unexpected spec payload: %+v", specPayload)
	}
}

func TestInitOutsideGitRepoFails(t *testing.T) {
	store := state.Store{RepoRoot: t.TempDir()}
	result, err := InitService{Store: store}.Run()
	if err == nil {
		t.Fatal("expected error outside git repo")
	}
	if result.Success || len(result.Errors) == 0 || result.Errors[0].Code != "not-git-repository" {
		data, _ := json.Marshal(result)
		t.Fatalf("unexpected result: %s", data)
	}
}

func newGitFixture(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runFixtureGit(t, repo, "init")
	runFixtureGit(t, repo, "config", "user.email", "test@example.com")
	runFixtureGit(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runFixtureGit(t, repo, "add", "README.md")
	runFixtureGit(t, repo, "commit", "-m", "init")
	return repo
}

func runFixtureGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
}
