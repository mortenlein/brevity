package cmux

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mortenlein/brevity/internal/contracts"
	runtimequeue "github.com/mortenlein/brevity/internal/runtime/queue"
)

// CMUXBlockedReportSchema is the schema identifier for the CMUX blocked-report
// JSON output.
const CMUXBlockedReportSchema = "brevity.cmux-blocked-report.v1"

// blockedReasonUnavailable is the standard message used when the blocking
// reason cannot be derived from the current runtime contracts.
const blockedReasonUnavailable = "reason unavailable from current contract"

// blockedSafetyNote is appended to every blocked-report text output to clarify
// that no runtime commands were executed.
const blockedSafetyNote = "Source: snapshot only. No runtime commands were executed."

// CMUXBlockedReport is the top-level typed output struct for --blocked-report
// mode.
//
// ProviderGated, Blocked, ReservedOrQueueGated, and Unknown are always
// non-nil slices (empty when no data falls into that group).
type CMUXBlockedReport struct {
	Schema               string                 `json:"schema"`
	Source               string                 `json:"source"`
	Options              CMUXBlockedOptions     `json:"options"`
	Errors               []string               `json:"errors"`
	Summary              CMUXBlockedSummary     `json:"summary"`
	ProviderGated        []CMUXBlockedTask      `json:"providerGated"`
	Blocked              []CMUXBlockedTask      `json:"blocked"`
	ReservedOrQueueGated []CMUXBlockedQueueItem `json:"reservedOrQueueGated"`
	Unknown              []CMUXBlockedTask      `json:"unknown"`
}

// CMUXBlockedOptions records the render parameters active when this blocked
// report was generated.
type CMUXBlockedOptions struct {
	Limit  int    `json:"limit"`
	Output string `json:"output"`
}

// CMUXBlockedSummary gives per-group and total item counts before any limit is
// applied.
type CMUXBlockedSummary struct {
	Total                int `json:"total"`
	ProviderGated        int `json:"providerGated"`
	Blocked              int `json:"blocked"`
	ReservedOrQueueGated int `json:"reservedOrQueueGated"`
	Unknown              int `json:"unknown"`
}

// CMUXBlockedTask is one blocked task entry.
// Reason is always non-empty; it uses blockedReasonUnavailable when no
// contract field provides a specific reason.
type CMUXBlockedTask struct {
	Slug             string `json:"slug"`
	State            string `json:"state"`
	Provider         string `json:"provider,omitempty"`
	ProviderHealth   string `json:"providerHealth,omitempty"`
	LastRunStatus    string `json:"lastRunStatus,omitempty"`
	LastRunFailType  string `json:"lastRunFailureType,omitempty"`
	WorktreePath     string `json:"worktreePath,omitempty"`
	WorktreePresence string `json:"worktreePresence,omitempty"`
	Reason           string `json:"reason"`
}

// CMUXBlockedQueueItem is one skipped scheduler-plan item from the
// reserved-or-queue-gated group.
type CMUXBlockedQueueItem struct {
	ID       string `json:"id"`
	Task     string `json:"task"`
	Provider string `json:"provider,omitempty"`
	Profile  string `json:"profile,omitempty"`
	Status   string `json:"status,omitempty"`
	Reason   string `json:"reason"`
}

// blockedGroups holds the four classified groups before limit is applied.
type blockedGroups struct {
	providerGated        []contracts.TaskSummary
	blocked              []contracts.TaskSummary
	reservedOrQueueGated []runtimequeue.PlanItem
	unknown              []contracts.TaskSummary
}

