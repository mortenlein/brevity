package bubbleteadashboard

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/pscontract"
)

const reviewGitTimeout = 750 * time.Millisecond

type reviewCandidate struct {
	task      contracts.TaskSummary
	reason    string
	blockers  []string
	available bool
}

type reviewGitSummary struct {
	available       bool
	statusLine      string
	diffLine        string
	branchLine      string
	changedFiles    []string
	fileFocus       reviewFileFocus
	stagedCount     int
	modifiedCount   int
	deletedCount    int
	untrackedCount  int
	inspectionError string
}

type reviewCommandRunner interface {
	Run(ctx context.Context, name string, args ...string) reviewCommandOutput
}

type reviewCommandOutput struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
	TimedOut bool
}

type execReviewCommandRunner struct{}

type reviewDiffView struct {
	task     contracts.TaskSummary
	worktree string
	loading  bool
	result   reviewDiffResult
	scroll   int
}

type reviewDiffResult struct {
	task           string
	worktree       string
	changedFiles   []string
	stat           string
	stagedCount    int
	unstagedCount  int
	untrackedCount int
	errorMessage   string
	commands       [][]string
}

type reviewFileFocus struct {
	code    int
	tests   int
	docs    int
	config  int
	unknown int
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
	if model.commandRun != nil {
		output.WriteString(model.renderCommandResult(renderedRows(output.String())))
	}
	return model.renderWithPinnedFooter(output.String())
}

