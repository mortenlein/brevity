package cmux

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mortenlein/brevity/internal/contracts"
)

// CMUXHandoffSchema is the schema identifier for the CMUX handoff JSON output.
const CMUXHandoffSchema = "brevity.cmux-handoff.v1"

// handoffSafetyNote is the standard read-only safety attestation appended to
// every handoff output.
const handoffSafetyNote = "No actions were executed. This is a deterministic report artifact."

// CMUXHandoffReport is the top-level typed output struct for --handoff mode.
//
// RuntimeSummary and Providers are nil when the runtime-state contract is
// unavailable.  ImportantTasks, ReviewCandidates, and SuggestedNextActions are
// always non-nil slices (empty when no data is available).
type CMUXHandoffReport struct {
	Schema               string                     `json:"schema"`
	Source               string                     `json:"source"`
	Options              CMUXHandoffOptions         `json:"options"`
	Errors               []string                   `json:"errors"`
	RuntimeSummary       *CMUXHandoffRuntimeSummary `json:"runtimeSummary,omitempty"`
	Providers            *CMUXProviders             `json:"providers,omitempty"`
	QueueScheduler       *CMUXQueueScheduler        `json:"queueScheduler,omitempty"`
	ImportantTasks       []CMUXTask                 `json:"importantTasks"`
	ReviewCandidates     []CMUXHandoffCandidate     `json:"reviewCandidates"`
	SuggestedNextActions []string                   `json:"suggestedNextActions"`
	Safety               CMUXHandoffSafety          `json:"safety"`
}

// CMUXHandoffOptions records the render parameters that were active when this
// handoff was generated.
type CMUXHandoffOptions struct {
	Limit  int    `json:"limit"`
	Output string `json:"output"`
}

// CMUXHandoffRuntimeSummary holds high-level metadata from the runtime-state
// contract.
type CMUXHandoffRuntimeSummary struct {
	Schema      string         `json:"schema"`
	GeneratedAt string         `json:"generatedAt,omitempty"`
	RepoRoot    string         `json:"repoRoot,omitempty"`
	TaskCounts  CMUXTaskCounts `json:"taskCounts"`
}

// CMUXHandoffCandidate extends CMUXTask with a review checklist and merge /
// cleanup readiness notes.  Used for tasks in reviewing or ready-for-merge
// state inside the reviewCandidates list.
type CMUXHandoffCandidate struct {
	Slug             string              `json:"slug"`
	State            string              `json:"state"`
	WorktreePath     string              `json:"worktreePath,omitempty"`
	WorktreePresence string              `json:"worktreePresence,omitempty"`
	PromptPath       string              `json:"promptPath,omitempty"`
	LastRunStatus    string              `json:"lastRunStatus,omitempty"`
	LastRunProvider  string              `json:"lastRunProvider,omitempty"`
	LastRunProfile   string              `json:"lastRunProfile,omitempty"`
	LastRunExitCode  string              `json:"lastRunExitCode,omitempty"`
	NextAction       string              `json:"nextAction"`
	MergeGate        string              `json:"mergeGate"`
	Attention        string              `json:"attention"`
	ReviewChecklist  []CMUXChecklistItem `json:"reviewChecklist"`
	MergeReadiness   string              `json:"mergeReadiness"`
	CleanupReadiness string              `json:"cleanupReadiness"`
}

// CMUXHandoffSafety carries the read-only safety attestation included in every
// handoff output.
type CMUXHandoffSafety struct {
	ReadOnly bool   `json:"readOnly"`
	Note     string `json:"note"`
}

// renderHandoff dispatches to the appropriate handoff renderer based on output
// mode.  Called only when opts.Handoff is true.
func renderHandoff(w io.Writer, snap Snapshot, opts RenderOptions) {
	switch opts.Output {
	case OutputMarkdown:
		renderMarkdownHandoff(w, snap, opts)
	case OutputJSON:
		renderJSONHandoff(w, snap, opts)
	default:
		renderTextHandoff(w, snap, opts)
	}
}

