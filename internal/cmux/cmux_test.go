package cmux_test

import (
	"bytes"
	"errors"
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
	cmux.Render(&buf, snap)
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
