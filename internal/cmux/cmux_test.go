package cmux_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mortenlein/brevity/internal/cmux"
	"github.com/mortenlein/brevity/internal/contracts"
	runtimescheduler "github.com/mortenlein/brevity/internal/runtime/scheduler"
)

// --- stub fetcher -------------------------------------------------------

type stubFetcher struct {
	stateJSON     []byte
	stateErr      error
	schedulerJSON []byte
	schedulerErr  error
}

func (s stubFetcher) RuntimeStateJSON() ([]byte, error)  { return s.stateJSON, s.stateErr }
func (s stubFetcher) SchedulerPlanJSON() ([]byte, error) { return s.schedulerJSON, s.schedulerErr }

// --- helpers ------------------------------------------------------------

func minimalStateJSON(t *testing.T) []byte {
	t.Helper()
	return []byte(`{
		"schema": "brevity.runtime-state.v1",
		"repoRoot": "/dev/test",
		"generatedAt": "2026-01-01T00:00:00Z",
		"providers": {"summary": {"total": 2, "degraded": 0, "unavailable": 0},
		              "health": {"codex": {"status": "healthy", "updatedAt": "", "note": ""},
		                         "gemini": {"status": "unknown", "updatedAt": "", "note": ""}}},
		"taskCounts": {"tracked": 3, "runnable": 1, "blocked": 0, "stale": 0, "providerGated": 0, "review": 1},
		"tasks": [
			{"slug": "alpha-task", "status": "ready-for-worker", "normalizedState": "ready-for-worker", "workerStatus": "", "branch": "task/alpha-task", "worktreePath": ""},
			{"slug": "beta-task",  "status": "reviewing",         "normalizedState": "reviewing",         "workerStatus": "succeeded", "branch": "task/beta-task",  "worktreePath": ""}
		],
		"suggestedNextActions": ["Run task alpha-task with brevity task run."],
		"orphanedTaskWorktrees": [],
		"activeWorktrees": [],
		"activeWorktreeCount": 0,
		"groups": {}
	}`)
}

func minimalSchedulerJSON() []byte {
	return []byte(`{
		"schema": "brevity.runtime-scheduler-plan.v1",
		"queuePath": ".brevity/runtime-queue.json",
		"queueState": "missing",
		"queueVersion": 0,
		"supportedQueueVersion": 1,
		"noSelectionReason": "no eligible runnable queue item",
		"reservationEligible": false,
		"reservationEligibility": "not eligible: no selected queue item",
		"skipped": [],
		"safetyChecks": [],
		"readOnly": true
	}`)
}

func renderSnapshot(snap cmux.Snapshot) string {
	var buf bytes.Buffer
	cmux.Render(&buf, snap, cmux.RenderOptions{})
	return buf.String()
}

func renderSnapshotOpts(snap cmux.Snapshot, opts cmux.RenderOptions) string {
	var buf bytes.Buffer
	cmux.Render(&buf, snap, opts)
	return buf.String()
}

// --- contract parsing tests ---------------------------------------------

func TestRead_ParsesRuntimeState(t *testing.T) {
	fetcher := stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	}
	snap := cmux.Read(fetcher)

	if !snap.HasRuntimeState {
		t.Fatal("expected HasRuntimeState=true")
	}
	if snap.RuntimeStateErr != nil {
		t.Fatalf("unexpected RuntimeStateErr: %v", snap.RuntimeStateErr)
	}
	if snap.RuntimeState.Schema != contracts.RuntimeStateSchema {
		t.Errorf("schema = %q, want %q", snap.RuntimeState.Schema, contracts.RuntimeStateSchema)
	}
	if snap.RuntimeState.TaskCounts.Tracked != 3 {
		t.Errorf("tracked = %d, want 3", snap.RuntimeState.TaskCounts.Tracked)
	}
	if len(snap.RuntimeState.Tasks) != 2 {
		t.Errorf("tasks len = %d, want 2", len(snap.RuntimeState.Tasks))
	}
}

func TestRead_ParsesSchedulerPlan(t *testing.T) {
	fetcher := stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	}
	snap := cmux.Read(fetcher)

	if !snap.HasSchedulerPlan {
		t.Fatal("expected HasSchedulerPlan=true")
	}
	if snap.SchedulerPlanErr != nil {
		t.Fatalf("unexpected SchedulerPlanErr: %v", snap.SchedulerPlanErr)
	}
	if snap.SchedulerPlan.Schema != runtimescheduler.PlanSchema {
		t.Errorf("scheduler schema = %q, want %q", snap.SchedulerPlan.Schema, runtimescheduler.PlanSchema)
	}
}

// --- error handling tests -----------------------------------------------

func TestRead_RuntimeStateFetchError(t *testing.T) {
	fetcher := stubFetcher{
		stateErr:      errors.New("runtime unavailable"),
		schedulerJSON: minimalSchedulerJSON(),
	}
	snap := cmux.Read(fetcher)

	if snap.HasRuntimeState {
		t.Error("expected HasRuntimeState=false after fetch error")
	}
	if snap.RuntimeStateErr == nil {
		t.Error("expected RuntimeStateErr to be set")
	}
	// Scheduler plan should still be parsed despite runtime state failure.
	if !snap.HasSchedulerPlan {
		t.Error("expected HasSchedulerPlan=true even when runtime state fails")
	}
}

func TestRead_SchedulerPlanFetchError(t *testing.T) {
	fetcher := stubFetcher{
		stateJSON:    minimalStateJSON(t),
		schedulerErr: errors.New("scheduler unavailable"),
	}
	snap := cmux.Read(fetcher)

	if snap.HasSchedulerPlan {
		t.Error("expected HasSchedulerPlan=false after fetch error")
	}
	if snap.SchedulerPlanErr == nil {
		t.Error("expected SchedulerPlanErr to be set")
	}
	// Runtime state should still be parsed.
	if !snap.HasRuntimeState {
		t.Error("expected HasRuntimeState=true even when scheduler plan fails")
	}
}

func TestRead_MalformedRuntimeStateJSON(t *testing.T) {
	fetcher := stubFetcher{
		stateJSON:     []byte(`{"not": "valid json for runtime state`),
		schedulerJSON: minimalSchedulerJSON(),
	}
	snap := cmux.Read(fetcher)

	if snap.HasRuntimeState {
		t.Error("expected HasRuntimeState=false for malformed JSON")
	}
	if snap.RuntimeStateErr == nil {
		t.Error("expected RuntimeStateErr for malformed JSON")
	}
}

func TestRead_MalformedSchedulerPlanJSON(t *testing.T) {
	fetcher := stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: []byte(`not json at all`),
	}
	snap := cmux.Read(fetcher)

	if snap.HasSchedulerPlan {
		t.Error("expected HasSchedulerPlan=false for malformed JSON")
	}
	if snap.SchedulerPlanErr == nil {
		t.Error("expected SchedulerPlanErr for malformed JSON")
	}
}

func TestRead_WrongRuntimeStateSchema(t *testing.T) {
	fetcher := stubFetcher{
		stateJSON:     []byte(`{"schema": "brevity.runtime-state.v99", "repoRoot": "/"}`),
		schedulerJSON: minimalSchedulerJSON(),
	}
	snap := cmux.Read(fetcher)

	if snap.HasRuntimeState {
		t.Error("expected HasRuntimeState=false for wrong schema")
	}
	if snap.RuntimeStateErr == nil {
		t.Error("expected RuntimeStateErr for wrong schema")
	}
}

func TestRead_BothErrors(t *testing.T) {
	fetcher := stubFetcher{
		stateErr:     errors.New("no runtime"),
		schedulerErr: errors.New("no scheduler"),
	}
	snap := cmux.Read(fetcher)

	if snap.HasRuntimeState || snap.HasSchedulerPlan {
		t.Error("expected both false when both fetches fail")
	}
	if snap.RuntimeStateErr == nil || snap.SchedulerPlanErr == nil {
		t.Error("expected both errors to be set")
	}
}

// --- empty-state handling tests -----------------------------------------

func TestRead_EmptyTasksAndProviders(t *testing.T) {
	stateJSON := []byte(`{
		"schema": "brevity.runtime-state.v1",
		"repoRoot": "/empty",
		"generatedAt": "2026-01-01T00:00:00Z",
		"providers": {"summary": {"total": 0, "degraded": 0, "unavailable": 0}, "health": {}},
		"taskCounts": {"tracked": 0, "runnable": 0, "blocked": 0, "stale": 0, "providerGated": 0, "review": 0},
		"tasks": [],
		"suggestedNextActions": [],
		"orphanedTaskWorktrees": [],
		"activeWorktrees": [],
		"activeWorktreeCount": 0,
		"groups": {}
	}`)
	fetcher := stubFetcher{
		stateJSON:     stateJSON,
		schedulerJSON: minimalSchedulerJSON(),
	}
	snap := cmux.Read(fetcher)

	if !snap.HasRuntimeState {
		t.Fatal("expected HasRuntimeState=true for empty state")
	}
	if snap.RuntimeState.TaskCounts.Tracked != 0 {
		t.Errorf("tracked = %d, want 0", snap.RuntimeState.TaskCounts.Tracked)
	}
	if len(snap.RuntimeState.Tasks) != 0 {
		t.Errorf("tasks len = %d, want 0", len(snap.RuntimeState.Tasks))
	}
}

// --- rendering invariants tests -----------------------------------------

func TestRender_ContainsHeader(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "CMUX OPERATOR") {
		t.Error("output missing CMUX OPERATOR header")
	}
}

func TestRender_ContainsProviderSection(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "Providers") {
		t.Error("output missing Providers section")
	}
	if !strings.Contains(out, "codex") {
		t.Error("output missing codex provider")
	}
	if !strings.Contains(out, "gemini") {
		t.Error("output missing gemini provider")
	}
}

func TestRender_ContainsTaskCountSection(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "Task Counts") {
		t.Error("output missing Task Counts section")
	}
	if !strings.Contains(out, "tracked=3") {
		t.Error("output missing tracked=3")
	}
}

func TestRender_ContainsTaskList(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "alpha-task") {
		t.Error("output missing alpha-task")
	}
	if !strings.Contains(out, "beta-task") {
		t.Error("output missing beta-task")
	}
	if !strings.Contains(out, "reviewing") {
		t.Error("output missing reviewing state")
	}
}

func TestRender_ContainsQueueSchedulerSection(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "Queue / Scheduler") {
		t.Error("output missing Queue / Scheduler section")
	}
}

func TestRender_ContainsSuggestedActions(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "Suggested Next Actions") {
		t.Error("output missing Suggested Next Actions section")
	}
	if !strings.Contains(out, "alpha-task") {
		t.Error("output missing suggested action text")
	}
}

func TestRender_EmptyStateGraceful(t *testing.T) {
	stateJSON := []byte(`{
		"schema": "brevity.runtime-state.v1",
		"repoRoot": "/empty",
		"generatedAt": "2026-01-01T00:00:00Z",
		"providers": {"summary": {"total": 0, "degraded": 0, "unavailable": 0}, "health": {}},
		"taskCounts": {"tracked": 0, "runnable": 0, "blocked": 0, "stale": 0, "providerGated": 0, "review": 0},
		"tasks": [],
		"suggestedNextActions": [],
		"orphanedTaskWorktrees": [],
		"activeWorktrees": [],
		"activeWorktreeCount": 0,
		"groups": {}
	}`)
	snap := cmux.Read(stubFetcher{
		stateJSON:     stateJSON,
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)

	// Must still produce all sections without panicking.
	for _, want := range []string{
		"CMUX OPERATOR", "Providers", "Task Counts", "tracked=0",
		"Task List: none tracked", "Queue / Scheduler", "Suggested Next Actions",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("empty-state output missing %q", want)
		}
	}
}

func TestRender_RuntimeStateError(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateErr:      errors.New("brevity runtime unavailable"),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)

	if !strings.Contains(out, "runtime-state: error:") {
		t.Error("output missing runtime-state error marker")
	}
	if !strings.Contains(out, "brevity runtime unavailable") {
		t.Error("output missing error message text")
	}
}

func TestRender_SchedulerPlanError(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:    minimalStateJSON(t),
		schedulerErr: errors.New("scheduler fetch failed"),
	})
	out := renderSnapshot(snap)

	if !strings.Contains(out, "scheduler: error:") {
		t.Error("output missing scheduler error marker")
	}
}

func TestRender_Deterministic(t *testing.T) {
	fetcher := stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	}
	snap := cmux.Read(fetcher)

	out1 := renderSnapshot(snap)
	out2 := renderSnapshot(snap)
	if out1 != out2 {
		t.Error("Render is not deterministic: two calls produced different output")
	}
}

func TestRender_NoANSISequences(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)

	if strings.Contains(out, "\x1b[") {
		t.Error("output contains ANSI escape sequences")
	}
}

func TestRender_SchedulerWithSelection(t *testing.T) {
	schedulerWithSelection := []byte(`{
		"schema": "brevity.runtime-scheduler-plan.v1",
		"queuePath": ".brevity/runtime-queue.json",
		"queueState": "valid",
		"queueVersion": 1,
		"supportedQueueVersion": 1,
		"selected": {
			"id": "abc-123",
			"task": "alpha-task",
			"provider": "",
			"profile": "",
			"status": "queued",
			"reason": "first eligible runnable queue item in queue order"
		},
		"reservationEligible": true,
		"reservationEligibility": "eligible: selected item is queued, runnable, unreserved, and has a valid task slug",
		"skipped": [],
		"safetyChecks": [],
		"readOnly": true
	}`)
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: schedulerWithSelection,
	})
	out := renderSnapshot(snap)

	if !strings.Contains(out, "scheduler-next:") {
		t.Error("output missing scheduler-next label")
	}
	if !strings.Contains(out, "alpha-task") {
		t.Error("output missing selected task slug")
	}
	if !strings.Contains(out, "abc-123") {
		t.Error("output missing selected item id")
	}
}

// --- header detail tests ------------------------------------------------

func TestRender_HeaderHasReadOnlyMarker(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "[read-only]") {
		t.Error("output missing [read-only] marker in header")
	}
}

func TestRender_HeaderHasSourceNative(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "source: native") {
		t.Error("output missing 'source: native' marker")
	}
}

func TestRender_HeaderHasSchemaWhenStateAvailable(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "schema: brevity.runtime-state.v1") {
		t.Errorf("output missing schema line; output:\n%s", out)
	}
}

func TestRender_HeaderHasGeneratedAtWhenPresent(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "generated: 2026-01-01T00:00:00Z") {
		t.Errorf("output missing generated timestamp; output:\n%s", out)
	}
}

func TestRender_HeaderHasRepoRoot(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "repo: /dev/test") {
		t.Errorf("output missing repo line; output:\n%s", out)
	}
}