func (model Model) renderReviewHeader() string {
	width := model.contentWidth()
	line := statusLine(width,
		statusSegment{text: "BREVITY REVIEW", priority: 0},
		statusSegment{text: fallback(model.source, "unknown"), priority: 1},
		statusSegment{text: "operator cockpit", compact: "cockpit", priority: 1},
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
	if model.reviewDiff != nil {
		return model.renderReviewDiffBody()
	}
	candidate, ok := model.selectedReviewCandidate()
	var output strings.Builder
	if !ok {
		renderSection(&output, "Decision")
		output.WriteString(model.renderLine("  No review candidate yet.") + "\n")
		output.WriteString(model.renderLine("  Queue or launch work, then return here.") + "\n")
		output.WriteString("\n")
		renderSection(&output, "Commands")
		output.WriteString(model.renderLine("  brevity queue add <task>") + "\n")
		output.WriteString(model.renderLine("  brevity --bubble --review") + "\n")
		return output.String()
	}

	task := candidate.task
	git := model.inspectReviewGit(taskWorktreePath(task))
	renderSection(&output, "Decision")
	output.WriteString(model.reviewDetail("next action", reviewNextAction(candidate, git)))
	output.WriteString(model.reviewWrappedDetail("why", candidate.reason))
	output.WriteString(model.reviewDetail("confidence", reviewConfidence(candidate, git)))
	output.WriteString(model.reviewDetail("merge gate", reviewMergeGate(candidate, git)))

	output.WriteString("\n")
	renderSection(&output, "Review Queue")
	for _, line := range reviewQueueLines(model.state, model.reviewSelected) {
		output.WriteString(model.renderLine("  "+line) + "\n")
	}

	output.WriteString("\n")
	renderSection(&output, "Candidate")
	output.WriteString(model.reviewDetail("task", fallback(task.Slug, "(unknown)")))
	output.WriteString(model.reviewDetail("task state", fallback(firstNonEmpty(task.NormalizedState, task.Status), "unknown")))
	output.WriteString(model.reviewDetail("latest execution", fallback(latestExecutionStatus(task, model.state), "(none)")))
	output.WriteString(model.reviewDetail("latest run", fallback(taskLatestRun(task), "(none)")))

	output.WriteString("\n")
	renderSection(&output, "Worktree Inspection")
	output.WriteString(model.reviewPathDetail("path", fallback(taskWorktreePath(task), "(unknown)")))
	output.WriteString(model.reviewDetail("branch", fallback(firstNonEmpty(task.Branch, nestedReviewWorktreeBranch(task)), "(unknown)")))
	output.WriteString(model.reviewDetail("git branch", fallback(git.branchLine, "(unknown)")))
	output.WriteString(model.reviewDetail("git status", fallback(git.statusLine, "(unknown)")))
	output.WriteString(model.reviewDetail("change mix", reviewChangeMix(git)))
	output.WriteString(model.reviewDetail("review focus", reviewFocusLine(git)))
	output.WriteString(model.reviewWrappedDetail("attention", reviewFocusGuidance(git)))
	output.WriteString(model.reviewDetail("diff summary", fallback(git.diffLine, "(unknown)")))
	for _, file := range firstNStrings(git.changedFiles, 5) {
		output.WriteString(model.renderLine("  "+file) + "\n")
	}
	if len(git.changedFiles) > 5 {
		output.WriteString(model.renderLine(fmt.Sprintf("  ... %d more files", len(git.changedFiles)-5)) + "\n")
	}
	if git.inspectionError != "" {
		output.WriteString(model.reviewWrappedDetail("inspection", git.inspectionError))
	}

	output.WriteString("\n")
	renderSection(&output, "Action Bar")
	for _, action := range reviewActionBar(candidate, task, git) {
		output.WriteString(model.renderLine("  "+action) + "\n")
	}

	output.WriteString("\n")
	renderSection(&output, "Command Plan")
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

func inspectReviewGit(worktree string) reviewGitSummary {
	return inspectReviewGitWithRunner(execReviewCommandRunner{}, worktree)
}

func (model Model) inspectReviewGit(worktree string) reviewGitSummary {
	return inspectReviewGitWithRunner(model.reviewCommandRunner(), worktree)
}

func inspectReviewGitWithRunner(runner reviewCommandRunner, worktree string) reviewGitSummary {
	worktree = strings.TrimSpace(worktree)
	if worktree == "" {
		return reviewGitSummary{inspectionError: "worktree path is unavailable"}
	}
	statusOutput, statusErr := runReviewGitWithRunner(runner, worktree, "status", "--short", "--branch")
	diffOutput, diffErr := runReviewGitWithRunner(runner, worktree, "diff", "--stat", "--compact-summary")
	if statusErr != nil {
		return reviewGitSummary{inspectionError: statusErr.Error()}
	}
	lines := nonEmptyGitStatusLines(statusOutput)
	summary := reviewGitSummary{available: true, branchLine: "(unknown)"}
	if len(lines) > 0 && strings.HasPrefix(lines[0], "## ") {
		summary.branchLine = strings.TrimSpace(strings.TrimPrefix(lines[0], "## "))
		lines = lines[1:]
	}
	summary.changedFiles = lines
	summary.stagedCount, summary.modifiedCount, summary.deletedCount, summary.untrackedCount = reviewGitCounts(lines)
	summary.fileFocus = reviewFileFocusCounts(lines)
	switch len(lines) {
	case 0:
		summary.statusLine = "clean"
	case 1:
		summary.statusLine = "1 changed file"
	default:
		summary.statusLine = fmt.Sprintf("%d changed files", len(lines))
	}
	if diffErr != nil {
		summary.diffLine = "diff unavailable: " + diffErr.Error()
	} else if strings.TrimSpace(diffOutput) == "" {
		summary.diffLine = "no unstaged diff"
	} else {
		summary.diffLine = firstLine(diffOutput)
	}
	return summary
}

func reviewFileFocusCounts(lines []string) reviewFileFocus {
	var focus reviewFileFocus
	for _, line := range lines {
		path := reviewGitStatusPath(line)
		if path == "" {
			focus.unknown++
			continue
		}
		lower := strings.ToLower(path)
		switch {
		case strings.Contains(lower, "_test.") || strings.Contains(lower, ".test.") || strings.Contains(lower, ".spec.") || strings.Contains(lower, "/test/") || strings.Contains(lower, "\\test\\") || strings.Contains(lower, "/tests/") || strings.Contains(lower, "\\tests\\"):
			focus.tests++
		case strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".txt") || strings.HasPrefix(lower, "docs/") || strings.HasPrefix(lower, "docs\\"):
			focus.docs++
		case strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".toml") || strings.HasSuffix(lower, ".mod") || strings.HasSuffix(lower, ".sum") || strings.HasPrefix(lower, ".github/") || strings.HasPrefix(lower, ".github\\"):
			focus.config++
		case strings.HasSuffix(lower, ".go") || strings.HasSuffix(lower, ".ps1") || strings.HasSuffix(lower, ".js") || strings.HasSuffix(lower, ".ts") || strings.HasSuffix(lower, ".tsx") || strings.HasSuffix(lower, ".jsx") || strings.HasSuffix(lower, ".py") || strings.HasSuffix(lower, ".cs") || strings.HasSuffix(lower, ".rs"):
			focus.code++
		default:
			focus.unknown++
		}
	}
	return focus
}

