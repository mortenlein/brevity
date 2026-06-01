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
		"[convert to task draft] placeholder",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("reloaded planning output missing %q:\n%s", want, output.String())
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