func TestRender_HeaderSourceNativeWhenStateErrored(t *testing.T) {
	// source: native should appear even when runtime state fetch fails.
	snap := cmux.Read(stubFetcher{
		stateErr:      errors.New("unavailable"),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "source: native") {
		t.Error("output missing 'source: native' when state errored")
	}
	if !strings.Contains(out, "[read-only]") {
		t.Error("output missing [read-only] when state errored")
	}
}

// --- provider detail tests ----------------------------------------------

// richProviderStateJSON returns a runtime-state fixture with provider
// updatedAt and note populated for detail rendering assertions.
func richProviderStateJSON() []byte {
	return []byte(`{
		"schema": "brevity.runtime-state.v1",
		"repoRoot": "/dev/repos/project",
		"generatedAt": "2026-05-23T10:00:00Z",
		"providers": {
			"summary": {"total": 2, "degraded": 1, "unavailable": 0},
			"health": {
				"codex": {
					"status": "healthy",
					"updatedAt": "2026-05-23T09:00:00Z",
					"note": "all systems go"
				},
				"gemini": {
					"status": "capacity-degraded",
					"updatedAt": "2026-05-23T08:30:00Z",
					"note": "rate limited"
				}
			}
		},
		"taskCounts": {"tracked": 0, "runnable": 0, "blocked": 0, "stale": 0, "providerGated": 0, "review": 0},
		"tasks": [],
		"suggestedNextActions": [],
		"orphanedTaskWorktrees": [],
		"activeWorktrees": [],
		"activeWorktreeCount": 0,
		"groups": {}
	}`)
}

func TestRender_ProviderRowIncludesUpdatedAt(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richProviderStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "2026-05-23T09:00:00Z") {
		t.Errorf("output missing codex updatedAt; output:\n%s", out)
	}
	if !strings.Contains(out, "2026-05-23T08:30:00Z") {
		t.Errorf("output missing gemini updatedAt; output:\n%s", out)
	}
}

func TestRender_ProviderRowIncludesNote(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richProviderStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "all systems go") {
		t.Errorf("output missing codex note; output:\n%s", out)
	}
	if !strings.Contains(out, "rate limited") {
		t.Errorf("output missing gemini note; output:\n%s", out)
	}
}

func TestRender_ProviderEmptyTimestampAndNoteShowDash(t *testing.T) {
	// minimalStateJSON has providers with empty updatedAt and note.
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	// Both providers should fall back to "-" for the empty fields.
	if !strings.Contains(out, "codex") {
		t.Error("output missing codex")
	}
	// Confirm the dash placeholder appears at least once in the provider section.
	providerSection := extractSection(out, "Providers", sectionSep)
	if !strings.Contains(providerSection, "-") {
		t.Errorf("provider section missing dash fallback; section:\n%s", providerSection)
	}
}

// --- task detail tests --------------------------------------------------

// richTaskStateJSON returns a runtime-state fixture with one task that has
// worktree, prompt, and last-run fields fully populated.
func richTaskStateJSON() []byte {
	return []byte(`{
		"schema": "brevity.runtime-state.v1",
		"repoRoot": "/dev/repos/project",
		"generatedAt": "2026-05-23T10:00:00Z",
		"providers": {
			"summary": {"total": 1, "degraded": 0, "unavailable": 0},
			"health": {"codex": {"status": "healthy", "updatedAt": "", "note": ""}}
		},
		"taskCounts": {"tracked": 1, "runnable": 1, "blocked": 0, "stale": 0, "providerGated": 0, "review": 0},
		"tasks": [{
			"slug": "rich-task",
			"status": "ready-for-worker",
			"normalizedState": "ready-for-worker",
			"workerStatus": "",
			"branch": "task/rich-task",
			"worktreePath": "/repo/worktrees/active/brevity-rich-task",
			"worktree": {
				"exists": true,
				"path": "/repo/worktrees/active/brevity-rich-task",
				"branch": "task/rich-task"
			},
			"promptPath": "/repo/worktrees/active/brevity-rich-task/prompt.md",
			"latestRunWorkerStatus": "succeeded",
			"latestRunProvider": "codex",
			"latestRunProfile": "default",
			"latestRunExitCode": 0
		}],
		"suggestedNextActions": [],
		"orphanedTaskWorktrees": [],
		"activeWorktrees": [],
		"activeWorktreeCount": 0,
		"groups": {}
	}`)
}

func TestRender_TaskRowIncludesWorktreePath(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "/repo/worktrees/active/brevity-rich-task") {
		t.Errorf("output missing worktree path; output:\n%s", out)
	}
}

func TestRender_TaskRowIncludesWorktreePresence(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "(present)") {
		t.Errorf("output missing (present) worktree marker; output:\n%s", out)
	}
}

func TestRender_TaskRowIncludesPromptPath(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "/repo/worktrees/active/brevity-rich-task/prompt.md") {
		t.Errorf("output missing prompt path; output:\n%s", out)
	}
}

func TestRender_TaskRowIncludesLastRunDetail(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	for _, want := range []string{"last-run:", "succeeded", "codex/default", "exit=0"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q in last-run detail; output:\n%s", want, out)
		}
	}
}

func TestRender_TaskWorktreeMissingShowsMissing(t *testing.T) {
	stateWithMissingWorktree := []byte(`{
		"schema": "brevity.runtime-state.v1",
		"repoRoot": "/dev/repos/project",
		"generatedAt": "2026-05-23T10:00:00Z",
		"providers": {"summary": {"total": 0, "degraded": 0, "unavailable": 0}, "health": {}},
		"taskCounts": {"tracked": 1, "runnable": 0, "blocked": 0, "stale": 0, "providerGated": 0, "review": 0},
		"tasks": [{
			"slug": "gone-task",
			"status": "merged",
			"normalizedState": "merged",
			"workerStatus": "",
			"branch": "task/gone-task",
			"worktreePath": "/repo/worktrees/active/brevity-gone-task",
			"worktree": {"exists": false, "path": "/repo/worktrees/active/brevity-gone-task", "branch": "task/gone-task"},
			"promptPath": ""
		}],
		"suggestedNextActions": [],
		"orphanedTaskWorktrees": [],
		"activeWorktrees": [],
		"activeWorktreeCount": 0,
		"groups": {}
	}`)
	snap := cmux.Read(stubFetcher{
		stateJSON:     stateWithMissingWorktree,
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "(missing)") {
		t.Errorf("output missing (missing) marker for absent worktree; output:\n%s", out)
	}
}

func TestRender_TaskNoWorktreePathShowsNone(t *testing.T) {
	// alpha-task in minimalStateJSON has empty worktreePath.
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "worktree: (none)") {
		t.Errorf("output missing 'worktree: (none)' for task with no path; output:\n%s", out)
	}
}

func TestRender_TaskNoPromptPathShowsNone(t *testing.T) {
	// alpha-task in minimalStateJSON has no promptPath.
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "prompt:   (none)") {
		t.Errorf("output missing 'prompt: (none)' for task with no promptPath; output:\n%s", out)
	}
}

func TestRender_MultipleTasksSeparatedByBlankLine(t *testing.T) {
	// minimalStateJSON has two tasks; they must be separated by a blank line.
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	// Find the position of alpha-task and beta-task; confirm there is at
	// least one blank line between their blocks.
	alphaIdx := strings.Index(out, "alpha-task")
	betaIdx := strings.Index(out, "beta-task")
	if alphaIdx < 0 || betaIdx < 0 {
		t.Fatal("could not find both tasks in output")
	}
	between := out[alphaIdx:betaIdx]
	if !strings.Contains(between, "\n\n") {
		t.Errorf("no blank line between alpha-task and beta-task blocks; between:\n%q", between)
	}
}

func TestRender_TaskWorkerStatusFallbackForLastRun(t *testing.T) {
	// beta-task has workerStatus=succeeded but no latestRunWorkerStatus set.
	// The fallback should render workerStatus.
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	// beta-task's workerStatus is "succeeded"; should appear as last-run fallback.
	betaIdx := strings.Index(out, "beta-task")
	if betaIdx < 0 {
		t.Fatal("beta-task not found in output")
	}
	betaBlock := out[betaIdx:]
	if !strings.Contains(betaBlock, "last-run: succeeded") {
		t.Errorf("beta-task missing workerStatus fallback in last-run; beta block:\n%s", betaBlock)
	}
}

func TestRender_SchedulerSelectionReasonAppears(t *testing.T) {
	schedulerWithReason := []byte(`{
		"schema": "brevity.runtime-scheduler-plan.v1",
		"queuePath": ".brevity/runtime-queue.json",
		"queueState": "valid",
		"queueVersion": 1,
		"supportedQueueVersion": 1,
		"selected": {
			"id": "item-xyz",
			"task": "gamma-task",
			"provider": "codex",
			"profile": "default",
			"status": "queued",
			"reason": "only eligible item"
		},
		"reservationEligible": true,
		"reservationEligibility": "eligible",
		"skipped": [],
		"safetyChecks": [],
		"readOnly": true
	}`)
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: schedulerWithReason,
	})
	out := renderSnapshot(snap)
	if !strings.Contains(out, "reason=only eligible item") {
		t.Errorf("output missing selection reason; output:\n%s", out)
	}
}

// --- contracts package round-trip ---------------------------------------

// TestRender_ContractsPackageNotDirectlyUsedInTests confirms that the
// contracts import in the test file is used (for the schema constant check
// in TestRead_ParsesRuntimeState) and the cmux package compiles with its
// new render helpers that accept contracts.TaskSummary.
func TestRender_RichFixtureNoPanic(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	// Render must not panic for any field combination.
	out := renderSnapshot(snap)
	if out == "" {
		t.Error("renderSnapshot returned empty output")
	}
}

// --- RenderOptions: section filtering tests --------------------------------

func TestRender_SectionAll_ContainsAllSections(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{Section: cmux.SectionAll})
	for _, want := range []string{"CMUX OPERATOR", "Providers", "Task Counts", "Queue / Scheduler", "Suggested Next Actions"} {
		if !strings.Contains(out, want) {
			t.Errorf("section=all missing %q", want)
		}
	}
}

func TestRender_SectionProviders_OnlyProviders(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{Section: cmux.SectionProviders})
	if !strings.Contains(out, "Providers") {
		t.Error("section=providers missing Providers heading")
	}
	if strings.Contains(out, "Task Counts") {
		t.Error("section=providers must not contain Task Counts")
	}
	if strings.Contains(out, "Queue / Scheduler") {
		t.Error("section=providers must not contain Queue / Scheduler")
	}
	if strings.Contains(out, "Suggested Next Actions") {
		t.Error("section=providers must not contain Suggested Next Actions")
	}
}

func TestRender_SectionTasks_OnlyTasksSection(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{Section: cmux.SectionTasks})
	if !strings.Contains(out, "Task Counts") {
		t.Error("section=tasks missing Task Counts")
	}
	if !strings.Contains(out, "Task List") {
		t.Error("section=tasks missing Task List")
	}
	if strings.Contains(out, "Providers") {
		t.Error("section=tasks must not contain Providers")
	}
	if strings.Contains(out, "Queue / Scheduler") {
		t.Error("section=tasks must not contain Queue / Scheduler")
	}
	if strings.Contains(out, "Suggested Next Actions") {
		t.Error("section=tasks must not contain Suggested Next Actions")
	}
}

func TestRender_SectionQueue_OnlyQueueScheduler(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{Section: cmux.SectionQueue})
	if !strings.Contains(out, "Queue / Scheduler") {
		t.Error("section=queue missing Queue / Scheduler")
	}
	if strings.Contains(out, "Task Counts") {
		t.Error("section=queue must not contain Task Counts")
	}
	if strings.Contains(out, "Providers") {
		t.Error("section=queue must not contain Providers")
	}
	if strings.Contains(out, "Suggested Next Actions") {
		t.Error("section=queue must not contain Suggested Next Actions")
	}
}

func TestRender_SectionActions_OnlySuggestedActions(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{Section: cmux.SectionActions})
	if !strings.Contains(out, "Suggested Next Actions") {
		t.Error("section=actions missing Suggested Next Actions")
	}
	if strings.Contains(out, "Providers") {
		t.Error("section=actions must not contain Providers")
	}
	if strings.Contains(out, "Task Counts") {
		t.Error("section=actions must not contain Task Counts")
	}
	if strings.Contains(out, "Queue / Scheduler") {
		t.Error("section=actions must not contain Queue / Scheduler")
	}
}

func TestRender_SectionEmptyString_RendersAll(t *testing.T) {
	// Empty Section is equivalent to "all".
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{})
	for _, want := range []string{"Providers", "Task Counts", "Queue / Scheduler", "Suggested Next Actions"} {
		if !strings.Contains(out, want) {
			t.Errorf("empty section missing %q", want)
		}
	}
}

// --- RenderOptions: limit tests --------------------------------------------

// manyTaskStateJSON returns a runtime-state fixture with n tasks named task-1 … task-N.
func manyTaskStateJSON(n int) []byte {
	taskItems := make([]string, n)
	for i := 0; i < n; i++ {
		taskItems[i] = fmt.Sprintf(`{"slug":"task-%d","status":"ready-for-worker","normalizedState":"ready-for-worker","workerStatus":"","branch":"task/task-%d","worktreePath":""}`, i+1, i+1)
	}
	return []byte(`{
		"schema": "brevity.runtime-state.v1",
		"repoRoot": "/dev/test",
		"generatedAt": "2026-01-01T00:00:00Z",
		"providers": {"summary": {"total": 0, "degraded": 0, "unavailable": 0}, "health": {}},
		"taskCounts": {"tracked": ` + fmt.Sprintf("%d", n) + `, "runnable": 0, "blocked": 0, "stale": 0, "providerGated": 0, "review": 0},
		"tasks": [` + strings.Join(taskItems, ",") + `],
		"suggestedNextActions": [],
		"orphanedTaskWorktrees": [],
		"activeWorktrees": [],
		"activeWorktreeCount": 0,
		"groups": {}
	}`)
}

func TestRender_LimitReducesTaskList(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     manyTaskStateJSON(5),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{Limit: 2})
	if !strings.Contains(out, "task-1") {
		t.Error("limit=2 missing task-1")
	}
	if !strings.Contains(out, "task-2") {
		t.Error("limit=2 missing task-2")
	}
	if strings.Contains(out, "task-3") {
		t.Error("limit=2 should not contain task-3")
	}
	if !strings.Contains(out, "showing 2 of 5") {
		t.Errorf("limit=2 missing truncation header; output:\n%s", out)
	}
}

func TestRender_LimitDefaultTenShowsAll_WhenFewer(t *testing.T) {
	// minimalStateJSON has 2 tasks; default limit=10 should show both without truncation.
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshot(snap)
	if strings.Contains(out, "showing") {
		t.Error("2 tasks with default limit should not show truncation header")
	}
}

func TestRender_LimitDefaultTenTruncatesAt11(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     manyTaskStateJSON(11),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{}) // default limit = 10
	if !strings.Contains(out, "showing 10 of 11") {
		t.Errorf("11 tasks with default limit missing truncation header; output:\n%s", out)
	}
	if strings.Contains(out, "task-11") {
		t.Error("11th task should be hidden at default limit=10")
	}
}

