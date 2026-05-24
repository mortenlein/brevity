package cmux

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mortenlein/brevity/internal/contracts"
)

const sectionSep = "---"

// Render writes a CMUX report to w from a Snapshot.
//
// opts controls which sections are rendered, how many tasks are shown,
// optional task filters, and output format (text, markdown, or json).
// When opts.Handoff is true, handoff-packet mode is activated and all section,
// task, and state filters are overridden; --limit and --output still apply.
// When opts.MergeReport is true, merge-readiness report mode is activated and
// section/task/state filters are overridden; --limit and --output still apply.
// When opts.BlockedReport is true, blocked-task report mode is activated and
// section/task/state filters are overridden; --limit and --output still apply.
// When opts.ReviewTask is non-empty, review-packet mode is activated and
// section/task filters are overridden; --output still applies.
// Output is deterministic for a given Snapshot and RenderOptions.
// No ANSI sequences, no TUI framework, no watch mode, no keyboard handling.
// Every section degrades gracefully when its contract is unavailable.
func Render(w io.Writer, snap Snapshot, opts RenderOptions) {
	if opts.Handoff {
		renderHandoff(w, snap, opts)
		return
	}
	if opts.MergeReport {
		renderMerge(w, snap, opts)
		return
	}
	if opts.BlockedReport {
		renderBlocked(w, snap, opts)
		return
	}
	if opts.ReviewTask != "" {
		renderReview(w, snap, opts)
		return
	}
	if opts.Output == OutputMarkdown {
		renderMarkdown(w, snap, opts)
		return
	}
	if opts.Output == OutputJSON {
		renderJSON(w, snap, opts)
		return
	}

	section := opts.effectiveSection()

	renderHeader(w, snap)

	switch section {
	case SectionAll:
		if snap.RuntimeStateErr != nil {
			fmt.Fprintf(w, "\nruntime-state: error: %v\n", snap.RuntimeStateErr)
		} else if !snap.HasRuntimeState {
			fmt.Fprintln(w, "\nruntime-state: unavailable")
		} else {
			renderProviderHealth(w, snap)
			renderTaskCounts(w, snap)
			renderTopTasks(w, snap, opts)
		}
		fmt.Fprintln(w, sectionSep)
		renderQueueScheduler(w, snap)
		fmt.Fprintln(w, sectionSep)
		renderSuggestedActions(w, snap)

	case SectionProviders:
		if snap.RuntimeStateErr != nil {
			fmt.Fprintf(w, "\nruntime-state: error: %v\n", snap.RuntimeStateErr)
		} else if !snap.HasRuntimeState {
			fmt.Fprintln(w, "\nruntime-state: unavailable")
		} else {
			renderProviderHealth(w, snap)
		}

	case SectionTasks:
		if snap.RuntimeStateErr != nil {
			fmt.Fprintf(w, "\nruntime-state: error: %v\n", snap.RuntimeStateErr)
		} else if !snap.HasRuntimeState {
			fmt.Fprintln(w, "\nruntime-state: unavailable")
		} else {
			renderTaskCounts(w, snap)
			renderTopTasks(w, snap, opts)
		}

	case SectionQueue:
		renderQueueScheduler(w, snap)

	case SectionActions:
		renderSuggestedActions(w, snap)
	}
}

// renderHeader writes the dashboard header including read-only/source markers
// and schema/repo metadata when runtime state is available.
func renderHeader(w io.Writer, snap Snapshot) {
	fmt.Fprintln(w, "CMUX OPERATOR  [read-only]")
	fmt.Fprintln(w, "==========================")
	fmt.Fprintln(w, "source: native")
	if snap.HasRuntimeState {
		rs := snap.RuntimeState
		fmt.Fprintf(w, "schema: %s\n", rs.Schema)
		if rs.GeneratedAt != "" {
			fmt.Fprintf(w, "generated: %s\n", rs.GeneratedAt)
		}
		if rs.RepoRoot != "" {
			fmt.Fprintf(w, "repo: %s\n", rs.RepoRoot)
		}
	}
}

// renderProviderHealth renders the provider health section with per-provider rows
// showing status, last-updated timestamp, and note.
func renderProviderHealth(w io.Writer, snap Snapshot) {
	p := snap.RuntimeState.Providers
	fmt.Fprintln(w, "\nProviders")
	fmt.Fprintf(w, "  total=%d  degraded=%d  unavailable=%d\n",
		p.Summary.Total, p.Summary.Degraded, p.Summary.Unavailable)

	names := make([]string, 0, len(p.Health))
	for name := range p.Health {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		h := p.Health[name]
		status := h.Status
		if strings.TrimSpace(status) == "" {
			status = "unknown"
		}
		updatedAt := fallbackDash(h.UpdatedAt)
		note := fallbackDash(h.Note)
		fmt.Fprintf(w, "  %-20s %-20s %-28s %s\n", name, status, updatedAt, note)
	}
}

