package cmux

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mortenlein/brevity/internal/contracts"
)

// CMUXReportSchema is the schema identifier for the CMUX JSON report contract.
const CMUXReportSchema = "brevity.cmux-report.v1"

// CMUXReport is the top-level typed output struct for --output json.
//
// Section fields (Providers, Tasks, Queue, Actions) are present only when the
// active section includes them.  Absent sections are omitted from the JSON
// entirely (not null).  All slice fields inside present sections use empty
// arrays [] rather than null.
type CMUXReport struct {
	Schema    string              `json:"schema"`
	Source    string              `json:"source"`
	Section   string              `json:"section"`
	Errors    []string            `json:"errors"`
	Providers *CMUXProviders      `json:"providers,omitempty"`
	Tasks     *CMUXTasks          `json:"tasks,omitempty"`
	Queue     *CMUXQueueScheduler `json:"queue,omitempty"`
	Actions   *[]string           `json:"actions,omitempty"`
}

// CMUXProviders is the provider health block in the JSON report.
type CMUXProviders struct {
	Total       int               `json:"total"`
	Degraded    int               `json:"degraded"`
	Unavailable int               `json:"unavailable"`
	Providers   []CMUXProviderRow `json:"providers"`
}

// CMUXProviderRow is one provider entry in the JSON providers list.
// Providers are sorted by name for deterministic output.
type CMUXProviderRow struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	Note      string `json:"note,omitempty"`
}

// CMUXTaskCounts mirrors the runtime-state task count summary.
type CMUXTaskCounts struct {
	Tracked  int `json:"tracked"`
	Runnable int `json:"runnable"`
	Blocked  int `json:"blocked"`
	Stale    int `json:"stale"`
	Review   int `json:"review"`
}

// CMUXTasks is the task block in the JSON report.
// Matched is the count after filtering; Shown is the count after the limit is applied.
type CMUXTasks struct {
	Counts  CMUXTaskCounts `json:"counts"`
	Matched int            `json:"matched"`
	Shown   int            `json:"shown"`
	Tasks   []CMUXTask     `json:"tasks"`
}

// CMUXTask is one task entry in the JSON tasks list.
type CMUXTask struct {
	Slug             string `json:"slug"`
	State            string `json:"state"`
	WorktreePath     string `json:"worktreePath,omitempty"`
	WorktreePresence string `json:"worktreePresence,omitempty"`
	PromptPath       string `json:"promptPath,omitempty"`
	LastRunStatus    string `json:"lastRunStatus,omitempty"`
	LastRunProvider  string `json:"lastRunProvider,omitempty"`
	LastRunProfile   string `json:"lastRunProfile,omitempty"`
	LastRunExitCode  string `json:"lastRunExitCode,omitempty"`
}

// CMUXQueueScheduler is the queue/scheduler block in the JSON report.
// Queue fields come from the runtime-state contract; Scheduler fields come from
// the scheduler-plan contract.
type CMUXQueueScheduler struct {
	QueueState             string `json:"queueState"`
	QueueTotal             int    `json:"queueTotal"`
	QueueReserved          int    `json:"queueReserved"`
	PlanRunnable           int    `json:"planRunnable,omitempty"`
	PlanSkipped            int    `json:"planSkipped,omitempty"`
	PlanReserved           int    `json:"planReserved,omitempty"`
	NextTask               string `json:"nextTask,omitempty"`
	SkipReason             string `json:"skipReason,omitempty"`
	SchedulerNext          string `json:"schedulerNext,omitempty"`
	SchedulerID            string `json:"schedulerID,omitempty"`
	SchedulerReason        string `json:"schedulerReason,omitempty"`
	ReservationEligibility string `json:"reservationEligibility,omitempty"`
	SchedulerError         string `json:"schedulerError,omitempty"`
}

// renderJSON serialises a Snapshot into a CMUXReport and writes it as
// indented JSON followed by a newline.
//
// All RenderOptions (section, limit, task slug, state filter) are respected
// identically to the text and markdown renderers.  Output is deterministic for
// a given Snapshot and RenderOptions.
func renderJSON(w io.Writer, snap Snapshot, opts RenderOptions) {
	section := opts.effectiveSection()

	report := CMUXReport{
		Schema:  CMUXReportSchema,
		Source:  "native",
		Section: section,
		Errors:  make([]string, 0),
	}

	// Collect contract-level errors into the top-level errors slice.
	if snap.RuntimeStateErr != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("runtime-state: %v", snap.RuntimeStateErr))
	}
	if snap.SchedulerPlanErr != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("scheduler-plan: %v", snap.SchedulerPlanErr))
	}

	// Populate only the sections that the active section flag includes.
	switch section {
	case SectionAll:
		report.Providers = buildJSONProviders(snap)
		report.Tasks = buildJSONTasks(snap, opts)
		report.Queue = buildJSONQueue(snap)
		actions := buildJSONActions(snap)
		report.Actions = &actions
	case SectionProviders:
		report.Providers = buildJSONProviders(snap)
	case SectionTasks:
		report.Tasks = buildJSONTasks(snap, opts)
	case SectionQueue:
		report.Queue = buildJSONQueue(snap)
	case SectionActions:
		actions := buildJSONActions(snap)
		report.Actions = &actions
	}

	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		// Fallback: emit a minimal error JSON so output is always valid.
		fmt.Fprintf(w, "{\"schema\":%q,\"error\":\"json marshal failed: %v\"}\n", CMUXReportSchema, err)
		return
	}
	_, _ = w.Write(append(out, '\n'))
}

