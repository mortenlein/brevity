package bubbleteadashboard

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mortenlein/brevity/internal/actions"
	runtimeexecution "github.com/mortenlein/brevity/internal/runtime/execution"
	runtimequeue "github.com/mortenlein/brevity/internal/runtime/queue"
	"github.com/mortenlein/brevity/internal/state"
)

func TestPlanningWorkspaceRenders(t *testing.T) {
	model := NewPlanningModel("Brevity exists to become the fastest and most pleasant place to review, approve, and merge AI-generated software work.", nil)

	output := model.View()

	for _, want := range []string{
		"BREVITY PLAN",
		"Product Goal",
		"Planning Flow",
		"idea",
		"plan",
		"task",
		"execute",
		"review",
		"merge",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("planning output missing %q:\n%s", want, output)
		}
	}
}

func TestPlanningWorkspaceSurfacesProductGoal(t *testing.T) {
	model := NewPlanningModel(`# Brevity Product Goal

Brevity exists to become the fastest and most pleasant place to review, approve, and merge AI-generated software work.

The primary product is operator leverage.
`, nil)

	output := model.View()

	for _, want := range []string{
		"Brevity Product Goal",
		"fastest and most pleasant place",
		"operator leverage",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("planning output missing product goal %q:\n%s", want, output)
		}
	}
}

func TestPlanningWorkspaceEmptyStateIsUseful(t *testing.T) {
	model := NewPlanningModel("Operator leverage.", nil)

	output := model.View()

	for _, want := range []string{
		"No ideas captured yet.",
		"Press n to capture",
		"operator intent",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("planning empty state missing %q:\n%s", want, output)
		}
	}
}

func TestPlanningWorkspaceActionsRender(t *testing.T) {
	model := NewPlanningModel("Operator leverage.", nil)

	output := model.View()

	for _, want := range []string{
		"n new idea",
		"enter inspect",
		"t task draft",
		"p promote",
		"q quit",
	} {
		if !strings.Contains(stripANSI(output), want) {
			t.Fatalf("planning actions missing %q:\n%s", want, output)
		}
	}
}

func TestPlanningWorkspaceCreatesAndPersistsIdea(t *testing.T) {
	root := planningFixture(t)
	model := NewPlanningModel("Operator leverage.", nil)
	model.repoRoot = root
	model.store = newPlanningIdeaStore(root)
	model.now = fixedPlanningTime

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	model = updated.(PlanningModel)
	model.inputValue = "File-level diff navigation"
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(PlanningModel)

	if len(model.ideas) != 1 {
		t.Fatalf("ideas = %#v, want one idea", model.ideas)
	}
	if model.ideas[0].Status != "captured" || model.ideas[0].CreatedAt != "2026-06-01T10:00:00Z" {
		t.Fatalf("idea metadata = %#v", model.ideas[0])
	}

	data, err := os.ReadFile(filepath.Join(root, planningIdeasPath))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) || !strings.Contains(string(data), "File-level diff navigation") {
		t.Fatalf("ideas store not durable JSON:\n%s", data)
	}
}

func TestPlanningWorkspaceCreatesTaskDraftFromIdea(t *testing.T) {
	root := planningFixture(t)
	model := NewPlanningModel("Operator leverage.", nil)
	model.repoRoot = root
	model.store = newPlanningIdeaStore(root)
	model.draftStore = newPlanningTaskDraftStore(root)
	model.now = fixedPlanningTime
	model.ideas = []PlanningIdea{{
		ID:        "idea-1",
		Title:     "File-level diff navigation",
		CreatedAt: "2026-06-01T09:00:00Z",
		Status:    "captured",
	}}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	model = updated.(PlanningModel)

	if len(model.drafts) != 1 {
		t.Fatalf("drafts = %#v, want one draft", model.drafts)
	}
	draft := model.drafts[0]
	if draft.Title != "File-level diff navigation" || draft.Status != "draft" || draft.IdeaID != "idea-1" {
		t.Fatalf("draft metadata = %#v", draft)
	}
	if draft.Description != "Generated from planning idea" ||
		draft.AcceptanceCriteria != "[placeholder]" ||
		draft.Validation != "[placeholder]" ||
		draft.CreatedAt != "2026-06-01T10:00:00Z" {
		t.Fatalf("draft content = %#v", draft)
	}
}

func TestPlanningWorkspacePersistsTaskDraft(t *testing.T) {
	root := planningFixture(t)
	model := NewPlanningModel("Operator leverage.", nil)
	model.repoRoot = root
	model.draftStore = newPlanningTaskDraftStore(root)
	model.now = fixedPlanningTime
	model.ideas = []PlanningIdea{{
		ID:        "idea-1",
		Title:     "Review notes persistence",
		CreatedAt: "2026-06-01T09:00:00Z",
		Status:    "captured",
	}}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	model = updated.(PlanningModel)

	data, err := os.ReadFile(filepath.Join(root, planningTaskDraftsPath))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) || !strings.Contains(string(data), "Review notes persistence") {
		t.Fatalf("task draft store not durable JSON:\n%s", data)
	}
}

func TestPlanningWorkspaceReloadsIdeas(t *testing.T) {
	root := planningFixture(t)
	store := newPlanningIdeaStore(root)
	if err := store.Save([]PlanningIdea{{
		ID:        "idea-1",
		Title:     "Merge experience improvements",
		CreatedAt: "2026-06-01T10:00:00Z",
		Status:    "planning",
	}}); err != nil {
		t.Fatal(err)
	}

	var output strings.Builder
	if err := RunPlan(context.Background(), strings.NewReader("q\n"), &output, root); err != nil {
		t.Fatalf("RunPlan returned error: %v", err)
	}

	for _, want := range []string{
		"Ideas",
		"Merge experience improvements",
		"Selected Idea",
		"Status:  planning",
		"[t] Create Task Draft",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("reloaded planning output missing %q:\n%s", want, output.String())
		}
	}
}

