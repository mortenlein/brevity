// Package cmux is the read-only CMUX operator layer over Brevity runtime contracts.
//
// CMUX consumes two existing Brevity CLI contracts:
//   - brevity.runtime-state.v1  (brevity runtime state --json)
//   - brevity.runtime-scheduler-plan.v1 (brevity scheduler plan --json)
//
// It never reads .brevity files directly, never mutates runtime state,
// and never owns a scheduler, task store, provider registry, or worker lifecycle.
package cmux

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mortenlein/brevity/internal/contracts"
	runtimescheduler "github.com/mortenlein/brevity/internal/runtime/scheduler"
)

// DefaultLimit is the default maximum number of tasks shown in the task list.
const DefaultLimit = 10

// Valid Section values for RenderOptions.
const (
	SectionAll       = "all"
	SectionProviders = "providers"
	SectionTasks     = "tasks"
	SectionQueue     = "queue"
	SectionActions   = "actions"
)

// OutputMode selects the rendered output format.
type OutputMode string

const (
	// OutputText selects the plain-text renderer (default).
	OutputText OutputMode = "text"
	// OutputMarkdown selects the GitHub-Flavoured Markdown renderer.
	OutputMarkdown OutputMode = "markdown"
	// OutputJSON selects the structured JSON renderer.
	OutputJSON OutputMode = "json"
)

// RenderOptions controls which sections are rendered, how many tasks are shown,
// optional task-level filters, and output format.
// The zero value selects all sections with the default task limit, no filtering,
// and plain-text output.
type RenderOptions struct {
	// Limit is the maximum number of tasks to show in the task list.
	// 0 or negative uses DefaultLimit.
	Limit int

	// Section restricts rendering to one named section.
	// Allowed values: "all", "providers", "tasks", "queue", "actions".
	// Empty string and "all" both render every section.
	Section string

	// TaskSlug restricts the task list to the single task with this slug.
	// Empty string disables slug filtering.
	TaskSlug string

	// StateFilter restricts the task list to tasks whose normalised state
	// matches this value (case-insensitive exact match).
	// Empty string disables state filtering.
	StateFilter string

	// Output selects the output format.
	// Empty string and OutputText both select the plain-text renderer.
	// OutputMarkdown selects the GitHub-Flavoured Markdown renderer.
	// OutputJSON selects the structured JSON renderer.
	Output OutputMode

	// ReviewTask, when non-empty, activates review-packet mode for this task
	// slug.  Review mode overrides --section and --task; --output still applies.
	// The rendered packet includes task detail, queue/scheduler context, a
	// review checklist, and merge/cleanup readiness notes.
	ReviewTask string

	// Handoff, when true, activates AI handoff packet mode.  Handoff mode
	// ignores --section, --task, and --state filters; --limit and --output
	// still apply.  The packet includes runtime summary, providers,
	// queue/scheduler, important tasks ranked by priority, review candidate
	// details with checklists, suggested next actions, and a read-only safety
	// attestation.
	Handoff bool

	// MergeReport, when true, activates merge-readiness report mode.
	// MergeReport mode ignores --section, --task, and --state filters;
	// --limit and --output still apply.  Tasks are grouped into six canonical
	// merge groups: ready-for-merge, reviewing, needs-run, blocked, merged,
	// and other.
	MergeReport bool

	// BlockedReport, when true, activates blocked-task report mode.
	// BlockedReport mode ignores --section, --task, and --state filters;
	// --limit and --output still apply.  Tasks are classified into four groups:
	// provider-gated, blocked, reserved-or-queue-gated, and unknown.
	BlockedReport bool
}

// effectiveLimit returns the active task limit, always >= 1.
func (o RenderOptions) effectiveLimit() int {
	if o.Limit <= 0 {
		return DefaultLimit
	}
	return o.Limit
}

// effectiveSection returns the normalised section name, always one of the
// Section* constants.
func (o RenderOptions) effectiveSection() string {
	switch o.Section {
	case SectionProviders, SectionTasks, SectionQueue, SectionActions:
		return o.Section
	default:
		return SectionAll
	}
}

// Snapshot is the parsed CMUX runtime view assembled from both contracts.
// HasRuntimeState and HasSchedulerPlan signal successful parses.
// Error fields capture failures without preventing partial rendering.
type Snapshot struct {
	RuntimeState    contracts.RuntimeState
	HasRuntimeState bool
	RuntimeStateErr error

	SchedulerPlan    runtimescheduler.Plan
	HasSchedulerPlan bool
	SchedulerPlanErr error
}

const MergeReadinessSchema = "brevity.cmux-merge-readiness-report.v1"

const (
	GroupReadyForReview  = "ready-for-review"
	GroupLikelyMergeable = "likely-mergeable"
	GroupNeedsInspection = "needs-inspection"
	GroupNotReady        = "not-ready"
	InspectNext          = "brevity task status / git diff / task merge dry-run"
	SafetyNote           = "This report is read-only guidance. It does not merge, approve, mutate task state, or replace human review."
)

type MergeReadinessReport struct {
	Schema          string               `json:"schema"`
	ReadyForReview  []MergeReadinessItem `json:"ready-for-review"`
	LikelyMergeable []MergeReadinessItem `json:"likely-mergeable"`
	NeedsInspection []MergeReadinessItem `json:"needs-inspection"`
	NotReady        []MergeReadinessItem `json:"not-ready"`
	SafetyNote      string               `json:"safetyNote"`
}

