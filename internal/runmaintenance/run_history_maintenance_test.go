package runmaintenance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mortenlein/brevity/internal/state"
	"github.com/mortenlein/brevity/internal/state/locking"
)

func TestBuildRunMaintenancePlanDetectsIssues(t *testing.T) {
	store := maintenanceStore(t)
	logPath := filepath.Join(store.RepoRoot, ".brevity", "logs", "ok.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRuns(t, store,
		`{"slug":"task-a","runId":"dup","startedAt":"2026-05-19T08:00:00Z","finishedAt":"2026-05-19T08:01:00Z","exitCode":0,"logPath":".brevity/logs/ok.log"}`+"\n"+
			`{"slug":"task-a","runId":"dup","startedAt":"2026-05-19T09:00:00Z","finishedAt":"2026-05-19T09:01:00Z","exitCode":0,"logPath":".brevity/logs/missing.log"}`+"\n"+
			`{"slug":"task-b","runId":"stale","workerStatus":"running","startedAt":"2026-05-19T08:00:00Z"}`+"\n"+
			"{not-json}\n")

	plan, _, err := BuildRunMaintenancePlan(RunMaintenanceOptions{Store: store, Now: maintenanceNow})
	if err != nil {
		t.Fatalf("BuildRunMaintenancePlan returned error: %v", err)
	}
	if plan.TotalRuns != 4 || plan.ValidRuns != 3 || len(plan.MalformedRows) != 1 || len(plan.DuplicateRunIDs) != 1 {
		t.Fatalf("plan counts = %#v", plan)
	}
	if len(plan.StaleIncompleteRuns) != 1 || len(plan.MissingLogReferences) != 1 {
		t.Fatalf("plan issue counts = %#v", plan)
	}
	if !plan.WouldRewriteRunsFile || !plan.RequiresForce || plan.WouldDeleteLogs {
		t.Fatalf("plan flags = rewrite %t force %t deleteLogs %t", plan.WouldRewriteRunsFile, plan.RequiresForce, plan.WouldDeleteLogs)
	}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"schema":"brevity.run-maintenance-plan.v1"`, `"duplicateRunIds"`, `"compactableRows"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("plan JSON missing %q:\n%s", want, data)
		}
	}
}

func TestExecuteRunCompactionRequiresForceAndPreservesLatest(t *testing.T) {
	store := maintenanceStore(t)
	writeRuns(t, store,
		`{"slug":"task-a","runId":"dup","startedAt":"2026-05-19T08:00:00Z","finishedAt":"2026-05-19T08:01:00Z","exitCode":0}`+"\n"+
			`{"slug":"task-a","runId":"dup","startedAt":"2026-05-19T09:00:00Z","finishedAt":"2026-05-19T09:01:00Z","exitCode":0}`+"\n"+
			"{not-json}\n")

	result, err := ExecuteRunCompaction(RunMaintenanceOptions{Store: store, Now: maintenanceNow}, false)
	if err == nil || result.Success {
		t.Fatalf("ExecuteRunCompaction without force result=%#v err=%v, want refusal", result, err)
	}

	result, err = ExecuteRunCompaction(RunMaintenanceOptions{Store: store, Now: maintenanceNow}, true)
	if err != nil || !result.Success {
		t.Fatalf("ExecuteRunCompaction with force result=%#v err=%v", result, err)
	}
	data, err := os.ReadFile(store.Path(state.RunsFile))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "{not-json}") || strings.Contains(content, `"startedAt":"2026-05-19T08:00:00Z"`) {
		t.Fatalf("compacted content kept malformed or older duplicate:\n%s", content)
	}
	if !strings.Contains(content, `"startedAt":"2026-05-19T09:00:00Z"`) {
		t.Fatalf("compacted content lost latest duplicate:\n%s", content)
	}
	if _, missing, err := state.LoadRuns(store, maintenanceNow()); err != nil || missing {
		t.Fatalf("LoadRuns after compaction missing=%t err=%v", missing, err)
	}
	if _, err := os.Stat(store.Path("runs-malformed.jsonl")); err != nil {
		t.Fatalf("malformed quarantine missing: %v", err)
	}
}

func TestBuildRunMaintenancePlanMissingFile(t *testing.T) {
	store := maintenanceStore(t)
	plan, _, err := BuildRunMaintenancePlan(RunMaintenanceOptions{Store: store, Now: maintenanceNow})
	if err != nil {
		t.Fatalf("BuildRunMaintenancePlan returned error: %v", err)
	}
	if !plan.MissingRunsFile || plan.TotalRuns != 0 || plan.WouldRewriteRunsFile {
		t.Fatalf("plan = %#v, want missing non-mutating plan", plan)
	}
}

func TestExecuteRunCompactionReportsLockContention(t *testing.T) {
	store := maintenanceStore(t)
	writeRuns(t, store,
		`{"slug":"task-a","runId":"dup","startedAt":"2026-05-19T08:00:00Z","finishedAt":"2026-05-19T08:01:00Z","exitCode":0}`+"\n"+
			`{"slug":"task-a","runId":"dup","startedAt":"2026-05-19T09:00:00Z","finishedAt":"2026-05-19T09:01:00Z","exitCode":0}`+"\n")
	lock, err := locking.Acquire(store.LockPath(), locking.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()

	result, err := ExecuteRunCompaction(RunMaintenanceOptions{Store: store, Now: maintenanceNow}, true)
	if err == nil || result.Success {
		t.Fatalf("ExecuteRunCompaction result=%#v err=%v, want lock failure", result, err)
	}
	if len(result.Errors) == 0 || result.Errors[0].Code != "state-lock-timeout" {
		t.Fatalf("errors = %#v, want state-lock-timeout", result.Errors)
	}
}

func maintenanceStore(t *testing.T) state.Store {
	t.Helper()
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, state.DirectoryName), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := state.NewStore(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func writeRuns(t *testing.T, store state.Store, content string) {
	t.Helper()
	if err := os.WriteFile(store.Path(state.RunsFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func maintenanceNow() time.Time {
	return time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
}
