package dashboard

import (
	"strings"
	"testing"

	"github.com/mortenlein/brevity/internal/contracts"
)

func TestInteractiveSelectionModelClampsAndMoves(t *testing.T) {
	model := InteractiveModel{SelectedIndex: 10}
	model.Clamp(2)
	if model.SelectedIndex != 1 {
		t.Fatalf("SelectedIndex = %d, want 1", model.SelectedIndex)
	}

	model.MoveUp(2)
	if model.SelectedIndex != 0 {
		t.Fatalf("SelectedIndex = %d, want 0 after up", model.SelectedIndex)
	}

	model.MoveDown(2)
	model.MoveDown(2)
	if model.SelectedIndex != 1 {
		t.Fatalf("SelectedIndex = %d, want 1 after bounded down", model.SelectedIndex)
	}
}

func TestSelectableItemsIncludesProviderTaskCleanupAndAction(t *testing.T) {
	state := interactiveState()

	items := SelectableItems(state)
	if len(items) != 4 {
		t.Fatalf("len(items) = %d, want 4: %#v", len(items), items)
	}
	if items[0].Kind != SelectionProvider || items[0].ProviderName != "codex" {
		t.Fatalf("provider item = %#v", items[0])
	}
	if items[1].Kind != SelectionTask || items[1].Task.Slug != "my-task" {
		t.Fatalf("task item = %#v", items[1])
	}
	if items[2].Kind != SelectionCleanup || items[2].CleanupCandidate.ID != "orphan-worktree:my-task" {
		t.Fatalf("cleanup item = %#v", items[2])
	}
	if items[3].Kind != SelectionAction || items[3].ActionText != "Inspect state." {
		t.Fatalf("action item = %#v", items[3])
	}
}

func TestRenderInteractiveDetailsForCleanupCandidate(t *testing.T) {
	state := interactiveState()
	model := InteractiveModel{SelectedIndex: 2, ShowDetails: true}

	output := RenderInteractiveString(state, model)

	assertOutputContains(t, output, "> requires-inspection: orphan-worktree:my-task")
	assertOutputContains(t, output, "type: cleanup candidate")
	assertOutputContains(t, output, "id: orphan-worktree:my-task")
	assertOutputContains(t, output, "dirty: true")
	assertOutputContains(t, output, "- modified tracked files detected")
	assertOutputContains(t, output, "- git status --short")
}

func TestRenderInteractiveHelpToggle(t *testing.T) {
	state := interactiveState()

	withoutHelp := RenderInteractiveString(state, InteractiveModel{})
	if strings.Contains(withoutHelp, "\nHelp\n") {
		t.Fatalf("output unexpectedly contains help:\n%s", withoutHelp)
	}

	withHelp := RenderInteractiveString(state, InteractiveModel{ShowHelp: true})
	assertOutputContains(t, withHelp, "Help")
	assertOutputContains(t, withHelp, "q: quit")
	assertOutputContains(t, withHelp, "Input is line-oriented")
}

func interactiveState() contracts.RuntimeState {
	return contracts.RuntimeState{
		Schema:      contracts.RuntimeStateSchema,
		RepoRoot:    `C:\repo`,
		GeneratedAt: "2026-05-19T10:00:00Z",
		Providers: contracts.Providers{
			Summary: contracts.ProviderSummary{Total: 1},
			Health: map[string]contracts.ProviderHealth{
				"codex": {Status: "unknown", Note: "ready", UpdatedAt: "2026-05-19T10:01:00Z"},
			},
		},
		TaskCounts: contracts.TaskCounts{Tracked: 1},
		Tasks: []contracts.TaskSummary{
			{
				Slug:            "my-task",
				Status:          "ready-for-worker",
				NormalizedState: "readyForWorker",
				Worktree:        &contracts.TaskWorktree{Path: `C:\repo\worktrees\active\brevity-my-task`},
				Execution:       &contracts.TaskExecution{Status: "failed", LastRunID: "run-1", LogPath: `C:\repo\.brevity\logs\my-task\run-1.log`},
			},
		},
		Cleanup: &contracts.Cleanup{
			Summary: &contracts.CleanupSummary{TotalCandidates: 1},
			OrphanedTaskWorktrees: []contracts.CleanupCandidate{
				{
					ID:                "orphan-worktree:my-task",
					Severity:          "warning",
					Category:          "requires-inspection",
					Path:              `C:\repo\worktrees\active\brevity-my-task`,
					Branch:            "task/my-task",
					Dirty:             true,
					DirtyReasons:      []string{"modified tracked files detected"},
					SuggestedCommands: []string{"git status --short"},
				},
			},
		},
		SuggestedNextActions: []string{"Inspect state."},
	}
}

func assertOutputContains(t *testing.T, output string, want string) {
	t.Helper()
	if !strings.Contains(output, want) {
		t.Fatalf("output missing %q\noutput:\n%s", want, output)
	}
}