func reviewGitStatusPath(line string) string {
	line = strings.TrimRight(line, "\r")
	if strings.TrimSpace(line) == "" {
		return ""
	}
	if strings.HasPrefix(line, "??") {
		return strings.TrimSpace(strings.TrimPrefix(line, "??"))
	}
	if len(line) <= 3 {
		return ""
	}
	path := strings.TrimSpace(line[3:])
	if renameParts := strings.Split(path, " -> "); len(renameParts) > 1 {
		path = strings.TrimSpace(renameParts[len(renameParts)-1])
	}
	return path
}

func reviewGitCounts(lines []string) (staged int, modified int, deleted int, untracked int) {
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, "??") {
			untracked++
			continue
		}
		if len(line) < 2 {
			continue
		}
		indexStatus := line[0]
		worktreeStatus := line[1]
		if indexStatus != ' ' && indexStatus != '?' {
			staged++
		}
		if indexStatus == 'M' || worktreeStatus == 'M' {
			modified++
		}
		if indexStatus == 'D' || worktreeStatus == 'D' {
			deleted++
		}
	}
	return staged, modified, deleted, untracked
}

func runReviewGit(worktree string, args ...string) (string, error) {
	return runReviewGitWithRunner(execReviewCommandRunner{}, worktree, args...)
}

func runReviewGitWithRunner(runner reviewCommandRunner, worktree string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), reviewGitTimeout)
	defer cancel()
	fullArgs := append([]string{"-C", worktree}, args...)
	result := runner.Run(ctx, "git", fullArgs...)
	if result.TimedOut || ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("git inspection timed out")
	}
	if result.Err != nil || result.ExitCode != 0 {
		message := strings.TrimSpace(firstNonEmpty(result.Stderr, result.Stdout))
		if message == "" {
			if result.Err != nil {
				message = result.Err.Error()
			} else {
				message = fmt.Sprintf("git exited with code %d", result.ExitCode)
			}
		}
		return "", errors.New(message)
	}
	return result.Stdout, nil
}

func (execReviewCommandRunner) Run(ctx context.Context, name string, args ...string) reviewCommandOutput {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	result := reviewCommandOutput{Stdout: string(output)}
	if ctx.Err() == context.DeadlineExceeded {
		result.ExitCode = 1
		result.TimedOut = true
		result.Err = ctx.Err()
		return result
	}
	if err != nil {
		result.ExitCode = 1
		result.Err = err
	}
	return result
}

func (model Model) reviewCommandRunner() reviewCommandRunner {
	if model.reviewRunner != nil {
		return model.reviewRunner
	}
	return execReviewCommandRunner{}
}

func reviewNextAction(candidate reviewCandidate, git reviewGitSummary) string {
	if len(candidate.blockers) > 0 {
		return "resolve blockers before approval"
	}
	if git.available && len(git.changedFiles) == 0 {
		return "inspect run output; no worktree diff found"
	}
	if git.available {
		return "review diff, then approve or reject"
	}
	return "inspect worktree before approval"
}

func reviewConfidence(candidate reviewCandidate, git reviewGitSummary) string {
	if len(candidate.blockers) > 0 {
		return "blocked"
	}
	if !git.available {
		return "limited; git inspection unavailable"
	}
	if len(git.changedFiles) == 0 {
		return "limited; no changed files detected"
	}
	return "ready for human review"
}

func reviewMergeGate(candidate reviewCandidate, git reviewGitSummary) string {
	if len(candidate.blockers) > 0 {
		return "blocked by task/runtime signals"
	}
	if !git.available {
		return "blocked until worktree is inspected"
	}
	if len(git.changedFiles) == 0 {
		return "blocked; no diff to merge"
	}
	if git.untrackedCount > 0 {
		return "caution; untracked files need review"
	}
	return "ready for merge prep after human approval"
}