func TestRender_LimitZeroUsesDefault(t *testing.T) {
	// Limit=0 should behave identically to Limit=10.
	snap := cmux.Read(stubFetcher{
		stateJSON:     manyTaskStateJSON(11),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{Limit: 0})
	if !strings.Contains(out, "showing 10 of 11") {
		t.Errorf("Limit=0 should fall back to DefaultLimit=10; output:\n%s", out)
	}
}

func TestRender_LimitExact_NoTruncationHeader(t *testing.T) {
	// Limit exactly equals the number of tasks — no truncation header.
	snap := cmux.Read(stubFetcher{
		stateJSON:     manyTaskStateJSON(3),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{Limit: 3})
	if strings.Contains(out, "showing") {
		t.Errorf("limit=3 with 3 tasks should not show truncation header; output:\n%s", out)
	}
}

func TestRender_SectionTasks_WithLimit(t *testing.T) {
	// Section=tasks combined with Limit=1 should limit the task list.
	snap := cmux.Read(stubFetcher{
		stateJSON:     manyTaskStateJSON(5),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{Section: cmux.SectionTasks, Limit: 1})
	if !strings.Contains(out, "task-1") {
		t.Error("section=tasks limit=1 missing task-1")
	}
	if strings.Contains(out, "task-2") {
		t.Error("section=tasks limit=1 should not contain task-2")
	}
	if !strings.Contains(out, "showing 1 of 5") {
		t.Errorf("section=tasks limit=1 missing truncation header; output:\n%s", out)
	}
}

// --- RenderOptions: task filtering tests -----------------------------------

// multiStateJSON returns a runtime-state fixture with four tasks covering
// different normalised states, used to exercise task-level filters.
// suggestedNextActions deliberately does not mention any task slugs so that
// "must not contain slug" assertions on the full output remain valid.
func multiStateJSON() []byte {
	return []byte(`{
		"schema": "brevity.runtime-state.v1",
		"repoRoot": "/dev/test",
		"generatedAt": "2026-01-01T00:00:00Z",
		"providers": {
			"summary": {"total": 1, "degraded": 0, "unavailable": 0},
			"health": {"codex": {"status": "healthy", "updatedAt": "", "note": ""}}
		},
		"taskCounts": {"tracked": 4, "runnable": 1, "blocked": 1, "stale": 0, "providerGated": 0, "review": 1},
		"tasks": [
			{"slug": "task-ready",    "status": "ready-for-worker", "normalizedState": "ready-for-worker", "workerStatus": "",          "branch": "task/task-ready",    "worktreePath": ""},
			{"slug": "task-review",   "status": "reviewing",        "normalizedState": "reviewing",        "workerStatus": "succeeded", "branch": "task/task-review",   "worktreePath": ""},
			{"slug": "task-blocked",  "status": "blocked",          "normalizedState": "blocked",          "workerStatus": "",          "branch": "task/task-blocked",  "worktreePath": ""},
			{"slug": "task-merged",   "status": "merged",           "normalizedState": "merged",           "workerStatus": "succeeded", "branch": "task/task-merged",   "worktreePath": ""}
		],
		"suggestedNextActions": ["Review your task status."],
		"orphanedTaskWorktrees": [],
		"activeWorktrees": [],
		"activeWorktreeCount": 0,
		"groups": {}
	}`)
}

func TestFilter_TaskSlug_Found(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     multiStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{TaskSlug: "task-review"})
	if !strings.Contains(out, "task-review") {
		t.Error("slug filter missing task-review")
	}
	if strings.Contains(out, "task-ready") {
		t.Error("slug filter must not contain task-ready")
	}
	if strings.Contains(out, "task-blocked") {
		t.Error("slug filter must not contain task-blocked")
	}
	if strings.Contains(out, "task-merged") {
		t.Error("slug filter must not contain task-merged")
	}
}

func TestFilter_TaskSlug_NotFound(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     multiStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{TaskSlug: "no-such-task"})
	if !strings.Contains(out, `"no-such-task" not found`) {
		t.Errorf("missing not-found message; output:\n%s", out)
	}
	// Task List section must still be present.
	if !strings.Contains(out, "Task List") {
		t.Error("Task List heading must still appear for not-found slug")
	}
}

func TestFilter_TaskSlug_EmptyTaskStore(t *testing.T) {
	// When the task store is empty (no tasks), slug filter still shows "none tracked".
	emptyState := []byte(`{
		"schema": "brevity.runtime-state.v1",
		"repoRoot": "/dev/test",
		"generatedAt": "2026-01-01T00:00:00Z",
		"providers": {"summary": {"total": 0, "degraded": 0, "unavailable": 0}, "health": {}},
		"taskCounts": {"tracked": 0, "runnable": 0, "blocked": 0, "stale": 0, "providerGated": 0, "review": 0},
		"tasks": [],
		"suggestedNextActions": [],
		"orphanedTaskWorktrees": [],
		"activeWorktrees": [],
		"activeWorktreeCount": 0,
		"groups": {}
	}`)
	snap := cmux.Read(stubFetcher{
		stateJSON:     emptyState,
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{TaskSlug: "anything"})
	if !strings.Contains(out, "Task List: none tracked") {
		t.Errorf("empty store with slug filter should show 'none tracked'; output:\n%s", out)
	}
}

func TestFilter_StateFilter_MatchesSome(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     multiStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{StateFilter: "reviewing"})
	if !strings.Contains(out, "task-review") {
		t.Error("state=reviewing missing task-review")
	}
	if strings.Contains(out, "task-ready") {
		t.Error("state=reviewing must not contain task-ready")
	}
	if strings.Contains(out, "task-blocked") {
		t.Error("state=reviewing must not contain task-blocked")
	}
}

func TestFilter_StateFilter_CaseInsensitive(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     multiStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{StateFilter: "REVIEWING"})
	if !strings.Contains(out, "task-review") {
		t.Errorf("case-insensitive state match failed; output:\n%s", out)
	}
}

func TestFilter_StateFilter_NoMatch(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     multiStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{StateFilter: "stale"})
	if !strings.Contains(out, `"stale"`) {
		t.Errorf("no-match state filter missing helpful message; output:\n%s", out)
	}
	if !strings.Contains(out, "Task List") {
		t.Error("Task List heading must still appear for no-match state filter")
	}
	if strings.Contains(out, "task-ready") {
		t.Error("no-match state filter must not show any tasks")
	}
}

func TestFilter_TaskAndState_BothMatch(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     multiStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		TaskSlug:    "task-review",
		StateFilter: "reviewing",
	})
	if !strings.Contains(out, "task-review") {
		t.Error("combined filter missing task-review when both match")
	}
	if strings.Contains(out, "task-ready") {
		t.Error("combined filter must not contain task-ready")
	}
}

func TestFilter_TaskAndState_SlugFoundButStateMismatch(t *testing.T) {
	// task-review has state "reviewing" — asking for state "blocked" should yield no match.
	snap := cmux.Read(stubFetcher{
		stateJSON:     multiStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		TaskSlug:    "task-review",
		StateFilter: "blocked",
	})
	// "task-review" will appear inside the "no tasks matching task=..." message, so
	// we cannot assert its absence from the full output.  Instead confirm that no
	// task-row detail lines (worktree/prompt/last-run) were rendered.
	if strings.Contains(out, "worktree:") {
		t.Error("slug+state mismatch must not render any task detail rows (worktree line found)")
	}
	if strings.Contains(out, "last-run:") {
		t.Error("slug+state mismatch must not render any task detail rows (last-run line found)")
	}
	if !strings.Contains(out, "Task List") {
		t.Error("Task List heading must still appear")
	}
	if !strings.Contains(out, "no tasks matching") {
		t.Errorf("combined mismatch missing 'no tasks matching' message; output:\n%s", out)
	}
}

func TestFilter_DoesNotAffectProviderSection(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     multiStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	// Even with a non-matching slug, Providers should still appear in section=all.
	out := renderSnapshotOpts(snap, cmux.RenderOptions{TaskSlug: "no-such-task"})
	if !strings.Contains(out, "Providers") {
		t.Error("task slug filter must not suppress Providers section")
	}
	if !strings.Contains(out, "codex") {
		t.Error("task slug filter must not suppress provider rows")
	}
}

func TestFilter_DoesNotAffectQueueSection(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     multiStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{StateFilter: "stale"})
	if !strings.Contains(out, "Queue / Scheduler") {
		t.Error("state filter must not suppress Queue / Scheduler section")
	}
}

func TestFilter_DoesNotAffectActionsSection(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     multiStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{StateFilter: "stale"})
	if !strings.Contains(out, "Suggested Next Actions") {
		t.Error("state filter must not suppress Suggested Next Actions section")
	}
}

func TestFilter_LimitAppliesAfterFilter(t *testing.T) {
	// Build a state with 4 "ready-for-worker" tasks and 1 "reviewing" task.
	// Filter state=ready-for-worker with limit=2 → show 2 of the 4 matching tasks.
	//
	// Note: slug "v1" is avoided because "v1" is a substring of "review=1" that
	// appears in the Task Counts line, which would cause false-positive Contains
	// matches.  "revw-task" is used instead.
	state := []byte(`{
		"schema": "brevity.runtime-state.v1",
		"repoRoot": "/dev/test",
		"generatedAt": "2026-01-01T00:00:00Z",
		"providers": {"summary": {"total": 0, "degraded": 0, "unavailable": 0}, "health": {}},
		"taskCounts": {"tracked": 5, "runnable": 4, "blocked": 0, "stale": 0, "providerGated": 0, "review": 1},
		"tasks": [
			{"slug": "rfw-alpha", "status": "ready-for-worker", "normalizedState": "ready-for-worker", "workerStatus": "", "branch": "task/rfw-alpha", "worktreePath": ""},
			{"slug": "rfw-beta",  "status": "ready-for-worker", "normalizedState": "ready-for-worker", "workerStatus": "", "branch": "task/rfw-beta",  "worktreePath": ""},
			{"slug": "rfw-gamma", "status": "ready-for-worker", "normalizedState": "ready-for-worker", "workerStatus": "", "branch": "task/rfw-gamma", "worktreePath": ""},
			{"slug": "rfw-delta", "status": "ready-for-worker", "normalizedState": "ready-for-worker", "workerStatus": "", "branch": "task/rfw-delta", "worktreePath": ""},
			{"slug": "revw-task", "status": "reviewing",        "normalizedState": "reviewing",        "workerStatus": "", "branch": "task/revw-task", "worktreePath": ""}
		],
		"suggestedNextActions": [],
		"orphanedTaskWorktrees": [],
		"activeWorktrees": [],
		"activeWorktreeCount": 0,
		"groups": {}
	}`)
	snap := cmux.Read(stubFetcher{
		stateJSON:     state,
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		StateFilter: "ready-for-worker",
		Limit:       2,
	})
	// 4 tasks match the filter; limit=2 shows first 2.
	if !strings.Contains(out, "showing 2 of 4") {
		t.Errorf("limit after filter expected 'showing 2 of 4'; output:\n%s", out)
	}
	if strings.Contains(out, "rfw-gamma") {
		t.Error("limit after filter must not show rfw-gamma (3rd match)")
	}
	// The reviewing task must never appear in the task list.
	if strings.Contains(out, "revw-task") {
		t.Error("state filter must exclude revw-task (reviewing)")
	}
}

func TestFilter_SectionTasks_WithSlugFilter(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     multiStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		Section:  cmux.SectionTasks,
		TaskSlug: "task-blocked",
	})
	if !strings.Contains(out, "task-blocked") {
		t.Error("section=tasks slug filter missing task-blocked")
	}
	if strings.Contains(out, "task-ready") {
		t.Error("section=tasks slug filter must not contain task-ready")
	}
	// Provider and queue sections must be absent.
	if strings.Contains(out, "Providers") {
		t.Error("section=tasks must not contain Providers")
	}
	if strings.Contains(out, "Queue / Scheduler") {
		t.Error("section=tasks must not contain Queue / Scheduler")
	}
}

// --- OutputMode: markdown rendering tests ----------------------------------

// markdownOpts is a shorthand for default markdown options.
func markdownOpts() cmux.RenderOptions {
	return cmux.RenderOptions{Output: cmux.OutputMarkdown}
}

func TestMarkdown_H1HeaderAtStart(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, markdownOpts())
	if !strings.HasPrefix(out, "# CMUX") {
		t.Errorf("markdown must start with # CMUX; actual prefix: %q", out[:min(len(out), 30)])
	}
}

func TestMarkdown_AllH2SectionsPresent(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, markdownOpts())
	for _, want := range []string{
		"## Providers",
		"## Task Counts",
		"## Task List",
		"## Queue / Scheduler",
		"## Suggested Next Actions",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown missing heading %q; output:\n%s", want, out)
		}
	}
}

func TestMarkdown_TaskH3Headings(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, markdownOpts())
	if !strings.Contains(out, "### alpha-task") {
		t.Errorf("markdown missing ### alpha-task; output:\n%s", out)
	}
	if !strings.Contains(out, "### beta-task") {
		t.Errorf("markdown missing ### beta-task; output:\n%s", out)
	}
}

func TestMarkdown_TaskDetail_BulletListItems(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: richTaskStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, markdownOpts())
	for _, want := range []string{
		"- **Worktree:**",
		"- **Prompt:**",
		"- **Last Run:**",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown task detail missing bullet %q; output:\n%s", want, out)
		}
	}
}

func TestMarkdown_TaskDetail_StateIsBold(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: richTaskStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, markdownOpts())
	if !strings.Contains(out, "**State:** ready-for-worker") {
		t.Errorf("markdown task state not bold or wrong value; output:\n%s", out)
	}
}

func TestMarkdown_ProviderTable_WithProviders(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: richProviderStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, markdownOpts())
	if !strings.Contains(out, "| Provider |") {
		t.Errorf("markdown providers missing table header row; output:\n%s", out)
	}
	if !strings.Contains(out, "|---|") {
		t.Errorf("markdown providers missing table separator row; output:\n%s", out)
	}
	if !strings.Contains(out, "| codex |") {
		t.Errorf("markdown providers missing codex table row; output:\n%s", out)
	}
	if !strings.Contains(out, "| gemini |") {
		t.Errorf("markdown providers missing gemini table row; output:\n%s", out)
	}
}

func TestMarkdown_ProviderTable_EmptyProviders_ItalicFallback(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: manyTaskStateJSON(0), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, markdownOpts())
	if !strings.Contains(out, "_No providers tracked._") {
		t.Errorf("markdown empty providers missing italic fallback; output:\n%s", out)
	}
	// Must not emit a table header for empty providers.
	if strings.Contains(out, "| Provider |") {
		t.Error("markdown empty providers must not emit table header")
	}
}

func TestMarkdown_EmptyTaskList_ItalicFallback(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: manyTaskStateJSON(0), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, markdownOpts())
	if !strings.Contains(out, "_No tasks tracked._") {
		t.Errorf("markdown empty task list missing italic fallback; output:\n%s", out)
	}
}

