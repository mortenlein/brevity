package execution

import (
	"strings"
	"time"

	runtimequeue "github.com/mortenlein/brevity/internal/runtime/queue"
	runtimescheduler "github.com/mortenlein/brevity/internal/runtime/scheduler"
)

const FlowSchema = "brevity.execution-flow.v1"

type FlowPlanner struct {
	Queue     runtimequeue.Store
	Execution Store
}

type FlowPlan struct {
	Schema            string                   `json:"schema"`
	ReadOnly          bool                     `json:"readOnly"`
	QueuePath         string                   `json:"queuePath"`
	ExecutionsPath    string                   `json:"executionsPath"`
	QueueState        string                   `json:"queueState"`
	ExecutionState    string                   `json:"executionState"`
	QueueSummary      runtimequeue.PlanSummary `json:"queueSummary"`
	ReservedItemCount int                      `json:"reservedItemCount"`
	NextState         string                   `json:"nextState"`
	Message           string                   `json:"message"`
	Commands          []string                 `json:"commands"`
	QueueItem         *runtimequeue.PlanItem   `json:"queueItem,omitempty"`
	Execution         *Record                  `json:"execution,omitempty"`
	Completed         *Record                  `json:"completed,omitempty"`
	Errors            []string                 `json:"errors,omitempty"`
}

func (planner FlowPlanner) Plan() FlowPlan {
	queueStore := planner.Queue
	if queueStore.Store.RepoRoot == "" {
		queueStore = planner.Execution.Queue
	}
	executionStore := planner.Execution
	if executionStore.Store.RepoRoot == "" {
		executionStore.Store = queueStore.Store
		executionStore.Queue = queueStore
	}

	queuePlan := queueStore.Plan()
	schedulerPlan := runtimescheduler.Planner{Queue: queueStore}.Plan()
	executions, missing, err := executionStore.Load()
	result := FlowPlan{
		Schema:            FlowSchema,
		ReadOnly:          true,
		QueuePath:         queuePlan.Path,
		ExecutionsPath:    executionStore.Path(),
		QueueState:        queuePlan.State,
		ExecutionState:    "valid",
		QueueSummary:      queuePlan.Summary,
		ReservedItemCount: schedulerPlan.ReservedItemCount,
	}
	if queuePlan.Error != "" {
		result.Errors = append(result.Errors, queuePlan.Error)
	}
	if err != nil {
		result.ExecutionState = "invalid"
		result.Errors = append(result.Errors, err.Error())
		return withGuidance(result, "inspect-state", "runtime state needs manual inspection", "brevity queue inspect", "brevity execution inspect")
	}
	if missing {
		result.ExecutionState = "missing"
	}

	if record := newestByStatus(executions.Records, StatusLaunching); record != nil {
		result.Execution = record
		return withGuidance(result, StatusLaunching, "execution currently launching", "brevity execution inspect")
	}
	if record := newestByStatus(executions.Records, StatusFailed); record != nil {
		result.Execution = record
		return withGuidance(result, StatusFailed, "failed execution needs inspection before any manual retry", "brevity execution inspect", "brevity queue inspect")
	}
	if record := newestByStatus(executions.Records, StatusReady); record != nil {
		result.Execution = record
		return withGuidance(result, StatusReady, "ready execution can be checked and launched manually",
			"brevity execution preflight "+record.ID,
			"brevity execution launch-dry-run "+record.ID,
			"brevity execution launch "+record.ID,
		)
	}
	if record := newestByStatus(executions.Records, StatusPlanned); record != nil {
		result.Execution = record
		return withGuidance(result, StatusPlanned, "planned execution should be marked ready", "brevity execution mark-ready "+record.ID)
	}
	if schedulerPlan.FirstReserved != nil {
		reserved := *schedulerPlan.FirstReserved
		result.QueueItem = &reserved
		return withGuidance(result, "reserved", "reserved queue item has no execution record", "brevity scheduler plan-execution")
	}
	if schedulerPlan.Selected != nil {
		item := runtimequeue.PlanItem{
			ID:       schedulerPlan.Selected.ID,
			Task:     schedulerPlan.Selected.Task,
			Provider: schedulerPlan.Selected.Provider,
			Profile:  schedulerPlan.Selected.Profile,
			Status:   schedulerPlan.Selected.Status,
			Reason:   schedulerPlan.Selected.Reason,
		}
		result.QueueItem = &item
		return withGuidance(result, "queued", "queued work is runnable and unreserved", "brevity scheduler reserve-next")
	}
	if record := newestByStatus(executions.Records, StatusCompleted); record != nil {
		result.Completed = record
		return withGuidance(result, StatusCompleted, "latest execution is completed; review queue state for remaining work", "brevity queue inspect")
	}
	return withGuidance(result, "empty", "queue has no runnable work", "brevity queue add <task>")
}

func withGuidance(plan FlowPlan, state string, message string, commands ...string) FlowPlan {
	plan.NextState = state
	plan.Message = message
	plan.Commands = commands
	return plan
}

func newestByStatus(records []Record, status string) *Record {
	var newest *Record
	var newestAt time.Time
	for index := range records {
		record := records[index]
		if strings.ToLower(strings.TrimSpace(record.Status)) != status {
			continue
		}
		updatedAt, err := parseTime(record.UpdatedAt)
		if err != nil {
			updatedAt, _ = parseTime(record.CreatedAt)
		}
		if newest == nil || newestAt.IsZero() || updatedAt.After(newestAt) {
			candidate := record
			newest = &candidate
			newestAt = updatedAt
		}
	}
	return newest
}