func reviewChangeMix(git reviewGitSummary) string {
	if !git.available {
		return "(unknown)"
	}
	if len(git.changedFiles) == 0 {
		return "clean"
	}
	parts := []string{}
	if git.stagedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d staged", git.stagedCount))
	}
	if git.modifiedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", git.modifiedCount))
	}
	if git.deletedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d deleted", git.deletedCount))
	}
	if git.untrackedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", git.untrackedCount))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%d changed", len(git.changedFiles))
	}
	return strings.Join(parts, " | ")
}

func reviewFocusLine(git reviewGitSummary) string {
	if !git.available {
		return "(unknown)"
	}
	if len(git.changedFiles) == 0 {
		return "no changed files"
	}
	parts := []string{}
	if git.fileFocus.code > 0 {
		parts = append(parts, fmt.Sprintf("%d code", git.fileFocus.code))
	}
	if git.fileFocus.tests > 0 {
		parts = append(parts, fmt.Sprintf("%d tests", git.fileFocus.tests))
	}
	if git.fileFocus.docs > 0 {
		parts = append(parts, fmt.Sprintf("%d docs", git.fileFocus.docs))
	}
	if git.fileFocus.config > 0 {
		parts = append(parts, fmt.Sprintf("%d config", git.fileFocus.config))
	}
	if git.fileFocus.unknown > 0 {
		parts = append(parts, fmt.Sprintf("%d other", git.fileFocus.unknown))
	}
	return strings.Join(parts, " | ")
}

func reviewFocusGuidance(git reviewGitSummary) string {
	if !git.available {
		return "inspect worktree manually; git summary is unavailable"
	}
	if len(git.changedFiles) == 0 {
		return "no diff detected; verify the run output before approval"
	}
	if git.fileFocus.tests == 0 && git.fileFocus.code > 0 {
		return "code changed without test files; review test coverage before approval"
	}
	if git.untrackedCount > 0 {
		return "untracked files present; decide whether they belong in the merge"
	}
	if git.fileFocus.config > 0 {
		return "configuration changed; verify runtime and CI impact"
	}
	return "review code and tests together before approval"
}

func reviewActionBar(candidate reviewCandidate, task contracts.TaskSummary, git reviewGitSummary) []string {
	slug := fallback(task.Slug, "<task>")
	worktree := fallback(taskWorktreePath(task), "<worktree>")
	approveState := "blocked"
	mergeState := "blocked"
	if len(candidate.blockers) == 0 && git.available && len(git.changedFiles) > 0 {
		approveState = "available"
		mergeState = "after approval"
	} else if len(candidate.blockers) == 0 {
		approveState = "inspect first"
		mergeState = "inspect first"
	}
	return []string{
		"s status      inspect       git -C " + worktree + " status",
		"d diff        inspect       git -C " + worktree + " diff",
		"o editor      external      code " + worktree,
		"e explorer    external      open " + worktree,
		"a approve     " + padRight(approveState, 15) + "approval gate for " + slug,
		"x reject      available     capture rejection notes for " + slug,
		"m merge prep  " + padRight(mergeState, 15) + "brevity task merge " + slug + " --plan",
	}
}

type reviewCommandFactory func(id int) tea.Cmd

func (model Model) reviewCommandForKey(key string) (ActionDescriptor, reviewCommandFactory, bool) {
	candidate, ok := model.selectedReviewCandidate()
	if !ok {
		return ActionDescriptor{}, nil, false
	}
	task := candidate.task
	worktree := strings.TrimSpace(taskWorktreePath(task))
	switch key {
	case "s":
		return reviewAction("Git status"), model.reviewExecCommand("Git status", "git", "-C", worktree, "status"), worktree != ""
	case "d":
		return ActionDescriptor{}, nil, false
	case "o":
		return reviewAction("Open editor"), model.reviewStartCommand("Open editor", "code", worktree), worktree != ""
	case "e":
		return reviewAction("Open worktree"), model.reviewStartCommand("Open worktree", "explorer", worktree), worktree != ""
	case "a":
		return reviewAction("Approve review"), model.reviewSyntheticCommand("Approve review", reviewApprovalOutput(candidate, task, model.inspectReviewGit(worktree))), true
	case "x":
		return reviewAction("Reject review"), model.reviewSyntheticCommand("Reject review", reviewRejectionOutput(task)), true
	case "m":
		return reviewAction("Merge prep"), model.reviewSyntheticCommand("Merge prep", strings.Join(reviewMergePrepOutput(candidate, task, model.inspectReviewGit(worktree)), "\n")), true
	default:
		return ActionDescriptor{}, nil, false
	}
}