func TestMarkdown_EmptyActions_ItalicFallback(t *testing.T) {
	// manyTaskStateJSON has empty suggestedNextActions.
	snap := cmux.Read(stubFetcher{stateJSON: manyTaskStateJSON(0), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, markdownOpts())
	if !strings.Contains(out, "_No actions suggested._") {
		t.Errorf("markdown empty actions missing italic fallback; output:\n%s", out)
	}
}

func TestMarkdown_SlugFilter_NotFound_ItalicMessage(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: multiStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		Output:   cmux.OutputMarkdown,
		TaskSlug: "no-such-task",
	})
	if !strings.Contains(out, `_Task "no-such-task" not found._`) {
		t.Errorf("markdown slug not-found missing italic message; output:\n%s", out)
	}
}

func TestMarkdown_StateFilter_NoMatch_ItalicMessage(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: multiStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		Output:      cmux.OutputMarkdown,
		StateFilter: "stale",
	})
	if !strings.Contains(out, `_No tasks with state "stale"._`) {
		t.Errorf("markdown state no-match missing italic message; output:\n%s", out)
	}
}

func TestMarkdown_SlugFilter_Found_ShowsOnlyThatTask(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: multiStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		Output:   cmux.OutputMarkdown,
		TaskSlug: "task-review",
	})
	if !strings.Contains(out, "### task-review") {
		t.Errorf("markdown slug filter missing ### task-review; output:\n%s", out)
	}
	if strings.Contains(out, "### task-ready") {
		t.Error("markdown slug filter must not contain ### task-ready")
	}
	if strings.Contains(out, "### task-blocked") {
		t.Error("markdown slug filter must not contain ### task-blocked")
	}
}

func TestMarkdown_StateFilter_ShowsMatchingTaskH3(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: multiStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		Output:      cmux.OutputMarkdown,
		StateFilter: "reviewing",
	})
	if !strings.Contains(out, "### task-review") {
		t.Errorf("markdown state filter missing ### task-review; output:\n%s", out)
	}
	if strings.Contains(out, "### task-ready") {
		t.Error("markdown state filter must not contain ### task-ready")
	}
}

func TestMarkdown_Limit_ShowsInH2Heading(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: manyTaskStateJSON(5), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{Output: cmux.OutputMarkdown, Limit: 2})
	if !strings.Contains(out, "## Task List (showing 2 of 5)") {
		t.Errorf("markdown limit missing truncation in heading; output:\n%s", out)
	}
	if strings.Contains(out, "### task-3") {
		t.Error("markdown limit must not show task-3")
	}
}

func TestMarkdown_SectionProviders_NoTaskH3(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		Output:  cmux.OutputMarkdown,
		Section: cmux.SectionProviders,
	})
	if !strings.Contains(out, "## Providers") {
		t.Error("markdown section=providers missing ## Providers")
	}
	if strings.Contains(out, "### ") {
		t.Error("markdown section=providers must not contain any ### task headings")
	}
	if strings.Contains(out, "## Task") {
		t.Error("markdown section=providers must not contain ## Task heading")
	}
}

func TestMarkdown_SectionTasks_NoProviderTable(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: richProviderStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		Output:  cmux.OutputMarkdown,
		Section: cmux.SectionTasks,
	})
	if !strings.Contains(out, "## Task") {
		t.Error("markdown section=tasks missing ## Task heading")
	}
	if strings.Contains(out, "## Providers") {
		t.Error("markdown section=tasks must not contain ## Providers")
	}
	if strings.Contains(out, "| Provider |") {
		t.Error("markdown section=tasks must not contain provider table")
	}
}

func TestMarkdown_SectionQueue_OnlyQueueH2(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		Output:  cmux.OutputMarkdown,
		Section: cmux.SectionQueue,
	})
	if !strings.Contains(out, "## Queue / Scheduler") {
		t.Error("markdown section=queue missing ## Queue / Scheduler")
	}
	if strings.Contains(out, "## Providers") {
		t.Error("markdown section=queue must not contain ## Providers")
	}
	if strings.Contains(out, "## Task") {
		t.Error("markdown section=queue must not contain ## Task heading")
	}
}

func TestMarkdown_SectionActions_OnlyActionsH2(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		Output:  cmux.OutputMarkdown,
		Section: cmux.SectionActions,
	})
	if !strings.Contains(out, "## Suggested Next Actions") {
		t.Error("markdown section=actions missing ## Suggested Next Actions")
	}
	if strings.Contains(out, "## Providers") {
		t.Error("markdown section=actions must not contain ## Providers")
	}
	if strings.Contains(out, "## Task") {
		t.Error("markdown section=actions must not contain ## Task heading")
	}
}

func TestMarkdown_RuntimeStateError_Rendered(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateErr:      errors.New("runtime unavailable"),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, markdownOpts())
	if !strings.Contains(out, "runtime-state error:") {
		t.Errorf("markdown runtime-state error not displayed; output:\n%s", out)
	}
	if !strings.Contains(out, "runtime unavailable") {
		t.Errorf("markdown runtime-state error message missing; output:\n%s", out)
	}
}

func TestMarkdown_NoANSISequences(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, markdownOpts())
	if strings.Contains(out, "\x1b[") {
		t.Error("markdown output contains ANSI escape sequences")
	}
}

func TestMarkdown_Deterministic(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	opts := markdownOpts()
	out1 := renderSnapshotOpts(snap, opts)
	out2 := renderSnapshotOpts(snap, opts)
	if out1 != out2 {
		t.Error("markdown Render is not deterministic: two calls produced different output")
	}
}

func TestMarkdown_OutputTextDefault_NoMarkdownHeadings(t *testing.T) {
	// OutputText (the default) must not produce markdown headings.
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshot(snap) // uses RenderOptions{} — zero value = text
	if strings.Contains(out, "# CMUX") {
		t.Error("text output must not contain # CMUX markdown heading")
	}
	if strings.Contains(out, "## Providers") {
		t.Error("text output must not contain ## Providers markdown heading")
	}
}

func TestMarkdown_SchedulerSelection_InQueueSection(t *testing.T) {
	schedulerWithSelection := []byte(`{
		"schema": "brevity.runtime-scheduler-plan.v1",
		"queuePath": ".brevity/runtime-queue.json",
		"queueState": "valid",
		"queueVersion": 1,
		"supportedQueueVersion": 1,
		"selected": {
			"id": "abc-123",
			"task": "alpha-task",
			"provider": "",
			"profile": "",
			"status": "queued",
			"reason": "first eligible item"
		},
		"reservationEligible": true,
		"reservationEligibility": "eligible",
		"skipped": [],
		"safetyChecks": [],
		"readOnly": true
	}`)
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: schedulerWithSelection})
	out := renderSnapshotOpts(snap, markdownOpts())
	if !strings.Contains(out, "**Scheduler Next:**") {
		t.Errorf("markdown queue section missing **Scheduler Next:** label; output:\n%s", out)
	}
	if !strings.Contains(out, "abc-123") {
		t.Errorf("markdown queue section missing selected item id; output:\n%s", out)
	}
}

// min is used only in tests to safely cap string indexing.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// extractSection returns the text between the first occurrence of startMarker
// and the next occurrence of endMarker (exclusive).  Used for targeted
// section assertions.
func extractSection(full, startMarker, endMarker string) string {
	start := strings.Index(full, startMarker)
	if start < 0 {
		return ""
	}
	rest := full[start:]
	end := strings.Index(rest, endMarker)
	if end < 0 {
		return rest
	}
	return rest[:end]
}

// sectionSep is the separator used by the renderer; duplicated here so
// extractSection can reference it without importing render internals.
const sectionSep = "---"

// Compile-time check: the test file uses contracts only for the schema
// constant in TestRead_ParsesRuntimeState; keep the import live.
var _ = contracts.RuntimeStateSchema

// --- OutputMode: JSON rendering tests -------------------------------------

// jsonOpts is a shorthand for default JSON options.
func jsonOpts() cmux.RenderOptions {
	return cmux.RenderOptions{Output: cmux.OutputJSON}
}

func TestJSON_ValidJSON(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, jsonOpts())
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("--output json produced invalid JSON: %v\noutput:\n%s", err, out)
	}
}

func TestJSON_Schema(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, jsonOpts())
	if !strings.Contains(out, `"brevity.cmux-report.v1"`) {
		t.Errorf("JSON output missing schema field; output:\n%s", out)
	}
}

func TestJSON_SourceNative(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, jsonOpts())
	if !strings.Contains(out, `"source": "native"`) {
		t.Errorf("JSON output missing source=native; output:\n%s", out)
	}
}

func TestJSON_ErrorsEmptyArray_WhenNoErrors(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, jsonOpts())
	if !strings.Contains(out, `"errors": []`) {
		t.Errorf("JSON errors must be [] when no errors; output:\n%s", out)
	}
}

func TestJSON_ErrorsContainMessage_WhenRuntimeStateFails(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateErr:      errors.New("runtime unavailable"),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, jsonOpts())
	if !strings.Contains(out, "runtime-state:") {
		t.Errorf("JSON errors missing runtime-state error; output:\n%s", out)
	}
	if !strings.Contains(out, "runtime unavailable") {
		t.Errorf("JSON errors missing error message; output:\n%s", out)
	}
}

func TestJSON_SectionAll_AllTopLevelKeysPresent(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, jsonOpts())
	for _, key := range []string{`"providers"`, `"tasks"`, `"queue"`, `"actions"`} {
		if !strings.Contains(out, key) {
			t.Errorf("JSON section=all missing key %s; output:\n%s", key, out)
		}
	}
}

func TestJSON_SectionProviders_OnlyProviders(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		Output:  cmux.OutputJSON,
		Section: cmux.SectionProviders,
	})
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := result["providers"]; !ok {
		t.Error("JSON section=providers missing providers key")
	}
	if _, ok := result["tasks"]; ok {
		t.Error("JSON section=providers must not contain tasks key")
	}
	if _, ok := result["queue"]; ok {
		t.Error("JSON section=providers must not contain queue key")
	}
	if _, ok := result["actions"]; ok {
		t.Error("JSON section=providers must not contain actions key")
	}
}

func TestJSON_SectionTasks_OnlyTasks(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		Output:  cmux.OutputJSON,
		Section: cmux.SectionTasks,
	})
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := result["tasks"]; !ok {
		t.Error("JSON section=tasks missing tasks key")
	}
	if _, ok := result["providers"]; ok {
		t.Error("JSON section=tasks must not contain providers key")
	}
	if _, ok := result["queue"]; ok {
		t.Error("JSON section=tasks must not contain queue key")
	}
}

func TestJSON_SectionQueue_OnlyQueue(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		Output:  cmux.OutputJSON,
		Section: cmux.SectionQueue,
	})
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := result["queue"]; !ok {
		t.Error("JSON section=queue missing queue key")
	}
	if _, ok := result["providers"]; ok {
		t.Error("JSON section=queue must not contain providers key")
	}
	if _, ok := result["tasks"]; ok {
		t.Error("JSON section=queue must not contain tasks key")
	}
}

func TestJSON_SectionActions_OnlyActions(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		Output:  cmux.OutputJSON,
		Section: cmux.SectionActions,
	})
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := result["actions"]; !ok {
		t.Error("JSON section=actions missing actions key")
	}
	if _, ok := result["providers"]; ok {
		t.Error("JSON section=actions must not contain providers key")
	}
	if _, ok := result["tasks"]; ok {
		t.Error("JSON section=actions must not contain tasks key")
	}
}

func TestJSON_ProvidersEmptyArray_WhenNoProviders(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: manyTaskStateJSON(0), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, jsonOpts())
	if !strings.Contains(out, `"providers": []`) {
		t.Errorf("JSON providers.providers must be [] not null for empty providers; output:\n%s", out)
	}
}

func TestJSON_TasksEmptyArray_WhenNoTasks(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: manyTaskStateJSON(0), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, jsonOpts())
	if !strings.Contains(out, `"tasks": []`) {
		t.Errorf("JSON tasks.tasks must be [] not null for empty tasks; output:\n%s", out)
	}
}

func TestJSON_ActionsEmptyArray_WhenNoActions(t *testing.T) {
	// manyTaskStateJSON(0) has empty suggestedNextActions.
	snap := cmux.Read(stubFetcher{stateJSON: manyTaskStateJSON(0), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, jsonOpts())
	if !strings.Contains(out, `"actions": []`) {
		t.Errorf("JSON actions must be [] not null when no actions; output:\n%s", out)
	}
}

func TestJSON_TaskSlugFilter_OnlyMatchingTask(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: multiStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		Output:   cmux.OutputJSON,
		TaskSlug: "task-review",
	})
	if !strings.Contains(out, `"task-review"`) {
		t.Errorf("JSON slug filter missing task-review; output:\n%s", out)
	}
	if strings.Contains(out, `"task-ready"`) {
		t.Error("JSON slug filter must not contain task-ready")
	}
	if strings.Contains(out, `"task-blocked"`) {
		t.Error("JSON slug filter must not contain task-blocked")
	}
}

func TestJSON_StateFilter_OnlyMatchingState(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: multiStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		Output:      cmux.OutputJSON,
		StateFilter: "reviewing",
	})
	if !strings.Contains(out, `"task-review"`) {
		t.Errorf("JSON state filter missing task-review; output:\n%s", out)
	}
	if strings.Contains(out, `"task-ready"`) {
		t.Error("JSON state filter must not contain task-ready")
	}
}

func TestJSON_Limit_AppliedToTasks(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: manyTaskStateJSON(5), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{Output: cmux.OutputJSON, Limit: 2})
	if !strings.Contains(out, `"task-1"`) {
		t.Error("JSON limit=2 missing task-1")
	}
	if !strings.Contains(out, `"task-2"`) {
		t.Error("JSON limit=2 missing task-2")
	}
	if strings.Contains(out, `"task-3"`) {
		t.Error("JSON limit=2 must not contain task-3")
	}
	// shown=2, matched=5
	if !strings.Contains(out, `"shown": 2`) {
		t.Errorf("JSON limit=2 missing shown=2; output:\n%s", out)
	}
	if !strings.Contains(out, `"matched": 5`) {
		t.Errorf("JSON limit=2 missing matched=5; output:\n%s", out)
	}
}

