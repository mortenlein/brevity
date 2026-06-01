package cmux

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mortenlein/brevity/internal/contracts"
)

// CMUXReviewReportSchema is the schema identifier for the CMUX review-packet
// JSON output.
const CMUXReviewReportSchema = "brevity.cmux-review-report.v1"

// CMUXChecklistItem is one entry in the review checklist.
type CMUXChecklistItem struct {
	Item string `json:"item"`
	Met  bool   `json:"met"`
}

// CMUXReviewReport is the typed JSON output for --review mode.
//
// Task, Providers, and Queue are nil when their contract is unavailable.
// ReviewChecklist and SuggestedActions are always non-nil slices (empty when
// the task was not found).
type CMUXReviewReport struct {
	Schema           string              `json:"schema"`
	Source           string              `json:"source"`
	Section          string              `json:"section"`
	ReviewMode       bool                `json:"reviewMode"`
	ReviewTask       string              `json:"reviewTask"`
	Errors           []string            `json:"errors"`
	Task             *CMUXTask           `json:"task,omitempty"`
	Providers        *CMUXProviders      `json:"providers,omitempty"`
	Queue            *CMUXQueueScheduler `json:"queue,omitempty"`
	NextAction       string              `json:"nextAction,omitempty"`
	MergeGate        string              `json:"mergeGate,omitempty"`
	Attention        string              `json:"attention,omitempty"`
	ReviewChecklist  []CMUXChecklistItem `json:"reviewChecklist"`
	SuggestedActions []string            `json:"suggestedActions"`
	MergeReadiness   string              `json:"mergeReadiness,omitempty"`
	CleanupReadiness string              `json:"cleanupReadiness,omitempty"`
}

type CMUXReviewDecision struct {
	NextAction string
	MergeGate  string
	Attention  string
}

// renderReview dispatches to the appropriate review renderer based on output
// mode.  Called only when opts.ReviewTask is non-empty.
func renderReview(w io.Writer, snap Snapshot, opts RenderOptions) {
	switch opts.Output {
	case OutputMarkdown:
		renderMarkdownReview(w, snap, opts)
	case OutputJSON:
		renderJSONReview(w, snap, opts)
	default:
		renderTextReview(w, snap, opts)
	}
}

// findReviewTask looks up a task by slug in the snapshot.
// Returns the task and true if found; zero value and false otherwise.
func findReviewTask(snap Snapshot, slug string) (contracts.TaskSummary, bool) {
	if !snap.HasRuntimeState {
		return contracts.TaskSummary{}, false
	}
	for _, t := range snap.RuntimeState.Tasks {
		if t.Slug == slug {
			return t, true
		}
	}
	return contracts.TaskSummary{}, false
}

// reviewTaskState returns the normalised state for a task, falling back to
// Status and then "unknown".
func reviewTaskState(t contracts.TaskSummary) string {
	if ns := strings.TrimSpace(t.NormalizedState); ns != "" {
		return ns
	}
	if s := strings.TrimSpace(t.Status); s != "" {
		return s
	}
	return "unknown"
}

// buildReviewChecklist builds the four standard review checklist items for a
// task.
func buildReviewChecklist(t contracts.TaskSummary) []CMUXChecklistItem {
	state := reviewTaskState(t)
	_, presence := resolveTaskWorktree(t)
	return []CMUXChecklistItem{
		{
			Item: "Last run succeeded",
			Met:  strings.TrimSpace(t.LatestRunWorkerStatus) == "succeeded",
		},
		{
			Item: "Worktree present",
			Met:  presence == "present",
		},
		{
			Item: "Prompt present",
			Met:  strings.TrimSpace(t.PromptPath) != "",
		},
		{
			Item: "State is reviewing or ready-for-merge",
			Met:  state == "reviewing" || state == "ready-for-merge",
		},
	}
}

// buildReviewSuggestedActions returns display-only suggested next steps based
// on the task state.  No command execution; no mutations; no AI calls.
func buildReviewSuggestedActions(t contracts.TaskSummary) []string {
	state := reviewTaskState(t)
	switch state {
	case "ready-for-merge":
		return []string{
			"Run brevity task merge " + t.Slug + " --plan to preview merge.",
		}
	case "reviewing":
		return []string{
			"Review worktree changes before merging.",
			"Run brevity task merge " + t.Slug + " --plan to preview merge.",
		}
	case "merged":
		return []string{
			"Task is already merged.",
			"Run brevity task cleanup " + t.Slug + " --plan to preview cleanup.",
		}
	case "blocked":
		return []string{
			"Task is blocked. Resolve blockers before proceeding.",
		}
	default:
		return []string{
			"Run brevity cmux --task " + t.Slug + " to see full task detail.",
		}
	}
}

