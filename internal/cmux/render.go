package cmux

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mortenlein/brevity/internal/contracts"
)

const sectionSep = "---"

// Render writes a detailed plain-text CMUX dashboard to w from a Snapshot.
//
// Output is deterministic for a given Snapshot. No ANSI sequences,
// no TUI framework, no watch mode, no keyboard handling.
// Every section degrades gracefully when its contract is unavailable.
func Render(w io.Writer, snap Snapshot) {
	renderHeader(w, snap)

	if snap.RuntimeStateErr != nil {
		fmt.Fprintf(w, "\nruntime-state: error: %v\n", snap.RuntimeStateErr)
	} else if !snap.HasRuntimeState {
		fmt.Fprintln(w, "\nruntime-state: unavailable")
	} else {
		renderProviderHealth(w, snap)
		renderTaskCounts(w, snap)
		renderTopTasks(w, snap)
	}

	fmt.Fprintln(w, sectionSep)
	renderQueueScheduler(w, snap)

	fmt.Fprintln(w, sectionSep)
	renderSuggestedActions(w, snap)
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

// renderTopTasks renders each task with its slug, normalized state, worktree
// presence/path, prompt path, and last-run summary.
func renderTopTasks(w io.Writer, snap Snapshot) {
	tasks := snap.RuntimeState.Tasks
	if len(tasks) == 0 {
		fmt.Fprintln(w, "\nTask List: none tracked")
		return
	}
	fmt.Fprintf(w, "\nTask List  (%d)\n", len(tasks))
	for i, t := range tasks {
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

// renderTaskWorktree writes the worktree presence and path line for a task.
func renderTaskWorktree(w io.Writer, t contracts.TaskSummary) {
	worktreePath := ""
	worktreePresence := "unknown"

	if t.Worktree != nil {
		worktreePath = t.Worktree.Path
		if t.Worktree.Exists {
			worktreePresence = "present"
		} else {
			worktreePresence = "missing"
		}
	} else if strings.TrimSpace(t.WorktreePath) != "" {
		worktreePath = t.WorktreePath
		if t.WorktreeExists != nil {
			if *t.WorktreeExists {
				worktreePresence = "present"
			} else {
				worktreePresence = "missing"
			}
		}
	}

	if worktreePath != "" {
		fmt.Fprintf(w, "    worktree: %s  (%s)\n", worktreePath, worktreePresence)
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