func TestJSON_RichTask_DetailFields(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: richTaskStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, jsonOpts())
	for _, want := range []string{
		`"rich-task"`,
		`"ready-for-worker"`,
		`/repo/worktrees/active/brevity-rich-task`,
		`"present"`,
		`prompt.md`,
		`"succeeded"`,
		`"codex"`,
		`"default"`,
		`"0"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("JSON rich task missing %q; output:\n%s", want, out)
		}
	}
}

func TestJSON_NoMarkdownHeadings(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, jsonOpts())
	if strings.Contains(out, "# CMUX") || strings.Contains(out, "## ") || strings.Contains(out, "### ") {
		t.Error("JSON output must not contain markdown headings")
	}
}

func TestJSON_NoANSISequences(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, jsonOpts())
	if strings.Contains(out, "\x1b[") {
		t.Error("JSON output contains ANSI escape sequences")
	}
}

func TestJSON_Deterministic(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	opts := jsonOpts()
	out1 := renderSnapshotOpts(snap, opts)
	out2 := renderSnapshotOpts(snap, opts)
	if out1 != out2 {
		t.Error("JSON Render is not deterministic: two calls produced different output")
	}
}

func TestJSON_ProvidersSortedByName(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: richProviderStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, jsonOpts())
	codexIdx := strings.Index(out, `"codex"`)
	geminiIdx := strings.Index(out, `"gemini"`)
	if codexIdx < 0 || geminiIdx < 0 {
		t.Fatalf("JSON providers missing codex or gemini; output:\n%s", out)
	}
	if codexIdx > geminiIdx {
		t.Error("JSON providers must be sorted: codex should appear before gemini")
	}
}

func TestJSON_OutputTextDefault_NotJSON(t *testing.T) {
	// OutputText (the default) must not produce JSON.
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshot(snap)
	if strings.Contains(out, `"schema"`) && strings.Contains(out, `"brevity.cmux-report.v1"`) {
		t.Error("text output must not contain JSON cmux-report schema")
	}
}

func TestJSON_RuntimeStateError_ReflectedInErrors(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateErr:      errors.New("state file missing"),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, jsonOpts())
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON on state error: %v\noutput:\n%s", err, out)
	}
	errList, _ := result["errors"].([]any)
	if len(errList) == 0 {
		t.Errorf("JSON errors must be non-empty when runtime state fails; output:\n%s", out)
	}
}

// --- review-packet mode tests -----------------------------------------------

// reviewOpts returns RenderOptions for text review mode.
func reviewOpts(slug string) cmux.RenderOptions {
	return cmux.RenderOptions{ReviewTask: slug}
}

// reviewMarkdownOpts returns RenderOptions for markdown review mode.
func reviewMarkdownOpts(slug string) cmux.RenderOptions {
	return cmux.RenderOptions{ReviewTask: slug, Output: cmux.OutputMarkdown}
}

// reviewJSONOpts returns RenderOptions for JSON review mode.
func reviewJSONOpts(slug string) cmux.RenderOptions {
	return cmux.RenderOptions{ReviewTask: slug, Output: cmux.OutputJSON}
}

func TestReview_Text_TaskNotFound(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewOpts("no-such-task"))
	if !strings.Contains(out, `"no-such-task" not found`) {
		t.Errorf("review text missing not-found message; output:\n%s", out)
	}
	// Must not show any unrelated task detail.
	if strings.Contains(out, "alpha-task") || strings.Contains(out, "beta-task") {
		t.Error("review text must not show unrelated tasks when target not found")
	}
}

func TestReview_Markdown_TaskNotFound(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewMarkdownOpts("no-such-task"))
	if !strings.Contains(out, `"no-such-task" not found`) {
		t.Errorf("review markdown missing not-found message; output:\n%s", out)
	}
}

func TestReview_JSON_TaskNotFound(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewJSONOpts("no-such-task"))
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("review JSON task-not-found produced invalid JSON: %v\noutput:\n%s", err, out)
	}
	// errors list must mention the slug.
	if !strings.Contains(out, "no-such-task") {
		t.Errorf("review JSON errors missing task slug; output:\n%s", out)
	}
	// task key must be absent when not found.
	if _, ok := result["task"]; ok {
		t.Error("review JSON must not have task key when task not found")
	}
}

func TestReview_Text_TaskFound_ContainsTaskInfo(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewOpts("rich-task"))
	if !strings.Contains(out, "CMUX REVIEW PACKET") {
		t.Error("review text missing CMUX REVIEW PACKET header")
	}
	if !strings.Contains(out, "review-task: rich-task") {
		t.Errorf("review text missing review-task slug line; output:\n%s", out)
	}
	if !strings.Contains(out, "rich-task") {
		t.Error("review text missing task slug")
	}
	if !strings.Contains(out, "ready-for-worker") {
		t.Error("review text missing task state")
	}
	if !strings.Contains(out, "[read-only]") {
		t.Error("review text missing [read-only] marker")
	}
}

func TestReview_Text_TaskFound_NoUnrelatedTasks(t *testing.T) {
	// minimalStateJSON has alpha-task and beta-task; review for alpha-task must
	// not show beta-task detail rows.
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewOpts("alpha-task"))
	if !strings.Contains(out, "alpha-task") {
		t.Error("review text missing alpha-task (the review target)")
	}
	// beta-task must not appear in task-detail rows.
	// It may appear in queue/scheduler context, so check only task rows.
	if strings.Contains(out, "Task: beta-task") {
		t.Error("review text must not show beta-task as a task row")
	}
}

func TestReview_Text_ChecklistPresent(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewOpts("rich-task"))
	if !strings.Contains(out, "Review Checklist") {
		t.Error("review text missing Review Checklist heading")
	}
	// Checklist items must have checkbox format.
	if !strings.Contains(out, "[x]") && !strings.Contains(out, "[ ]") {
		t.Errorf("review text missing checklist checkbox markers; output:\n%s", out)
	}
}

func TestReview_Text_ReadinessNotesPresent(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewOpts("rich-task"))
	if !strings.Contains(out, "Readiness") {
		t.Error("review text missing Readiness section")
	}
	if !strings.Contains(out, "merge:") {
		t.Error("review text missing merge: readiness line")
	}
	if !strings.Contains(out, "cleanup:") {
		t.Error("review text missing cleanup: readiness line")
	}
}

func TestReview_Text_DecisionFirst(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewOpts("rich-task"))
	decisionIndex := strings.Index(out, "Decision")
	taskIndex := strings.Index(out, "Task: rich-task")
	if decisionIndex < 0 {
		t.Fatalf("review text missing Decision section; output:\n%s", out)
	}
	if taskIndex < 0 || decisionIndex > taskIndex {
		t.Fatalf("review text must show Decision before task detail; output:\n%s", out)
	}
	for _, want := range []string{
		"next action:",
		"merge gate:",
		"attention:",
		"Inspect task state before review.",
		"not ready for approval",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("review decision missing %q; output:\n%s", want, out)
		}
	}
}

func TestReview_Text_SuggestedActionsPresent(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewOpts("rich-task"))
	if !strings.Contains(out, "Suggested Next Actions") {
		t.Error("review text missing Suggested Next Actions section")
	}
}

func TestReview_Text_MergedTaskCleanupCommandUsesCLIOrder(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     multiStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewOpts("task-merged"))
	if !strings.Contains(out, "Run brevity task cleanup task-merged --plan to preview cleanup.") {
		t.Fatalf("review cleanup guidance must use task cleanup <slug> --plan; output:\n%s", out)
	}
	if strings.Contains(out, "brevity task cleanup --plan task-merged") {
		t.Fatalf("review cleanup guidance used invalid flag order; output:\n%s", out)
	}
}

func TestReview_Markdown_HeadingsPresent(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewMarkdownOpts("rich-task"))
	for _, want := range []string{
		"# CMUX Review Packet:",
		"## Decision",
		"## Task:",
		"## Queue / Scheduler",
		"## Review Checklist",
		"## Merge Readiness",
		"## Cleanup Readiness",
		"## Suggested Follow-up",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("review markdown missing heading %q; output:\n%s", want, out)
		}
	}
}

func TestReview_Markdown_ChecklistPresent(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewMarkdownOpts("rich-task"))
	// Markdown checklist uses GFM-style "- [x]" or "- [ ]" format.
	if !strings.Contains(out, "- [x]") && !strings.Contains(out, "- [ ]") {
		t.Errorf("review markdown missing GFM checklist markers; output:\n%s", out)
	}
}

func TestReview_Markdown_NoUnrelatedTaskH3(t *testing.T) {
	// When reviewing alpha-task, beta-task must not appear as a ### heading.
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewMarkdownOpts("alpha-task"))
	if strings.Contains(out, "### beta-task") {
		t.Error("review markdown must not contain ### beta-task heading")
	}
}

func TestReview_JSON_Structure(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewJSONOpts("rich-task"))
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("review JSON produced invalid JSON: %v\noutput:\n%s", err, out)
	}
	// Schema must be the review schema.
	if !strings.Contains(out, `"brevity.cmux-review-report.v1"`) {
		t.Errorf("review JSON missing review schema; output:\n%s", out)
	}
	// reviewMode must be true.
	if result["reviewMode"] != true {
		t.Errorf("review JSON reviewMode must be true; got %v", result["reviewMode"])
	}
	// section must be "review".
	if result["section"] != "review" {
		t.Errorf("review JSON section must be \"review\"; got %v", result["section"])
	}
	// reviewTask must match the slug.
	if result["reviewTask"] != "rich-task" {
		t.Errorf("review JSON reviewTask must be \"rich-task\"; got %v", result["reviewTask"])
	}
	// task, queue, reviewChecklist, suggestedActions must be present.
	for _, key := range []string{"task", "queue", "reviewChecklist", "suggestedActions"} {
		if _, ok := result[key]; !ok {
			t.Errorf("review JSON missing key %q; output:\n%s", key, out)
		}
	}
}

func TestReview_JSON_DecisionFields(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewJSONOpts("rich-task"))
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, key := range []string{"nextAction", "mergeGate", "attention"} {
		if value, ok := result[key].(string); !ok || strings.TrimSpace(value) == "" {
			t.Errorf("review JSON decision field %q missing or empty; output:\n%s", key, out)
		}
	}
	if result["mergeGate"] != "not ready for approval" {
		t.Errorf("mergeGate = %v, want not ready for approval", result["mergeGate"])
	}
}

func TestReview_JSON_ChecklistNotEmpty(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewJSONOpts("rich-task"))
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	checklist, _ := result["reviewChecklist"].([]any)
	if len(checklist) == 0 {
		t.Errorf("review JSON reviewChecklist must not be empty when task is found; output:\n%s", out)
	}
}

func TestReview_JSON_ReadinessPresent(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewJSONOpts("rich-task"))
	if !strings.Contains(out, `"mergeReadiness"`) {
		t.Errorf("review JSON missing mergeReadiness; output:\n%s", out)
	}
	if !strings.Contains(out, `"cleanupReadiness"`) {
		t.Errorf("review JSON missing cleanupReadiness; output:\n%s", out)
	}
}

func TestReview_Text_Deterministic(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	opts := reviewOpts("rich-task")
	out1 := renderSnapshotOpts(snap, opts)
	out2 := renderSnapshotOpts(snap, opts)
	if out1 != out2 {
		t.Error("review text Render is not deterministic")
	}
}

func TestReview_Markdown_Deterministic(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	opts := reviewMarkdownOpts("rich-task")
	out1 := renderSnapshotOpts(snap, opts)
	out2 := renderSnapshotOpts(snap, opts)
	if out1 != out2 {
		t.Error("review markdown Render is not deterministic")
	}
}

func TestReview_JSON_Deterministic(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	opts := reviewJSONOpts("rich-task")
	out1 := renderSnapshotOpts(snap, opts)
	out2 := renderSnapshotOpts(snap, opts)
	if out1 != out2 {
		t.Error("review JSON Render is not deterministic")
	}
}

func TestReview_NoANSI(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	for _, name := range []string{"text", "markdown", "json"} {
		var opts cmux.RenderOptions
		switch name {
		case "text":
			opts = reviewOpts("rich-task")
		case "markdown":
			opts = reviewMarkdownOpts("rich-task")
		case "json":
			opts = reviewJSONOpts("rich-task")
		}
		out := renderSnapshotOpts(snap, opts)
		if strings.Contains(out, "\x1b[") {
			t.Errorf("review %s output contains ANSI escape sequences", name)
		}
	}
}

func TestReview_Checklist_LastRunSucceeded_IsMarked(t *testing.T) {
	// richTaskStateJSON has latestRunWorkerStatus="succeeded".
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewOpts("rich-task"))
	// The "Last run succeeded" checklist item must be checked.
	if !strings.Contains(out, "[x] Last run succeeded") {
		t.Errorf("review text 'Last run succeeded' must be [x] when last run succeeded; output:\n%s", out)
	}
}

func TestReview_Checklist_LastRunNotSucceeded_IsUnchecked(t *testing.T) {
	// alpha-task in minimalStateJSON has no latestRunWorkerStatus.
	snap := cmux.Read(stubFetcher{
		stateJSON:     minimalStateJSON(t),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewOpts("alpha-task"))
	// The "Last run succeeded" checklist item must be unchecked.
	if !strings.Contains(out, "[ ] Last run succeeded") {
		t.Errorf("review text 'Last run succeeded' must be [ ] when no successful run; output:\n%s", out)
	}
}

func TestReview_RuntimeStateError_GracefulDegradation(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateErr:      errors.New("runtime unavailable"),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, reviewOpts("any-task"))
	if !strings.Contains(out, "CMUX REVIEW PACKET") {
		t.Error("review text missing header even on error")
	}
	if !strings.Contains(out, "runtime-state: error:") {
		t.Errorf("review text missing runtime-state error; output:\n%s", out)
	}
}

// --- handoff packet mode tests ---------------------------------------------

// handoffOpts is a shorthand for default text handoff options.
func handoffOpts() cmux.RenderOptions {
	return cmux.RenderOptions{Handoff: true}
}

// handoffMarkdownOpts is a shorthand for markdown handoff options.
func handoffMarkdownOpts() cmux.RenderOptions {
	return cmux.RenderOptions{Handoff: true, Output: cmux.OutputMarkdown}
}

// handoffJSONOpts is a shorthand for JSON handoff options.
func handoffJSONOpts() cmux.RenderOptions {
	return cmux.RenderOptions{Handoff: true, Output: cmux.OutputJSON}
}

func TestHandoff_DispatchActivated(t *testing.T) {
	// With Handoff:true the output must start with the handoff header, not the
	// normal CMUX OPERATOR header.
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, handoffOpts())
	if !strings.Contains(out, "CMUX HANDOFF PACKET") {
		t.Errorf("handoff dispatch missing CMUX HANDOFF PACKET header; output:\n%s", out)
	}
	if strings.Contains(out, "CMUX OPERATOR") {
		t.Error("handoff output must not contain CMUX OPERATOR (normal report) header")
	}
}

func TestHandoff_Text_Structure(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, handoffOpts())
	for _, want := range []string{
		"CMUX HANDOFF PACKET",
		"[read-only]",
		"source: native",
		"Runtime Summary",
		"Providers",
		"Queue / Scheduler",
		"Important Tasks",
		"Review Candidates",
		"Suggested Next Actions",
		"[read-only]", // safety attestation
	} {
		if !strings.Contains(out, want) {
			t.Errorf("handoff text missing section %q; output:\n%s", want, out)
		}
	}
}

func TestHandoff_Markdown_Structure(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, handoffMarkdownOpts())
	for _, want := range []string{
		"# CMUX Handoff Packet [read-only]",
		"## Runtime Summary",
		"## Providers",
		"## Queue / Scheduler",
		"## Important Tasks",
		"## Review Candidates",
		"## Suggested Next Actions",
		"Safety note:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("handoff markdown missing heading %q; output:\n%s", want, out)
		}
	}
}

func TestHandoff_Markdown_StartsWithH1(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, handoffMarkdownOpts())
	if !strings.HasPrefix(out, "# CMUX Handoff Packet") {
		t.Errorf("handoff markdown must start with # CMUX Handoff Packet; prefix: %q", out[:min(len(out), 40)])
	}
}

func TestHandoff_JSON_Schema(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, handoffJSONOpts())
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("handoff JSON invalid: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, `"brevity.cmux-handoff.v1"`) {
		t.Errorf("handoff JSON missing schema field; output:\n%s", out)
	}
	for _, key := range []string{
		"schema", "source", "options", "errors",
		"importantTasks", "reviewCandidates", "suggestedNextActions", "safety",
	} {
		if _, ok := result[key]; !ok {
			t.Errorf("handoff JSON missing key %q; output:\n%s", key, out)
		}
	}
}

func TestHandoff_JSON_SafetyBlock(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, handoffJSONOpts())
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	safety, ok := result["safety"].(map[string]any)
	if !ok {
		t.Fatalf("handoff JSON safety must be an object; output:\n%s", out)
	}
	if safety["readOnly"] != true {
		t.Errorf("handoff JSON safety.readOnly must be true; got %v", safety["readOnly"])
	}
	if _, ok := safety["note"]; !ok {
		t.Error("handoff JSON safety must have a note field")
	}
}

func TestHandoff_JSON_EmptyArraysNotNull(t *testing.T) {
	// With no tasks or actions in manyTaskStateJSON(0), slices must be [] not null.
	snap := cmux.Read(stubFetcher{stateJSON: manyTaskStateJSON(0), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, handoffJSONOpts())
	for _, want := range []string{`"importantTasks": []`, `"reviewCandidates": []`, `"suggestedNextActions": []`} {
		if !strings.Contains(out, want) {
			t.Errorf("handoff JSON slice must be [] not null: missing %q; output:\n%s", want, out)
		}
	}
}

func TestHandoff_ImportantTask_Ordering(t *testing.T) {
	// multiStateJSON has tasks: task-ready (ready-for-worker), task-review
	// (reviewing), task-blocked (blocked), task-merged (merged).
	// After ranking: task-review (0) → task-blocked (1) → task-ready (2) → task-merged (3).
	snap := cmux.Read(stubFetcher{stateJSON: multiStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, handoffOpts())

	reviewIdx := strings.Index(out, "task-review")
	blockedIdx := strings.Index(out, "task-blocked")
	readyIdx := strings.Index(out, "task-ready")
	if reviewIdx < 0 || blockedIdx < 0 || readyIdx < 0 {
		t.Fatalf("handoff text missing expected task slugs; output:\n%s", out)
	}
	if reviewIdx > blockedIdx {
		t.Errorf("handoff: task-review (priority 0) must appear before task-blocked (priority 1)")
	}
	if blockedIdx > readyIdx {
		t.Errorf("handoff: task-blocked (priority 1) must appear before task-ready (priority 2)")
	}
}

func TestHandoff_ReviewCandidates_OnlyReviewingTasks(t *testing.T) {
	// Only task-review (reviewing) should appear in Review Candidates.
	// task-blocked and task-ready must not.
	snap := cmux.Read(stubFetcher{stateJSON: multiStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, handoffOpts())

	// Extract the Review Candidates section (between its heading and the next ---).
	candidateSection := extractSection(out, "Review Candidates", sectionSep)
	if !strings.Contains(candidateSection, "task-review") {
		t.Errorf("handoff Review Candidates missing task-review; section:\n%s", candidateSection)
	}
	if strings.Contains(candidateSection, "task-blocked") {
		t.Error("handoff Review Candidates must not contain task-blocked")
	}
	if strings.Contains(candidateSection, "task-ready") {
		t.Error("handoff Review Candidates must not contain task-ready")
	}
}

func TestHandoff_ReviewCandidates_ChecklistPresent(t *testing.T) {
	// Review candidates must include inline checklist items.
	snap := cmux.Read(stubFetcher{stateJSON: multiStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, handoffOpts())
	candidateSection := extractSection(out, "Review Candidates", sectionSep)
	if !strings.Contains(candidateSection, "[x]") && !strings.Contains(candidateSection, "[ ]") {
		t.Errorf("handoff Review Candidates missing checklist markers; section:\n%s", candidateSection)
	}
}

func TestHandoff_ReviewCandidates_DecisionSupportPresent(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: multiStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, handoffOpts())
	candidateSection := extractSection(out, "Review Candidates", sectionSep)
	for _, want := range []string{
		"next:    Restore or locate the worktree before review.",
		"gate:    blocked until worktree is present",
		"watch:   No durable worktree path is available for diff inspection.",
	} {
		if !strings.Contains(candidateSection, want) {
			t.Errorf("handoff Review Candidates missing decision support %q; section:\n%s", want, candidateSection)
		}
	}
}

func TestHandoff_Limit_Applied(t *testing.T) {
	// 15 tasks, limit=3: important tasks shows first 3 and a truncation header.
	snap := cmux.Read(stubFetcher{stateJSON: manyTaskStateJSON(15), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{Handoff: true, Limit: 3})
	if !strings.Contains(out, "showing 3 of 15") {
		t.Errorf("handoff limit=3 with 15 tasks missing truncation header; output:\n%s", out)
	}
	if !strings.Contains(out, "task-1") {
		t.Error("handoff limit=3 missing task-1")
	}
	if strings.Contains(out, "task-4") {
		t.Error("handoff limit=3 must not show task-4 in important tasks")
	}
}

func TestHandoff_EmptyState_NoTasks(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: manyTaskStateJSON(0), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, handoffOpts())
	if !strings.Contains(out, "Important Tasks: none tracked") {
		t.Errorf("handoff empty state missing 'Important Tasks: none tracked'; output:\n%s", out)
	}
	if !strings.Contains(out, "Review Candidates: none") {
		t.Errorf("handoff empty state missing 'Review Candidates: none'; output:\n%s", out)
	}
}

func TestHandoff_EmptyState_Markdown(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: manyTaskStateJSON(0), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, handoffMarkdownOpts())
	if !strings.Contains(out, "_No tasks tracked._") {
		t.Errorf("handoff markdown empty state missing '_No tasks tracked._'; output:\n%s", out)
	}
	if !strings.Contains(out, "_No review candidates._") {
		t.Errorf("handoff markdown empty state missing '_No review candidates._'; output:\n%s", out)
	}
}

func TestHandoff_RuntimeStateError_GracefulDegradation(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateErr:      errors.New("runtime unavailable"),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, handoffOpts())
	if !strings.Contains(out, "CMUX HANDOFF PACKET") {
		t.Error("handoff text missing header even on error")
	}
	if !strings.Contains(out, "runtime unavailable") {
		t.Errorf("handoff text missing runtime error message; output:\n%s", out)
	}
}

func TestHandoff_NoANSI(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	for _, name := range []string{"text", "markdown", "json"} {
		var opts cmux.RenderOptions
		switch name {
		case "text":
			opts = handoffOpts()
		case "markdown":
			opts = handoffMarkdownOpts()
		case "json":
			opts = handoffJSONOpts()
		}
		if out := renderSnapshotOpts(snap, opts); strings.Contains(out, "\x1b[") {
			t.Errorf("handoff %s output contains ANSI escape sequences", name)
		}
	}
}

func TestHandoff_Text_Deterministic(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	opts := handoffOpts()
	if out1, out2 := renderSnapshotOpts(snap, opts), renderSnapshotOpts(snap, opts); out1 != out2 {
		t.Error("handoff text Render is not deterministic")
	}
}

func TestHandoff_Markdown_Deterministic(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	opts := handoffMarkdownOpts()
	if out1, out2 := renderSnapshotOpts(snap, opts), renderSnapshotOpts(snap, opts); out1 != out2 {
		t.Error("handoff markdown Render is not deterministic")
	}
}

func TestHandoff_JSON_Deterministic(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	opts := handoffJSONOpts()
	if out1, out2 := renderSnapshotOpts(snap, opts), renderSnapshotOpts(snap, opts); out1 != out2 {
		t.Error("handoff JSON Render is not deterministic")
	}
}

func TestHandoff_NormalOutput_Unchanged(t *testing.T) {
	// Without Handoff:true the default text render must not produce handoff output.
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshot(snap) // zero RenderOptions
	if strings.Contains(out, "CMUX HANDOFF PACKET") {
		t.Error("normal cmux output must not contain CMUX HANDOFF PACKET header")
	}
	if !strings.Contains(out, "CMUX OPERATOR") {
		t.Error("normal cmux output must still contain CMUX OPERATOR header")
	}
}

func TestHandoff_JSON_OptionsBlock(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{Handoff: true, Output: cmux.OutputJSON, Limit: 5})
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	options, ok := result["options"].(map[string]any)
	if !ok {
		t.Fatalf("handoff JSON options must be an object; output:\n%s", out)
	}
	if options["limit"] != float64(5) {
		t.Errorf("handoff JSON options.limit must be 5; got %v", options["limit"])
	}
	if options["output"] != "json" {
		t.Errorf("handoff JSON options.output must be \"json\"; got %v", options["output"])
	}
}

func TestHandoff_JSON_RuntimeSummaryPresent(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, handoffJSONOpts())
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := result["runtimeSummary"]; !ok {
		t.Errorf("handoff JSON missing runtimeSummary when runtime state available; output:\n%s", out)
	}
}

func TestHandoff_Markdown_ReviewCandidatesHaveChecklist(t *testing.T) {
	// multiStateJSON has task-review in reviewing state — it should appear in
	// the markdown Review Candidates section with a GFM checklist.
	snap := cmux.Read(stubFetcher{stateJSON: multiStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, handoffMarkdownOpts())
	if !strings.Contains(out, "#### Review Checklist") {
		t.Errorf("handoff markdown Review Candidates missing #### Review Checklist heading; output:\n%s", out)
	}
	if !strings.Contains(out, "**Merge Readiness:**") {
		t.Errorf("handoff markdown Review Candidates missing **Merge Readiness:**; output:\n%s", out)
	}
}

func TestHandoff_Markdown_ReviewCandidatesHaveDecisionSupport(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: multiStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, handoffMarkdownOpts())
	for _, want := range []string{
		"**Next action:** Restore or locate the worktree before review.",
		"**Merge gate:** blocked until worktree is present",
		"**Attention:** No durable worktree path is available for diff inspection.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("handoff markdown Review Candidates missing decision support %q; output:\n%s", want, out)
		}
	}
}

func TestHandoff_JSON_ReviewCandidatesHaveDecisionSupport(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: multiStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, handoffJSONOpts())
	var result struct {
		ReviewCandidates []struct {
			NextAction string `json:"nextAction"`
			MergeGate  string `json:"mergeGate"`
			Attention  string `json:"attention"`
		} `json:"reviewCandidates"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput:\n%s", err, out)
	}
	if len(result.ReviewCandidates) != 1 {
		t.Fatalf("expected one review candidate, got %d; output:\n%s", len(result.ReviewCandidates), out)
	}
	candidate := result.ReviewCandidates[0]
	if candidate.NextAction != "Restore or locate the worktree before review." {
		t.Errorf("nextAction = %q", candidate.NextAction)
	}
	if candidate.MergeGate != "blocked until worktree is present" {
		t.Errorf("mergeGate = %q", candidate.MergeGate)
	}
	if candidate.Attention != "No durable worktree path is available for diff inspection." {
		t.Errorf("attention = %q", candidate.Attention)
	}
}

