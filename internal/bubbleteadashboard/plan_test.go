package bubbleteadashboard

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
		"activate task",
		"execute task",
		"review task",
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
		"activate task",
		"execute task",
		"review task",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("promotion guidance missing %q:\n%s", want, output)
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

func fixedPlanningTime() time.Time {
	return time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
}