func TestPlanningWorkspaceReloadsTaskDrafts(t *testing.T) {
	root := planningFixture(t)
	if err := newPlanningIdeaStore(root).Save([]PlanningIdea{{
		ID:        "idea-1",
		Title:     "Merge experience improvements",
		CreatedAt: "2026-06-01T09:00:00Z",
		Status:    "captured",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := newPlanningTaskDraftStore(root).Save([]PlanningTaskDraft{{
		ID:                 "draft-1",
		IdeaID:             "idea-1",
		Title:              "Merge experience improvements",
		Description:        "Generated from planning idea",
		Status:             "draft",
		AcceptanceCriteria: "[placeholder]",
		Validation:         "[placeholder]",
		CreatedAt:          "2026-06-01T10:00:00Z",
	}}); err != nil {
		t.Fatal(err)
	}

	var output strings.Builder
	if err := RunPlan(context.Background(), strings.NewReader("q\n"), &output, root); err != nil {
		t.Fatalf("RunPlan returned error: %v", err)
	}

	for _, want := range []string{
		"Task Drafts",
		"Merge experience improvements",
		"Selected Draft",
		"Acceptance Criteria",
		"Validation",
		"[p] Promote To Task",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("reloaded planning output missing %q:\n%s", want, output.String())
		}
	}
}

func TestPlanningWorkspaceDoesNotDuplicateTaskDraftForIdea(t *testing.T) {
	root := planningFixture(t)
	model := NewPlanningModel("Operator leverage.", nil)
	model.repoRoot = root
	model.draftStore = newPlanningTaskDraftStore(root)
	model.now = fixedPlanningTime
	model.ideas = []PlanningIdea{{
		ID:        "idea-1",
		Title:     "File-level diff navigation",
		CreatedAt: "2026-06-01T09:00:00Z",
		Status:    "captured",
	}}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	model = updated.(PlanningModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	model = updated.(PlanningModel)

	if len(model.drafts) != 1 {
		t.Fatalf("drafts = %#v, want one draft after duplicate create", model.drafts)
	}
	if !strings.Contains(model.message, "already exists") {
		t.Fatalf("message = %q, want duplicate guidance", model.message)
	}
}

func TestPlanningWorkspaceTaskDraftRendering(t *testing.T) {
	model := NewPlanningModel("Operator leverage.", nil)
	model.ideas = []PlanningIdea{{
		ID:        "idea-1",
		Title:     "Review notes persistence",
		CreatedAt: "2026-06-01T09:00:00Z",
		Status:    "captured",
	}}
	model.drafts = []PlanningTaskDraft{{
		ID:                 "draft-1",
		IdeaID:             "idea-1",
		Title:              "Review notes persistence",
		Description:        "Generated from planning idea",
		Status:             "draft",
		AcceptanceCriteria: "[placeholder]",
		Validation:         "[placeholder]",
		CreatedAt:          "2026-06-01T10:00:00Z",
	}}

	output := model.View()

	for _, want := range []string{
		"Task Drafts",
		"status: draft",
		"Selected Draft",
		"Title:   Review notes persistence",
		"Acceptance Criteria",
		"[placeholder]",
		"[refine] placeholder",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("task draft output missing %q:\n%s", want, output)
		}
	}
}

func TestPlanningWorkspaceTaskDraftEmptyState(t *testing.T) {
	model := NewPlanningModel("Operator leverage.", nil)
	model.ideas = []PlanningIdea{{
		ID:        "idea-1",
		Title:     "Review notes persistence",
		CreatedAt: "2026-06-01T09:00:00Z",
		Status:    "captured",
	}}

	output := model.View()

	for _, want := range []string{
		"Task Drafts",
		"No task drafts yet.",
		"press t",
		"without touching",
		"execution state",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("task draft empty state missing %q:\n%s", want, output)
		}
	}
}

func TestPlanningWorkspacePromotesTaskDraft(t *testing.T) {
	root := planningFixture(t)
	model := NewPlanningModel("Operator leverage.", nil)
	model.repoRoot = root
	model.draftStore = newPlanningTaskDraftStore(root)
	model.now = fixedPlanningTime
	model.drafts = []PlanningTaskDraft{{
		ID:                 "draft-1",
		IdeaID:             "idea-1",
		Title:              "File-level diff navigation",
		Description:        "Generated from planning idea",
		Status:             "draft",
		AcceptanceCriteria: "[placeholder]",
		Validation:         "[placeholder]",
		CreatedAt:          "2026-06-01T09:00:00Z",
	}}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	model = updated.(PlanningModel)

	if len(model.drafts) != 1 || model.drafts[0].Status != "promoted" || model.drafts[0].TaskSlug != "file-level-diff-navigation" {
		t.Fatalf("promoted draft = %#v", model.drafts)
	}
	if model.drafts[0].PromotedAt != "2026-06-01T10:00:00Z" {
		t.Fatalf("promotedAt = %q", model.drafts[0].PromotedAt)
	}
}

func TestPlanningWorkspacePromotedTaskPersistsToTasks(t *testing.T) {
	root := planningFixture(t)
	model := NewPlanningModel("Operator leverage.", nil)
	model.repoRoot = root
	model.draftStore = newPlanningTaskDraftStore(root)
	model.now = fixedPlanningTime
	model.drafts = []PlanningTaskDraft{{
		ID:                 "draft-1",
		IdeaID:             "idea-1",
		Title:              "Review notes persistence",
		Description:        "Generated from planning idea",
		Status:             "draft",
		AcceptanceCriteria: "[placeholder]",
		Validation:         "[placeholder]",
		CreatedAt:          "2026-06-01T09:00:00Z",
	}}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	model = updated.(PlanningModel)

	data, err := os.ReadFile(filepath.Join(root, ".brevity", "tasks.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"slug": "review-notes-persistence"`,
		`"description": "Generated from planning idea"`,
		`"status": "draft"`,
		`"normalizedState": "draft"`,
		`"createdAt": "2026-06-01T09:00:00Z"`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("tasks.json missing %q:\n%s", want, string(data))
		}
	}
}

func TestPlanningWorkspaceDuplicatePromotionProtection(t *testing.T) {
	root := planningFixture(t)
	model := NewPlanningModel("Operator leverage.", nil)
	model.repoRoot = root
	model.draftStore = newPlanningTaskDraftStore(root)
	model.now = fixedPlanningTime
	model.drafts = []PlanningTaskDraft{{
		ID:                 "draft-1",
		IdeaID:             "idea-1",
		Title:              "Review notes persistence",
		Description:        "Generated from planning idea",
		Status:             "draft",
		AcceptanceCriteria: "[placeholder]",
		Validation:         "[placeholder]",
		CreatedAt:          "2026-06-01T09:00:00Z",
	}}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	model = updated.(PlanningModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	model = updated.(PlanningModel)

	if !strings.Contains(model.message, "already promoted") {
		t.Fatalf("message = %q, want duplicate promotion guidance", model.message)
	}
	data, err := os.ReadFile(filepath.Join(root, ".brevity", "tasks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(data), "review-notes-persistence"); count != 2 {
		t.Fatalf("task occurrence count = %d, want slug and id only:\n%s", count, string(data))
	}
}

func TestPlanningWorkspaceReloadsPromotedDrafts(t *testing.T) {
	root := planningFixture(t)
	if err := newPlanningTaskDraftStore(root).Save([]PlanningTaskDraft{{
		ID:                 "draft-1",
		IdeaID:             "idea-1",
		TaskSlug:           "review-notes-persistence",
		Title:              "Review notes persistence",
		Description:        "Generated from planning idea",
		Status:             "promoted",
		AcceptanceCriteria: "[placeholder]",
		Validation:         "[placeholder]",
		CreatedAt:          "2026-06-01T09:00:00Z",
		PromotedAt:         "2026-06-01T10:00:00Z",
	}}); err != nil {
		t.Fatal(err)
	}

	var output strings.Builder
	if err := RunPlan(context.Background(), strings.NewReader("q\n"), &output, root); err != nil {
		t.Fatalf("RunPlan returned error: %v", err)
	}

	for _, want := range []string{
		"Status:  promoted",
		"Task:    review-notes-persistence",
		"Promoted: 2026-06-01T10:00:00Z",
		"[a] Activate Task",
		"> Task Activated",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("promoted draft output missing %q:\n%s", want, output.String())
		}
	}
}

func TestPlanningWorkspacePromotionGuidanceRendering(t *testing.T) {
	model := NewPlanningModel("Operator leverage.", nil)
	model.drafts = []PlanningTaskDraft{{
		ID:                 "draft-1",
		IdeaID:             "idea-1",
		TaskSlug:           "file-level-diff-navigation",
		Title:              "File-level diff navigation",
		Description:        "Generated from planning idea",
		Status:             "promoted",
		AcceptanceCriteria: "[placeholder]",
		Validation:         "[placeholder]",
		CreatedAt:          "2026-06-01T09:00:00Z",
		PromotedAt:         "2026-06-01T10:00:00Z",
	}}

	output := model.View()

	for _, want := range []string{
		"status: promoted",
		"Selected Draft",
		"Task:    file-level-diff-navigation",
		"[a] Activate Task",
		"> Task Activated",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("promotion guidance missing %q:\n%s", want, output)
		}
	}
}

func TestPlanningWorkspaceActivatesPromotedTask(t *testing.T) {
	root := planningActivationFixture(t)
	model := promotedActivationModel(t, root)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = updated.(PlanningModel)

	if !strings.Contains(model.message, "Task activated: review-notes-persistence") {
		t.Fatalf("message = %q, want activation result", model.message)
	}
	active, task := model.taskActivation("review-notes-persistence")
	if !active {
		t.Fatalf("task not active after activation: %#v", model.tasks)
	}
	for _, path := range []string{task.WorktreePath, task.PromptPath, task.SpecPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("activation artifact missing %s: %v", path, err)
		}
	}
	if task.Status != "ready-for-worker" || task.NormalizedState != "ready-for-worker" {
		t.Fatalf("task status = %#v, want ready-for-worker", task)
	}
}

