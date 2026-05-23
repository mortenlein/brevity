package scheduler

import (
	"strings"

	runtimequeue "github.com/mortenlein/brevity/internal/runtime/queue"
)

const PlanSchema = "brevity.runtime-scheduler-plan.v1"

type Planner struct {
	Queue runtimequeue.Store
}

type Plan struct {
	Schema                 string                  `json:"schema"`
	QueuePath              string                  `json:"queuePath"`
	QueueState             string                  `json:"queueState"`
	QueueVersion           int                     `json:"queueVersion"`
	SupportedQueueVersion  int                     `json:"supportedQueueVersion"`
	Selected               *Selection              `json:"selected,omitempty"`
	Skipped                []runtimequeue.PlanItem `json:"skipped"`
	NoSelectionReason      string                  `json:"noSelectionReason,omitempty"`
	ReservedItemCount      int                     `json:"reservedItemCount"`
	FirstReserved          *runtimequeue.PlanItem  `json:"firstReserved,omitempty"`
	AllQueuedWorkReserved  bool                    `json:"allQueuedWorkReserved"`
	ReservationEligible    bool                    `json:"reservationEligible"`
	ReservationEligibility string                  `json:"reservationEligibility"`
	SafetyChecks           []SafetyCheck           `json:"safetyChecks"`
	ReadOnly               bool                    `json:"readOnly"`
	Error                  string                  `json:"error,omitempty"`
}

type Selection struct {
	ID       string `json:"id"`
	Task     string `json:"task"`
	Provider string `json:"provider,omitempty"`
	Profile  string `json:"profile,omitempty"`
	Status   string `json:"status"`
	Reason   string `json:"reason"`
}

type SafetyCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Reason string `json:"reason"`
}

func (planner Planner) Plan() Plan {
	queuePlan := planner.Queue.Plan()
	plan := Plan{
		Schema:                PlanSchema,
		QueuePath:             queuePlan.Path,
		QueueState:            queuePlan.State,
		QueueVersion:          queuePlan.Version,
		SupportedQueueVersion: queuePlan.SupportedVersion,
		Skipped:               queuePlan.Skipped,
		ReservedItemCount:     queuePlan.Summary.Reserved,
		FirstReserved:         firstReservedItem(queuePlan.Skipped),
		ReadOnly:              true,
		Error:                 queuePlan.Error,
	}

	if queuePlan.State != "valid" && queuePlan.State != "missing" {
		plan.NoSelectionReason = fallbackReason(queuePlan.Error, "queue is not valid")
		plan.ReservationEligibility = "not eligible: no selected queue item"
		plan.SafetyChecks = safetyChecks(false, plan.NoSelectionReason)
		return plan
	}

	if len(queuePlan.Runnable) == 0 {
		plan.NoSelectionReason = "no eligible runnable queue item"
		if queuePlan.Summary.Reserved > 0 && queuePlan.Summary.Skipped == queuePlan.Summary.Reserved {
			plan.NoSelectionReason = "all queued work is already reserved"
			plan.AllQueuedWorkReserved = true
		}
		plan.ReservationEligibility = "not eligible: no selected queue item"
		plan.SafetyChecks = safetyChecks(false, plan.NoSelectionReason)
		return plan
	}

	item := queuePlan.Runnable[0]
	plan.Selected = &Selection{
		ID:       item.ID,
		Task:     item.Task,
		Provider: item.Provider,
		Profile:  item.Profile,
		Status:   item.Status,
		Reason:   "first eligible runnable queue item in queue order",
	}
	plan.ReservationEligible = true
	plan.ReservationEligibility = "eligible: selected item is queued, runnable, unreserved, and has a valid task slug"
	plan.SafetyChecks = safetyChecks(true, "selected item remains orchestration intent only")
	return plan
}

func firstReservedItem(items []runtimequeue.PlanItem) *runtimequeue.PlanItem {
	for _, item := range items {
		if strings.HasPrefix(item.Reason, "reserved by ") {
			reserved := item
			return &reserved
		}
	}
	return nil
}

func fallbackReason(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func safetyChecks(selectionAvailable bool, selectionReason string) []SafetyCheck {
	return []SafetyCheck{
		{
			Name:   "read-only planning",
			Passed: true,
			Reason: "scheduler plan does not mutate the queue or task state",
		},
		{
			Name:   "no provider execution",
			Passed: true,
			Reason: "scheduler plan does not launch providers or workers",
		},
		{
			Name:   "no supervisor lifecycle",
			Passed: true,
			Reason: "scheduler plan does not start or stop the runtime supervisor",
		},
		{
			Name:   "reservation gate",
			Passed: selectionAvailable,
			Reason: selectionReason,
		},
	}
}
