package cmux

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

const sectionSep = "---"

// Render writes a compact plain-text CMUX dashboard to w from a Snapshot.
//
// Output is deterministic for a given Snapshot. No ANSI sequences,
// no TUI framework, no watch mode, no keyboard handling.
// Every section degrades gracefully when its contract is unavailable.
func Render(w io.Writer, snap Snapshot) {
	fmt.Fprintln(w, "CMUX OPERATOR")
	fmt.Fprintln(w, "=============")

	if snap.RuntimeStateErr != nil {
		fmt.Fprintf(w, "runtime-state: error: %v\n", snap.RuntimeStateErr)
	} else if !snap.HasRuntimeState {
		fmt.Fprintln(w, "runtime-state: unavailable")
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

// renderProviderHealth renders the provider health summary section.
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
		line := fmt.Sprintf("  %-20s %s", name, status)
		if strings.TrimSpace(h.Note) != "" {
			line += "  (" + h.Note + ")"
		}
		fmt.Fprintln(w, line)
	}
}

// renderTaskCounts renders the task count bar by normalized state.
func renderTaskCounts(w io.Writer, snap Snapshot) {
	tc := snap.RuntimeState.TaskCounts
	fmt.Fprintln(w, "\nTask Counts")
	fmt.Fprintf(w, "  tracked=%d  runnable=%d  blocked=%d  stale=%d  review=%d\n",
		tc.Tracked, tc.Runnable, tc.Blocked, tc.Stale, tc.Review)
}

// renderTopTasks renders the task list with normalized state and worker status.
func renderTopTasks(w io.Writer, snap Snapshot) {
	tasks := snap.RuntimeState.Tasks
	if len(tasks) == 0 {
		fmt.Fprintln(w, "\nTask List: none tracked")
		return
	}
	fmt.Fprintln(w, "\nTask List")
	for _, t := range tasks {
		normalizedState := strings.TrimSpace(t.NormalizedState)
		if normalizedState == "" {
			normalizedState = strings.TrimSpace(t.Status)
		}
		if normalizedState == "" {
			normalizedState = "unknown"
		}
		workerStatus := strings.TrimSpace(t.WorkerStatus)
		if workerStatus == "" {
			workerStatus = "-"
		}
		fmt.Fprintf(w, "  %-32s %-22s worker=%s\n", t.Slug, normalizedState, workerStatus)
	}
}

// renderQueueScheduler renders the queue plan summary and scheduler next selection.
func renderQueueScheduler(w io.Writer, snap Snapshot) {
	fmt.Fprintln(w, "\nQueue / Scheduler")

	// Queue section from runtime-state contract.
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

	// Scheduler section from scheduler-plan contract.
	if snap.SchedulerPlanErr != nil {
		fmt.Fprintf(w, "  scheduler: error: %v\n", snap.SchedulerPlanErr)
	} else if snap.HasSchedulerPlan {
		sp := snap.SchedulerPlan
		if sp.Selected != nil {
			fmt.Fprintf(w, "  scheduler-next: %s  id=%s\n", sp.Selected.Task, sp.Selected.ID)
		} else {
			reason := fallbackDash(sp.NoSelectionReason)
			fmt.Fprintf(w, "  scheduler-next: none — %s\n", reason)
		}
		fmt.Fprintf(w, "  reservation: %s\n", fallbackDash(sp.ReservationEligibility))
	}
}

// renderSuggestedActions renders the suggested next actions from runtime state.
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

func fallbackDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
