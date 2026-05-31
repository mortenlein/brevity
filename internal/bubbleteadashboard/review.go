package bubbleteadashboard

import (
	"fmt"
	"strings"

	"github.com/mortenlein/brevity/internal/contracts"
)

type reviewCandidate struct {
	task      contracts.TaskSummary
	reason    string
	blockers  []string
	available bool
}

func (model Model) reviewView() string {
	if model.usesUltraSmallHeightMode() {
		return model.renderUltraSmallHeightView()
	}
	if !model.hasState {
		return model.renderReviewLoadingView()
	}

	var output strings.Builder
	output.WriteString(model.renderReviewHeader())
	output.WriteString(model.renderReviewBody())
	if model.lastError != nil {
		output.WriteString("\n")
		renderSection(&output, "Warnings")
		output.WriteString(model.renderRuntimeErrorLine())
	}
	return model.renderWithPinnedFooter(output.String())
}

func (model Model) renderReviewHeader() string {
	width := model.contentWidth()
	line := statusLine(width,
		statusSegment{text: "REVIEW WORKSPACE", priority: 0},
		statusSegment{text: fallback(model.source, "unknown"), priority: 1},
		statusSegment{text: "read-only", priority: 1},
	)
	return dashboardStyles.title.Render(line) + "\n"
}

func (model Model) renderReviewLoadingView() string {
	var output strings.Builder
	output.WriteString(model.renderReviewHeader())
	output.WriteString("\n")
	renderSection(&output, "Review Candidate")
	output.WriteString(model.renderLine("  status  Loading runtime state") + "\n")
	output.WriteString(model.renderLine("  note    Review workspace will not mutate tasks, queue, executions, or providers") + "\n")
	return model.renderWithPinnedFooter(output.String())
}

func (model Model) renderReviewBody() string {
	candidate, ok := selectReviewCandidate(model.state)
	var output strings.Builder
	if !ok {
		renderSection(&output, "Review Candidate")
		output.WriteString(model.renderLine("  No review candidate yet.") + "\n")
		output.WriteString(model.renderLine("  Queue or launch work, then return here.") + "\n")
		output.WriteString("\n")
		renderSection(&output, "Next Commands")
		output.WriteString(model.renderLine("  brevity queue add <task>") + "\n")
		output.WriteString(model.renderLine("  brevity --bubble --review") + "\n")
		return output.String()
	}

	task := candidate.task
	renderSection(&output, "Review Candidate")
	output.WriteString(model.reviewDetail("task", fallback(task.Slug, "(unknown)")))
	output.WriteString(model.reviewDetail("task state", fallback(firstNonEmpty(task.NormalizedState, task.Status), "unknown")))
	output.WriteString(model.reviewDetail("latest execution", fallback(latestExecutionStatus(task, model.state), "(none)")))
	output.WriteString(model.reviewDetail("latest run", fallback(taskLatestRun(task), "(none)")))
	output.WriteString(model.reviewWrappedDetail("reason", candidate.reason))

	output.WriteString("\n")
	renderSection(&output, "Worktree")
	output.WriteString(model.reviewPathDetail("path", fallback(taskWorktreePath(task), "(unknown)")))
	output.WriteString(model.reviewDetail("branch", fallback(firstNonEmpty(task.Branch, nestedReviewWorktreeBranch(task)), "(unknown)")))
	output.WriteString(model.reviewDetail("changed files", "(unknown)"))

	output.WriteString("\n")
	renderSection(&output, "Next Commands")
	for _, command := range reviewCommands(task) {
		output.WriteString(model.renderLine("  "+command) + "\n")
	}

	output.WriteString("\n")
	renderSection(&output, "Blockers")
	if len(candidate.blockers) == 0 {
		output.WriteString(model.renderLine("  none detected; candidate for review, not a merge verdict") + "\n")
	} else {
		for _, blocker := range candidate.blockers {
			output.WriteString(model.renderLine("  - "+blocker) + "\n")
		}
	}
	return output.String()
}