// classifyBlockedTasks partitions the task list into provider-gated, blocked,
// and unknown groups.  Tasks are sorted by slug within each group for
// deterministic output.  Queue-gated items come from snap.SchedulerPlan and
// are sorted by task slug then item ID.
func classifyBlockedTasks(snap Snapshot) blockedGroups {
	var pg, bl, unk []contracts.TaskSummary
	if snap.HasRuntimeState {
		for _, t := range snap.RuntimeState.Tasks {
			switch reviewTaskState(t) {
			case "provider-gated":
				pg = append(pg, t)
			case "blocked":
				bl = append(bl, t)
			default:
				// Unknown: task shows an incomplete-run signal and is not
				// already classified.
				if t.LatestRunIncomplete {
					unk = append(unk, t)
				}
			}
		}
	}
	sortBySlug := func(tasks []contracts.TaskSummary) {
		sort.Slice(tasks, func(i, j int) bool { return tasks[i].Slug < tasks[j].Slug })
	}
	sortBySlug(pg)
	sortBySlug(bl)
	sortBySlug(unk)

	var queueGated []runtimequeue.PlanItem
	if snap.HasSchedulerPlan {
		queueGated = append(queueGated, snap.SchedulerPlan.Skipped...)
		sort.Slice(queueGated, func(i, j int) bool {
			if queueGated[i].Task != queueGated[j].Task {
				return queueGated[i].Task < queueGated[j].Task
			}
			return queueGated[i].ID < queueGated[j].ID
		})
	}

	return blockedGroups{
		providerGated:        pg,
		blocked:              bl,
		reservedOrQueueGated: queueGated,
		unknown:              unk,
	}
}

// buildBlockedTask converts a TaskSummary into a CMUXBlockedTask, deriving
// the best available reason string from the contract fields.
func buildBlockedTask(t contracts.TaskSummary, group string) CMUXBlockedTask {
	state := reviewTaskState(t)
	wtPath, wtPresence := resolveTaskWorktree(t)
	if wtPath == "" && wtPresence == "unknown" {
		wtPresence = ""
	}

	var reason string
	switch group {
	case "provider-gated":
		if strings.TrimSpace(t.ProviderHealth) != "" && t.ProviderHealth != "healthy" {
			reason = "likely provider gated — provider health: " + t.ProviderHealth
		} else if strings.TrimSpace(t.Provider) != "" {
			reason = "likely provider gated — assigned provider: " + t.Provider
		} else {
			reason = "likely provider unavailable — " + blockedReasonUnavailable
		}
	case "blocked":
		if strings.TrimSpace(t.LatestRunFailureType) != "" {
			reason = t.LatestRunFailureType
		} else {
			reason = blockedReasonUnavailable
		}
	default: // unknown
		if t.LatestRunIncomplete {
			reason = "likely stuck — last run was incomplete"
		} else {
			reason = blockedReasonUnavailable
		}
	}

	return CMUXBlockedTask{
		Slug:             t.Slug,
		State:            state,
		Provider:         strings.TrimSpace(t.Provider),
		ProviderHealth:   strings.TrimSpace(t.ProviderHealth),
		LastRunStatus:    strings.TrimSpace(t.LatestRunWorkerStatus),
		LastRunFailType:  strings.TrimSpace(t.LatestRunFailureType),
		WorktreePath:     wtPath,
		WorktreePresence: wtPresence,
		Reason:           reason,
	}
}

// buildBlockedQueueItem converts a runtimequeue.PlanItem into a
// CMUXBlockedQueueItem.
func buildBlockedQueueItem(item runtimequeue.PlanItem) CMUXBlockedQueueItem {
	reason := strings.TrimSpace(item.Reason)
	if reason == "" {
		reason = blockedReasonUnavailable
	}
	return CMUXBlockedQueueItem{
		ID:       strings.TrimSpace(item.ID),
		Task:     strings.TrimSpace(item.Task),
		Provider: strings.TrimSpace(item.Provider),
		Profile:  strings.TrimSpace(item.Profile),
		Status:   strings.TrimSpace(item.Status),
		Reason:   reason,
	}
}

// renderBlocked dispatches to the appropriate blocked-report renderer.
// Called only when opts.BlockedReport is true.
func renderBlocked(w io.Writer, snap Snapshot, opts RenderOptions) {
	switch opts.Output {
	case OutputMarkdown:
		renderMarkdownBlocked(w, snap, opts)
	case OutputJSON:
		renderJSONBlocked(w, snap, opts)
	default:
		renderTextBlocked(w, snap, opts)
	}
}