// rankTasksForHandoff returns a copy of tasks sorted by operational priority:
//
//  1. reviewing / ready-for-merge  (priority 0 — highest)
//  2. blocked / provider-gated     (priority 1)
//  3. ready-for-worker / runnable  (priority 2)
//  4. all other states             (priority 3 — lowest)
//
// Relative order within each priority tier is preserved (stable sort).
func rankTasksForHandoff(tasks []contracts.TaskSummary) []contracts.TaskSummary {
	priority := func(t contracts.TaskSummary) int {
		switch reviewTaskState(t) {
		case "reviewing", "ready-for-merge":
			return 0
		case "blocked", "provider-gated":
			return 1
		case "ready-for-worker", "runnable":
			return 2
		default:
			return 3
		}
	}
	ranked := make([]contracts.TaskSummary, len(tasks))
	copy(ranked, tasks)
	sort.SliceStable(ranked, func(i, j int) bool {
		return priority(ranked[i]) < priority(ranked[j])
	})
	return ranked
}

// selectReviewCandidates returns the subset of tasks whose normalised state is
// "reviewing" or "ready-for-merge".
func selectReviewCandidates(tasks []contracts.TaskSummary) []contracts.TaskSummary {
	var result []contracts.TaskSummary
	for _, t := range tasks {
		s := reviewTaskState(t)
		if s == "reviewing" || s == "ready-for-merge" {
			result = append(result, t)
		}
	}
	return result
}

// buildHandoffCandidate converts a TaskSummary into a CMUXHandoffCandidate by
// combining the standard JSON task fields with review checklist and readiness
// notes.
func buildHandoffCandidate(t contracts.TaskSummary) CMUXHandoffCandidate {
	jt := buildJSONTask(t)
	state := reviewTaskState(t)
	_, presence := resolveTaskWorktree(t)
	decision := buildReviewDecision(t)
	return CMUXHandoffCandidate{
		Slug:             jt.Slug,
		State:            jt.State,
		WorktreePath:     jt.WorktreePath,
		WorktreePresence: jt.WorktreePresence,
		PromptPath:       jt.PromptPath,
		LastRunStatus:    jt.LastRunStatus,
		LastRunProvider:  jt.LastRunProvider,
		LastRunProfile:   jt.LastRunProfile,
		LastRunExitCode:  jt.LastRunExitCode,
		NextAction:       decision.NextAction,
		MergeGate:        decision.MergeGate,
		Attention:        decision.Attention,
		ReviewChecklist:  buildReviewChecklist(t),
		MergeReadiness:   mergeReadinessNote(state),
		CleanupReadiness: cleanupReadinessNote(state, presence),
	}
}

