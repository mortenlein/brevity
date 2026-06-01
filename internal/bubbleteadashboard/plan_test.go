package bubbleteadashboard

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
		"No plan exists yet.",
		"Start with the product goal",
		"one reviewable task",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("planning empty state missing %q:\n%s", want, output)
		}
	}
}

func TestPlanningWorkspaceNextCommandsRender(t *testing.T) {
	model := NewPlanningModel("Operator leverage.", nil)

	output := model.View()

	for _, want := range []string{
		"brevity task new <slug>",
		"brevity task status",
		"brevity review",
		"brevity task merge <slug>",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("planning next commands missing %q:\n%s", want, output)
		}
	}
}

func TestPlanningWorkspaceNarrowWidthReadable(t *testing.T) {
	model := NewPlanningModel("Operator leverage starts with a focused planning workspace.", nil)
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