// --- merge-readiness report tests ------------------------------------------

// mergeReportStateJSON returns a runtime-state fixture covering all six merge
// groups: ready-for-merge, reviewing, needs-run (ready-for-worker), blocked,
// merged, and a task in an unrecognised state that lands in "other".
func mergeReportStateJSON() []byte {
	return []byte(`{
		"schema": "brevity.runtime-state.v1",
		"repoRoot": "/dev/test",
		"generatedAt": "2026-01-01T00:00:00Z",
		"providers": {
			"summary": {"total": 1, "degraded": 0, "unavailable": 0},
			"health": {"codex": {"status": "healthy", "updatedAt": "", "note": ""}}
		},
		"taskCounts": {"tracked": 6, "runnable": 1, "blocked": 1, "stale": 0, "providerGated": 0, "review": 2},
		"tasks": [
			{"slug": "mrg-rfm",     "status": "ready-for-merge", "normalizedState": "ready-for-merge", "workerStatus": "succeeded", "branch": "task/mrg-rfm",     "worktreePath": ""},
			{"slug": "mrg-review",  "status": "reviewing",       "normalizedState": "reviewing",       "workerStatus": "succeeded", "branch": "task/mrg-review",  "worktreePath": ""},
			{"slug": "mrg-run",     "status": "ready-for-worker","normalizedState": "ready-for-worker","workerStatus": "",          "branch": "task/mrg-run",     "worktreePath": ""},
			{"slug": "mrg-blocked", "status": "blocked",         "normalizedState": "blocked",         "workerStatus": "",          "branch": "task/mrg-blocked", "worktreePath": ""},
			{"slug": "mrg-merged",  "status": "merged",          "normalizedState": "merged",          "workerStatus": "succeeded", "branch": "task/mrg-merged",  "worktreePath": ""},
			{"slug": "mrg-other",   "status": "queued",          "normalizedState": "queued",          "workerStatus": "",          "branch": "task/mrg-other",   "worktreePath": ""}
		],
		"suggestedNextActions": ["Check merge candidates."],
		"orphanedTaskWorktrees": [],
		"activeWorktrees": [],
		"activeWorktreeCount": 0,
		"groups": {}
	}`)
}

// mergeOpts is a shorthand for merge-report text options.
func mergeOpts() cmux.RenderOptions {
	return cmux.RenderOptions{MergeReport: true}
}

// mergeMarkdownOpts is a shorthand for merge-report markdown options.
func mergeMarkdownOpts() cmux.RenderOptions {
	return cmux.RenderOptions{MergeReport: true, Output: cmux.OutputMarkdown}
}

// mergeJSONOpts is a shorthand for merge-report JSON options.
func mergeJSONOpts() cmux.RenderOptions {
	return cmux.RenderOptions{MergeReport: true, Output: cmux.OutputJSON}
}

func TestMerge_DispatchActivated(t *testing.T) {
	// MergeReport:true must produce the merge header, not the normal CMUX OPERATOR header.
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, mergeOpts())
	if !strings.Contains(out, "CMUX MERGE READINESS") {
		t.Errorf("merge dispatch missing CMUX MERGE READINESS header; output:\n%s", out)
	}
	if strings.Contains(out, "CMUX OPERATOR") {
		t.Error("merge output must not contain CMUX OPERATOR (normal report) header")
	}
}

func TestMerge_Text_Structure(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: mergeReportStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, mergeOpts())
	for _, want := range []string{
		"CMUX MERGE READINESS",
		"[read-only]",
		"source: native",
		"Action Summary",
		"next action: run merge prep for ready-for-merge tasks",
		"ready:       1 ready-for-merge | 1 reviewing",
		"ready-for-merge",
		"reviewing",
		"needs-run",
		"blocked",
		"merged",
		"other",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("merge text missing %q; output:\n%s", want, out)
		}
	}
}