func TestPlanningWorkspaceDuplicateActivationProtection(t *testing.T) {
	root := planningActivationFixture(t)
	model := promotedActivationModel(t, root)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = updated.(PlanningModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = updated.(PlanningModel)

	if !strings.Contains(model.message, "already activated") {
		t.Fatalf("message = %q, want duplicate activation guidance", model.message)
	}
	tasks, _, err := state.LoadTasks(state.Store{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks.Items) != 1 {
		t.Fatalf("tasks = %#v, want one task", tasks.Items)
	}
}

func TestPlanningWorkspaceActivationStatusRendering(t *testing.T) {
	root := planningActivationFixture(t)
	model := promotedActivationModel(t, root)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = updated.(PlanningModel)

	output := model.View()

	for _, want := range []string{
		"Active:  yes",
		"Branch:  task/review-notes-persistence",
		"Worktree:",
		"Prompt:",
		"v Task Activated",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("activation status output missing %q:\n%s", want, output)
		}
	}
}

func TestPlanningWorkspaceActivationGuidanceRendering(t *testing.T) {
	root := planningActivationFixture(t)
	model := promotedActivationModel(t, root)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = updated.(PlanningModel)

	output := model.View()

	for _, want := range []string{
		"Workflow",
		"v Idea Captured",
		"v Draft Created",
		"v Task Created",
		"Next Steps",
		"queue task",
		"execute task",
		"review task",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("activation guidance missing %q:\n%s", want, output)
		}
	}
}

func TestPlanningExecutionHandoffActivatedNoQueueRecommendsQueueAdd(t *testing.T) {
	root := planningActivationFixture(t)
	model := activatedPlanningModel(t, root)

	output := model.View()

	for _, want := range []string{
		"Execution Handoff",
		"Task activated",
		"Queue: not queued",
		"Reservation: none",
		"Execution: none",
		"Queue this task",
		"brevity queue add review-notes-persistence",
		"brevity queue inspect",
		"brevity scheduler plan",
		"[q] Queue Task",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("handoff output missing %q:\n%s", want, output)
		}
	}
	for _, hidden := range []string{"[r] Reserve Task", "[e] Create Execution Plan"} {
		if strings.Contains(output, hidden) {
			t.Fatalf("invalid action should be hidden %q:\n%s", hidden, output)
		}
	}
}