// renderTextBlocked writes a plain-text blocked-report to w.
func renderTextBlocked(w io.Writer, snap Snapshot, opts RenderOptions) {
	limit := opts.effectiveLimit()

	fmt.Fprintln(w, "CMUX BLOCKED REPORT  [read-only]")
	fmt.Fprintln(w, "=================================")
	fmt.Fprintln(w, "source: native")
	if snap.HasRuntimeState {
		rs := snap.RuntimeState
		if rs.GeneratedAt != "" {
			fmt.Fprintf(w, "generated: %s\n", rs.GeneratedAt)
		}
		if rs.RepoRoot != "" {
			fmt.Fprintf(w, "repo: %s\n", rs.RepoRoot)
		}
	}

	if snap.RuntimeStateErr != nil {
		fmt.Fprintf(w, "\nruntime-state: error: %v\n", snap.RuntimeStateErr)
		// Still render queue-gated items if the scheduler plan is available.
	}
	if !snap.HasRuntimeState && snap.RuntimeStateErr == nil {
		fmt.Fprintln(w, "\nruntime-state: unavailable")
	}

	groups := classifyBlockedTasks(snap)
	total := len(groups.providerGated) + len(groups.blocked) +
		len(groups.reservedOrQueueGated) + len(groups.unknown)

	// Summary line
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Summary")
	fmt.Fprintf(w, "  total: %d  provider-gated: %d  blocked: %d  queue-gated: %d  unknown: %d\n",
		total,
		len(groups.providerGated),
		len(groups.blocked),
		len(groups.reservedOrQueueGated),
		len(groups.unknown),
	)

	// --- provider-gated ---
	fmt.Fprintln(w, sectionSep)
	renderTextBlockedTaskGroup(w, "provider-gated", groups.providerGated, "provider-gated", limit)

	// --- blocked ---
	fmt.Fprintln(w, sectionSep)
	renderTextBlockedTaskGroup(w, "blocked", groups.blocked, "blocked", limit)

	// --- reserved-or-queue-gated ---
	fmt.Fprintln(w, sectionSep)
	shown := groups.reservedOrQueueGated
	truncated := false
	if limit > 0 && len(shown) > limit {
		shown = shown[:limit]
		truncated = true
	}
	if truncated {
		fmt.Fprintf(w, "\nreserved-or-queue-gated  (showing %d of %d)  [from scheduler plan]\n",
			len(shown), len(groups.reservedOrQueueGated))
	} else {
		fmt.Fprintf(w, "\nreserved-or-queue-gated  (%d)  [from scheduler plan]\n",
			len(groups.reservedOrQueueGated))
	}
	if len(groups.reservedOrQueueGated) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		for i, item := range shown {
			if i > 0 {
				fmt.Fprintln(w)
			}
			q := buildBlockedQueueItem(item)
			taskPart := q.Task
			if taskPart == "" {
				taskPart = "(unknown)"
			}
			statusPart := ""
			if q.Status != "" {
				statusPart = "  status: " + q.Status
			}
			providerPart := ""
			if q.Provider != "" {
				providerPart = "  provider: " + q.Provider
			}
			fmt.Fprintf(w, "  %s  task: %s%s%s\n", q.ID, taskPart, statusPart, providerPart)
			fmt.Fprintf(w, "    reason: %s\n", q.Reason)
		}
		if !snap.HasSchedulerPlan && snap.SchedulerPlanErr != nil {
			fmt.Fprintf(w, "  scheduler-plan: error: %v\n", snap.SchedulerPlanErr)
		}
	}

	// --- unknown ---
	fmt.Fprintln(w, sectionSep)
	renderTextBlockedTaskGroup(w, "unknown", groups.unknown, "unknown", limit)

	// Safety note
	fmt.Fprintln(w, sectionSep)
	fmt.Fprintf(w, "\n[read-only] %s\n", blockedSafetyNote)
}

// renderTextBlockedTaskGroup renders one task-level blocked group in text mode.
func renderTextBlockedTaskGroup(w io.Writer, label string, tasks []contracts.TaskSummary, groupKey string, limit int) {
	shown := tasks
	truncated := false
	if limit > 0 && len(shown) > limit {
		shown = shown[:limit]
		truncated = true
	}
	if truncated {
		fmt.Fprintf(w, "\n%s  (showing %d of %d)\n", label, len(shown), len(tasks))
	} else {
		fmt.Fprintf(w, "\n%s  (%d)\n", label, len(tasks))
	}
	if len(tasks) == 0 {
		fmt.Fprintln(w, "  (none)")
		return
	}
	for i, t := range shown {
		if i > 0 {
			fmt.Fprintln(w)
		}
		bt := buildBlockedTask(t, groupKey)
		fmt.Fprintf(w, "  %-32s %s\n", bt.Slug, bt.State)
		if bt.Provider != "" {
			providerLine := "    provider: " + bt.Provider
			if bt.ProviderHealth != "" {
				providerLine += "  health: " + bt.ProviderHealth
			}
			fmt.Fprintln(w, providerLine)
		}
		if bt.LastRunStatus != "" {
			runLine := "    last-run: " + bt.LastRunStatus
			if bt.LastRunFailType != "" {
				runLine += "  failure-type: " + bt.LastRunFailType
			}
			fmt.Fprintln(w, runLine)
		}
		renderTaskWorktree(w, t)
		renderTaskPrompt(w, t)
		fmt.Fprintf(w, "    reason:   %s\n", bt.Reason)
	}
}