func TestMerge_Text_ActionSummaryBeforeGroups(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: mergeReportStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, mergeOpts())
	summaryIndex := strings.Index(out, "Action Summary")
	groupIndex := strings.Index(out, "\nready-for-merge")
	if summaryIndex < 0 {
		t.Fatalf("merge text missing Action Summary; output:\n%s", out)
	}
	if groupIndex < 0 || summaryIndex > groupIndex {
		t.Fatalf("merge Action Summary must appear before grouped tasks; output:\n%s", out)
	}
}

func TestMerge_Text_TaskDecisionSupportPresent(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: mergeReportStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, mergeOpts())
	for _, want := range []string{
		"next:    Restore or locate the worktree before review.",
		"gate:    blocked until worktree is present",
		"command: brevity task merge mrg-rfm --plan",
		"command: brevity cmux --review mrg-review",
		"command: brevity task runtime-info mrg-run",
		"command: brevity task cleanup mrg-merged --plan",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("merge text missing task decision support %q; output:\n%s", want, out)
		}
	}
}

func TestMerge_Text_GroupingByState(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: mergeReportStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, mergeOpts())

	// Each task must appear in its correct group.
	// We verify positional ordering: the group header appears before the task slug.
	rfmIdx := strings.Index(out, "ready-for-merge")
	rfmTaskIdx := strings.Index(out, "mrg-rfm")
	reviewIdx := strings.Index(out, "\nreviewing")
	reviewTaskIdx := strings.Index(out, "mrg-review")
	needsRunIdx := strings.Index(out, "needs-run")
	needsRunTaskIdx := strings.Index(out, "mrg-run")
	blockedIdx := strings.Index(out, "\nblocked")
	blockedTaskIdx := strings.Index(out, "mrg-blocked")
	mergedIdx := strings.Index(out, "\nmerged")
	mergedTaskIdx := strings.Index(out, "mrg-merged")
	otherIdx := strings.Index(out, "\nother")
	otherTaskIdx := strings.Index(out, "mrg-other")

	if rfmIdx < 0 || rfmTaskIdx < rfmIdx {
		t.Errorf("mrg-rfm must appear after ready-for-merge group header")
	}
	if reviewIdx < 0 || reviewTaskIdx < reviewIdx {
		t.Errorf("mrg-review must appear after reviewing group header")
	}
	if needsRunIdx < 0 || needsRunTaskIdx < needsRunIdx {
		t.Errorf("mrg-run must appear after needs-run group header")
	}
	if blockedIdx < 0 || blockedTaskIdx < blockedIdx {
		t.Errorf("mrg-blocked must appear after blocked group header")
	}
	if mergedIdx < 0 || mergedTaskIdx < mergedIdx {
		t.Errorf("mrg-merged must appear after merged group header")
	}
	if otherIdx < 0 || otherTaskIdx < otherIdx {
		t.Errorf("mrg-other must appear after other group header")
	}
}

func TestMerge_Markdown_Structure(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: mergeReportStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, mergeMarkdownOpts())
	for _, want := range []string{
		"# CMUX Merge Readiness [read-only]",
		"## Action Summary",
		"**Next action:** run merge prep for ready-for-merge tasks",
		"## ready-for-merge",
		"## reviewing",
		"## needs-run",
		"## blocked",
		"## merged",
		"## other",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("merge markdown missing %q; output:\n%s", want, out)
		}
	}
}

func TestMerge_Markdown_TaskDecisionSupportPresent(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: mergeReportStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, mergeMarkdownOpts())
	for _, want := range []string{
		"**Next action:** Restore or locate the worktree before review.",
		"**Merge gate:** blocked until worktree is present",
		"**Command:** `brevity task merge mrg-rfm --plan`",
		"**Command:** `brevity cmux --review mrg-review`",
		"**Command:** `brevity task runtime-info mrg-run`",
		"**Command:** `brevity task cleanup mrg-merged --plan`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("merge markdown missing task decision support %q; output:\n%s", want, out)
		}
	}
}

func TestMerge_Markdown_StartsWithH1(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, mergeMarkdownOpts())
	if !strings.HasPrefix(out, "# CMUX Merge Readiness") {
		t.Errorf("merge markdown must start with # CMUX Merge Readiness; prefix: %q", out[:min(len(out), 40)])
	}
}

func TestMerge_JSON_Schema(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, mergeJSONOpts())
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("merge JSON invalid: %v\noutput:\n%s", err, out)
	}
	if result["schema"] != "brevity.cmux-merge-report.v1" {
		t.Errorf("merge JSON schema = %v, want brevity.cmux-merge-report.v1", result["schema"])
	}
}

func TestMerge_JSON_GroupsPresent(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: mergeReportStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, mergeJSONOpts())
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	groups, ok := result["groups"].([]any)
	if !ok {
		t.Fatalf("merge JSON missing groups array; output:\n%s", out)
	}
	if len(groups) != 6 {
		t.Errorf("merge JSON groups len = %d, want 6", len(groups))
	}
	// Verify order: first group must be ready-for-merge.
	first, ok := groups[0].(map[string]any)
	if !ok {
		t.Fatal("merge JSON groups[0] is not an object")
	}
	if first["group"] != "ready-for-merge" {
		t.Errorf("merge JSON groups[0].group = %v, want ready-for-merge", first["group"])
	}
}

func TestMerge_JSON_SummaryPresent(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: mergeReportStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, mergeJSONOpts())
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	summary, ok := result["summary"].(map[string]any)
	if !ok {
		t.Fatalf("merge JSON missing summary object; output:\n%s", out)
	}
	if summary["nextAction"] != "run merge prep for ready-for-merge tasks" {
		t.Errorf("summary.nextAction = %v", summary["nextAction"])
	}
	if summary["readyCount"] != float64(1) || summary["reviewCount"] != float64(1) || summary["blockedCount"] != float64(1) || summary["needsRunCount"] != float64(1) {
		t.Errorf("summary counts unexpected: %#v", summary)
	}
}