type MergeReadinessItem struct {
	Task        string `json:"task"`
	Group       string `json:"group"`
	Reason      string `json:"reason"`
	InspectNext string `json:"inspectNext"`
	Source      string `json:"source,omitempty"`
}

func BuildMergeReadinessReport(state contracts.RuntimeState) MergeReadinessReport {
	report := MergeReadinessReport{
		Schema:          MergeReadinessSchema,
		ReadyForReview:  []MergeReadinessItem{},
		LikelyMergeable: []MergeReadinessItem{},
		NeedsInspection: []MergeReadinessItem{},
		NotReady:        []MergeReadinessItem{},
		SafetyNote:      SafetyNote,
	}
	tasks := append([]contracts.TaskSummary{}, state.Tasks...)
	sort.SliceStable(tasks, func(i, j int) bool {
		return tasks[i].Slug < tasks[j].Slug
	})
	for _, task := range tasks {
		item := classifyTask(task)
		switch item.Group {
		case GroupReadyForReview:
			report.ReadyForReview = append(report.ReadyForReview, item)
		case GroupLikelyMergeable:
			report.LikelyMergeable = append(report.LikelyMergeable, item)
		case GroupNeedsInspection:
			report.NeedsInspection = append(report.NeedsInspection, item)
		default:
			report.NotReady = append(report.NotReady, item)
		}
	}
	return report
}

func classifyTask(task contracts.TaskSummary) MergeReadinessItem {
	state := normalizedState(task)
	item := MergeReadinessItem{
		Task:        firstNonEmpty(task.Slug, "(unknown-task)"),
		Group:       GroupNeedsInspection,
		Reason:      "reason unavailable from current contract",
		InspectNext: InspectNext,
		Source:      "task",
	}
	if isNotReadyState(state) || task.ProviderGated {
		item.Group = GroupNotReady
		item.Reason = notReadyReason(task, state)
		return item
	}
	if state == "merged" {
		item.Group = GroupNotReady
		item.Reason = "task already marked merged"
		return item
	}
	if state == "review" || state == "needs-review" {
		item.Group = GroupReadyForReview
		item.Reason = successfulSignalReason(task, "task state indicates review")
		item.Source = successfulSignalSource(task, "task")
		return item
	}
	if latestRunCompleted(task) {
		item.Group = GroupReadyForReview
		item.Reason = "latest run completed"
		item.Source = "latestRun"
		if state == "" || state == "unknown" {
			item.Group = GroupNeedsInspection
			item.Reason = "latest run completed but task state unclear"
		}
		return item
	}
	if latestExecutionCompleted(task) {
		item.Group = GroupReadyForReview
		item.Reason = "latest execution completed"
		item.Source = "execution"
		if state == "" || state == "unknown" {
			item.Group = GroupNeedsInspection
			item.Reason = "execution completed but task state unclear"
		}
		return item
	}
	if task.RunCount > 0 || strings.TrimSpace(task.LatestRunID) != "" || strings.TrimSpace(task.LastRunID) != "" {
		item.Group = GroupNeedsInspection
		item.Reason = "run history exists but latest task state not clearly reviewable"
		item.Source = "runHistory"
	}
	return item
}

func normalizedState(task contracts.TaskSummary) string {
	return strings.ToLower(strings.TrimSpace(firstNonEmpty(task.NormalizedState, task.Status)))
}

func isNotReadyState(state string) bool {
	switch state {
	case "blocked", "provider-gated", "failed", "reserved", "queued", "launching", "running", "starting", "planned", "ready", "ready-for-worker", "runnable", "stale":
		return true
	default:
		return false
	}
}

func notReadyReason(task contracts.TaskSummary, state string) string {
	if task.ProviderGated || state == "provider-gated" {
		return "provider gated"
	}
	if task.LatestRunIncomplete {
		return "latest run incomplete"
	}
	if isActiveState(state) {
		return "queue item still active"
	}
	if state == "" {
		return "reason unavailable from current contract"
	}
	return state
}

func isActiveState(state string) bool {
	switch state {
	case "queued", "reserved", "ready", "ready-for-worker", "runnable", "planned", "launching", "running", "starting":
		return true
	default:
		return false
	}
}

func latestRunCompleted(task contracts.TaskSummary) bool {
	status := strings.ToLower(strings.TrimSpace(task.LatestRunWorkerStatus))
	return !task.LatestRunIncomplete && strings.TrimSpace(task.LatestRunID) != "" && (status == "succeeded" || status == "completed") && exitCodeOK(task.LatestRunExitCode)
}

func latestExecutionCompleted(task contracts.TaskSummary) bool {
	status := strings.ToLower(strings.TrimSpace(task.WorkerStatus))
	if task.Execution != nil && strings.TrimSpace(task.Execution.Status) != "" {
		status = strings.ToLower(strings.TrimSpace(task.Execution.Status))
	}
	return status == "succeeded" || status == "completed"
}

func exitCodeOK(value any) bool {
	if value == nil {
		return true
	}
	return strings.Trim(strings.ToLower(strings.TrimSpace(fmt.Sprint(value))), `"`) == "0"
}

func successfulSignalReason(task contracts.TaskSummary, fallback string) string {
	if latestRunCompleted(task) {
		return "latest run completed"
	}
	if latestExecutionCompleted(task) {
		return "latest execution completed"
	}
	return fallback
}

func successfulSignalSource(task contracts.TaskSummary, fallback string) string {
	if latestRunCompleted(task) {
		return "latestRun"
	}
	if latestExecutionCompleted(task) {
		return "execution"
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