func (model *Model) openReviewDiff() tea.Cmd {
	candidate, ok := model.selectedReviewCandidate()
	if !ok {
		return nil
	}
	task := candidate.task
	worktree := strings.TrimSpace(taskWorktreePath(task))
	model.reviewDiff = &reviewDiffView{
		task:     task,
		worktree: worktree,
		loading:  worktree != "",
		result: reviewDiffResult{
			task:     fallback(task.Slug, "(unknown)"),
			worktree: fallback(worktree, "(unknown)"),
		},
	}
	if worktree == "" {
		model.reviewDiff.result.errorMessage = "worktree path is unavailable; diff view is disabled"
		return nil
	}
	return model.reviewDiffCmd()
}

func (model Model) reviewDiffCmd() tea.Cmd {
	if model.reviewDiff == nil {
		return nil
	}
	view := *model.reviewDiff
	if strings.TrimSpace(view.worktree) == "" {
		return func() tea.Msg {
			return reviewDiffLoadedMsg{result: reviewDiffResult{
				task:         fallback(view.task.Slug, "(unknown)"),
				worktree:     "(unknown)",
				errorMessage: "worktree path is unavailable; diff view is disabled",
			}}
		}
	}
	runner := model.reviewCommandRunner()
	return func() tea.Msg {
		return reviewDiffLoadedMsg{result: loadReviewDiff(runner, view.task, view.worktree)}
	}
}

func loadReviewDiff(runner reviewCommandRunner, task contracts.TaskSummary, worktree string) reviewDiffResult {
	result := reviewDiffResult{
		task:     fallback(task.Slug, "(unknown)"),
		worktree: worktree,
		commands: [][]string{
			{"git", "-C", worktree, "status", "--porcelain"},
			{"git", "-C", worktree, "diff", "--stat"},
		},
	}
	statusOutput, statusErr := runReviewGitWithRunner(runner, worktree, "status", "--porcelain")
	if statusErr != nil {
		result.errorMessage = statusErr.Error()
		return result
	}
	lines := nonEmptyGitStatusLines(statusOutput)
	result.changedFiles = reviewChangedFilePaths(lines)
	result.stagedCount, result.unstagedCount, result.untrackedCount = reviewDiffCounts(lines)
	statOutput, statErr := runReviewGitWithRunner(runner, worktree, "diff", "--stat")
	if statErr != nil {
		result.errorMessage = statErr.Error()
		return result
	}
	result.stat = strings.TrimSpace(statOutput)
	if result.stat == "" {
		result.stat = "(no unstaged diff)"
	}
	return result
}