func TestPlanningExecutionHandoffQueuedTaskRecommendsSchedulerPlan(t *testing.T) {
	root := planningActivationFixture(t)
	model := activatedPlanningModel(t, root)
	writePlanningQueue(t, root, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		planningQueueItem("queue-1", "review-notes-persistence", nil),
	}})

	output := model.View()

	for _, want := range []string{
		"Queue: queue-1 (queued)",
		"Reservation: none",
		"Plan and reserve queued task",
		"brevity scheduler plan",
		"brevity scheduler reserve-next",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("queued handoff missing %q:\n%s", want, output)
		}
	}
}

func TestPlanningExecutionHandoffReservedTaskRecommendsPlanExecution(t *testing.T) {
	root := planningActivationFixture(t)
	model := activatedPlanningModel(t, root)
	reservation := &runtimequeue.Reservation{Owner: "runtime-supervisor", ReservedAt: "2026-06-01T10:05:00Z", ReservationID: "res-1"}
	writePlanningQueue(t, root, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		planningQueueItem("queue-1", "review-notes-persistence", reservation),
	}})

	output := model.View()

	for _, want := range []string{
		"Reservation: res-1 by runtime-supervisor",
		"Plan execution from reservation",
		"brevity scheduler plan-execution",
		"brevity execution list",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("reserved handoff missing %q:\n%s", want, output)
		}
	}
}

func TestPlanningExecutionHandoffExecutionStates(t *testing.T) {
	cases := []struct {
		name     string
		status   string
		expected []string
	}{
		{name: "planned", status: runtimeexecution.StatusPlanned, expected: []string{"Execution: exec-1 (planned)", "Mark execution ready", "brevity execution mark-ready exec-1"}},
		{name: "ready", status: runtimeexecution.StatusReady, expected: []string{"Execution: exec-1 (ready)", "Preflight and dry-run launch", "brevity execution preflight exec-1", "brevity execution launch-dry-run exec-1"}},
		{name: "completed", status: runtimeexecution.StatusCompleted, expected: []string{"Execution: exec-1 (completed)", "Review generated work", "brevity review", "brevity cmux --review review-notes-persistence"}},
		{name: "failed", status: runtimeexecution.StatusFailed, expected: []string{"Execution: exec-1 (failed)", "Inspect execution failure", "brevity execution inspect", "brevity review"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := planningActivationFixture(t)
			model := activatedPlanningModel(t, root)
			reservation := &runtimequeue.Reservation{Owner: "runtime-supervisor", ReservedAt: "2026-06-01T10:05:00Z", ReservationID: "res-1"}
			writePlanningQueue(t, root, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
				planningQueueItem("queue-1", "review-notes-persistence", reservation),
			}})
			writePlanningExecutions(t, root, runtimeexecution.Executions{Version: runtimeexecution.Version, Records: []runtimeexecution.Record{
				planningExecutionRecord("exec-1", "queue-1", "review-notes-persistence", "res-1", tc.status),
			}})

			output := model.View()
			for _, want := range tc.expected {
				if !strings.Contains(output, want) {
					t.Fatalf("%s handoff missing %q:\n%s", tc.name, want, output)
				}
			}
		})
	}
}

func TestPlanningReviewHandoffCompletedExecutionRecommendsReview(t *testing.T) {
	root := planningActivationFixture(t)
	model := activatedPlanningModel(t, root)
	writeCompletedPlanningExecution(t, root, runtimeexecution.StatusCompleted)

	output := model.View()

	for _, want := range []string{
		"Workflow Progress",
		"v Execution Planned",
		"v Execution Completed",
		"Review Handoff",
		"Execution completed",
		"Review generated work",
		"Execution completed successfully and is ready for operator review.",
		"[w] Open Review Workspace",
		"[v] View Execution",
		"[b] Back",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("completed review handoff missing %q:\n%s", want, output)
		}
	}
	for _, hidden := range []string{"[q] Queue Task", "[r] Reserve Task", "[e] Create Execution Plan"} {
		if strings.Contains(output, hidden) {
			t.Fatalf("completed review handoff should hide %q:\n%s", hidden, output)
		}
	}
}

func TestPlanningReviewHandoffFailedExecutionRecommendsFailureReview(t *testing.T) {
	root := planningActivationFixture(t)
	model := activatedPlanningModel(t, root)
	writeCompletedPlanningExecution(t, root, runtimeexecution.StatusFailed)

	output := model.View()

	for _, want := range []string{
		"v Execution Failed",
		"Review Handoff",
		"Execution failed",
		"Inspect execution failure",
		"[v] View Execution",
		"[r] Review Failure Details",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("failed review handoff missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "[w] Open Review Workspace") {
		t.Fatalf("failed execution should not show open review workspace action:\n%s", output)
	}
}

func TestPlanningReviewHandoffPlannedExecutionStaysInExecutionWorkflow(t *testing.T) {
	root := planningActivationFixture(t)
	model := activatedPlanningModel(t, root)
	writeCompletedPlanningExecution(t, root, runtimeexecution.StatusPlanned)

	output := model.View()

	for _, want := range []string{
		"Execution Handoff",
		"Mark execution ready",
		"[v] View Execution",
		"[b] Back",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("planned execution handoff missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "Review Handoff") || strings.Contains(output, "[w] Open Review Workspace") {
		t.Fatalf("planned execution should not enter review handoff:\n%s", output)
	}
}

func TestPlanningReviewHandoffOpenReviewActionWiring(t *testing.T) {
	root := planningActivationFixture(t)
	model := activatedPlanningModel(t, root)
	writeCompletedPlanningExecution(t, root, runtimeexecution.StatusCompleted)
	beforeQueue := readPlanningFile(t, filepath.Join(root, ".brevity", runtimequeue.FileName))
	beforeExecutions := readPlanningFile(t, filepath.Join(root, ".brevity", runtimeexecution.FileName))

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	model = updated.(PlanningModel)

	if !strings.Contains(model.message, "Open review workspace: brevity review") {
		t.Fatalf("message = %q, want review workspace handoff", model.message)
	}
	if after := readPlanningFile(t, filepath.Join(root, ".brevity", runtimequeue.FileName)); after != beforeQueue {
		t.Fatalf("queue mutated\nbefore: %s\nafter: %s", beforeQueue, after)
	}
	if after := readPlanningFile(t, filepath.Join(root, ".brevity", runtimeexecution.FileName)); after != beforeExecutions {
		t.Fatalf("executions mutated\nbefore: %s\nafter: %s", beforeExecutions, after)
	}
	assertPlanningNoLaunchState(t, root)
}

func TestPlanningReviewHandoffFailureReviewActionWiring(t *testing.T) {
	root := planningActivationFixture(t)
	model := activatedPlanningModel(t, root)
	writeCompletedPlanningExecution(t, root, runtimeexecution.StatusFailed)
	beforeQueue := readPlanningFile(t, filepath.Join(root, ".brevity", runtimequeue.FileName))
	beforeExecutions := readPlanningFile(t, filepath.Join(root, ".brevity", runtimeexecution.FileName))

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model = updated.(PlanningModel)

	if !strings.Contains(model.message, "Review failure details for execution exec-1.") {
		t.Fatalf("message = %q, want failure review context", model.message)
	}
	if after := readPlanningFile(t, filepath.Join(root, ".brevity", runtimequeue.FileName)); after != beforeQueue {
		t.Fatalf("queue mutated\nbefore: %s\nafter: %s", beforeQueue, after)
	}
	if after := readPlanningFile(t, filepath.Join(root, ".brevity", runtimeexecution.FileName)); after != beforeExecutions {
		t.Fatalf("executions mutated\nbefore: %s\nafter: %s", beforeExecutions, after)
	}
	assertPlanningNoLaunchState(t, root)
}

func TestPlanningReviewHandoffNarrowWidthReadable(t *testing.T) {
	root := planningActivationFixture(t)
	model := activatedPlanningModel(t, root)
	writeCompletedPlanningExecution(t, root, runtimeexecution.StatusCompleted)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 32, Height: 30})
	model = updated.(PlanningModel)

	output := model.View()

	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		if visibleWidth(stripANSI(line)) > model.contentWidth() {
			t.Fatalf("line exceeds width %d: %q\n%s", model.contentWidth(), line, output)
		}
	}
	if !strings.Contains(output, "Review Handoff") || !strings.Contains(output, "Open Review") {
		t.Fatalf("narrow review handoff dropped key sections:\n%s", output)
	}
}

