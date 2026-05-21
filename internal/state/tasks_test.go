package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mortenlein/brevity/internal/state/locking"
)

func TestLoadTasksParsesExistingShape(t *testing.T) {
	store := taskTestStore(t)
	writeTaskTestFile(t, store, TasksFile, `[
		{
			"slug":"z-task",
			"status":"ready-for-worker",
			"normalizedState":"ready-for-worker",
			"branch":"task/z-task",
			"worktree":{"exists":true,"path":"C:\\repo\\worktrees\\active\\z-task","branch":"task/z-task","registered":true},
			"prompt":{"exists":true,"path":"C:\\repo\\worktrees\\active\\z-task\\prompt.md"},
			"provider":"codex",
			"profile":"default",
			"execution":{"status":"succeeded","lastRunId":"run-1","lastLogPath":"C:\\repo\\.brevity\\logs\\z-task\\run-1.log","lastProvider":"codex","lastProfile":"default"}
		},
		{"slug":"a-task","status":"blocked"}
	]`)

	tasks, missing, err := LoadTasks(store)
	if err != nil {
		t.Fatalf("LoadTasks returned error: %v", err)
	}
	if missing {
		t.Fatal("missing = true, want false")
	}
	if len(tasks.Items) != 2 {
		t.Fatalf("tasks = %d, want 2", len(tasks.Items))
	}
	if tasks.Items[0].Slug != "a-task" || tasks.Items[1].Slug != "z-task" {
		t.Fatalf("tasks not sorted deterministically: %#v", tasks.Items)
	}
	summary := tasks.Items[1].ToContract()
	if summary.WorktreePath != `C:\repo\worktrees\active\z-task` {
		t.Fatalf("WorktreePath = %q", summary.WorktreePath)
	}
	if summary.PromptPath != `C:\repo\worktrees\active\z-task\prompt.md` {
		t.Fatalf("PromptPath = %q", summary.PromptPath)
	}
	if summary.WorkerStatus != "succeeded" || summary.LastRunID != "run-1" {
		t.Fatalf("execution summary = %#v", summary)
	}
}

func TestLoadTasksMissingFileIsEmpty(t *testing.T) {
	tasks, missing, err := LoadTasks(taskTestStore(t))
	if err != nil {
		t.Fatalf("LoadTasks returned error: %v", err)
	}
	if !missing {
		t.Fatal("missing = false, want true")
	}
	if len(tasks.Items) != 0 {
		t.Fatalf("tasks = %#v, want empty", tasks.Items)
	}
}

func TestLoadTasksToleratesMissingOptionalFields(t *testing.T) {
	store := taskTestStore(t)
	writeTaskTestFile(t, store, TasksFile, `[{"slug":"minimal","status":"planned"}]`)

	tasks, _, err := LoadTasks(store)
	if err != nil {
		t.Fatalf("LoadTasks returned error: %v", err)
	}
	summary := tasks.Items[0].ToContract()
	if summary.Slug != "minimal" || summary.Status != "planned" {
		t.Fatalf("summary = %#v, want minimal task", summary)
	}
}

func TestLoadTasksRejectsInvalidJSON(t *testing.T) {
	store := taskTestStore(t)
	writeTaskTestFile(t, store, TasksFile, `[{"slug":`)

	_, _, err := LoadTasks(store)
	if err == nil {
		t.Fatal("LoadTasks returned nil error")
	}
	if !strings.Contains(err.Error(), "parse tasks.json") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadTasksRejectsMissingCriticalSlug(t *testing.T) {
	store := taskTestStore(t)
	writeTaskTestFile(t, store, TasksFile, `[{"status":"planned"}]`)

	_, _, err := LoadTasks(store)
	if err == nil {
		t.Fatal("LoadTasks returned nil error")
	}
	if !strings.Contains(err.Error(), "no slug or id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTasksRoundTripForFixtures(t *testing.T) {
	input := Tasks{Items: []Task{{Slug: "roundtrip", Status: "planned", WorktreePath: `C:\repo\worktrees\active\roundtrip`}}}
	data, err := json.Marshal(input.Items)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	var output Tasks
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if output.Items[0].Slug != "roundtrip" || output.Items[0].WorktreePath == "" {
		t.Fatalf("output = %#v, want roundtrip task", output.Items[0])
	}
}

func TestUpdateTaskPreservesUnknownAndUnrelatedFields(t *testing.T) {
	store := taskTestStore(t)
	writeTaskTestFile(t, store, TasksFile, `[
	  {"slug":"alpha","status":"planned","extraField":{"keep":true}},
	  {"slug":"beta","status":"blocked","note":"leave me"}
	]`)

	_, err := UpdateTask(store, "alpha", TaskUpdateOptions{}, func(task map[string]json.RawMessage) error {
		task["status"] = json.RawMessage(`"ready-for-worker"`)
		task["updatedAt"] = json.RawMessage(`"2026-05-21T10:00:00Z"`)
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateTask returned error: %v", err)
	}
	data, err := os.ReadFile(store.Path(TasksFile))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	text := string(data)
	for _, want := range []string{`"extraField": {`, `"keep": true`, `"note": "leave me"`, `"slug": "beta"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("updated tasks missing %q:\n%s", want, text)
		}
	}
}

func TestUpdateTaskMissingTaskAndMalformedFile(t *testing.T) {
	store := taskTestStore(t)
	writeTaskTestFile(t, store, TasksFile, `[{"slug":"alpha","status":"planned"}]`)
	if _, err := UpdateTask(store, "missing", TaskUpdateOptions{}, func(task map[string]json.RawMessage) error { return nil }); err == nil || !strings.Contains(err.Error(), "task not found") {
		t.Fatalf("missing task error = %v, want task not found", err)
	}

	writeTaskTestFile(t, store, TasksFile, `[{"slug":`)
	if _, err := UpdateTask(store, "alpha", TaskUpdateOptions{}, func(task map[string]json.RawMessage) error { return nil }); err == nil || !strings.Contains(err.Error(), "parse tasks.json") {
		t.Fatalf("malformed error = %v, want parse tasks.json", err)
	}
}

func TestUpdateTaskLockedWriteReturnsError(t *testing.T) {
	store := taskTestStore(t)
	writeTaskTestFile(t, store, TasksFile, `[{"slug":"alpha","status":"planned"}]`)
	if err := os.WriteFile(store.LockPath(), []byte("pid=1\ncreatedAt=2026-05-21T10:00:00Z\n"), 0o644); err != nil {
		t.Fatalf("WriteFile lock returned error: %v", err)
	}
	_, err := UpdateTask(store, "alpha", TaskUpdateOptions{LockOptions: locking.Options{Timeout: 20 * time.Millisecond, Interval: time.Millisecond}}, func(task map[string]json.RawMessage) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "task metadata locked") {
		t.Fatalf("lock error = %v, want task metadata locked", err)
	}
}

func taskTestStore(t *testing.T) Store {
	t.Helper()
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, DirectoryName), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	store, err := NewStore(repoRoot)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	return store
}

func writeTaskTestFile(t *testing.T, store Store, name string, content string) {
	t.Helper()
	if err := os.WriteFile(store.Path(name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}