// renderTaskCounts renders the task count summary line.
func renderTaskCounts(w io.Writer, snap Snapshot) {
	tc := snap.RuntimeState.TaskCounts
	fmt.Fprintln(w, "\nTask Counts")
	fmt.Fprintf(w, "  tracked=%d  runnable=%d  blocked=%d  stale=%d  review=%d\n",
		tc.Tracked, tc.Runnable, tc.Blocked, tc.Stale, tc.Review)
}

// filterTasks applies TaskSlug and StateFilter from opts to tasks.
// It returns the filtered slice and a human-readable active-filter description
// (empty string when no filters are set).
// TaskSlug filter applies first (exact match); StateFilter applies second
// (case-insensitive match against normalised state, falling back to status).
func filterTasks(tasks []contracts.TaskSummary, opts RenderOptions) ([]contracts.TaskSummary, string) {
	slug := strings.TrimSpace(opts.TaskSlug)
	state := strings.TrimSpace(opts.StateFilter)
	if slug == "" && state == "" {
		return tasks, ""
	}

	filtered := tasks
	if slug != "" {
		var matched []contracts.TaskSummary
		for _, t := range filtered {
			if t.Slug == slug {
				matched = append(matched, t)
			}
		}
		filtered = matched
	}
	if state != "" {
		var matched []contracts.TaskSummary
		for _, t := range filtered {
			ns := strings.TrimSpace(t.NormalizedState)
			if ns == "" {
				ns = strings.TrimSpace(t.Status)
			}
			if strings.EqualFold(ns, state) {
				matched = append(matched, t)
			}
		}
		filtered = matched
	}

	// Build a compact filter description for use in empty-state messages.
	var parts []string
	if slug != "" {
		parts = append(parts, fmt.Sprintf("task=%q", slug))
	}
	if state != "" {
		parts = append(parts, fmt.Sprintf("state=%q", state))
	}
	return filtered, strings.Join(parts, " ")
}

// renderTopTasks renders each task with its slug, normalised state, worktree
// presence/path, prompt path, and last-run summary.
//
// Filtering (TaskSlug, StateFilter) is applied first; the limit from opts caps
// the number of rows shown.  When the list is truncated, a "(showing N of M)"
// header is emitted.  Empty results after filtering produce a focused message
// rather than a generic "none tracked" line.
func renderTopTasks(w io.Writer, snap Snapshot, opts RenderOptions) {
	tasks := snap.RuntimeState.Tasks
	if len(tasks) == 0 {
		fmt.Fprintln(w, "\nTask List: none tracked")
		return
	}

	filtered, filterDesc := filterTasks(tasks, opts)

	if len(filtered) == 0 {
		slug := strings.TrimSpace(opts.TaskSlug)
		state := strings.TrimSpace(opts.StateFilter)
		switch {
		case slug != "" && state == "":
			fmt.Fprintf(w, "\nTask List: task %q not found\n", slug)
		case slug == "" && state != "":
			fmt.Fprintf(w, "\nTask List: no tasks with state %q\n", state)
		default:
			fmt.Fprintf(w, "\nTask List: no tasks matching %s\n", filterDesc)
		}
		return
	}

	limit := opts.effectiveLimit()
	shown := filtered
	if limit > 0 && len(shown) > limit {
		shown = shown[:limit]
		fmt.Fprintf(w, "\nTask List  (showing %d of %d)\n", limit, len(filtered))
	} else {
		fmt.Fprintf(w, "\nTask List  (%d)\n", len(filtered))
	}
	for i, t := range shown {
		if i > 0 {
			fmt.Fprintln(w)
		}
		normalizedState := strings.TrimSpace(t.NormalizedState)
		if normalizedState == "" {
			normalizedState = strings.TrimSpace(t.Status)
		}
		if normalizedState == "" {
			normalizedState = "unknown"
		}
		fmt.Fprintf(w, "  %-32s %s\n", t.Slug, normalizedState)
		renderTaskWorktree(w, t)
		renderTaskPrompt(w, t)
		renderTaskLastRun(w, t)
	}
}

// resolveTaskWorktree extracts the worktree path and presence label from a
// task summary.  path is empty when no worktree information is available.
// presence is one of "present", "missing", or "unknown".
// Both renderers (text and markdown) use this helper to avoid duplicating the
// field-priority logic.
func resolveTaskWorktree(t contracts.TaskSummary) (path, presence string) {
	presence = "unknown"
	if t.Worktree != nil {
		path = t.Worktree.Path
		if t.Worktree.Exists {
			presence = "present"
		} else {
			presence = "missing"
		}
		return
	}
	if strings.TrimSpace(t.WorktreePath) != "" {
		path = t.WorktreePath
		if t.WorktreeExists != nil {
			if *t.WorktreeExists {
				presence = "present"
			} else {
				presence = "missing"
			}
		}
	}
	return
}

