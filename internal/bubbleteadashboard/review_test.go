package bubbleteadashboard

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mortenlein/brevity/internal/contracts"
)

func TestReviewWorkspaceNoCandidate(t *testing.T) {
	model := reviewTestModel(emptyBubbleState(), 96)

	output := plainView(model.View())
	for _, want := range []string{
		"REVIEW WORKSPACE",
		"No review candidate yet.",
		"Queue or launch work, then return here.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("review empty state missing %q:\n%s", want, output)
		}
	}
	assertLinesWithinWidth(t, output, model.width)
}

func TestReviewWorkspaceCompletedExecutionCandidate(t *testing.T) {
	state := emptyBubbleState()
	state.Tasks = []contracts.TaskSummary{reviewTask("done-task", "ready-for-review", "completed", "succeeded")}
	model := reviewTestModel(state, 120)

	output := plainView(model.View())
	for _, want := range []string{
		"task             done-task",
		"task state       ready-for-review",
		"latest execution completed",
		"latest run       id=run-1 status=succeeded exit=0",
		"none detected; candidate for review, not a merge verdict",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("completed review candidate missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "safe to merge") {
		t.Fatalf("review workspace claimed merge safety:\n%s", output)
	}
}

func TestReviewWorkspaceFailedExecutionBlocker(t *testing.T) {
	state := emptyBubbleState()
	state.Tasks = []contracts.TaskSummary{reviewTask("failed-task", "needs-inspection", "failed", "failed")}
	model := reviewTestModel(state, 100)

	output := plainView(model.View())
	for _, want := range []string{
		"task             failed-task",
		"failed execution: inspect logs before review",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("failed blocker missing %q:\n%s", want, output)
		}
	}
}

func TestReviewWorkspaceProviderGatedBlocker(t *testing.T) {
	state := emptyBubbleState()
	task := reviewTask("gated-task", "provider-gated", "", "")
	task.ProviderGated = true
	state.Tasks = []contracts.TaskSummary{task}
	model := reviewTestModel(state, 100)

	output := plainView(model.View())
	for _, want := range []string{
		"task             gated-task",
		"provider gated",
		"missing signals",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("provider blocker missing %q:\n%s", want, output)
		}
	}
}

func TestReviewWorkspaceCommandRendering(t *testing.T) {
	state := emptyBubbleState()
	state.Tasks = []contracts.TaskSummary{reviewTask("review-me", "ready-for-review", "completed", "succeeded")}
	model := reviewTestModel(state, 120)

	output := plainView(model.View())
	for _, want := range []string{
		"brevity cmux --review review-me",
		"git -C C:\\repo\\worktrees\\active\\review-me status",
		"git -C C:\\repo\\worktrees\\active\\review-me diff --stat",
		"git -C C:\\repo\\worktrees\\active\\review-me log --oneline -5",
		"brevity task merge review-me --plan",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("command rendering missing %q:\n%s", want, output)
		}
	}
}

func TestReviewWorkspaceWidths(t *testing.T) {
	state := emptyBubbleState()
	state.Tasks = []contracts.TaskSummary{reviewTask("review-task-with-a-long-name", "ready-for-review", "completed", "succeeded")}

	for _, width := range []int{42, 82} {
		t.Run("width", func(t *testing.T) {
			model := reviewTestModel(state, width)
			output := plainView(model.View())
			assertLinesWithinWidth(t, output, width)
			for _, want := range []string{"REVIEW WORKSPACE", "Review Candidate", "Next Commands", "Blockers"} {
				if !strings.Contains(output, want) {
					t.Fatalf("width %d missing %q:\n%s", width, want, output)
				}
			}
		})
	}
}

func TestReviewWorkspaceDoesNotMutateRuntimeFilesOrBridge(t *testing.T) {
	dir := t.TempDir()
	queueFile := dir + string(os.PathSeparator) + "runtime-queue.json"
	execFile := dir + string(os.PathSeparator) + "runtime-executions.json"
	if err := os.WriteFile(queueFile, []byte("queue-before"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(execFile, []byte("exec-before"), 0o600); err != nil {
		t.Fatal(err)
	}

	state := emptyBubbleState()
	state.Queue = &contracts.RuntimeQueue{Path: queueFile, State: "valid", CountsByStatus: map[string]int{"queued": 1}, TotalItems: 1}
	state.Executions = &contracts.RuntimeExecution{Path: execFile, State: "valid", CountsByStatus: map[string]int{"completed": 1}, TotalExecutions: 1}
	state.Tasks = []contracts.TaskSummary{reviewTask("done-task", "ready-for-review", "completed", "succeeded")}
	bridge := &fakeCommandBridge{state: state}
	model := reviewTestModel(state, 100)
	model.commandBridge = bridge

	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("p")},
		{Type: tea.KeyEnter},
		{Type: tea.KeyRunes, Runes: []rune("d")},
	} {
		updated, cmd := model.Update(key)
		model = updated.(Model)
		if cmd != nil {
			t.Fatalf("review key %q returned command, want nil", key.String())
		}
	}
	if bridge.executeCalls != 0 || bridge.mutateCalls != 0 || bridge.startCalls != 0 || bridge.runCalls != 0 {
		t.Fatalf("review mode called command bridge: %+v", bridge)
	}
	if got, _ := os.ReadFile(queueFile); string(got) != "queue-before" {
		t.Fatalf("queue file mutated by review render: %q", got)
	}
	if got, _ := os.ReadFile(execFile); string(got) != "exec-before" {
		t.Fatalf("execution file mutated by review render: %q", got)
	}
}

func reviewTestModel(state contracts.RuntimeState, width int) Model {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.reviewMode = true
	model.state = state
	model.hasState = true
	model.width = width
	return model
}

func reviewTask(slug string, state string, executionStatus string, runStatus string) contracts.TaskSummary {
	exitCode := any(0)
	if runStatus == "failed" {
		exitCode = any(1)
	}
	return contracts.TaskSummary{
		Slug:                  slug,
		Status:                state,
		NormalizedState:       state,
		Branch:                "task/" + slug,
		WorktreePath:          `C:\repo\worktrees\active\` + slug,
		Execution:             &contracts.TaskExecution{Status: executionStatus, LastRunID: "run-1"},
		LatestRunID:           "run-1",
		LatestRunWorkerStatus: runStatus,
		LatestRunExitCode:     exitCode,
	}
}