func buildReviewDecision(t contracts.TaskSummary) CMUXReviewDecision {
	state := reviewTaskState(t)
	_, presence := resolveTaskWorktree(t)
	runStatus := strings.TrimSpace(t.LatestRunWorkerStatus)
	if runStatus == "" {
		runStatus = strings.TrimSpace(t.WorkerStatus)
	}
	if t.ProviderGated || state == "provider-gated" || strings.Contains(strings.ToLower(t.ProviderHealth), "gated") {
		return CMUXReviewDecision{
			NextAction: "Resolve provider gate before review.",
			MergeGate:  "blocked by provider gate",
			Attention:  "Provider health or profile prevents confident approval.",
		}
	}
	if state == "blocked" || runStatus == "failed" {
		return CMUXReviewDecision{
			NextAction: "Inspect failure context before rerun or approval.",
			MergeGate:  "blocked by failed or blocked task state",
			Attention:  reviewFailureAttention(t),
		}
	}
	if presence != "present" {
		return CMUXReviewDecision{
			NextAction: "Restore or locate the worktree before review.",
			MergeGate:  "blocked until worktree is present",
			Attention:  "No durable worktree path is available for diff inspection.",
		}
	}
	switch state {
	case "ready-for-merge":
		return CMUXReviewDecision{
			NextAction: "Run merge prep, then approve or reject.",
			MergeGate:  "ready for merge prep after human review",
			Attention:  "Verify diff, test coverage, and merge plan before integrating.",
		}
	case "reviewing", "ready-for-review", "needs-inspection":
		return CMUXReviewDecision{
			NextAction: "Review worktree diff, then approve or reject.",
			MergeGate:  "needs human approval before merge prep",
			Attention:  "Use git status and diff before accepting generated work.",
		}
	case "merged":
		return CMUXReviewDecision{
			NextAction: "Prepare cleanup if the merge is already integrated.",
			MergeGate:  "already merged",
			Attention:  "Confirm branch integration before removing worktree artifacts.",
		}
	default:
		return CMUXReviewDecision{
			NextAction: "Inspect task state before review.",
			MergeGate:  "not ready for approval",
			Attention:  "Current state does not prove review readiness.",
		}
	}
}

func reviewFailureAttention(t contracts.TaskSummary) string {
	if failureType := strings.TrimSpace(t.LatestRunFailureType); failureType != "" {
		return "Latest run failed with " + failureType + "."
	}
	if logPath := strings.TrimSpace(t.LatestRunLogPath); logPath != "" {
		return "Inspect latest run log at " + logPath + "."
	}
	if logPath := strings.TrimSpace(t.LastLogPath); logPath != "" {
		return "Inspect latest run log at " + logPath + "."
	}
	return "Failure details are limited in the current runtime contract."
}

// mergeReadinessNote returns a human-readable merge readiness label for a
// normalised task state.
func mergeReadinessNote(state string) string {
	switch state {
	case "ready-for-merge":
		return "merge ready"
	case "reviewing":
		return "ready for merge review"
	case "merged":
		return "already merged"
	case "blocked":
		return "blocked — resolve before merging"
	default:
		return "not yet ready (state: " + state + ")"
	}
}

// cleanupReadinessNote returns a human-readable cleanup readiness label.
func cleanupReadinessNote(state, presence string) string {
	if state == "merged" {
		if presence == "present" {
			return "eligible for cleanup (worktree present)"
		}
		return "worktree already removed"
	}
	return "not ready: merge first"
}

// renderTextReview writes a compact terminal-friendly review packet to w.
func renderTextReview(w io.Writer, snap Snapshot, opts RenderOptions) {
	slug := opts.ReviewTask
	fmt.Fprintln(w, "CMUX REVIEW PACKET  [read-only]")
	fmt.Fprintln(w, "================================")
	fmt.Fprintf(w, "source: native\n")
	fmt.Fprintf(w, "review-task: %s\n", slug)

	if snap.RuntimeStateErr != nil {
		fmt.Fprintf(w, "\nruntime-state: error: %v\n", snap.RuntimeStateErr)
		return
	}
	if !snap.HasRuntimeState {
		fmt.Fprintln(w, "\nruntime-state: unavailable")
		return
	}

	task, found := findReviewTask(snap, slug)
	if !found {
		fmt.Fprintf(w, "\ntask %q not found\n", slug)
		return
	}

	state := reviewTaskState(task)
	_, presence := resolveTaskWorktree(task)
	decision := buildReviewDecision(task)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Decision")
	fmt.Fprintf(w, "  next action: %s\n", decision.NextAction)
	fmt.Fprintf(w, "  merge gate:  %s\n", decision.MergeGate)
	fmt.Fprintf(w, "  attention:   %s\n", decision.Attention)

	// Task detail.
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Task: %s  [%s]\n", task.Slug, state)
	renderTaskWorktree(w, task)
	renderTaskPrompt(w, task)
	renderTaskLastRun(w, task)

	// Queue / Scheduler context.
	fmt.Fprintln(w, sectionSep)
	renderQueueScheduler(w, snap)

	// Review checklist.
	fmt.Fprintln(w, sectionSep)
	fmt.Fprintln(w, "\nReview Checklist")
	for _, item := range buildReviewChecklist(task) {
		mark := "[ ]"
		if item.Met {
			mark = "[x]"
		}
		fmt.Fprintf(w, "  %s %s\n", mark, item.Item)
	}

	// Readiness notes.
	fmt.Fprintln(w, sectionSep)
	fmt.Fprintln(w, "\nReadiness")
	fmt.Fprintf(w, "  merge:   %s\n", mergeReadinessNote(state))
	fmt.Fprintf(w, "  cleanup: %s\n", cleanupReadinessNote(state, presence))

	// Suggested actions.
	fmt.Fprintln(w, sectionSep)
	fmt.Fprintln(w, "\nSuggested Next Actions")
	for _, action := range buildReviewSuggestedActions(task) {
		fmt.Fprintf(w, "  - %s\n", action)
	}
}