func TestPlanningExecutionHandoffNarrowWidthReadable(t *testing.T) {
	root := planningActivationFixture(t)
	model := activatedPlanningModel(t, root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 32, Height: 30})
	model = updated.(PlanningModel)

	output := model.View()

	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		if visibleWidth(stripANSI(line)) > model.contentWidth() {
			t.Fatalf("line exceeds width %d: %q\n%s", model.contentWidth(), line, output)
		}
	}
	if !strings.Contains(output, "Execution Handoff") || !strings.Contains(output, "Queue this task") {
		t.Fatalf("narrow handoff dropped key sections:\n%s", output)
	}
}

func TestPlanningExecutionHandoffGuidanceDoesNotMutateRuntimeState(t *testing.T) {
	root := planningActivationFixture(t)
	model := activatedPlanningModel(t, root)
	reservation := &runtimequeue.Reservation{Owner: "runtime-supervisor", ReservedAt: "2026-06-01T10:05:00Z", ReservationID: "res-1"}
	writePlanningQueue(t, root, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		planningQueueItem("queue-1", "review-notes-persistence", reservation),
	}})
	writePlanningExecutions(t, root, runtimeexecution.Executions{Version: runtimeexecution.Version, Records: []runtimeexecution.Record{
		planningExecutionRecord("exec-1", "queue-1", "review-notes-persistence", "res-1", runtimeexecution.StatusReady),
	}})
	stateFiles := map[string]string{
		filepath.Join(root, ".brevity", runtimequeue.FileName):     readPlanningFile(t, filepath.Join(root, ".brevity", runtimequeue.FileName)),
		filepath.Join(root, ".brevity", runtimeexecution.FileName): readPlanningFile(t, filepath.Join(root, ".brevity", runtimeexecution.FileName)),
	}

	_ = model.View()

	for path, before := range stateFiles {
		if after := readPlanningFile(t, path); after != before {
			t.Fatalf("%s mutated\nbefore: %s\nafter: %s", path, before, after)
		}
	}
}

func TestPlanningWorkflowProgressRendering(t *testing.T) {
	root := planningActivationFixture(t)
	model := activatedPlanningModel(t, root)
	writePlanningQueue(t, root, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		planningQueueItem("queue-1", "review-notes-persistence", &runtimequeue.Reservation{Owner: "runtime-supervisor", ReservedAt: "2026-06-01T10:05:00Z", ReservationID: "res-1"}),
	}})

	output := model.View()

	for _, want := range []string{
		"Workflow Progress",
		"v Idea Captured",
		"v Draft Created",
		"v Task Created",
		"v Task Activated",
		"v Task Queued",
		"v Task Reserved",
		"> Execution Planned",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("workflow progress missing %q:\n%s", want, output)
		}
	}
}

func TestPlanningWorkflowQueueAction(t *testing.T) {
	root := planningActivationFixture(t)
	model := activatedPlanningModel(t, root)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	model = updated.(PlanningModel)

	if !strings.Contains(model.message, "Queued task: review-notes-persistence") {
		t.Fatalf("message = %q, want queue success", model.message)
	}
	queueState := readPlanningQueue(t, root)
	if len(queueState.Items) != 1 || queueState.Items[0].Task != "review-notes-persistence" || queueState.Items[0].Reservation != nil {
		t.Fatalf("queue = %#v, want one unreserved task item", queueState.Items)
	}
	assertPlanningNoLaunchState(t, root)
}

func TestPlanningWorkflowReserveAction(t *testing.T) {
	root := planningActivationFixture(t)
	model := activatedPlanningModel(t, root)
	writePlanningQueue(t, root, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		planningQueueItem("queue-1", "review-notes-persistence", nil),
	}})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model = updated.(PlanningModel)

	if !strings.Contains(model.message, "Reserved task: review-notes-persistence") {
		t.Fatalf("message = %q, want reserve success", model.message)
	}
	item := readPlanningQueue(t, root).Items[0]
	if item.Reservation == nil || strings.TrimSpace(item.Reservation.ReservationID) == "" {
		t.Fatalf("queue item not reserved: %#v", item)
	}
	assertPlanningNoExecutions(t, root)
	assertPlanningNoLaunchState(t, root)
}

