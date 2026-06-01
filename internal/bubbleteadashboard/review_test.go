package bubbleteadashboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
		"BREVITY REVIEW",
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
		"next action      inspect worktree before approval",
		"task             done-task",
		"task state       ready-for-review",
		"latest execution completed",
		"latest run       id=run-1 status=succeeded exit=0",
		"git status       (unknown)",
		"merge gate       blocked until worktree is inspected",
		"attention        inspect worktree manually; git summary is unavailable",
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

func TestReviewWorkspaceShowsPrioritizedReviewQueue(t *testing.T) {
	state := emptyBubbleState()
	state.Tasks = []contracts.TaskSummary{
		reviewTask("queued-task", "queued", "", ""),
		reviewTask("ready-task", "ready-for-review", "completed", "succeeded"),
		reviewTask("inspect-task", "needs-inspection", "completed", "succeeded"),
	}
	model := reviewTestModel(state, 130)

	output := plainView(model.View())
	for _, want := range []string{
		"Review Queue",
		"> ready-task",
		"ready-for-review",
		"task state is ready-for-review",
		"inspect-task",
		"needs-inspection",
		"queued-task",
		"blocked: queued: work has not been reserved or executed yet",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("review queue missing %q:\n%s", want, output)
		}
	}
	if strings.Index(output, "> ready-task") > strings.Index(output, "inspect-task") {
		t.Fatalf("ready review candidate should rank before needs-inspection:\n%s", output)
	}
}

func TestReviewWorkspaceKeyboardMovesReviewQueueSelection(t *testing.T) {
	state := emptyBubbleState()
	state.Tasks = []contracts.TaskSummary{
		reviewTask("queued-task", "queued", "", ""),
		reviewTask("ready-task", "ready-for-review", "completed", "succeeded"),
		reviewTask("inspect-task", "needs-inspection", "completed", "succeeded"),
	}
	model := reviewTestModel(state, 130)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	model = updated.(Model)
	output := plainView(model.View())
	for _, want := range []string{
		"candidate 2 of 3",
		"> inspect-task",
		"task             inspect-task",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("next review selection missing %q:\n%s", want, output)
		}
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	model = updated.(Model)
	output = plainView(model.View())
	for _, want := range []string{
		"candidate 1 of 3",
		"> ready-task",
		"task             ready-task",
		"n/p s d o e a x m r q",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("previous review selection missing %q:\n%s", want, output)
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
		"s status      inspect       git -C C:\\repo\\worktrees\\active\\review-me status",
		"d diff        inspect       git -C C:\\repo\\worktrees\\active\\review-me diff",
		"o editor      external      code C:\\repo\\worktrees\\active\\review-me",
		"e explorer    external      open C:\\repo\\worktrees\\active\\review-me",
		"a approve     inspect first  approval gate for review-me",
		"x reject      available     capture rejection notes for review-me",
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
			for _, want := range []string{"BREVITY REVIEW", "Decision", "Action Bar", "Blockers"} {
				if !strings.Contains(output, want) {
					t.Fatalf("width %d missing %q:\n%s", width, want, output)
				}
			}
		})
	}
}

func TestReviewWorkspaceGitInspectionSummary(t *testing.T) {
	dir := t.TempDir()
	runReviewTestGit(t, dir, "init")
	runReviewTestGit(t, dir, "config", "core.autocrlf", "false")
	runReviewTestGit(t, dir, "config", "user.email", "brevity@example.test")
	runReviewTestGit(t, dir, "config", "user.name", "Brevity Test")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runReviewTestGit(t, dir, "add", ".")
	runReviewTestGit(t, dir, "commit", "-m", "initial")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file_test.go"), []byte("package test\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	state := emptyBubbleState()
	task := reviewTask("git-task", "ready-for-review", "completed", "succeeded")
	task.WorktreePath = dir
	state.Tasks = []contracts.TaskSummary{task}
	model := reviewTestModel(state, 120)

	output := plainView(model.View())
	for _, want := range []string{
		"next action      review diff, then approve or reject",
		"confidence       ready for human review",
		"merge gate       caution; untracked files need review",
		"git status       3 changed files",
		"change mix       1 modified | 2 untracked",
		"review focus     1 code | 1 tests | 1 docs",
		"attention        untracked files present; decide whether they belong in the merge",
		"a approve     available      approval gate for git-task",
		"m merge prep  after approval brevity task merge git-task --plan",
		"M main.go",
		"?? new.txt",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("git inspection output missing %q:\n%s", want, output)
		}
	}
}

func runReviewTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

type fakeReviewRunner struct {
	calls     [][]string
	responses map[string]reviewCommandOutput
}

func (runner *fakeReviewRunner) Run(_ context.Context, name string, args ...string) reviewCommandOutput {
	call := append([]string{name}, args...)
	runner.calls = append(runner.calls, call)
	if runner.responses == nil {
		return reviewCommandOutput{}
	}
	if response, ok := runner.responses[strings.Join(call, "\x00")]; ok {
		return response
	}
	return reviewCommandOutput{ExitCode: 127, Err: exec.ErrNotFound}
}

func equalStringSlices2D(a [][]string, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				return false
			}
		}
	}
	return true
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