func reviewChangedFilePaths(lines []string) []string {
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		path := reviewGitStatusPath(line)
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func reviewDiffCounts(lines []string) (staged int, unstaged int, untracked int) {
	for _, line := range lines {
		if strings.HasPrefix(line, "??") {
			untracked++
			continue
		}
		if len(line) < 2 {
			continue
		}
		if line[0] != ' ' {
			staged++
		}
		if line[1] != ' ' {
			unstaged++
		}
	}
	return staged, unstaged, untracked
}

func (model Model) renderReviewDiffBody() string {
	view := model.reviewDiff
	if view == nil {
		return ""
	}
	result := view.result
	lines := []string{
		"  Task: " + fallback(result.task, fallback(view.task.Slug, "(unknown)")),
		"  Worktree: " + fallback(result.worktree, fallback(view.worktree, "(unknown)")),
		"",
		"  Files changed:",
	}
	if view.loading {
		lines = append(lines, "  loading diff summary...")
	} else if len(result.changedFiles) == 0 {
		lines = append(lines, "  - (none)")
	} else {
		for _, file := range result.changedFiles {
			lines = append(lines, "  - "+file)
		}
	}
	lines = append(lines,
		"",
		"  Counts:",
		fmt.Sprintf("  staged=%d unstaged=%d untracked=%d", result.stagedCount, result.unstagedCount, result.untrackedCount),
		"",
		"  Stat:",
	)
	if view.loading {
		lines = append(lines, "  (loading)")
	} else if strings.TrimSpace(result.stat) != "" {
		for _, line := range strings.Split(strings.ReplaceAll(result.stat, "\r\n", "\n"), "\n") {
			lines = append(lines, "  "+line)
		}
	} else {
		lines = append(lines, "  (empty)")
	}
	if strings.TrimSpace(result.errorMessage) != "" {
		lines = append(lines, "", "  Error:", "  "+result.errorMessage)
	}
	lines = append(lines,
		"",
		"  Actions:",
		"  [d] refresh diff  [s] status  [b] back  [o] code  [e] explorer  [q] quit",
	)
	return model.renderScrollablePanel("DIFF SUMMARY", lines, view.scroll, detailTruncatedIndicator)
}

func reviewAction(label string) ActionDescriptor {
	return ActionDescriptor{
		Label:             label,
		Kind:              ActionKindReadOnly,
		Enabled:           true,
		ExecutesViaBridge: false,
	}
}

func (model Model) reviewExecCommand(label string, name string, args ...string) reviewCommandFactory {
	return func(id int) tea.Cmd {
		return func() tea.Msg {
			started := time.Now()
			result := pscontract.ExecutionResult{
				CommandDisplayLabel: label,
				StartedAt:           started,
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, name, args...)
			output, err := cmd.CombinedOutput()
			result.CompletedAt = time.Now()
			result.Stdout = string(output)
			if ctx.Err() == context.DeadlineExceeded {
				result.ExitCode = 1
				result.TimedOut = true
				result.Error = "command timed out"
			} else if err != nil {
				result.ExitCode = 1
				result.Error = err.Error()
			}
			return commandResultMsg{id: id, result: result}
		}
	}
}

func (model Model) reviewStartCommand(label string, name string, args ...string) reviewCommandFactory {
	return func(id int) tea.Cmd {
		return func() tea.Msg {
			started := time.Now()
			result := pscontract.ExecutionResult{
				CommandDisplayLabel: label,
				StartedAt:           started,
				CompletedAt:         time.Now(),
				Stdout:              name + " " + strings.Join(args, " "),
			}
			cmd := exec.Command(name, args...)
			if err := cmd.Start(); err != nil {
				result.ExitCode = 1
				result.Error = err.Error()
			}
			return commandResultMsg{id: id, result: result}
		}
	}
}

func (model Model) reviewSyntheticCommand(label string, output string) reviewCommandFactory {
	return func(id int) tea.Cmd {
		return func() tea.Msg {
			now := time.Now()
			return commandResultMsg{id: id, result: pscontract.ExecutionResult{
				CommandDisplayLabel: label,
				StartedAt:           now,
				CompletedAt:         now,
				Stdout:              output,
			}}
		}
	}
}

func (model Model) renderReviewCommandResult(run commandRunState, lines []string, usedRows ...int) string {
	if run.status == commandRunning {
		lines = append(lines,
			"  outcome       waiting for command output",
			"  next          review the result before approving or merging",
			"  close         disabled while command is running",
		)
		return model.renderPanel("Review Action", lines, detailTruncatedIndicator, usedRows...)
	}
	if run.result == nil {
		lines = append(lines,
			"  outcome       no result yet",
			"  next          wait for completion or refresh review state",
			"  close         esc or q closes action",
		)
		return model.renderPanel("Review Action", lines, detailTruncatedIndicator, usedRows...)
	}

	result := *run.result
	outcome := "completed"
	if !result.Success() {
		outcome = "needs attention"
	}
	if result.TimedOut {
		outcome = "timed out"
	}
	if result.Canceled {
		outcome = "canceled"
	}
	lines = append(lines,
		"  outcome       "+outcome,
		"  exit code     "+fmt.Sprint(result.ExitCode),
		"  next          "+reviewCommandFollowUp(run.action.Label, result),
	)
	if result.TimedOut {
		lines = append(lines, "  timeout       command exceeded its read-only timeout")
	}
	if stdout := strings.TrimSpace(result.Stdout); stdout != "" {
		lines = append(lines, commandOutputLines("output", stdout)...)
	} else if result.Success() {
		lines = append(lines, "  output        (empty)")
	}
	if stderr := strings.TrimSpace(result.Stderr); stderr != "" {
		lines = append(lines, commandOutputLines("stderr", stderr)...)
	}
	if result.Error != "" && strings.TrimSpace(result.Stderr) == "" {
		lines = append(lines, "  error         "+result.Error)
	}
	lines = append(lines, "  close         esc or q closes action")
	return model.renderScrollablePanel("Review Action", lines, run.scroll, detailTruncatedIndicator, usedRows...)
}

func reviewCommandFollowUp(label string, result pscontract.ExecutionResult) string {
	if !result.Success() {
		return "inspect the error before making a review decision"
	}
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "git status":
		return "open the diff if the worktree contains reviewable changes"
	case "git diff":
		return "approve, reject, or run merge prep after human diff review"
	case "open worktree", "open editor":
		return "inspect files, then return to approve, reject, or merge prep"
	case "approve review":
		return "run merge prep only after the diff is acceptable"
	case "reject review":
		return "record required changes and leave the worktree intact"
	case "merge prep":
		return "run the listed merge-plan command after approval"
	default:
		return "continue review from the current decision gate"
	}
}

