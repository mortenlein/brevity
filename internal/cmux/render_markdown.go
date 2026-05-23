package cmux

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mortenlein/brevity/internal/contracts"
)

// renderMarkdown writes a GitHub-Flavoured Markdown CMUX report to w.
//
// Structure mirrors the text renderer: the same section filtering, task
// filtering, and limit logic apply.  Only the output format differs.
// No ANSI sequences, no HTML, no code fences wrapping the whole document.
func renderMarkdown(w io.Writer, snap Snapshot, opts RenderOptions) {
	section := opts.effectiveSection()

	renderMarkdownHeader(w, snap)

	switch section {
	case SectionAll:
		if snap.RuntimeStateErr != nil {
			fmt.Fprintf(w, "\n> **runtime-state error:** %v\n", snap.RuntimeStateErr)
		} else if !snap.HasRuntimeState {
			fmt.Fprintln(w, "\n> **runtime-state:** unavailable")
		} else {
			renderMarkdownProviderHealth(w, snap)
			renderMarkdownTaskCounts(w, snap)
			renderMarkdownTopTasks(w, snap, opts)
		}
		renderMarkdownQueueScheduler(w, snap)
		renderMarkdownSuggestedActions(w, snap)

	case SectionProviders:
		if snap.RuntimeStateErr != nil {
			fmt.Fprintf(w, "\n> **runtime-state error:** %v\n", snap.RuntimeStateErr)
		} else if !snap.HasRuntimeState {
			fmt.Fprintln(w, "\n> **runtime-state:** unavailable")
		} else {
			renderMarkdownProviderHealth(w, snap)
		}

	case SectionTasks:
		if snap.RuntimeStateErr != nil {
			fmt.Fprintf(w, "\n> **runtime-state error:** %v\n", snap.RuntimeStateErr)
		} else if !snap.HasRuntimeState {
			fmt.Fprintln(w, "\n> **runtime-state:** unavailable")
		} else {
			renderMarkdownTaskCounts(w, snap)
			renderMarkdownTopTasks(w, snap, opts)
		}

	case SectionQueue:
		renderMarkdownQueueScheduler(w, snap)

	case SectionActions:
		renderMarkdownSuggestedActions(w, snap)
	}
}

// renderMarkdownHeader writes the H1 title and metadata block.
func renderMarkdownHeader(w io.Writer, snap Snapshot) {
	fmt.Fprintln(w, "# CMUX Operator Report [read-only]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "**Source:** native")
	if snap.HasRuntimeState {
		rs := snap.RuntimeState
		fmt.Fprintf(w, "**Schema:** %s\n", rs.Schema)
		if rs.GeneratedAt != "" {
			fmt.Fprintf(w, "**Generated:** %s\n", rs.GeneratedAt)
		}
		if rs.RepoRoot != "" {
			fmt.Fprintf(w, "**Repo:** %s\n", rs.RepoRoot)
		}
	}
}

// renderMarkdownProviderHealth writes the ## Providers section as a markdown
// table.  Falls back to an italic empty-state note when no providers exist.
func renderMarkdownProviderHealth(w io.Writer, snap Snapshot) {
	p := snap.RuntimeState.Providers
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Providers")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "**Summary:** total=%d  degraded=%d  unavailable=%d\n",
		p.Summary.Total, p.Summary.Degraded, p.Summary.Unavailable)

	names := make([]string, 0, len(p.Health))
	for name := range p.Health {
		names = append(names, name)
	}
	sort.Strings(names)

	if len(names) == 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "_No providers tracked._")
		return
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Provider | Status | Updated | Note |")
	fmt.Fprintln(w, "|---|---|---|---|")
	for _, name := range names {
		h := p.Health[name]
		status := h.Status
		if strings.TrimSpace(status) == "" {
			status = "unknown"
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s |\n",
			name, status, fallbackDash(h.UpdatedAt), fallbackDash(h.Note))
	}
}

// renderMarkdownTaskCounts writes the ## Task Counts section.
func renderMarkdownTaskCounts(w io.Writer, snap Snapshot) {
	tc := snap.RuntimeState.TaskCounts
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Task Counts")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "tracked=%d  runnable=%d  blocked=%d  stale=%d  review=%d\n",
		tc.Tracked, tc.Runnable, tc.Blocked, tc.Stale, tc.Review)
}

// renderMarkdownTopTasks writes the ## Task List section.  Each task gets a
// ### sub-heading with a bullet-list of its properties.  Filtering and limit
// from opts are applied identically to the text renderer.
func renderMarkdownTopTasks(w io.Writer, snap Snapshot, opts RenderOptions) {
	tasks := snap.RuntimeState.Tasks

	if len(tasks) == 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Task List")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "_No tasks tracked._")
		return
	}

	filtered, filterDesc := filterTasks(tasks, opts)

	if len(filtered) == 0 {
		slug := strings.TrimSpace(opts.TaskSlug)
		state := strings.TrimSpace(opts.StateFilter)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Task List")
		fmt.Fprintln(w)
		switch {
		case slug != "" && state == "":
			fmt.Fprintf(w, "_Task %q not found._\n", slug)
		case slug == "" && state != "":
			fmt.Fprintf(w, "_No tasks with state %q._\n", state)
		default:
			fmt.Fprintf(w, "_No tasks matching %s._\n", filterDesc)
		}
		return
	}

	limit := opts.effectiveLimit()
	shown := filtered
	fmt.Fprintln(w)
	if limit > 0 && len(shown) > limit {
		shown = shown[:limit]
		fmt.Fprintf(w, "## Task List (showing %d of %d)\n", limit, len(filtered))
	} else {
		fmt.Fprintf(w, "## Task List (%d)\n", len(filtered))
	}

	for _, t := range shown {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "### %s\n", t.Slug)
		fmt.Fprintln(w)

		normalizedState := strings.TrimSpace(t.NormalizedState)
		if normalizedState == "" {
			normalizedState = strings.TrimSpace(t.Status)
		}
		if normalizedState == "" {
			normalizedState = "unknown"
		}
		fmt.Fprintf(w, "**State:** %s\n", normalizedState)
		fmt.Fprintln(w)

		renderMarkdownTaskDetail(w, t)
	}
}