// renderTextHandoff writes a plain-text handoff packet to w.
func renderTextHandoff(w io.Writer, snap Snapshot, opts RenderOptions) {
	limit := opts.effectiveLimit()

	// Header
	fmt.Fprintln(w, "CMUX HANDOFF PACKET  [read-only]")
	fmt.Fprintln(w, "=================================")
	fmt.Fprintln(w, "source: native")
	if snap.HasRuntimeState {
		if snap.RuntimeState.GeneratedAt != "" {
			fmt.Fprintf(w, "generated: %s\n", snap.RuntimeState.GeneratedAt)
		}
		if snap.RuntimeState.RepoRoot != "" {
			fmt.Fprintf(w, "repo: %s\n", snap.RuntimeState.RepoRoot)
		}
	}

	// Runtime summary
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Runtime Summary")
	if snap.RuntimeStateErr != nil {
		fmt.Fprintf(w, "  error: %v\n", snap.RuntimeStateErr)
	} else if !snap.HasRuntimeState {
		fmt.Fprintln(w, "  runtime-state: unavailable")
	} else {
		rs := snap.RuntimeState
		fmt.Fprintf(w, "  schema: %s\n", rs.Schema)
		tc := rs.TaskCounts
		fmt.Fprintf(w, "  tasks: tracked=%d  runnable=%d  blocked=%d  stale=%d  review=%d\n",
			tc.Tracked, tc.Runnable, tc.Blocked, tc.Stale, tc.Review)
	}
	fmt.Fprintln(w, sectionSep)

	// Providers — reuse the existing section renderer.
	if snap.RuntimeStateErr != nil {
		fmt.Fprintf(w, "\nProviders: error: %v\n", snap.RuntimeStateErr)
	} else if !snap.HasRuntimeState {
		fmt.Fprintln(w, "\nProviders: unavailable")
	} else {
		renderProviderHealth(w, snap)
	}
	fmt.Fprintln(w, sectionSep)

	// Queue / Scheduler — reuse the existing section renderer.
	renderQueueScheduler(w, snap)
	fmt.Fprintln(w, sectionSep)

	// Important tasks — ranked by priority, bounded by limit.
	if !snap.HasRuntimeState || len(snap.RuntimeState.Tasks) == 0 {
		fmt.Fprintln(w, "\nImportant Tasks: none tracked")
	} else {
		ranked := rankTasksForHandoff(snap.RuntimeState.Tasks)
		shown := ranked
		if limit > 0 && len(shown) > limit {
			shown = shown[:limit]
			fmt.Fprintf(w, "\nImportant Tasks  (showing %d of %d)\n", len(shown), len(ranked))
		} else {
			fmt.Fprintf(w, "\nImportant Tasks  (%d)\n", len(shown))
		}
		for i, t := range shown {
			if i > 0 {
				fmt.Fprintln(w)
			}
			fmt.Fprintf(w, "  %-32s %s\n", t.Slug, reviewTaskState(t))
			renderTaskWorktree(w, t)
			renderTaskPrompt(w, t)
			renderTaskLastRun(w, t)
		}
	}
	fmt.Fprintln(w, sectionSep)

	// Review candidates — reviewing/ready-for-merge, bounded by limit, with
	// inline checklist and readiness notes.
	var candidates []contracts.TaskSummary
	if snap.HasRuntimeState {
		candidates = selectReviewCandidates(snap.RuntimeState.Tasks)
		if limit > 0 && len(candidates) > limit {
			candidates = candidates[:limit]
		}
	}
	if len(candidates) == 0 {
		fmt.Fprintln(w, "\nReview Candidates: none")
	} else {
		fmt.Fprintf(w, "\nReview Candidates  (%d)\n", len(candidates))
		for _, t := range candidates {
			state := reviewTaskState(t)
			_, presence := resolveTaskWorktree(t)
			decision := buildReviewDecision(t)
			fmt.Fprintln(w)
			fmt.Fprintf(w, "  %s  [%s]\n", t.Slug, state)
			fmt.Fprintf(w, "  next:    %s\n", decision.NextAction)
			fmt.Fprintf(w, "  gate:    %s\n", decision.MergeGate)
			fmt.Fprintf(w, "  watch:   %s\n", decision.Attention)
			renderTaskWorktree(w, t)
			renderTaskPrompt(w, t)
			renderTaskLastRun(w, t)
			fmt.Fprintln(w, "  Checklist:")
			for _, item := range buildReviewChecklist(t) {
				mark := "[ ]"
				if item.Met {
					mark = "[x]"
				}
				fmt.Fprintf(w, "    %s %s\n", mark, item.Item)
			}
			fmt.Fprintf(w, "  merge:   %s\n", mergeReadinessNote(state))
			fmt.Fprintf(w, "  cleanup: %s\n", cleanupReadinessNote(state, presence))
		}
	}
	fmt.Fprintln(w, sectionSep)

	// Suggested next actions.
	fmt.Fprintln(w, "\nSuggested Next Actions")
	if snap.HasRuntimeState && len(snap.RuntimeState.SuggestedNextActions) > 0 {
		for _, action := range snap.RuntimeState.SuggestedNextActions {
			if strings.TrimSpace(action) != "" {
				fmt.Fprintf(w, "  - %s\n", action)
			}
		}
	} else {
		fmt.Fprintln(w, "  none")
	}
	fmt.Fprintln(w, sectionSep)

	// Safety attestation.
	fmt.Fprintf(w, "\n[read-only] %s\n", handoffSafetyNote)
}