func TestReviewWorkspaceKeyboardActionsOpenCommandPanels(t *testing.T) {
	dir := t.TempDir()
	runReviewTestGit(t, dir, "init")
	runReviewTestGit(t, dir, "config", "core.autocrlf", "false")
	runReviewTestGit(t, dir, "config", "user.email", "brevity@example.test")
	runReviewTestGit(t, dir, "config", "user.name", "Brevity Test")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runReviewTestGit(t, dir, "add", ".")
	runReviewTestGit(t, dir, "commit", "-m", "initial")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	state := emptyBubbleState()
	task := reviewTask("keyboard-task", "ready-for-review", "completed", "succeeded")
	task.WorktreePath = dir
	state.Tasks = []contracts.TaskSummary{task}
	model := reviewTestModel(state, 120)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("review diff shortcut returned nil command")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	output := plainView(model.View())
	for _, want := range []string{
		"DIFF SUMMARY",
		"Task: keyboard-task",
		"Files changed:",
		"Stat:",
		"main.go",
		"[d] refresh diff",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("review diff command output missing %q:\n%s", want, output)
		}
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	model = updated.(Model)
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("merge prep shortcut returned nil command")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	output = plainView(model.View())
	for _, want := range []string{
		"action        Merge prep",
		"outcome       completed",
		"next          run the listed merge-plan command after approval",
		"Merge preparation",
		"gate: ready for merge prep after human approval",
		"attention: code changed without test files; review test coverage before approval",
		"brevity task merge keyboard-task --plan",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("merge prep command output missing %q:\n%s", want, output)
		}
	}
}

func TestReviewWorkspaceDiffViewUsesFakeRunnerAndShowsStat(t *testing.T) {
	state := emptyBubbleState()
	task := reviewTask("diff-task", "ready-for-review", "completed", "succeeded")
	task.WorktreePath = `C:\repo\worktrees\active\diff-task`
	state.Tasks = []contracts.TaskSummary{task}
	runner := &fakeReviewRunner{responses: map[string]reviewCommandOutput{
		"git\x00-C\x00" + task.WorktreePath + "\x00status\x00--porcelain": {Stdout: " M src/foo.go\nA  internal/bar_test.go\n?? docs/note.md\n"},
		"git\x00-C\x00" + task.WorktreePath + "\x00diff\x00--stat":        {Stdout: " src/foo.go | 2 ++\n 1 file changed, 2 insertions(+)\n"},
	}}
	model := reviewTestModel(state, 44)
	model.reviewRunner = runner

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("diff view returned nil command")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)

	output := plainView(model.View())
	for _, want := range []string{
		"DIFF SUMMARY",
		"Task: diff-task",
		"- src/foo.go",
		"- internal/bar_test.go",
		"- docs/note.md",
		"staged=1 unstaged=1 untracked=1",
		"src/foo.go | 2 ++",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("diff view missing %q:\n%s", want, output)
		}
	}
	assertLinesWithinWidth(t, output, model.width)
	wantCommands := [][]string{
		{"git", "-C", task.WorktreePath, "status", "--porcelain"},
		{"git", "-C", task.WorktreePath, "diff", "--stat"},
	}
	if got := runner.calls; !equalStringSlices2D(got, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", got, wantCommands)
	}
}

func TestReviewWorkspaceDiffViewRendersErrorsClearly(t *testing.T) {
	state := emptyBubbleState()
	task := reviewTask("bad-diff", "ready-for-review", "completed", "succeeded")
	state.Tasks = []contracts.TaskSummary{task}
	runner := &fakeReviewRunner{responses: map[string]reviewCommandOutput{
		"git\x00-C\x00" + task.WorktreePath + "\x00status\x00--porcelain": {Stderr: "fatal: not a git repository", ExitCode: 128},
	}}
	model := reviewTestModel(state, 90)
	model.reviewRunner = runner

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)

	output := plainView(model.View())
	for _, want := range []string{"DIFF SUMMARY", "Error:", "fatal: not a git repository"} {
		if !strings.Contains(output, want) {
			t.Fatalf("diff error missing %q:\n%s", want, output)
		}
	}
}

func TestReviewWorkspaceMissingWorktreeDisablesDiffGracefully(t *testing.T) {
	state := emptyBubbleState()
	task := reviewTask("missing-worktree", "ready-for-review", "completed", "succeeded")
	task.WorktreePath = ""
	state.Tasks = []contracts.TaskSummary{task}
	runner := &fakeReviewRunner{}
	model := reviewTestModel(state, 80)
	model.reviewRunner = runner

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("missing worktree returned command")
	}
	output := plainView(model.View())
	if !strings.Contains(output, "worktree path is unavailable; diff view is disabled") {
		t.Fatalf("missing worktree message not rendered:\n%s", output)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner called for missing worktree: %#v", runner.calls)
	}
}

func TestReviewWorkspaceDiffBackReturnsToReviewQueue(t *testing.T) {
	state := emptyBubbleState()
	state.Tasks = []contracts.TaskSummary{reviewTask("back-task", "ready-for-review", "completed", "succeeded")}
	model := reviewTestModel(state, 90)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(Model)
	if cmd != nil {
		updated, _ = model.Update(cmd())
		model = updated.(Model)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	model = updated.(Model)

	output := plainView(model.View())
	if strings.Contains(output, "DIFF SUMMARY") || !strings.Contains(output, "Review Queue") {
		t.Fatalf("back did not return to review workspace:\n%s", output)
	}
}

func TestReviewWorkspaceApprovalGateUsesGitAndBlockers(t *testing.T) {
	state := emptyBubbleState()
	task := reviewTask("blocked-task", "provider-gated", "", "")
	task.ProviderGated = true
	state.Tasks = []contracts.TaskSummary{task}
	model := reviewTestModel(state, 120)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("approve shortcut returned nil command")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	output := plainView(model.View())
	for _, want := range []string{
		"action        Approve review",
		"outcome       completed",
		"next          run merge prep only after the diff is acceptable",
		"Approval gate",
		"merge gate: blocked by task/runtime signals",
		"attention: inspect worktree manually; git summary is unavailable",
		"decision: blocked; resolve blockers before approval",
		"provider gated",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("approval gate output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "approval can proceed") {
		t.Fatalf("approval gate allowed a blocked task:\n%s", output)
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