func TestPlanningWorkflowExecutionPlanAction(t *testing.T) {
	root := planningActivationFixture(t)
	model := activatedPlanningModel(t, root)
	writePlanningQueue(t, root, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		planningQueueItem("queue-1", "review-notes-persistence", &runtimequeue.Reservation{Owner: "runtime-supervisor", ReservedAt: "2026-06-01T10:05:00Z", ReservationID: "res-1"}),
	}})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	model = updated.(PlanningModel)

	if !strings.Contains(model.message, "Execution planned:") {
		t.Fatalf("message = %q, want execution plan success", model.message)
	}
	executions := readPlanningExecutions(t, root)
	if len(executions.Records) != 1 {
		t.Fatalf("executions = %#v, want one record", executions.Records)
	}
	record := executions.Records[0]
	if record.QueueItemID != "queue-1" || record.Task != "review-notes-persistence" || record.ReservationID != "res-1" || record.Status != runtimeexecution.StatusPlanned {
		t.Fatalf("execution record = %#v", record)
	}
	assertPlanningNoLaunchState(t, root)
}

func TestPlanningWorkflowPlannedExecutionActionsRenderViewOnly(t *testing.T) {
	root := planningActivationFixture(t)
	model := activatedPlanningModel(t, root)
	reservation := &runtimequeue.Reservation{Owner: "runtime-supervisor", ReservedAt: "2026-06-01T10:05:00Z", ReservationID: "res-1"}
	writePlanningQueue(t, root, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		planningQueueItem("queue-1", "review-notes-persistence", reservation),
	}})
	writePlanningExecutions(t, root, runtimeexecution.Executions{Version: runtimeexecution.Version, Records: []runtimeexecution.Record{
		planningExecutionRecord("exec-1", "queue-1", "review-notes-persistence", "res-1", runtimeexecution.StatusPlanned),
	}})

	output := model.View()

	for _, want := range []string{"[v] View Execution", "[b] Back"} {
		if !strings.Contains(output, want) {
			t.Fatalf("planned execution action missing %q:\n%s", want, output)
		}
	}
	for _, hidden := range []string{"[q] Queue Task", "[r] Reserve Task", "[e] Create Execution Plan"} {
		if strings.Contains(output, hidden) {
			t.Fatalf("invalid planned execution action visible %q:\n%s", hidden, output)
		}
	}
}

func TestPlanningWorkflowViewExecutionAction(t *testing.T) {
	root := planningActivationFixture(t)
	model := activatedPlanningModel(t, root)
	writePlanningQueue(t, root, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		planningQueueItem("queue-1", "review-notes-persistence", &runtimequeue.Reservation{Owner: "runtime-supervisor", ReservedAt: "2026-06-01T10:05:00Z", ReservationID: "res-1"}),
	}})
	writePlanningExecutions(t, root, runtimeexecution.Executions{Version: runtimeexecution.Version, Records: []runtimeexecution.Record{
		planningExecutionRecord("exec-1", "queue-1", "review-notes-persistence", "res-1", runtimeexecution.StatusPlanned),
	}})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	model = updated.(PlanningModel)

	if !strings.Contains(model.message, "Execution exec-1 is planned.") {
		t.Fatalf("message = %q, want execution view context", model.message)
	}
}

func TestPlanningWorkflowActionErrorRendering(t *testing.T) {
	root := planningActivationFixture(t)
	model := activatedPlanningModel(t, root)
	writeRawPlanningRuntimeQueue(t, root, `{"version":1,"items":[`)

	model = model.queueSelectedTask()

	if !strings.Contains(model.message, "Could not queue task:") {
		t.Fatalf("message = %q, want queue error context", model.message)
	}
	if !strings.Contains(model.View(), "Could not queue task:") {
		t.Fatalf("view did not keep error in context:\n%s", model.View())
	}
}

func TestPlanningWorkspaceReloadsActivatedTasks(t *testing.T) {
	root := planningActivationFixture(t)
	model := promotedActivationModel(t, root)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = updated.(PlanningModel)

	var output strings.Builder
	if err := RunPlan(context.Background(), strings.NewReader("q\n"), &output, root); err != nil {
		t.Fatalf("RunPlan returned error: %v", err)
	}

	for _, want := range []string{
		"Active:  yes",
		"v Task Activated",
		"queue task",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("reloaded activation output missing %q:\n%s", want, output.String())
		}
	}
}

func TestPlanningWorkspaceActivatedNarrowWidthReadable(t *testing.T) {
	root := planningActivationFixture(t)
	model := promotedActivationModel(t, root)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = updated.(PlanningModel)
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 32, Height: 30})
	model = updated.(PlanningModel)

	output := model.View()

	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		if visibleWidth(stripANSI(line)) > model.contentWidth() {
			t.Fatalf("line exceeds width %d: %q\n%s", model.contentWidth(), line, output)
		}
	}
	if !strings.Contains(output, "Workflow") || !strings.Contains(output, "Task Activated") {
		t.Fatalf("narrow activation output dropped key sections:\n%s", output)
	}
}

func TestPlanningWorkspaceActivationDoesNotMutateQueueExecutionOrRuntime(t *testing.T) {
	root := planningActivationFixture(t)
	model := promotedActivationModel(t, root)
	stateFiles := map[string]string{
		filepath.Join(root, ".brevity", "queue.json"):         `[{"id":"q1","task":"alpha","status":"queued"}]` + "\n",
		filepath.Join(root, ".brevity", "executions.json"):    `[{"id":"e1","task":"alpha","status":"planned"}]` + "\n",
		filepath.Join(root, ".brevity", "runtime-state.json"): `{"status":"idle"}` + "\n",
		filepath.Join(root, ".brevity", "runs.jsonl"):         `{"runId":"r1","slug":"alpha"}` + "\n",
	}
	for path, data := range stateFiles {
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = updated.(PlanningModel)

	for path, before := range stateFiles {
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != before {
			t.Fatalf("%s mutated\nbefore: %s\nafter: %s", path, before, after)
		}
	}
}

func TestPlanningWorkspaceStatusRendering(t *testing.T) {
	model := NewPlanningModel("Operator leverage.", nil)
	model.ideas = []PlanningIdea{{
		ID:        "idea-1",
		Title:     "Review notes persistence",
		CreatedAt: "2026-06-01T10:00:00Z",
		Status:    "accepted",
	}}

	output := model.View()

	for _, want := range []string{
		"Review notes persistence [accepted]",
		"Status:  accepted",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("status output missing %q:\n%s", want, output)
		}
	}
}