// buildJSONProviders constructs the CMUXProviders block from snap.
// Providers are sorted by name for deterministic output.
// Returns a non-nil pointer with an empty Providers slice when runtime state
// is unavailable.
func buildJSONProviders(snap Snapshot) *CMUXProviders {
	p := &CMUXProviders{
		Providers: make([]CMUXProviderRow, 0),
	}
	if !snap.HasRuntimeState {
		return p
	}
	rs := snap.RuntimeState
	p.Total = rs.Providers.Summary.Total
	p.Degraded = rs.Providers.Summary.Degraded
	p.Unavailable = rs.Providers.Summary.Unavailable

	names := make([]string, 0, len(rs.Providers.Health))
	for name := range rs.Providers.Health {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		h := rs.Providers.Health[name]
		status := h.Status
		if strings.TrimSpace(status) == "" {
			status = "unknown"
		}
		p.Providers = append(p.Providers, CMUXProviderRow{
			Name:      name,
			Status:    status,
			UpdatedAt: h.UpdatedAt,
			Note:      h.Note,
		})
	}
	return p
}

// buildJSONTasks constructs the CMUXTasks block from snap, applying the same
// filterTasks and effectiveLimit logic as the text and markdown renderers.
// Returns a non-nil pointer with an empty Tasks slice when runtime state is
// unavailable.
func buildJSONTasks(snap Snapshot, opts RenderOptions) *CMUXTasks {
	t := &CMUXTasks{
		Tasks: make([]CMUXTask, 0),
	}
	if !snap.HasRuntimeState {
		return t
	}
	rs := snap.RuntimeState
	t.Counts = CMUXTaskCounts{
		Tracked:  rs.TaskCounts.Tracked,
		Runnable: rs.TaskCounts.Runnable,
		Blocked:  rs.TaskCounts.Blocked,
		Stale:    rs.TaskCounts.Stale,
		Review:   rs.TaskCounts.Review,
	}

	filtered, _ := filterTasks(rs.Tasks, opts)
	t.Matched = len(filtered)

	limit := opts.effectiveLimit()
	shown := filtered
	if limit > 0 && len(shown) > limit {
		shown = shown[:limit]
	}
	t.Shown = len(shown)

	for _, task := range shown {
		t.Tasks = append(t.Tasks, buildJSONTask(task))
	}
	return t
}

// buildJSONTask converts one contracts.TaskSummary into a CMUXTask.
func buildJSONTask(t contracts.TaskSummary) CMUXTask {
	state := strings.TrimSpace(t.NormalizedState)
	if state == "" {
		state = strings.TrimSpace(t.Status)
	}
	if state == "" {
		state = "unknown"
	}

	worktreePath, worktreePresence := resolveTaskWorktree(t)
	// resolveTaskWorktree returns "unknown" presence when path is empty and no
	// worktree struct is present; normalise that to "" so the field is omitted.
	if worktreePath == "" && worktreePresence == "unknown" {
		worktreePresence = ""
	}

	lastRunStatus := strings.TrimSpace(t.LatestRunWorkerStatus)
	lastRunProvider := ""
	lastRunProfile := ""
	lastRunExitCode := ""
	if lastRunStatus != "" {
		lastRunProvider = t.LatestRunProvider
		lastRunProfile = t.LatestRunProfile
		lastRunExitCode = exitCodeStr(t.LatestRunExitCode)
	} else {
		ws := strings.TrimSpace(t.WorkerStatus)
		if ws != "" && ws != "-" {
			lastRunStatus = ws
		}
	}

	return CMUXTask{
		Slug:             t.Slug,
		State:            state,
		WorktreePath:     worktreePath,
		WorktreePresence: worktreePresence,
		PromptPath:       t.PromptPath,
		LastRunStatus:    lastRunStatus,
		LastRunProvider:  lastRunProvider,
		LastRunProfile:   lastRunProfile,
		LastRunExitCode:  lastRunExitCode,
	}
}

// buildJSONQueue constructs the CMUXQueueScheduler block from snap.
// Queue data comes from the runtime-state contract; scheduler data comes from
// the scheduler-plan contract.
func buildJSONQueue(snap Snapshot) *CMUXQueueScheduler {
	q := &CMUXQueueScheduler{}

	if snap.HasRuntimeState && snap.RuntimeState.Queue != nil {
		rq := snap.RuntimeState.Queue
		q.QueueState = fallbackDash(rq.State)
		q.QueueTotal = rq.TotalItems
		q.QueueReserved = rq.ReservedItems
		if rq.Plan != nil {
			q.PlanRunnable = rq.Plan.Runnable
			q.PlanSkipped = rq.Plan.Skipped
			q.PlanReserved = rq.Plan.Reserved
			q.NextTask = rq.Plan.NextRunnableTask
			q.SkipReason = rq.Plan.FirstSkipReason
		}
	}

	if snap.SchedulerPlanErr != nil {
		q.SchedulerError = snap.SchedulerPlanErr.Error()
	} else if snap.HasSchedulerPlan {
		sp := snap.SchedulerPlan
		if sp.Selected != nil {
			q.SchedulerNext = sp.Selected.Task
			q.SchedulerID = sp.Selected.ID
			q.SchedulerReason = sp.Selected.Reason
		}
		q.ReservationEligibility = sp.ReservationEligibility
	}

	return q
}

// buildJSONActions returns the suggested next actions slice from snap.
// Always returns a non-nil slice (empty when no actions or runtime state is
// unavailable) so the JSON field renders as [] rather than null.
func buildJSONActions(snap Snapshot) []string {
	actions := make([]string, 0)
	if !snap.HasRuntimeState {
		return actions
	}
	for _, action := range snap.RuntimeState.SuggestedNextActions {
		if strings.TrimSpace(action) != "" {
			actions = append(actions, action)
		}
	}
	return actions
}