func reviewApprovalOutput(candidate reviewCandidate, task contracts.TaskSummary, git reviewGitSummary) string {
	lines := []string{
		"Approval gate",
		"task: " + fallback(task.Slug, "(unknown)"),
		"state: " + fallback(firstNonEmpty(task.NormalizedState, task.Status), "unknown"),
		"git: " + fallback(git.statusLine, "(unknown)"),
		"merge gate: " + reviewMergeGate(candidate, git),
		"focus: " + reviewFocusLine(git),
		"attention: " + reviewFocusGuidance(git),
	}
	if len(candidate.blockers) > 0 {
		lines = append(lines, "decision: blocked; resolve blockers before approval")
		for _, blocker := range candidate.blockers {
			lines = append(lines, "- "+blocker)
		}
		return strings.Join(lines, "\n")
	}
	if !git.available || len(git.changedFiles) == 0 {
		lines = append(lines, "decision: blocked; inspect worktree and confirm there is a diff")
		return strings.Join(lines, "\n")
	}
	lines = append(lines,
		"decision: approval can proceed after human diff review",
		"next: run merge prep when the diff is acceptable",
	)
	return strings.Join(lines, "\n")
}

func reviewRejectionOutput(task contracts.TaskSummary) string {
	return strings.Join([]string{
		"Rejection notes",
		"task: " + fallback(task.Slug, "(unknown)"),
		"record: summarize what must change before rerun",
		"next: leave the worktree intact for the worker or operator to inspect",
	}, "\n")
}

func reviewMergePrepOutput(candidate reviewCandidate, task contracts.TaskSummary, git reviewGitSummary) []string {
	slug := fallback(task.Slug, "<task>")
	worktree := fallback(taskWorktreePath(task), "<worktree>")
	lines := []string{
		"Merge preparation",
		"gate: " + reviewMergeGate(candidate, git),
		"status: " + fallback(git.statusLine, "(unknown)"),
		"change mix: " + reviewChangeMix(git),
		"focus: " + reviewFocusLine(git),
		"attention: " + reviewFocusGuidance(git),
		"1. review git -C " + worktree + " diff",
		"2. verify git -C " + worktree + " status",
		"3. inspect git -C " + worktree + " log --oneline -5",
		"4. run brevity task merge " + slug + " --plan",
	}
	if len(candidate.blockers) > 0 {
		lines = append(lines, "blockers:")
		for _, blocker := range candidate.blockers {
			lines = append(lines, "- "+blocker)
		}
	}
	return lines
}

func nonEmptyLines(value string) []string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			kept = append(kept, strings.TrimSpace(line))
		}
	}
	return kept
}

func nonEmptyGitStatusLines(value string) []string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			kept = append(kept, strings.TrimRight(line, "\r"))
		}
	}
	return kept
}