// renderMarkdownTaskDetail writes the bullet-list property rows for one task.
func renderMarkdownTaskDetail(w io.Writer, t contracts.TaskSummary) {
	path, presence := resolveTaskWorktree(t)
	if path != "" {
		fmt.Fprintf(w, "- **Worktree:** %s (%s)\n", path, presence)
	} else {
		fmt.Fprintln(w, "- **Worktree:** (none)")
	}

	if strings.TrimSpace(t.PromptPath) != "" {
		fmt.Fprintf(w, "- **Prompt:** %s\n", t.PromptPath)
	} else {
		fmt.Fprintln(w, "- **Prompt:** (none)")
	}

	if strings.TrimSpace(t.LatestRunWorkerStatus) != "" {
		exitCode := exitCodeStr(t.LatestRunExitCode)
		provider := fallbackDash(t.LatestRunProvider)
		profile := fallbackDash(t.LatestRunProfile)
		fmt.Fprintf(w, "- **Last Run:** %s  %s/%s  exit=%s\n",
			t.LatestRunWorkerStatus, provider, profile, exitCode)
	} else if ws := strings.TrimSpace(t.WorkerStatus); ws != "" && ws != "-" {
		fmt.Fprintf(w, "- **Last Run:** %s\n", ws)
	} else {
		fmt.Fprintln(w, "- **Last Run:** (none)")
	}
}

// renderMarkdownQueueScheduler writes the ## Queue / Scheduler section as a
// bullet list.
func renderMarkdownQueueScheduler(w io.Writer, snap Snapshot) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Queue / Scheduler")
	fmt.Fprintln(w)

	// Queue block from runtime-state contract.
	if snap.HasRuntimeState && snap.RuntimeState.Queue != nil {
		q := snap.RuntimeState.Queue
		fmt.Fprintf(w, "- **Queue:** state=%s  total=%d  reserved=%d\n",
			fallbackDash(q.State), q.TotalItems, q.ReservedItems)
		if q.Plan != nil {
			p := q.Plan
			fmt.Fprintf(w, "- **Plan:** runnable=%d  skipped=%d  reserved=%d\n",
				p.Runnable, p.Skipped, p.Reserved)
			if strings.TrimSpace(p.NextRunnableTask) != "" {
				fmt.Fprintf(w, "- **Next:** %s\n", p.NextRunnableTask)
			} else {
				fmt.Fprintln(w, "- **Next:** none")
			}
			if strings.TrimSpace(p.FirstSkipReason) != "" {
				fmt.Fprintf(w, "- **Skip:** %s\n", p.FirstSkipReason)
			}
		}
	} else if snap.HasRuntimeState {
		fmt.Fprintln(w, "- **Queue:** missing")
	}

	// Scheduler block from scheduler-plan contract.
	if snap.SchedulerPlanErr != nil {
		fmt.Fprintf(w, "- **Scheduler:** error: %v\n", snap.SchedulerPlanErr)
	} else if snap.HasSchedulerPlan {
		sp := snap.SchedulerPlan
		if sp.Selected != nil {
			line := fmt.Sprintf("- **Scheduler Next:** %s  id=%s", sp.Selected.Task, sp.Selected.ID)
			if strings.TrimSpace(sp.Selected.Reason) != "" {
				line += "  reason=" + sp.Selected.Reason
			}
			fmt.Fprintln(w, line)
		} else {
			reason := fallbackDash(sp.NoSelectionReason)
			fmt.Fprintf(w, "- **Scheduler Next:** none — %s\n", reason)
		}
		fmt.Fprintf(w, "- **Reservation:** %s\n", fallbackDash(sp.ReservationEligibility))
	}
}

// renderMarkdownSuggestedActions writes the ## Suggested Next Actions section.
// Falls back to an italic empty-state note when no actions are available.
func renderMarkdownSuggestedActions(w io.Writer, snap Snapshot) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Suggested Next Actions")
	fmt.Fprintln(w)
	if !snap.HasRuntimeState || len(snap.RuntimeState.SuggestedNextActions) == 0 {
		fmt.Fprintln(w, "_No actions suggested._")
		return
	}
	for _, action := range snap.RuntimeState.SuggestedNextActions {
		if strings.TrimSpace(action) == "" {
			continue
		}
		fmt.Fprintf(w, "- %s\n", action)
	}
}