// renderMarkdownReview writes a GitHub-Flavoured Markdown review packet to w.
func renderMarkdownReview(w io.Writer, snap Snapshot, opts RenderOptions) {
	slug := opts.ReviewTask
	fmt.Fprintf(w, "# CMUX Review Packet: %s [read-only]\n", slug)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "**Source:** native")
	fmt.Fprintf(w, "**Review Task:** %s\n", slug)

	if snap.RuntimeStateErr != nil {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "> **runtime-state error:** %v\n", snap.RuntimeStateErr)
		return
	}
	if !snap.HasRuntimeState {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "> **runtime-state:** unavailable")
		return
	}

	task, found := findReviewTask(snap, slug)
	if !found {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "_Task %q not found._\n", slug)
		return
	}

	state := reviewTaskState(task)
	_, presence := resolveTaskWorktree(task)
	decision := buildReviewDecision(task)

	// Task section.
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Decision")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- **Next action:** %s\n", decision.NextAction)
	fmt.Fprintf(w, "- **Merge gate:** %s\n", decision.MergeGate)
	fmt.Fprintf(w, "- **Attention:** %s\n", decision.Attention)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "## Task: %s\n", task.Slug)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "**State:** %s\n", state)
	fmt.Fprintln(w)
	renderMarkdownTaskDetail(w, task)

	// Queue / Scheduler context.
	renderMarkdownQueueScheduler(w, snap)

	// Review Checklist.
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Review Checklist")
	fmt.Fprintln(w)
	for _, item := range buildReviewChecklist(task) {
		check := " "
		if item.Met {
			check = "x"
		}
		fmt.Fprintf(w, "- [%s] %s\n", check, item.Item)
	}

	// Merge Readiness.
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Merge Readiness")
	fmt.Fprintln(w)
	fmt.Fprintln(w, mergeReadinessNote(state))

	// Cleanup Readiness.
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Cleanup Readiness")
	fmt.Fprintln(w)
	fmt.Fprintln(w, cleanupReadinessNote(state, presence))

	// Suggested Follow-up.
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Suggested Follow-up")
	fmt.Fprintln(w)
	for _, action := range buildReviewSuggestedActions(task) {
		fmt.Fprintf(w, "- %s\n", action)
	}
}

// renderJSONReview writes a CMUXReviewReport as indented JSON to w.
// Providers and Queue are always populated when their contracts are available.
// Task is nil when the slug was not found.
func renderJSONReview(w io.Writer, snap Snapshot, opts RenderOptions) {
	slug := opts.ReviewTask
	report := CMUXReviewReport{
		Schema:           CMUXReviewReportSchema,
		Source:           "native",
		Section:          "review",
		ReviewMode:       true,
		ReviewTask:       slug,
		Errors:           make([]string, 0),
		ReviewChecklist:  make([]CMUXChecklistItem, 0),
		SuggestedActions: make([]string, 0),
	}

	if snap.RuntimeStateErr != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("runtime-state: %v", snap.RuntimeStateErr))
	}
	if snap.SchedulerPlanErr != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("scheduler-plan: %v", snap.SchedulerPlanErr))
	}

	if snap.HasRuntimeState {
		task, found := findReviewTask(snap, slug)
		if found {
			t := buildJSONTask(task)
			report.Task = &t
			report.ReviewChecklist = buildReviewChecklist(task)
			report.SuggestedActions = buildReviewSuggestedActions(task)
			state := reviewTaskState(task)
			_, presence := resolveTaskWorktree(task)
			decision := buildReviewDecision(task)
			report.NextAction = decision.NextAction
			report.MergeGate = decision.MergeGate
			report.Attention = decision.Attention
			report.MergeReadiness = mergeReadinessNote(state)
			report.CleanupReadiness = cleanupReadinessNote(state, presence)
		} else {
			report.Errors = append(report.Errors, fmt.Sprintf("task %q not found", slug))
		}
		report.Providers = buildJSONProviders(snap)
	}
	report.Queue = buildJSONQueue(snap)

	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(w, "{\"schema\":%q,\"error\":\"json marshal failed: %v\"}\n", CMUXReviewReportSchema, err)
		return
	}
	_, _ = w.Write(append(out, '\n'))
}