func TestMerge_JSON_TaskDecisionSupportPresent(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: mergeReportStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, mergeJSONOpts())
	var result struct {
		Groups []struct {
			Group string `json:"group"`
			Tasks []struct {
				Slug       string `json:"slug"`
				NextAction string `json:"nextAction"`
				MergeGate  string `json:"mergeGate"`
				Attention  string `json:"attention"`
				Command    string `json:"command"`
			} `json:"tasks"`
		} `json:"groups"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput:\n%s", err, out)
	}
	if len(result.Groups) == 0 || len(result.Groups[0].Tasks) == 0 {
		t.Fatalf("merge JSON missing ready-for-merge task; output:\n%s", out)
	}
	task := result.Groups[0].Tasks[0]
	if task.Slug != "mrg-rfm" {
		t.Fatalf("first ready task slug = %q", task.Slug)
	}
	if task.NextAction != "Restore or locate the worktree before review." {
		t.Errorf("nextAction = %q", task.NextAction)
	}
	if task.MergeGate != "blocked until worktree is present" {
		t.Errorf("mergeGate = %q", task.MergeGate)
	}
	if task.Attention != "No durable worktree path is available for diff inspection." {
		t.Errorf("attention = %q", task.Attention)
	}
	if task.Command != "brevity task merge mrg-rfm --plan" {
		t.Errorf("command = %q", task.Command)
	}
	needsRun := result.Groups[2].Tasks[0]
	if needsRun.Slug != "mrg-run" {
		t.Fatalf("needs-run task slug = %q", needsRun.Slug)
	}
	if needsRun.Command != "brevity task runtime-info mrg-run" {
		t.Errorf("needs-run command = %q", needsRun.Command)
	}
	merged := result.Groups[4].Tasks[0]
	if merged.Slug != "mrg-merged" {
		t.Fatalf("merged task slug = %q", merged.Slug)
	}
	if merged.Command != "brevity task cleanup mrg-merged --plan" {
		t.Errorf("merged command = %q", merged.Command)
	}
}

func TestMerge_JSON_EmptyArraysNotNull(t *testing.T) {
	// With no tasks, every group's tasks field must be [] not null.
	snap := cmux.Read(stubFetcher{stateJSON: manyTaskStateJSON(0), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, mergeJSONOpts())
	// A null tasks field would appear as `"tasks":null`; [] never does.
	if strings.Contains(out, `"tasks":null`) {
		t.Errorf("merge JSON must not contain null tasks array; output:\n%s", out)
	}
	if strings.Contains(out, `"errors":null`) {
		t.Errorf("merge JSON must not contain null errors array; output:\n%s", out)
	}
}

func TestMerge_Limit_Applied(t *testing.T) {
	// mergeReportStateJSON has 1 task per group; all groups with 1 task should
	// show it; groups with 0 tasks show (none).
	// Now give needs-run 3 tasks and set limit=1 to trigger truncation.
	multiRunStateJSON := []byte(`{
		"schema": "brevity.runtime-state.v1",
		"repoRoot": "/dev/test",
		"generatedAt": "2026-01-01T00:00:00Z",
		"providers": {"summary": {"total": 0, "degraded": 0, "unavailable": 0}, "health": {}},
		"taskCounts": {"tracked": 3, "runnable": 3, "blocked": 0, "stale": 0, "providerGated": 0, "review": 0},
		"tasks": [
			{"slug": "run-a", "status": "ready-for-worker", "normalizedState": "ready-for-worker", "workerStatus": "", "branch": "task/run-a", "worktreePath": ""},
			{"slug": "run-b", "status": "ready-for-worker", "normalizedState": "ready-for-worker", "workerStatus": "", "branch": "task/run-b", "worktreePath": ""},
			{"slug": "run-c", "status": "ready-for-worker", "normalizedState": "ready-for-worker", "workerStatus": "", "branch": "task/run-c", "worktreePath": ""}
		],
		"suggestedNextActions": [],
		"orphanedTaskWorktrees": [],
		"activeWorktrees": [],
		"activeWorktreeCount": 0,
		"groups": {}
	}`)
	snap := cmux.Read(stubFetcher{stateJSON: multiRunStateJSON, schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{MergeReport: true, Limit: 1})
	if !strings.Contains(out, "showing 1 of 3") {
		t.Errorf("merge limit=1 with 3 needs-run tasks must show truncation header; output:\n%s", out)
	}
	if strings.Contains(out, "run-b") {
		t.Errorf("merge limit=1 must not show run-b (second task); output:\n%s", out)
	}
}

func TestMerge_EmptyState_NoTasks(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: manyTaskStateJSON(0), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, mergeOpts())
	// All groups should show (none).
	if !strings.Contains(out, "(none)") {
		t.Errorf("merge empty state must show (none) for groups; output:\n%s", out)
	}
}

func TestMerge_EmptyState_Markdown(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: manyTaskStateJSON(0), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, mergeMarkdownOpts())
	if !strings.Contains(out, "_None._") {
		t.Errorf("merge markdown empty state must show _None._; output:\n%s", out)
	}
}

func TestMerge_RuntimeStateError_GracefulDegradation(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateErr:      fmt.Errorf("runtime unreachable"),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, mergeOpts())
	if !strings.Contains(out, "CMUX MERGE READINESS") {
		t.Error("merge header must still appear on runtime error")
	}
	if !strings.Contains(out, "runtime-state: error") {
		t.Errorf("merge error degradation must show runtime-state error; output:\n%s", out)
	}
}

func TestMerge_NoANSI(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: mergeReportStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	for _, opts := range []cmux.RenderOptions{mergeOpts(), mergeMarkdownOpts(), mergeJSONOpts()} {
		out := renderSnapshotOpts(snap, opts)
		if strings.Contains(out, "\x1b[") {
			t.Errorf("merge output must not contain ANSI escape sequences (mode=%v); output:\n%s", opts.Output, out)
		}
	}
}

func TestMerge_Text_Deterministic(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: mergeReportStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out1 := renderSnapshotOpts(snap, mergeOpts())
	out2 := renderSnapshotOpts(snap, mergeOpts())
	if out1 != out2 {
		t.Error("merge text output is not deterministic")
	}
}

func TestMerge_Markdown_Deterministic(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: mergeReportStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out1 := renderSnapshotOpts(snap, mergeMarkdownOpts())
	out2 := renderSnapshotOpts(snap, mergeMarkdownOpts())
	if out1 != out2 {
		t.Error("merge markdown output is not deterministic")
	}
}

func TestMerge_JSON_Deterministic(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: mergeReportStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out1 := renderSnapshotOpts(snap, mergeJSONOpts())
	out2 := renderSnapshotOpts(snap, mergeJSONOpts())
	if out1 != out2 {
		t.Error("merge JSON output is not deterministic")
	}
}

func TestMerge_NormalOutput_Unchanged(t *testing.T) {
	// Without MergeReport:true the normal text render must not produce merge output.
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{})
	if strings.Contains(out, "CMUX MERGE READINESS") {
		t.Error("normal output must not contain CMUX MERGE READINESS header")
	}
}

func TestMerge_JSON_OptionsBlock(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: mergeReportStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{MergeReport: true, Output: cmux.OutputJSON, Limit: 5})
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	options, ok := result["options"].(map[string]any)
	if !ok {
		t.Fatalf("merge JSON missing options object; output:\n%s", out)
	}
	if options["limit"] != float64(5) {
		t.Errorf("merge JSON options.limit must be 5; got %v", options["limit"])
	}
	if options["output"] != "json" {
		t.Errorf("merge JSON options.output must be \"json\"; got %v", options["output"])
	}
}

func TestMerge_GroupOrder_ReadyForMergeFirst(t *testing.T) {
	// The groups slice must follow mergeGroupOrder: ready-for-merge is index 0.
	snap := cmux.Read(stubFetcher{stateJSON: mergeReportStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, mergeJSONOpts())
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	groups := result["groups"].([]any)
	wantOrder := []string{"ready-for-merge", "reviewing", "needs-run", "blocked", "merged", "other"}
	for i, wantGroup := range wantOrder {
		g := groups[i].(map[string]any)
		if g["group"] != wantGroup {
			t.Errorf("merge JSON groups[%d].group = %v, want %s", i, g["group"], wantGroup)
		}
	}
}

func TestReview_OverridesSection(t *testing.T) {
	// --review overrides --section; section=providers is ignored.
	snap := cmux.Read(stubFetcher{
		stateJSON:     richTaskStateJSON(),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{
		ReviewTask: "rich-task",
		Section:    cmux.SectionProviders, // should be ignored
	})
	// Should still produce review packet, not providers-only output.
	if !strings.Contains(out, "CMUX REVIEW PACKET") {
		t.Error("review mode must override --section and produce review packet header")
	}
	if !strings.Contains(out, "Review Checklist") {
		t.Error("review mode with section override must still show Review Checklist")
	}
}

// --- blocked report tests --------------------------------------------------

// blockedStateJSON returns a runtime-state fixture covering the blocked and
// provider-gated task states used by the blocked-report tests.
func blockedStateJSON() []byte {
	return []byte(`{
		"schema": "brevity.runtime-state.v1",
		"repoRoot": "/dev/test",
		"generatedAt": "2026-01-01T00:00:00Z",
		"providers": {
			"summary": {"total": 2, "degraded": 1, "unavailable": 0},
			"health": {
				"codex":  {"status": "healthy",  "updatedAt": "", "note": ""},
				"gemini": {"status": "degraded", "updatedAt": "", "note": "quota exceeded"}
			}
		},
		"taskCounts": {"tracked": 3, "runnable": 0, "blocked": 1, "stale": 0, "providerGated": 1, "review": 0},
		"tasks": [
			{
				"slug": "blk-pgated",
				"status": "provider-gated",
				"normalizedState": "provider-gated",
				"providerGated": true,
				"provider": "gemini",
				"providerHealth": "degraded",
				"workerStatus": "",
				"branch": "task/blk-pgated",
				"worktreePath": ""
			},
			{
				"slug": "blk-blocked",
				"status": "blocked",
				"normalizedState": "blocked",
				"providerGated": false,
				"provider": "codex",
				"providerHealth": "healthy",
				"workerStatus": "failed",
				"latestRunWorkerStatus": "failed",
				"latestRunFailureType": "context-exceeded",
				"branch": "task/blk-blocked",
				"worktreePath": ""
			},
			{
				"slug": "blk-runnable",
				"status": "ready-for-worker",
				"normalizedState": "ready-for-worker",
				"providerGated": false,
				"provider": "codex",
				"providerHealth": "healthy",
				"workerStatus": "",
				"branch": "task/blk-runnable",
				"worktreePath": ""
			}
		],
		"suggestedNextActions": ["Check blocked tasks."],
		"orphanedTaskWorktrees": [],
		"activeWorktrees": [],
		"activeWorktreeCount": 0,
		"groups": {}
	}`)
}

// skippedSchedulerJSON returns a scheduler-plan fixture with one skipped item.
func skippedSchedulerJSON() []byte {
	return []byte(`{
		"schema": "brevity.runtime-scheduler-plan.v1",
		"queuePath": ".brevity/runtime-queue.json",
		"queueState": "valid",
		"queueVersion": 1,
		"supportedQueueVersion": 1,
		"noSelectionReason": "all items skipped",
		"reservationEligible": false,
		"reservationEligibility": "not eligible: no selected queue item",
		"skipped": [
			{
				"id":       "queue-item-abc123",
				"task":     "blk-blocked",
				"provider": "codex",
				"profile":  "default",
				"status":   "reserved",
				"reason":   "item is already reserved by another worker"
			}
		],
		"safetyChecks": [],
		"readOnly": true
	}`)
}

// blockedOpts is a shorthand for blocked-report text options.
func blockedOpts() cmux.RenderOptions {
	return cmux.RenderOptions{BlockedReport: true}
}

// blockedMarkdownOpts is a shorthand for blocked-report markdown options.
func blockedMarkdownOpts() cmux.RenderOptions {
	return cmux.RenderOptions{BlockedReport: true, Output: cmux.OutputMarkdown}
}

// blockedJSONOpts is a shorthand for blocked-report JSON options.
func blockedJSONOpts() cmux.RenderOptions {
	return cmux.RenderOptions{BlockedReport: true, Output: cmux.OutputJSON}
}

func TestBlocked_DispatchActivated(t *testing.T) {
	// BlockedReport:true must produce the blocked header, not the normal header.
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, blockedOpts())
	if !strings.Contains(out, "CMUX BLOCKED REPORT") {
		t.Errorf("blocked dispatch missing CMUX BLOCKED REPORT header; output:\n%s", out)
	}
	if strings.Contains(out, "CMUX OPERATOR") {
		t.Error("blocked output must not contain CMUX OPERATOR (normal report) header")
	}
}

func TestBlocked_Text_Structure(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: blockedStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, blockedOpts())
	for _, want := range []string{
		"CMUX BLOCKED REPORT",
		"[read-only]",
		"source: native",
		"Summary",
		"provider-gated",
		"blocked",
		"reserved-or-queue-gated",
		"unknown",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("blocked text missing %q; output:\n%s", want, out)
		}
	}
}

func TestBlocked_Text_GroupingByState(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: blockedStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, blockedOpts())

	// blk-pgated must appear in the provider-gated section.
	pgIdx := strings.Index(out, "provider-gated")
	pgTaskIdx := strings.Index(out, "blk-pgated")
	blkIdx := strings.Index(out, "\nblocked")
	blkTaskIdx := strings.Index(out, "blk-blocked")

	if pgIdx < 0 || pgTaskIdx < pgIdx {
		t.Errorf("blk-pgated must appear after provider-gated header; output:\n%s", out)
	}
	if blkIdx < 0 || blkTaskIdx < blkIdx {
		t.Errorf("blk-blocked must appear after blocked header; output:\n%s", out)
	}
	// blk-runnable is ready-for-worker and must NOT appear in any blocked group.
	if strings.Contains(out, "blk-runnable") {
		t.Errorf("blk-runnable (ready-for-worker) must not appear in blocked report; output:\n%s", out)
	}
}

func TestBlocked_Text_SkippedQueueItemsIncluded(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: blockedStateJSON(), schedulerJSON: skippedSchedulerJSON()})
	out := renderSnapshotOpts(snap, blockedOpts())
	if !strings.Contains(out, "queue-item-abc123") {
		t.Errorf("skipped queue item ID must appear in blocked report; output:\n%s", out)
	}
	if !strings.Contains(out, "already reserved") {
		t.Errorf("skipped queue item reason must appear in blocked report; output:\n%s", out)
	}
}

func TestBlocked_Text_ReasonForProviderGated(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: blockedStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, blockedOpts())
	// blk-pgated has provider=gemini, providerHealth=degraded; reason must mention provider.
	if !strings.Contains(out, "likely provider") {
		t.Errorf("provider-gated task must include likely provider reason; output:\n%s", out)
	}
}

func TestBlocked_Text_ReasonForBlocked(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: blockedStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, blockedOpts())
	// blk-blocked has latestRunFailureType=context-exceeded.
	if !strings.Contains(out, "context-exceeded") {
		t.Errorf("blocked task with failure type must show that type as reason; output:\n%s", out)
	}
}

func TestBlocked_Text_ReasonUnavailableWhenMissing(t *testing.T) {
	// A task with normalizedState=blocked but no latestRunFailureType must show
	// "reason unavailable from current contract".
	noReasonStateJSON := []byte(`{
		"schema": "brevity.runtime-state.v1",
		"repoRoot": "/dev/test",
		"generatedAt": "2026-01-01T00:00:00Z",
		"providers": {"summary": {"total": 0, "degraded": 0, "unavailable": 0}, "health": {}},
		"taskCounts": {"tracked": 1, "runnable": 0, "blocked": 1, "stale": 0, "providerGated": 0, "review": 0},
		"tasks": [
			{
				"slug": "stuck-task",
				"status": "blocked",
				"normalizedState": "blocked",
				"providerGated": false,
				"workerStatus": "",
				"branch": "task/stuck-task",
				"worktreePath": ""
			}
		],
		"suggestedNextActions": [],
		"orphanedTaskWorktrees": [],
		"activeWorktrees": [],
		"activeWorktreeCount": 0,
		"groups": {}
	}`)
	snap := cmux.Read(stubFetcher{stateJSON: noReasonStateJSON, schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, blockedOpts())
	if !strings.Contains(out, "reason unavailable from current contract") {
		t.Errorf("blocked task with no failure type must show unavailable reason; output:\n%s", out)
	}
}

func TestBlocked_Markdown_Structure(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: blockedStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, blockedMarkdownOpts())
	for _, want := range []string{
		"# CMUX Blocked Report [read-only]",
		"## Summary",
		"## provider-gated",
		"## blocked",
		"## reserved-or-queue-gated",
		"## unknown",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("blocked markdown missing %q; output:\n%s", want, out)
		}
	}
}

func TestBlocked_Markdown_StartsWithH1(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, blockedMarkdownOpts())
	if !strings.HasPrefix(out, "# CMUX Blocked Report") {
		t.Errorf("blocked markdown must start with # CMUX Blocked Report; prefix: %q", out[:min(len(out), 40)])
	}
}

func TestBlocked_JSON_Schema(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: minimalStateJSON(t), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, blockedJSONOpts())
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("blocked JSON invalid: %v\noutput:\n%s", err, out)
	}
	if result["schema"] != "brevity.cmux-blocked-report.v1" {
		t.Errorf("blocked JSON schema = %v, want brevity.cmux-blocked-report.v1", result["schema"])
	}
}

func TestBlocked_JSON_StructureFields(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: blockedStateJSON(), schedulerJSON: skippedSchedulerJSON()})
	out := renderSnapshotOpts(snap, blockedJSONOpts())
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("blocked JSON invalid: %v\noutput:\n%s", err, out)
	}
	for _, field := range []string{"schema", "source", "options", "errors", "summary", "providerGated", "blocked", "reservedOrQueueGated", "unknown"} {
		if _, ok := result[field]; !ok {
			t.Errorf("blocked JSON missing top-level field %q; output:\n%s", field, out)
		}
	}
}

func TestBlocked_JSON_EmptyArraysNotNull(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: manyTaskStateJSON(0), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, blockedJSONOpts())
	if strings.Contains(out, `null`) {
		t.Errorf("blocked JSON must not contain null values for array/slice fields; output:\n%s", out)
	}
}

func TestBlocked_JSON_SummaryTotals(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: blockedStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, blockedJSONOpts())
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	summary := result["summary"].(map[string]any)
	// blockedStateJSON has 1 provider-gated + 1 blocked = 2 total (no queue items in minimal scheduler).
	if summary["providerGated"] != float64(1) {
		t.Errorf("summary.providerGated = %v, want 1", summary["providerGated"])
	}
	if summary["blocked"] != float64(1) {
		t.Errorf("summary.blocked = %v, want 1", summary["blocked"])
	}
	if summary["total"] != float64(2) {
		t.Errorf("summary.total = %v, want 2", summary["total"])
	}
}

func TestBlocked_JSON_SkippedQueueItem(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: blockedStateJSON(), schedulerJSON: skippedSchedulerJSON()})
	out := renderSnapshotOpts(snap, blockedJSONOpts())
	if !strings.Contains(out, "queue-item-abc123") {
		t.Errorf("blocked JSON must include skipped queue item ID; output:\n%s", out)
	}
	if !strings.Contains(out, "already reserved") {
		t.Errorf("blocked JSON must include skip reason; output:\n%s", out)
	}
}

func TestBlocked_Limit_Applied(t *testing.T) {
	// Build state with 3 blocked tasks; limit=1 must truncate to 1 and show header.
	threeBlockedJSON := []byte(`{
		"schema": "brevity.runtime-state.v1",
		"repoRoot": "/dev/test",
		"generatedAt": "2026-01-01T00:00:00Z",
		"providers": {"summary": {"total": 0, "degraded": 0, "unavailable": 0}, "health": {}},
		"taskCounts": {"tracked": 3, "runnable": 0, "blocked": 3, "stale": 0, "providerGated": 0, "review": 0},
		"tasks": [
			{"slug": "blk-a", "status": "blocked", "normalizedState": "blocked", "workerStatus": "", "branch": "task/blk-a", "worktreePath": ""},
			{"slug": "blk-b", "status": "blocked", "normalizedState": "blocked", "workerStatus": "", "branch": "task/blk-b", "worktreePath": ""},
			{"slug": "blk-c", "status": "blocked", "normalizedState": "blocked", "workerStatus": "", "branch": "task/blk-c", "worktreePath": ""}
		],
		"suggestedNextActions": [],
		"orphanedTaskWorktrees": [],
		"activeWorktrees": [],
		"activeWorktreeCount": 0,
		"groups": {}
	}`)
	snap := cmux.Read(stubFetcher{stateJSON: threeBlockedJSON, schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{BlockedReport: true, Limit: 1})
	if !strings.Contains(out, "showing 1 of 3") {
		t.Errorf("blocked limit=1 with 3 blocked tasks must show truncation header; output:\n%s", out)
	}
	if strings.Contains(out, "blk-b") {
		t.Errorf("blocked limit=1 must not show blk-b (second task); output:\n%s", out)
	}
}

func TestBlocked_EmptyState_NoTasks(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: manyTaskStateJSON(0), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, blockedOpts())
	if !strings.Contains(out, "(none)") {
		t.Errorf("blocked empty state must show (none) for groups with no tasks; output:\n%s", out)
	}
}

func TestBlocked_EmptyState_Markdown(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: manyTaskStateJSON(0), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, blockedMarkdownOpts())
	if !strings.Contains(out, "_None._") {
		t.Errorf("blocked markdown empty state must show _None._; output:\n%s", out)
	}
}

func TestBlocked_NoANSI(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: blockedStateJSON(), schedulerJSON: skippedSchedulerJSON()})
	for _, opts := range []cmux.RenderOptions{blockedOpts(), blockedMarkdownOpts(), blockedJSONOpts()} {
		out := renderSnapshotOpts(snap, opts)
		if strings.Contains(out, "\x1b[") {
			t.Errorf("blocked output must not contain ANSI escape sequences (mode=%v); output:\n%s", opts.Output, out)
		}
	}
}

func TestBlocked_Text_Deterministic(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: blockedStateJSON(), schedulerJSON: skippedSchedulerJSON()})
	out1 := renderSnapshotOpts(snap, blockedOpts())
	out2 := renderSnapshotOpts(snap, blockedOpts())
	if out1 != out2 {
		t.Error("blocked text output is not deterministic")
	}
}

func TestBlocked_Markdown_Deterministic(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: blockedStateJSON(), schedulerJSON: skippedSchedulerJSON()})
	out1 := renderSnapshotOpts(snap, blockedMarkdownOpts())
	out2 := renderSnapshotOpts(snap, blockedMarkdownOpts())
	if out1 != out2 {
		t.Error("blocked markdown output is not deterministic")
	}
}

func TestBlocked_JSON_Deterministic(t *testing.T) {
	snap := cmux.Read(stubFetcher{stateJSON: blockedStateJSON(), schedulerJSON: skippedSchedulerJSON()})
	out1 := renderSnapshotOpts(snap, blockedJSONOpts())
	out2 := renderSnapshotOpts(snap, blockedJSONOpts())
	if out1 != out2 {
		t.Error("blocked JSON output is not deterministic")
	}
}

func TestBlocked_NormalOutput_Unchanged(t *testing.T) {
	// Without BlockedReport:true the default text render must not produce blocked output.
	snap := cmux.Read(stubFetcher{stateJSON: blockedStateJSON(), schedulerJSON: minimalSchedulerJSON()})
	out := renderSnapshotOpts(snap, cmux.RenderOptions{})
	if strings.Contains(out, "CMUX BLOCKED REPORT") {
		t.Error("normal output must not contain CMUX BLOCKED REPORT header")
	}
}

func TestBlocked_RuntimeStateError_GracefulDegradation(t *testing.T) {
	snap := cmux.Read(stubFetcher{
		stateErr:      fmt.Errorf("runtime unreachable"),
		schedulerJSON: minimalSchedulerJSON(),
	})
	out := renderSnapshotOpts(snap, blockedOpts())
	if !strings.Contains(out, "CMUX BLOCKED REPORT") {
		t.Error("blocked header must still appear on runtime error")
	}
	if !strings.Contains(out, "runtime-state: error") {
		t.Errorf("blocked error degradation must show runtime-state error; output:\n%s", out)
	}
}