// renderMarkdownHandoff writes a GitHub-Flavoured Markdown handoff packet to w.
func renderMarkdownHandoff(w io.Writer, snap Snapshot, opts RenderOptions) {
	limit := opts.effectiveLimit()

	// H1 header and metadata.
	fmt.Fprintln(w, "# CMUX Handoff Packet [read-only]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "**Source:** native")
	if snap.HasRuntimeState {
		rs := snap.RuntimeState
		if rs.GeneratedAt != "" {
			fmt.Fprintf(w, "**Generated:** %s\n", rs.GeneratedAt)
		}
		if rs.RepoRoot != "" {
			fmt.Fprintf(w, "**Repo:** %s\n", rs.RepoRoot)
		}
		fmt.Fprintf(w, "**Schema:** %s\n", rs.Schema)
	}

	// Runtime summary.
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Runtime Summary")
	fmt.Fprintln(w)
	if snap.RuntimeStateErr != nil {
		fmt.Fprintf(w, "> **runtime-state error:** %v\n", snap.RuntimeStateErr)
	} else if !snap.HasRuntimeState {
		fmt.Fprintln(w, "> **runtime-state:** unavailable")
	} else {
		tc := snap.RuntimeState.TaskCounts
		fmt.Fprintf(w, "tracked=%d  runnable=%d  blocked=%d  stale=%d  review=%d\n",
			tc.Tracked, tc.Runnable, tc.Blocked, tc.Stale, tc.Review)
	}

	// Providers — reuse the existing markdown section renderer.
	if snap.RuntimeStateErr != nil {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "> **Providers error:** %v\n", snap.RuntimeStateErr)
	} else if !snap.HasRuntimeState {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "> **Providers:** unavailable")
	} else {
		renderMarkdownProviderHealth(w, snap)
	}

	// Queue / Scheduler — reuse the existing markdown section renderer.
	renderMarkdownQueueScheduler(w, snap)

	// Important tasks.
	fmt.Fprintln(w)
	if !snap.HasRuntimeState || len(snap.RuntimeState.Tasks) == 0 {
		fmt.Fprintln(w, "## Important Tasks")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "_No tasks tracked._")
	} else {
		ranked := rankTasksForHandoff(snap.RuntimeState.Tasks)
		shown := ranked
		if limit > 0 && len(shown) > limit {
			shown = shown[:limit]
			fmt.Fprintf(w, "## Important Tasks (showing %d of %d)\n", len(shown), len(ranked))
		} else {
			fmt.Fprintf(w, "## Important Tasks (%d)\n", len(shown))
		}
		for _, t := range shown {
			fmt.Fprintln(w)
			fmt.Fprintf(w, "### %s\n", t.Slug)
			fmt.Fprintln(w)
			fmt.Fprintf(w, "**State:** %s\n", reviewTaskState(t))
			fmt.Fprintln(w)
			renderMarkdownTaskDetail(w, t)
		}
	}

	// Review candidates.
	fmt.Fprintln(w)
	var candidates []contracts.TaskSummary
	if snap.HasRuntimeState {
		candidates = selectReviewCandidates(snap.RuntimeState.Tasks)
		if limit > 0 && len(candidates) > limit {
			candidates = candidates[:limit]
		}
	}
	if len(candidates) == 0 {
		fmt.Fprintln(w, "## Review Candidates")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "_No review candidates._")
	} else {
		fmt.Fprintf(w, "## Review Candidates (%d)\n", len(candidates))
		for _, t := range candidates {
			state := reviewTaskState(t)
			_, presence := resolveTaskWorktree(t)
			decision := buildReviewDecision(t)
			fmt.Fprintln(w)
			fmt.Fprintf(w, "### %s [%s]\n", t.Slug, state)
			fmt.Fprintln(w)
			fmt.Fprintf(w, "**Next action:** %s\n", decision.NextAction)
			fmt.Fprintf(w, "**Merge gate:** %s\n", decision.MergeGate)
			fmt.Fprintf(w, "**Attention:** %s\n", decision.Attention)
			fmt.Fprintln(w)
			renderMarkdownTaskDetail(w, t)
			fmt.Fprintln(w)
			fmt.Fprintln(w, "#### Review Checklist")
			fmt.Fprintln(w)
			for _, item := range buildReviewChecklist(t) {
				check := " "
				if item.Met {
					check = "x"
				}
				fmt.Fprintf(w, "- [%s] %s\n", check, item.Item)
			}
			fmt.Fprintln(w)
			fmt.Fprintf(w, "**Merge Readiness:** %s\n", mergeReadinessNote(state))
			fmt.Fprintf(w, "**Cleanup Readiness:** %s\n", cleanupReadinessNote(state, presence))
		}
	}

	// Suggested next actions — reuse the existing markdown section renderer.
	renderMarkdownSuggestedActions(w, snap)

	// Safety attestation.
	fmt.Fprintln(w)
	fmt.Fprintln(w, "---")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "> **Safety note:** Read-only artifact. %s\n", handoffSafetyNote)
}

