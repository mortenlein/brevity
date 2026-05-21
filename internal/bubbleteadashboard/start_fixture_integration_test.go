package bubbleteadashboard

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mortenlein/brevity/internal/runtimeclient"
)

const fixtureStartTaskSlug = "fixture-start-task"

func TestStartTaskFixtureIntegration(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell fixture smoke currently targets the Windows PowerShell path")
	}
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skipf("powershell.exe not available: %v", err)
	}

	fixture := newStartTaskFixture(t)
	previousWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(fixture.repoRoot); err != nil {
		t.Fatalf("chdir fixture repo: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousWorkingDirectory); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	client := runtimeclient.PowerShellClient{ScriptPath: fixture.scriptPath}
	bridge := RuntimeClientCommandBridge{Client: client}
	state, err := bridge.RefreshRuntimeState()
	if err != nil {
		t.Fatalf("refresh fixture runtime state: %v", err)
	}
	if !samePath(state.RepoRoot, fixture.repoRoot) {
		t.Fatalf("runtime state repoRoot = %q, want fixture %q", state.RepoRoot, fixture.repoRoot)
	}

	model := NewModelWithSource(client, time.Second, "powershell-fixture")
	model.commandBridge = bridge
	model.state = state
	model.hasState = true
	model.selection.SelectedIndex = indexOfTaskRow(t, model, fixtureStartTaskSlug)
	model.paletteOpen = true
	model.paletteSelected = indexOfAction(t, model, ActionStartTask)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("opening Start task confirmation returned command, want nil")
	}
	if model.confirmation == nil {
		t.Fatal("Start task confirmation did not open")
	}

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("confirmed Start task returned nil command")
	}
	if model.commandRun == nil || model.commandRun.status != commandRunning {
		t.Fatalf("commandRun = %#v, want running", model.commandRun)
	}

	updated, followup := model.Update(cmd())
	model = updated.(Model)
	if model.commandRun == nil || model.commandRun.status != commandSucceeded {
		t.Fatalf("commandRun = %#v, want succeeded", model.commandRun)
	}
	if got := strings.TrimSpace(model.commandRun.result.Stdout); !strings.Contains(got, "Task: "+fixtureStartTaskSlug) || !strings.Contains(got, "Worker: codex -C") {
		t.Fatalf("Start task stdout did not prove fixture command path:\n%s", got)
	}
	if len(model.activities) == 0 || model.activities[0].status != commandSucceeded {
		t.Fatalf("activity row not recorded: %#v", model.activities)
	}
	if followup == nil {
		t.Fatal("successful Start task did not schedule runtime refresh")
	}

	updated, refreshCmd := model.Update(followup())
	model = updated.(Model)
	if refreshCmd == nil || !model.polling {
		t.Fatalf("follow-up did not request refresh: cmd=%v polling=%t", refreshCmd, model.polling)
	}
	updated, _ = model.Update(refreshCmd())
	model = updated.(Model)
	if !model.hasState || !samePath(model.state.RepoRoot, fixture.repoRoot) {
		t.Fatalf("refreshed state repoRoot = %q, want fixture %q", model.state.RepoRoot, fixture.repoRoot)
	}

	prompt, err := os.ReadFile(fixture.promptPath)
	if err != nil {
		t.Fatalf("read fixture prompt: %v", err)
	}
	if !strings.Contains(string(prompt), fixtureStartTaskSlug) {
		t.Fatalf("fixture prompt was not materialized for %s:\n%s", fixtureStartTaskSlug, string(prompt))
	}
	if _, err := os.Stat(filepath.Join(sourceRepoRoot(t), ".brevity", "tasks.json")); err != nil {
		t.Fatalf("real repo task metadata should remain present and untouched by fixture setup: %v", err)
	}
}

