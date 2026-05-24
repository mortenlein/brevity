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