// renderJSONHandoff writes a CMUXHandoffReport as indented JSON to w.
func renderJSONHandoff(w io.Writer, snap Snapshot, opts RenderOptions) {
	limit := opts.effectiveLimit()
	outputStr := string(opts.Output)
	if strings.TrimSpace(outputStr) == "" {
		outputStr = string(OutputText)
	}

	report := CMUXHandoffReport{
		Schema: CMUXHandoffSchema,
		Source: "native",
		Options: CMUXHandoffOptions{
			Limit:  limit,
			Output: outputStr,
		},
		Errors:               make([]string, 0),
		ImportantTasks:       make([]CMUXTask, 0),
		ReviewCandidates:     make([]CMUXHandoffCandidate, 0),
		SuggestedNextActions: make([]string, 0),
		Safety: CMUXHandoffSafety{
			ReadOnly: true,
			Note:     handoffSafetyNote,
		},
	}

	if snap.RuntimeStateErr != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("runtime-state: %v", snap.RuntimeStateErr))
	}
	if snap.SchedulerPlanErr != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("scheduler-plan: %v", snap.SchedulerPlanErr))
	}

	if snap.HasRuntimeState {
		rs := snap.RuntimeState
		report.RuntimeSummary = &CMUXHandoffRuntimeSummary{
			Schema:      rs.Schema,
			GeneratedAt: rs.GeneratedAt,
			RepoRoot:    rs.RepoRoot,
			TaskCounts: CMUXTaskCounts{
				Tracked:  rs.TaskCounts.Tracked,
				Runnable: rs.TaskCounts.Runnable,
				Blocked:  rs.TaskCounts.Blocked,
				Stale:    rs.TaskCounts.Stale,
				Review:   rs.TaskCounts.Review,
			},
		}
		report.Providers = buildJSONProviders(snap)

		// Important tasks — ranked by priority, bounded by limit.
		ranked := rankTasksForHandoff(rs.Tasks)
		shown := ranked
		if limit > 0 && len(shown) > limit {
			shown = shown[:limit]
		}
		for _, t := range shown {
			report.ImportantTasks = append(report.ImportantTasks, buildJSONTask(t))
		}

		// Review candidates — reviewing/ready-for-merge, bounded by limit.
		candidates := selectReviewCandidates(rs.Tasks)
		if limit > 0 && len(candidates) > limit {
			candidates = candidates[:limit]
		}
		for _, t := range candidates {
			report.ReviewCandidates = append(report.ReviewCandidates, buildHandoffCandidate(t))
		}

		// Suggested next actions.
		for _, action := range rs.SuggestedNextActions {
			if strings.TrimSpace(action) != "" {
				report.SuggestedNextActions = append(report.SuggestedNextActions, action)
			}
		}
	}
	report.QueueScheduler = buildJSONQueue(snap)

	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(w, "{\"schema\":%q,\"error\":\"json marshal failed: %v\"}\n", CMUXHandoffSchema, err)
		return
	}
	_, _ = w.Write(append(out, '\n'))
}