func TestPlanningWorkspaceNarrowWidthReadable(t *testing.T) {
	model := NewPlanningModel("Operator leverage starts with a focused planning workspace.", nil)
	model.ideas = []PlanningIdea{{
		ID:        "idea-1",
		Title:     "A very long planning idea title that must not break narrow rendering",
		CreatedAt: "2026-06-01T10:00:00Z",
		Status:    "captured",
	}}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 32, Height: 30})
	model = updated.(PlanningModel)

	output := model.View()

	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		if visibleWidth(stripANSI(line)) > model.contentWidth() {
			t.Fatalf("line exceeds width %d: %q\n%s", model.contentWidth(), line, output)
		}
	}
	if !strings.Contains(output, "BREVITY PLAN") || !strings.Contains(output, "Next Commands") {
		t.Fatalf("narrow planning output dropped key sections:\n%s", output)
	}
}

func TestPlanningWorkspaceTaskDraftNarrowWidthReadable(t *testing.T) {
	model := NewPlanningModel("Operator leverage starts with a focused planning workspace.", nil)
	model.ideas = []PlanningIdea{{
		ID:        "idea-1",
		Title:     "A very long planning idea title that must not break narrow rendering",
		CreatedAt: "2026-06-01T09:00:00Z",
		Status:    "captured",
	}}
	model.drafts = []PlanningTaskDraft{{
		ID:                 "draft-1",
		IdeaID:             "idea-1",
		Title:              "A very long planning idea title that must not break narrow rendering",
		Description:        "Generated from planning idea",
		Status:             "draft",
		AcceptanceCriteria: "[placeholder]",
		Validation:         "[placeholder]",
		CreatedAt:          "2026-06-01T10:00:00Z",
	}}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 32, Height: 30})
	model = updated.(PlanningModel)

	output := model.View()

	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		if visibleWidth(stripANSI(line)) > model.contentWidth() {
			t.Fatalf("line exceeds width %d: %q\n%s", model.contentWidth(), line, output)
		}
	}
	if !strings.Contains(output, "Task Drafts") || !strings.Contains(output, "Selected Draft") {
		t.Fatalf("narrow draft output dropped key sections:\n%s", output)
	}
}