func TestRunWorkerDryRunFixtureIntegrationDoesNotExecute(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell fixture smoke currently targets the Windows PowerShell path")
	}
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skipf("powershell.exe not available: %v", err)
	}

	fixture := newStartTaskFixture(t)
	previousWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(fixture.repoRoot); err != nil {
		t.Fatalf("chdir fixture repo: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousWorkingDirectory); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	client := runtimeclient.PowerShellClient{ScriptPath: fixture.scriptPath}
	bridge := RuntimeClientCommandBridge{Client: client}
	state, err := bridge.RefreshRuntimeState()
	if err != nil {
		t.Fatalf("refresh fixture runtime state: %v", err)
	}
	if len(state.Tasks) != 1 {
		t.Fatalf("fixture tasks = %d, want 1", len(state.Tasks))
	}
	state.Tasks[0].Provider = "codex"
	state.Tasks[0].Profile = ""
	state.Tasks[0].Status = "ready-for-worker"
	state.Tasks[0].NormalizedState = "ready-for-worker"

	model := NewModelWithSource(client, time.Second, "powershell-fixture")
	model.commandBridge = bridge
	model.state = state
	model.hasState = true
	model.selection.SelectedIndex = indexOfTaskRow(t, model, fixtureStartTaskSlug)
	model.paletteOpen = true
	model.paletteSelected = indexOfAction(t, model, ActionRunWorker)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("Run worker dry-run fixture did not request execution plan")
	}
	if model.commandRun == nil || model.confirmation != nil {
		t.Fatalf("Run worker dry-run did not enter plan loading state: command=%#v confirmation=%#v", model.commandRun, model.confirmation)
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	output := plainView(model.View())
	for _, want := range []string{"Run Worker Execution Plan", fixtureStartTaskSlug, "no worker/provider launched", "dry-run       yes", "PowerShell-owned execution plan"} {
		if !strings.Contains(output, want) {
			t.Fatalf("fixture dry-run preview missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "task run --execute") {
		t.Fatal("fixture dry-run unexpectedly prepared an executable command")
	}
}

type startTaskFixture struct {
	repoRoot   string
	scriptPath string
	promptPath string
}

func newStartTaskFixture(t *testing.T) startTaskFixture {
	t.Helper()

	sourceRoot := sourceRepoRoot(t)
	repoRoot := t.TempDir()
	runGit(t, repoRoot, "init")
	runGit(t, repoRoot, "config", "user.email", "fixture@example.invalid")
	runGit(t, repoRoot, "config", "user.name", "Brevity Fixture")
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatalf("write fixture README: %v", err)
	}
	runGit(t, repoRoot, "add", "README.md")
	runGit(t, repoRoot, "commit", "-m", "fixture root")

	scriptPath := filepath.Join(repoRoot, "brevity.ps1")
	copyFile(t, filepath.Join(sourceRoot, "brevity.ps1"), scriptPath)

	worktreePath := filepath.Join(repoRoot, "worktrees", "active", fixtureStartTaskSlug)
	promptPath := filepath.Join(worktreePath, "prompt.md")
	vaultPath := filepath.Join(repoRoot, "vaults", "AI-Vault", "10-Projects", "fixture")
	mustMkdirAll(t,
		filepath.Join(repoRoot, ".brevity"),
		worktreePath,
		vaultPath,
	)
	writeFile(t, filepath.Join(repoRoot, ".brevity", "provider-health.json"), `{
  "codex": {
    "status": "unknown",
    "updatedAt": "2026-05-21T00:00:00Z",
    "note": "fixture only; no provider or worker is started"
  }
}`)
	writeFile(t, filepath.Join(repoRoot, ".brevity", "config.json"), `{
  "projectName": "fixture",
  "devRoot": "`+jsonPath(repoRoot)+`",
  "vaultPath": "`+jsonPath(vaultPath)+`",
  "worktreesRoot": "`+jsonPath(filepath.Join(repoRoot, "worktrees", "active"))+`"
}`)
	writeFile(t, filepath.Join(repoRoot, ".brevity", "tasks.json"), `[
  {
    "slug": "`+fixtureStartTaskSlug+`",
    "branch": "task/`+fixtureStartTaskSlug+`",
    "worktreePath": "`+jsonPath(worktreePath)+`",
    "promptPath": "`+jsonPath(promptPath)+`",
    "specPath": "",
    "status": "ready-for-worker",
    "createdAt": "2026-05-21T00:00:00Z"
  }
]`)
	writeFile(t, promptPath, "# Fixture prompt\n\nNo provider or worker should be launched during plan generation.\n")

	return startTaskFixture{repoRoot: repoRoot, scriptPath: scriptPath, promptPath: promptPath}
}

func indexOfTaskRow(t *testing.T, model Model, slug string) int {
	t.Helper()
	for index, item := range model.selectableItems() {
		if item.Task.Slug == slug {
			return index
		}
	}
	t.Fatalf("task row %q not found in selectable items", slug)
	return 0
}

func indexOfAction(t *testing.T, model Model, id ActionID) int {
	t.Helper()
	for index, action := range model.actionDescriptors() {
		if action.ID == id {
			return index
		}
	}
	t.Fatalf("action %q not found", id)
	return 0
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
}

func mustMkdirAll(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func copyFile(t *testing.T, source string, destination string) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read %s: %v", source, err)
	}
	writeFile(t, destination, string(data))
}

func jsonPath(path string) string {
	return strings.ReplaceAll(path, `\`, `\\`)
}

func samePath(left string, right string) bool {
	return strings.EqualFold(filepath.Clean(filepath.FromSlash(left)), filepath.Clean(filepath.FromSlash(right)))
}

func sourceRepoRoot(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for directory := workingDirectory; ; directory = filepath.Dir(directory) {
		if _, err := os.Stat(filepath.Join(directory, "brevity.ps1")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatalf("could not find source repo root from %s", workingDirectory)
		}
	}
}