func selectReviewCandidate(state contracts.RuntimeState) (reviewCandidate, bool) {
	if len(state.Tasks) == 0 {
		return reviewCandidate{}, false
	}
	priorities := []func(contracts.TaskSummary) (string, bool){
		func(task contracts.TaskSummary) (string, bool) {
			if normalizedReviewState(task) == "ready-for-review" {
				return "task state is ready-for-review", true
			}
			return "", false
		},
		func(task contracts.TaskSummary) (string, bool) {
			status := strings.ToLower(strings.TrimSpace(task.LatestRunWorkerStatus))
			exec := strings.ToLower(strings.TrimSpace(latestExecutionStatus(task, state)))
			if status == "succeeded" || exec == "completed" {
				return "latest execution completed; inspect the worktree outcome", true
			}
			return "", false
		},
		func(task contracts.TaskSummary) (string, bool) {
			if normalizedReviewState(task) == "needs-inspection" {
				return "task state is needs-inspection", true
			}
			return "", false
		},
	}
	for _, priority := range priorities {
		for _, task := range state.Tasks {
			if reason, ok := priority(task); ok {
				return buildReviewCandidate(task, state, reason), true
			}
		}
	}
	for _, task := range state.Tasks {
		candidate := buildReviewCandidate(task, state, "no ready review signal; showing the most actionable blocker")
		if len(candidate.blockers) > 0 {
			return candidate, true
		}
	}
	return buildReviewCandidate(state.Tasks[0], state, "missing review signals; inspect task state before acting"), true
}

func buildReviewCandidate(task contracts.TaskSummary, state contracts.RuntimeState, reason string) reviewCandidate {
	blockers := reviewBlockers(task, state)
	return reviewCandidate{task: task, reason: reason, blockers: blockers, available: len(blockers) == 0}
}

func reviewBlockers(task contracts.TaskSummary, state contracts.RuntimeState) []string {
	blockers := []string{}
	taskState := normalizedReviewState(task)
	execStatus := strings.ToLower(strings.TrimSpace(latestExecutionStatus(task, state)))
	runStatus := strings.ToLower(strings.TrimSpace(task.LatestRunWorkerStatus))
	if task.ProviderGated || taskState == "provider-gated" || strings.Contains(strings.ToLower(task.ProviderHealth), "gated") {
		blockers = append(blockers, "provider gated: provider health or profile blocks review confidence")
	}
	switch taskState {
	case "queued":
		blockers = append(blockers, "queued: work has not been reserved or executed yet")
	case "reserved":
		blockers = append(blockers, "reserved: execution is not completed yet")
	case "launching":
		blockers = append(blockers, "launching: provider execution is still in progress")
	}
	if execStatus == "failed" || runStatus == "failed" {
		blockers = append(blockers, "failed execution: inspect logs before review")
	}
	if execStatus == "" && runStatus == "" && taskState != "ready-for-review" && taskState != "needs-inspection" {
		blockers = append(blockers, "missing signals: no latest execution or run status available")
	}
	if strings.TrimSpace(taskWorktreePath(task)) == "" {
		blockers = append(blockers, "missing signals: worktree path is unavailable")
	}
	return blockers
}

func reviewCommands(task contracts.TaskSummary) []string {
	slug := fallback(task.Slug, "<task>")
	worktree := fallback(taskWorktreePath(task), "<worktree>")
	return []string{
		"brevity cmux --review " + slug,
		"git -C " + worktree + " status",
		"git -C " + worktree + " diff --stat",
		"git -C " + worktree + " log --oneline -5",
		"brevity task merge " + slug + " --plan",
	}
}

func latestExecutionStatus(task contracts.TaskSummary, state contracts.RuntimeState) string {
	if task.Execution != nil && strings.TrimSpace(task.Execution.Status) != "" {
		return task.Execution.Status
	}
	if state.Executions != nil && task.Slug != "" && state.Executions.NewestExecutionTask == task.Slug {
		return state.Executions.NewestExecutionStatus
	}
	return ""
}

func normalizedReviewState(task contracts.TaskSummary) string {
	return strings.ToLower(strings.TrimSpace(firstNonEmpty(task.NormalizedState, task.Status)))
}

func nestedReviewWorktreeBranch(task contracts.TaskSummary) string {
	if task.Worktree == nil {
		return ""
	}
	return task.Worktree.Branch
}

func (model Model) reviewDetail(label string, value string) string {
	return model.renderLine(fmt.Sprintf("  %-16s %s", label, value)) + "\n"
}

func (model Model) reviewWrappedDetail(label string, value string) string {
	width := model.contentWidth() - len("  ") - 16 - 1
	if width < 12 {
		return model.reviewDetail(label, value)
	}
	lines := wrapDetailValue(value, width)
	if len(lines) == 0 {
		lines = []string{""}
	}
	var output strings.Builder
	output.WriteString(model.reviewDetail(label, lines[0]))
	for _, line := range lines[1:] {
		output.WriteString(model.renderLine("  "+strings.Repeat(" ", 17)+line) + "\n")
	}
	return output.String()
}

func (model Model) reviewPathDetail(label string, value string) string {
	prefix := fmt.Sprintf("  %-16s ", label)
	return model.renderLine(prefix+truncatePath(value, model.contentWidth()-visibleWidth(prefix))) + "\n"
}