// renderTaskWorktree writes the worktree presence and path line for a task.
func renderTaskWorktree(w io.Writer, t contracts.TaskSummary) {
	path, presence := resolveTaskWorktree(t)
	if path != "" {
		fmt.Fprintf(w, "    worktree: %s  (%s)\n", path, presence)
	} else {
		fmt.Fprintln(w, "    worktree: (none)")
	}
}

// renderTaskPrompt writes the prompt path line for a task.
func renderTaskPrompt(w io.Writer, t contracts.TaskSummary) {
	if strings.TrimSpace(t.PromptPath) != "" {
		fmt.Fprintf(w, "    prompt:   %s\n", t.PromptPath)
	} else {
		fmt.Fprintln(w, "    prompt:   (none)")
	}
}

// renderTaskLastRun writes the last-run summary (status, provider/profile, exit code)
// for a task.  Falls back to WorkerStatus when LatestRunWorkerStatus is absent.
func renderTaskLastRun(w io.Writer, t contracts.TaskSummary) {
	if strings.TrimSpace(t.LatestRunWorkerStatus) != "" {
		exitCode := exitCodeStr(t.LatestRunExitCode)
		provider := fallbackDash(t.LatestRunProvider)
		profile := fallbackDash(t.LatestRunProfile)
		fmt.Fprintf(w, "    last-run: %s  %s/%s  exit=%s\n",
			t.LatestRunWorkerStatus, provider, profile, exitCode)
	} else if ws := strings.TrimSpace(t.WorkerStatus); ws != "" && ws != "-" {
		fmt.Fprintf(w, "    last-run: %s\n", ws)
	} else {
		fmt.Fprintln(w, "    last-run: (none)")
	}
}

// renderQueueScheduler renders the runtime-queue summary and scheduler next-selection.
func renderQueueScheduler(w io.Writer, snap Snapshot) {
	fmt.Fprintln(w, "\nQueue / Scheduler")

	// Queue block from runtime-state contract.
	if snap.HasRuntimeState && snap.RuntimeState.Queue != nil {
		q := snap.RuntimeState.Queue
		fmt.Fprintf(w, "  queue  state=%-12s total=%d  reserved=%d\n",
			fallbackDash(q.State), q.TotalItems, q.ReservedItems)

		if q.Plan != nil {
			p := q.Plan
			fmt.Fprintf(w, "  plan   runnable=%d  skipped=%d  reserved=%d\n",
				p.Runnable, p.Skipped, p.Reserved)
			if strings.TrimSpace(p.NextRunnableTask) != "" {
				fmt.Fprintf(w, "  next   %s\n", p.NextRunnableTask)
			} else {
				fmt.Fprintln(w, "  next   none")
			}
			if strings.TrimSpace(p.FirstSkipReason) != "" {
				fmt.Fprintf(w, "  skip   %s\n", p.FirstSkipReason)
			}
		}
	} else if snap.HasRuntimeState {
		fmt.Fprintln(w, "  queue: missing")
	}

	// Scheduler block from scheduler-plan contract.
	if snap.SchedulerPlanErr != nil {
		fmt.Fprintf(w, "  scheduler: error: %v\n", snap.SchedulerPlanErr)
	} else if snap.HasSchedulerPlan {
		sp := snap.SchedulerPlan
		if sp.Selected != nil {
			line := fmt.Sprintf("  scheduler-next: %s  id=%s", sp.Selected.Task, sp.Selected.ID)
			if strings.TrimSpace(sp.Selected.Reason) != "" {
				line += "  reason=" + sp.Selected.Reason
			}
			fmt.Fprintln(w, line)
		} else {
			reason := fallbackDash(sp.NoSelectionReason)
			fmt.Fprintf(w, "  scheduler-next: none — %s\n", reason)
		}
		fmt.Fprintf(w, "  reservation: %s\n", fallbackDash(sp.ReservationEligibility))
	}
}

// renderSuggestedActions renders guidance-only suggested next actions.
// These are display hints; no executable buttons or interactive elements.
func renderSuggestedActions(w io.Writer, snap Snapshot) {
	fmt.Fprintln(w, "\nSuggested Next Actions")
	if !snap.HasRuntimeState || len(snap.RuntimeState.SuggestedNextActions) == 0 {
		fmt.Fprintln(w, "  none")
		return
	}
	for _, action := range snap.RuntimeState.SuggestedNextActions {
		if strings.TrimSpace(action) == "" {
			continue
		}
		fmt.Fprintf(w, "  - %s\n", action)
	}
}

// exitCodeStr converts an any-typed exit code to a display string.
// JSON numbers decode to float64; nil and empty values are rendered as "-".
func exitCodeStr(code any) string {
	if code == nil {
		return "-"
	}
	s := fmt.Sprint(code)
	if s == "<nil>" || strings.TrimSpace(s) == "" {
		return "-"
	}
	// JSON decodes numbers as float64; render integer values without ".0".
	if f, ok := code.(float64); ok {
		if f == float64(int64(f)) {
			return fmt.Sprintf("%d", int64(f))
		}
	}
	return s
}

func fallbackDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