func firstLine(value string) string {
	lines := nonEmptyLines(value)
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

func firstNStrings(values []string, count int) []string {
	if count < 0 {
		count = 0
	}
	if len(values) <= count {
		return values
	}
	return values[:count]
}

func selectReviewCandidate(state contracts.RuntimeState) (reviewCandidate, bool) {
	candidates := collectReviewCandidates(state)
	if len(candidates) == 0 {
		return reviewCandidate{}, false
	}
	return candidates[0], true
}

func (model Model) selectedReviewCandidate() (reviewCandidate, bool) {
	candidates := collectReviewCandidates(model.state)
	if len(candidates) == 0 {
		return reviewCandidate{}, false
	}
	index := clampReviewSelection(model.reviewSelected, len(candidates))
	return candidates[index], true
}

func (model *Model) moveReviewSelection(delta int) {
	candidates := collectReviewCandidates(model.state)
	if len(candidates) == 0 {
		model.reviewSelected = 0
		return
	}
	model.reviewSelected = clampReviewSelection(model.reviewSelected+delta, len(candidates))
}

func clampReviewSelection(index int, count int) int {
	if count <= 0 {
		return 0
	}
	if index < 0 {
		return count - 1
	}
	if index >= count {
		return 0
	}
	return index
}

func collectReviewCandidates(state contracts.RuntimeState) []reviewCandidate {
	if len(state.Tasks) == 0 {
		return nil
	}
	candidates := make([]reviewCandidate, 0, len(state.Tasks))
	seen := map[string]bool{}
	add := func(task contracts.TaskSummary, taskIndex int, reason string) {
		key := reviewTaskKey(task, taskIndex)
		if seen[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, buildReviewCandidate(task, state, reason))
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
		for index, task := range state.Tasks {
			if reason, ok := priority(task); ok {
				add(task, index, reason)
			}
		}
	}
	for index, task := range state.Tasks {
		candidate := buildReviewCandidate(task, state, "no ready review signal; showing the most actionable blocker")
		if len(candidate.blockers) > 0 {
			add(task, index, candidate.reason)
		}
	}
	if len(candidates) == 0 {
		add(state.Tasks[0], 0, "missing review signals; inspect task state before acting")
	}
	return candidates
}

func reviewTaskKey(task contracts.TaskSummary, fallbackIndex int) string {
	if slug := strings.TrimSpace(task.Slug); slug != "" {
		return "slug:" + slug
	}
	return fmt.Sprintf("index:%d", fallbackIndex)
}

func reviewQueueLines(state contracts.RuntimeState, selectedIndex int) []string {
	candidates := collectReviewCandidates(state)
	if len(candidates) == 0 {
		return []string{"none"}
	}
	selectedIndex = clampReviewSelection(selectedIndex, len(candidates))
	windowStart := reviewQueueWindowStart(selectedIndex, len(candidates), 4)
	windowEnd := windowStart + 4
	if windowEnd > len(candidates) {
		windowEnd = len(candidates)
	}
	lines := []string{fmt.Sprintf("candidate %d of %d", selectedIndex+1, len(candidates))}
	for index := windowStart; index < windowEnd; index++ {
		candidate := candidates[index]
		task := candidate.task
		marker := " "
		if index == selectedIndex {
			marker = ">"
		}
		lines = append(lines, fmt.Sprintf("%s %-24s %-16s %s", marker, fallback(task.Slug, "(unknown)"), fallback(firstNonEmpty(task.NormalizedState, task.Status), "unknown"), reviewQueueReason(candidate)))
	}
	if windowStart > 0 {
		lines = append(lines, fmt.Sprintf("  ... %d earlier candidates", windowStart))
	}
	if windowEnd < len(candidates) {
		lines = append(lines, fmt.Sprintf("  ... %d more candidates", len(candidates)-windowEnd))
	}
	return lines
}

func reviewQueueWindowStart(selectedIndex int, count int, windowSize int) int {
	if windowSize <= 0 || count <= windowSize {
		return 0
	}
	start := selectedIndex - windowSize + 1
	if start < 0 {
		return 0
	}
	if start > count-windowSize {
		return count - windowSize
	}
	return start
}

func reviewQueueReason(candidate reviewCandidate) string {
	if len(candidate.blockers) > 0 {
		return "blocked: " + candidate.blockers[0]
	}
	return candidate.reason
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