// renderMarkdownBlocked writes a GitHub-Flavoured Markdown blocked-report to w.
func renderMarkdownBlocked(w io.Writer, snap Snapshot, opts RenderOptions) {
	limit := opts.effectiveLimit()

	fmt.Fprintln(w, "# CMUX Blocked Report [read-only]")
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
	}

	if snap.RuntimeStateErr != nil {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "> **runtime-state error:** %v\n", snap.RuntimeStateErr)
	} else if !snap.HasRuntimeState {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "> **runtime-state:** unavailable")
	}

	groups := classifyBlockedTasks(snap)
	total := len(groups.providerGated) + len(groups.blocked) +
		len(groups.reservedOrQueueGated) + len(groups.unknown)

	// Summary
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Summary")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "total=%d  provider-gated=%d  blocked=%d  queue-gated=%d  unknown=%d\n",
		total,
		len(groups.providerGated),
		len(groups.blocked),
		len(groups.reservedOrQueueGated),
		len(groups.unknown),
	)

	// provider-gated
	fmt.Fprintln(w)
	renderMarkdownBlockedTaskGroup(w, "provider-gated", groups.providerGated, "provider-gated", limit)

	// blocked
	fmt.Fprintln(w)
	renderMarkdownBlockedTaskGroup(w, "blocked", groups.blocked, "blocked", limit)

	// reserved-or-queue-gated
	fmt.Fprintln(w)
	shown := groups.reservedOrQueueGated
	truncated := false
	if limit > 0 && len(shown) > limit {
		shown = shown[:limit]
		truncated = true
	}
	if truncated {
		fmt.Fprintf(w, "## reserved-or-queue-gated (showing %d of %d)\n", len(shown), len(groups.reservedOrQueueGated))
	} else {
		fmt.Fprintf(w, "## reserved-or-queue-gated (%d)\n", len(groups.reservedOrQueueGated))
	}
	fmt.Fprintln(w)
	if len(groups.reservedOrQueueGated) == 0 {
		fmt.Fprintln(w, "_None._")
	} else {
		for _, item := range shown {
			q := buildBlockedQueueItem(item)
			taskPart := q.Task
			if taskPart == "" {
				taskPart = "(unknown)"
			}
			fmt.Fprintf(w, "### %s\n", taskPart)
			fmt.Fprintln(w)
			fmt.Fprintf(w, "- **Queue ID:** %s\n", q.ID)
			if q.Status != "" {
				fmt.Fprintf(w, "- **Status:** %s\n", q.Status)
			}
			if q.Provider != "" {
				fmt.Fprintf(w, "- **Provider:** %s\n", q.Provider)
			}
			if q.Profile != "" {
				fmt.Fprintf(w, "- **Profile:** %s\n", q.Profile)
			}
			fmt.Fprintf(w, "- **Reason:** %s\n", q.Reason)
			fmt.Fprintln(w)
		}
		if snap.SchedulerPlanErr != nil {
			fmt.Fprintf(w, "> **scheduler-plan error:** %v\n", snap.SchedulerPlanErr)
		}
	}

	// unknown
	fmt.Fprintln(w)
	renderMarkdownBlockedTaskGroup(w, "unknown", groups.unknown, "unknown", limit)

	// Safety note
	fmt.Fprintln(w)
	fmt.Fprintln(w, "---")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "> **Read-only:** %s\n", blockedSafetyNote)
}

