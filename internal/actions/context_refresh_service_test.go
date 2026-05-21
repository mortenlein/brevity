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

func TestTaskContextRefreshMaterializesPromptAndVaultContext(t *testing.T) {
	root, worktree, promptPath := contextRefreshFixture(t, true)
	store, err := state.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	result, runErr := TaskContextRefreshService{
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC) },
	}.Refresh("fixture-task")
	if runErr != nil {
		t.Fatalf("Refresh returned error: %v", runErr)
	}
	if !result.Success {
		t.Fatalf("Success = false: %#v", result.Errors)
	}
	payload, err := contracts.ParseTaskContextRefreshPayload(result)
	if err != nil {
		t.Fatal(err)
	}
	if payload.PromptRefreshStatus != "fresh" || !payload.NoProviderExecution || !payload.NoWorkerExecution {
		t.Fatalf("payload missing refresh/no-execution metadata: %#v", payload)
	}
	prompt := readActionTestFile(t, promptPath)
	for _, want := range []string{"Slug: fixture-task", "Source:", "Implement deterministic context refresh.", ".brevity\\context\\project.md"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if got := readActionTestFile(t, filepath.Join(worktree, ".brevity", "context", "project.md")); !strings.Contains(got, "Fixture project") {
		t.Fatalf("context was not copied:\n%s", got)
	}
	data := readActionTestFile(t, filepath.Join(root, ".brevity", "tasks.json"))
	if !strings.Contains(data, `"promptRefreshStatus": "fresh"`) || !strings.Contains(data, `"promptRefreshedAt": "2026-05-21T12:00:00Z"`) {
		t.Fatalf("tasks metadata was not updated:\n%s", data)
	}
}

func TestTaskContextRefreshToleratesMissingVault(t *testing.T) {
	root, _, promptPath := contextRefreshFixture(t, false)
	store, err := state.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	result, runErr := TaskContextRefreshService{Store: store}.Refresh("fixture-task")
	if runErr != nil {
		t.Fatalf("Refresh returned error: %v", runErr)
	}
	payload, err := contracts.ParseTaskContextRefreshPayload(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.MaterializedFiles) != 0 || len(payload.MissingFiles) == 0 {
		t.Fatalf("missing vault context not reported as optional missing files: %#v", payload)
	}
	prompt := readActionTestFile(t, promptPath)
	if !strings.Contains(prompt, "No vault task spec was materialized") {
		t.Fatalf("prompt did not tolerate missing spec:\n%s", prompt)
	}
}

func TestRenderTaskPromptIsDeterministic(t *testing.T) {
	context := promptContext{
		Slug:         "alpha",
		State:        "ready-for-worker",
		SpecPath:     `C:\vault\tasks\alpha.md`,
		SpecContents: "# Goal\nDo the thing.",
		ContextFiles: []string{"architecture.md", "project.md"},
		PromptPath:   `C:\work\prompt.md`,
		WorktreePath: `C:\work`,
	}
	first := renderTaskPrompt(context)
	second := renderTaskPrompt(context)
	if first != second {
		t.Fatalf("prompt rendering is nondeterministic")
	}
	if strings.Contains(first, time.Now().Format("2006")) {
		t.Fatalf("prompt unexpectedly embeds wall-clock data:\n%s", first)
	}
}

func contextRefreshFixture(t *testing.T, withVault bool) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	worktree := filepath.Join(root, "worktree")
	promptPath := filepath.Join(worktree, "prompt.md")
	vault := filepath.Join(root, "vault")
	for _, dir := range []string{filepath.Join(root, ".brevity"), worktree} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	config := map[string]string{"vaultPath": filepath.Join(root, "missing-vault")}
	if withVault {
		config["vaultPath"] = vault
		if err := os.MkdirAll(filepath.Join(vault, "tasks"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeActionTestFile(t, filepath.Join(vault, "project.md"), "# Project\nFixture project.\n")
		writeActionTestFile(t, filepath.Join(vault, "tasks", "fixture-task.md"), "# Goal\nImplement deterministic context refresh.\n")
	}
	configData, _ := json.Marshal(config)
	writeActionTestFile(t, filepath.Join(root, ".brevity", "config.json"), string(configData)+"\n")
	writeActionTestFile(t, filepath.Join(root, ".brevity", "provider-health.json"), `{"codex":{"status":"healthy"}}`+"\n")
	tasks := `[{"slug":"fixture-task","status":"ready-for-worker","normalizedState":"ready-for-worker","worktreePath":` + quoteJSON(worktree) + `,"promptPath":` + quoteJSON(promptPath) + `}]`
	writeActionTestFile(t, filepath.Join(root, ".brevity", "tasks.json"), tasks+"\n")
	return root, worktree, promptPath
}

func writeActionTestFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readActionTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func quoteJSON(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}
