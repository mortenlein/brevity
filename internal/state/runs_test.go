package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadRunsMissingFileIsEmpty(t *testing.T) {
	runs, missing, err := LoadRuns(runTestStore(t), runTestNow())
	if err != nil {
		t.Fatalf("LoadRuns returned error: %v", err)
	}
	if !missing {
		t.Fatal("missing = false, want true")
	}
	if len(runs.Items) != 0 {
		t.Fatalf("runs = %#v, want empty", runs.Items)
	}
}

func TestLoadRunsEmptyFileIsEmpty(t *testing.T) {
	store := runTestStore(t)
	writeRunTestFile(t, store, "")

	runs, missing, err := LoadRuns(store, runTestNow())
	if err != nil {
		t.Fatalf("LoadRuns returned error: %v", err)
	}
	if missing || len(runs.Items) != 0 {
		t.Fatalf("missing=%v runs=%#v, want existing empty history", missing, runs.Items)
	}
}

func TestLoadRunsParsesPopulatedJSONL(t *testing.T) {
	store := runTestStore(t)
	writeRunTestFile(t,
		store,
		`{"slug":"task-a","runId":"run-1","workerStatus":"succeeded","startedAt":"2026-05-19T09:00:00Z","finishedAt":"2026-05-19T09:02:00Z","exitCode":0,"provider":"codex","profile":"default","logPath":"C:\\repo\\.brevity\\logs\\task-a\\run-1.log","stdoutPath":"C:\\repo\\.brevity\\logs\\task-a\\run-1.out","stderrPath":"C:\\repo\\.brevity\\logs\\task-a\\run-1.err","summary":"ok","message":"done"}`+"\n")

	runs, _, err := LoadRuns(store, runTestNow())
	if err != nil {
		t.Fatalf("LoadRuns returned error: %v", err)
	}
	run := runs.Items[0]
	if run.RunID != "run-1" || run.Slug != "task-a" || run.Provider != "codex" || run.Profile != "default" {
		t.Fatalf("run identity = %#v, want parsed fields", run)
	}
	if run.LogPath != `C:\repo\.brevity\logs\task-a\run-1.log` || run.StdoutPath == "" || run.StderrPath == "" {
		t.Fatalf("run paths = %#v, want Windows paths preserved", run)
	}
	if run.WorkerStatus != "succeeded" || run.Source != "index" || len(run.Raw) == 0 {
		t.Fatalf("run status/source/raw = %#v, want enriched index record", run)
	}
}

func TestLoadRunsMalformedLineReturnsClearError(t *testing.T) {
	store := runTestStore(t)
	writeRunTestFile(t, store, `{"slug":"ok"}`+"\n{not-json}\n")

	_, _, err := LoadRuns(store, runTestNow())
	if err == nil {
		t.Fatal("LoadRuns returned nil error")
	}
	if !strings.Contains(err.Error(), "parse runs.jsonl line 2") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRunsSortsLatestDeterministically(t *testing.T) {
	store := runTestStore(t)
	writeRunTestFile(t,
		store,
		`{"slug":"task-a","runId":"old","finishedAt":"2026-05-19T09:00:00Z"}`+"\n"+
			`{"slug":"task-a","runId":"newer","finishedAt":"2026-05-19T10:00:00Z"}`+"\n"+
			`{"slug":"task-a","runId":"last-same-time","finishedAt":"2026-05-19T10:00:00Z"}`+"\n")

	runs, _, err := LoadRuns(store, runTestNow())
	if err != nil {
		t.Fatalf("LoadRuns returned error: %v", err)
	}
	if runs.Items[0].RunID != "last-same-time" || runs.Items[1].RunID != "newer" || runs.Items[2].RunID != "old" {
		t.Fatalf("sorted runs = %#v, want latest timestamp then later line", runs.Items)
	}
	if latest := runs.LatestByTask()["task-a"]; latest.RunID != "last-same-time" {
		t.Fatalf("latest = %#v, want last-same-time", latest)
	}
}

func TestLoadRunsNormalizesRunningIncompleteAndStale(t *testing.T) {
	store := runTestStore(t)
	writeRunTestFile(t, store, `{"slug":"task-a","runId":"stale","workerStatus":"running","startedAt":"2026-05-19T09:00:00Z"}`+"\n")

	runs, _, err := LoadRuns(store, runTestNow())
	if err != nil {
		t.Fatalf("LoadRuns returned error: %v", err)
	}
	run := runs.Items[0]
	if run.WorkerStatus != "stale" || !run.Incomplete || !run.Stale || run.RunAgeMinutes == nil {
		t.Fatalf("run = %#v, want stale incomplete run", run)
	}
}

func runTestStore(t *testing.T) Store {
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

func writeRunTestFile(t *testing.T, store Store, content string) {
	t.Helper()
	if err := os.WriteFile(store.Path(RunsFile), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}

func runTestNow() time.Time {
	return time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
}