func TestRunPlanDoesNotMutateRuntimeState(t *testing.T) {
	root := planningFixture(t)

	stateFiles := map[string]string{
		filepath.Join(root, ".brevity", "tasks.json"):      `[{"slug":"alpha","status":"planned"}]` + "\n",
		filepath.Join(root, ".brevity", "queue.json"):      `[{"id":"q1","task":"alpha","status":"queued"}]` + "\n",
		filepath.Join(root, ".brevity", "executions.json"): `[{"id":"e1","task":"alpha","status":"planned"}]` + "\n",
		filepath.Join(root, ".brevity", "runs.jsonl"):      `{"runId":"r1","slug":"alpha"}` + "\n",
	}
	for path, data := range stateFiles {
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var output strings.Builder
	if err := RunPlan(context.Background(), strings.NewReader("q\n"), &output, root); err != nil {
		t.Fatalf("RunPlan returned error: %v", err)
	}
	if !strings.Contains(output.String(), "Mutation Boundary") {
		t.Fatalf("planning output missing mutation boundary:\n%s", output.String())
	}

	for path, before := range stateFiles {
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != before {
			t.Fatalf("%s mutated\nbefore: %s\nafter: %s", path, before, after)
		}
	}
}

func TestRunPlanTaskDraftDoesNotMutateRuntimeState(t *testing.T) {
	root := planningFixture(t)
	if err := newPlanningIdeaStore(root).Save([]PlanningIdea{{
		ID:        "idea-1",
		Title:     "Review notes persistence",
		CreatedAt: "2026-06-01T09:00:00Z",
		Status:    "captured",
	}}); err != nil {
		t.Fatal(err)
	}
	stateFiles := map[string]string{
		filepath.Join(root, ".brevity", "tasks.json"):      `[{"slug":"alpha","status":"planned"}]` + "\n",
		filepath.Join(root, ".brevity", "queue.json"):      `[{"id":"q1","task":"alpha","status":"queued"}]` + "\n",
		filepath.Join(root, ".brevity", "executions.json"): `[{"id":"e1","task":"alpha","status":"planned"}]` + "\n",
		filepath.Join(root, ".brevity", "runs.jsonl"):      `{"runId":"r1","slug":"alpha"}` + "\n",
	}
	for path, data := range stateFiles {
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var output strings.Builder
	if err := RunPlan(context.Background(), strings.NewReader("t\nq\n"), &output, root); err != nil {
		t.Fatalf("RunPlan returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, planningTaskDraftsPath)); err != nil {
		t.Fatalf("task draft store was not created: %v", err)
	}

	for path, before := range stateFiles {
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != before {
			t.Fatalf("%s mutated\nbefore: %s\nafter: %s", path, before, after)
		}
	}
}

func TestRunPlanNewIdeaDoesNotMutateRuntimeState(t *testing.T) {
	root := planningFixture(t)
	stateFiles := map[string]string{
		filepath.Join(root, ".brevity", "tasks.json"):      `[{"slug":"alpha","status":"planned"}]` + "\n",
		filepath.Join(root, ".brevity", "queue.json"):      `[{"id":"q1","task":"alpha","status":"queued"}]` + "\n",
		filepath.Join(root, ".brevity", "executions.json"): `[{"id":"e1","task":"alpha","status":"planned"}]` + "\n",
		filepath.Join(root, ".brevity", "runs.jsonl"):      `{"runId":"r1","slug":"alpha"}` + "\n",
	}
	for path, data := range stateFiles {
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var output strings.Builder
	if err := RunPlan(context.Background(), strings.NewReader("n\nReview notes persistence\nq\n"), &output, root); err != nil {
		t.Fatalf("RunPlan returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, planningIdeasPath)); err != nil {
		t.Fatalf("ideas store was not created: %v", err)
	}

	for path, before := range stateFiles {
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != before {
			t.Fatalf("%s mutated\nbefore: %s\nafter: %s", path, before, after)
		}
	}
}

func activatedPlanningModel(t *testing.T, root string) PlanningModel {
	t.Helper()
	model := promotedActivationModel(t, root)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	return updated.(PlanningModel)
}

func writePlanningQueue(t *testing.T, root string, queue runtimequeue.Queue) {
	t.Helper()
	store, err := runtimequeue.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Store.WriteJSON(runtimequeue.FileName, queue); err != nil {
		t.Fatal(err)
	}
}

func writeRawPlanningRuntimeQueue(t *testing.T, root string, data string) {
	t.Helper()
	path := filepath.Join(root, ".brevity", runtimequeue.FileName)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readPlanningQueue(t *testing.T, root string) runtimequeue.Queue {
	t.Helper()
	store, err := runtimequeue.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	queueState, _, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return queueState
}

func writePlanningExecutions(t *testing.T, root string, executions runtimeexecution.Executions) {
	t.Helper()
	store, err := runtimeexecution.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Store.WriteJSON(runtimeexecution.FileName, executions); err != nil {
		t.Fatal(err)
	}
}

func writeCompletedPlanningExecution(t *testing.T, root string, status string) {
	t.Helper()
	reservation := &runtimequeue.Reservation{Owner: "runtime-supervisor", ReservedAt: "2026-06-01T10:05:00Z", ReservationID: "res-1"}
	writePlanningQueue(t, root, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		planningQueueItem("queue-1", "review-notes-persistence", reservation),
	}})
	writePlanningExecutions(t, root, runtimeexecution.Executions{Version: runtimeexecution.Version, Records: []runtimeexecution.Record{
		planningExecutionRecord("exec-1", "queue-1", "review-notes-persistence", "res-1", status),
	}})
}

func readPlanningExecutions(t *testing.T, root string) runtimeexecution.Executions {
	t.Helper()
	store, err := runtimeexecution.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	executions, _, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return executions
}

func assertPlanningNoExecutions(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, ".brevity", runtimeexecution.FileName)); !os.IsNotExist(err) {
		t.Fatalf("execution records were created unexpectedly: %v", err)
	}
}

func assertPlanningNoLaunchState(t *testing.T, root string) {
	t.Helper()
	for _, relative := range []string{
		filepath.Join(".brevity", "runs.jsonl"),
		filepath.Join(".brevity", "runtime-state.json"),
	} {
		if _, err := os.Stat(filepath.Join(root, relative)); !os.IsNotExist(err) {
			t.Fatalf("launch/runtime state exists unexpectedly at %s: %v", relative, err)
		}
	}
}

func planningQueueItem(id string, task string, reservation *runtimequeue.Reservation) runtimequeue.Item {
	return runtimequeue.Item{
		ID:          id,
		Task:        task,
		Provider:    "gemini",
		Profile:     "default",
		Status:      runtimequeue.StatusQueued,
		CreatedAt:   "2026-06-01T10:00:00Z",
		UpdatedAt:   "2026-06-01T10:00:00Z",
		Reservation: reservation,
	}
}

func planningExecutionRecord(id string, queueItemID string, task string, reservationID string, status string) runtimeexecution.Record {
	return runtimeexecution.Record{
		ID:            id,
		QueueItemID:   queueItemID,
		Task:          task,
		ReservationID: reservationID,
		Status:        status,
		CreatedAt:     "2026-06-01T10:10:00Z",
		UpdatedAt:     "2026-06-01T10:10:00Z",
	}
}

func readPlanningFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func planningFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".brevity"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, productGoalPath), []byte("Operator leverage.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func planningActivationFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	runPlanningGit(t, root, "init")
	runPlanningGit(t, root, "config", "user.email", "test@example.com")
	runPlanningGit(t, root, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, productGoalPath), []byte("Operator leverage.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runPlanningGit(t, root, "add", ".")
	runPlanningGit(t, root, "commit", "-m", "init")
	if _, err := (actions.InitService{Store: state.Store{RepoRoot: root}}).Run(); err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	return root
}

func promotedActivationModel(t *testing.T, root string) PlanningModel {
	t.Helper()
	model := NewPlanningModel("Operator leverage.", nil)
	model.repoRoot = root
	model.store = newPlanningIdeaStore(root)
	model.draftStore = newPlanningTaskDraftStore(root)
	model.now = fixedPlanningTime
	model.ideas = []PlanningIdea{{
		ID:        "idea-1",
		Title:     "Review notes persistence",
		CreatedAt: "2026-06-01T08:00:00Z",
		Status:    "captured",
	}}
	model.drafts = []PlanningTaskDraft{{
		ID:                 "draft-1",
		IdeaID:             "idea-1",
		TaskSlug:           "review-notes-persistence",
		Title:              "Review notes persistence",
		Description:        "Generated from planning idea",
		Status:             "promoted",
		AcceptanceCriteria: "Operator can activate the promoted task.",
		Validation:         "go test ./...",
		CreatedAt:          "2026-06-01T09:00:00Z",
		PromotedAt:         "2026-06-01T10:00:00Z",
	}}
	if err := model.store.Save(model.ideas); err != nil {
		t.Fatal(err)
	}
	if err := model.draftStore.Save(model.drafts); err != nil {
		t.Fatal(err)
	}
	if _, err := state.CreateTask(state.Store{RepoRoot: root}, state.Task{
		Slug:            "review-notes-persistence",
		ID:              "review-notes-persistence",
		Description:     "Generated from planning idea",
		Status:          "draft",
		NormalizedState: "draft",
		CreatedAt:       "2026-06-01T09:00:00Z",
		UpdatedAt:       "2026-06-01T10:00:00Z",
	}, state.TaskCreateOptions{}); err != nil {
		t.Fatal(err)
	}
	model.reloadPlanningTasks()
	return model
}

func runPlanningGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
}

func fixedPlanningTime() time.Time {
	return time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
}