// renderMarkdownBlockedTaskGroup renders one task-level blocked group in
// markdown mode.
func renderMarkdownBlockedTaskGroup(w io.Writer, label string, tasks []contracts.TaskSummary, groupKey string, limit int) {
	shown := tasks
	truncated := false
	if limit > 0 && len(shown) > limit {
		shown = shown[:limit]
		truncated = true
	}
	if truncated {
		fmt.Fprintf(w, "## %s (showing %d of %d)\n", label, len(shown), len(tasks))
	} else {
		fmt.Fprintf(w, "## %s (%d)\n", label, len(tasks))
	}
	fmt.Fprintln(w)
	if len(tasks) == 0 {
		fmt.Fprintln(w, "_None._")
		return
	}
	for _, t := range shown {
		bt := buildBlockedTask(t, groupKey)
		fmt.Fprintf(w, "### %s\n", bt.Slug)
		fmt.Fprintln(w)
		fmt.Fprintf(w, "**State:** %s\n", bt.State)
		fmt.Fprintln(w)
		renderMarkdownTaskDetail(w, t)
		if bt.Provider != "" {
			fmt.Fprintf(w, "- **Provider:** %s\n", bt.Provider)
			if bt.ProviderHealth != "" {
				fmt.Fprintf(w, "- **Provider Health:** %s\n", bt.ProviderHealth)
			}
		}
		if bt.LastRunFailType != "" {
			fmt.Fprintf(w, "- **Failure Type:** %s\n", bt.LastRunFailType)
		}
		fmt.Fprintf(w, "- **Reason:** %s\n", bt.Reason)
		fmt.Fprintln(w)
	}
}

// renderJSONBlocked writes a CMUXBlockedReport as indented JSON to w.
func renderJSONBlocked(w io.Writer, snap Snapshot, opts RenderOptions) {
	limit := opts.effectiveLimit()
	outputStr := string(opts.Output)
	if strings.TrimSpace(outputStr) == "" {
		outputStr = string(OutputText)
	}

	groups := classifyBlockedTasks(snap)

	// Build summary from full (pre-limit) counts.
	total := len(groups.providerGated) + len(groups.blocked) +
		len(groups.reservedOrQueueGated) + len(groups.unknown)

	report := CMUXBlockedReport{
		Schema: CMUXBlockedReportSchema,
		Source: "native",
		Options: CMUXBlockedOptions{
			Limit:  limit,
			Output: outputStr,
		},
		Errors: make([]string, 0),
		Summary: CMUXBlockedSummary{
			Total:                total,
			ProviderGated:        len(groups.providerGated),
			Blocked:              len(groups.blocked),
			ReservedOrQueueGated: len(groups.reservedOrQueueGated),
			Unknown:              len(groups.unknown),
		},
		ProviderGated:        make([]CMUXBlockedTask, 0),
		Blocked:              make([]CMUXBlockedTask, 0),
		ReservedOrQueueGated: make([]CMUXBlockedQueueItem, 0),
		Unknown:              make([]CMUXBlockedTask, 0),
	}

	if snap.RuntimeStateErr != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("runtime-state: %v", snap.RuntimeStateErr))
	}
	if snap.SchedulerPlanErr != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("scheduler-plan: %v", snap.SchedulerPlanErr))
	}

	// Populate each group with limit applied.
	pgShown := groups.providerGated
	if limit > 0 && len(pgShown) > limit {
		pgShown = pgShown[:limit]
	}
	for _, t := range pgShown {
		report.ProviderGated = append(report.ProviderGated, buildBlockedTask(t, "provider-gated"))
	}

	blShown := groups.blocked
	if limit > 0 && len(blShown) > limit {
		blShown = blShown[:limit]
	}
	for _, t := range blShown {
		report.Blocked = append(report.Blocked, buildBlockedTask(t, "blocked"))
	}

	qgShown := groups.reservedOrQueueGated
	if limit > 0 && len(qgShown) > limit {
		qgShown = qgShown[:limit]
	}
	for _, item := range qgShown {
		report.ReservedOrQueueGated = append(report.ReservedOrQueueGated, buildBlockedQueueItem(item))
	}

	unkShown := groups.unknown
	if limit > 0 && len(unkShown) > limit {
		unkShown = unkShown[:limit]
	}
	for _, t := range unkShown {
		report.Unknown = append(report.Unknown, buildBlockedTask(t, "unknown"))
	}

	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(w, "{\"schema\":%q,\"error\":\"json marshal failed: %v\"}\n", CMUXBlockedReportSchema, err)
		return
	}
	_, _ = w.Write(append(out, '\n'))
}
