package execution

import (
	"fmt"
	"strings"

	runtimequeue "github.com/mortenlein/brevity/internal/runtime/queue"
)

const (
	CheckExecutionExists     = "execution exists"
	CheckStatusReady         = "status ready"
	CheckQueueItemExists     = "queue item exists"
	CheckQueueItemHasReserve = "queue item has reservation"
	CheckReservationMatches  = "reservation matches"
	CheckTaskMatches         = "task matches"
	CheckLaunchEligible      = "execution status is launch-eligible"
)

type PreflightCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Reason string `json:"reason,omitempty"`
}

type PreflightResult struct {
	ExecutionID string           `json:"executionId"`
	QueueItemID string           `json:"queueItemId,omitempty"`
	Task        string           `json:"task,omitempty"`
	Status      string           `json:"status,omitempty"`
	Passed      bool             `json:"passed"`
	Checks      []PreflightCheck `json:"checks"`
	Reason      string           `json:"reason,omitempty"`
}

func (store Store) Preflight(executionID string) (PreflightResult, error) {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return PreflightResult{}, fmt.Errorf("execution id is required")
	}
	result := PreflightResult{ExecutionID: executionID}

	executions, _, err := store.Load()
	if err != nil {
		return result, err
	}
	var record Record
	foundExecution := false
	for _, candidate := range executions.Records {
		if candidate.ID == executionID {
			record = candidate
			foundExecution = true
			break
		}
	}
	if !foundExecution {
		result.addCheck(CheckExecutionExists, false, fmt.Sprintf("execution not found: %s", executionID))
		result.finish()
		return result, nil
	}
	result.Task = record.Task
	result.QueueItemID = record.QueueItemID
	result.Status = strings.ToLower(strings.TrimSpace(record.Status))
	result.addCheck(CheckExecutionExists, true, "")

	if result.Status != StatusReady {
		result.addCheck(CheckStatusReady, false, fmt.Sprintf("execution %s status is %s, want ready", executionID, fallbackStatus(result.Status)))
		result.finish()
		return result, nil
	}
	result.addCheck(CheckStatusReady, true, "")

	queue, _, err := store.Queue.Load()
	if err != nil {
		return result, err
	}
	item, foundQueueItem := findQueueItem(queue, record.QueueItemID)
	if !foundQueueItem {
		result.addCheck(CheckQueueItemExists, false, fmt.Sprintf("queue item not found: %s", record.QueueItemID))
		result.finish()
		return result, nil
	}
	result.addCheck(CheckQueueItemExists, true, "")

	if item.Reservation == nil {
		result.addCheck(CheckQueueItemHasReserve, false, fmt.Sprintf("queue item is not reserved: %s", item.ID))
		result.finish()
		return result, nil
	}
	result.addCheck(CheckQueueItemHasReserve, true, "")

	reservationID := strings.TrimSpace(item.Reservation.ReservationID)
	if reservationID != strings.TrimSpace(record.ReservationID) {
		result.addCheck(CheckReservationMatches, false, fmt.Sprintf("queue reservation %s does not match execution reservation %s", fallbackValue(reservationID), record.ReservationID))
		result.finish()
		return result, nil
	}
	result.addCheck(CheckReservationMatches, true, "")

	if item.Task != record.Task {
		result.addCheck(CheckTaskMatches, false, fmt.Sprintf("queue task %s does not match execution task %s", item.Task, record.Task))
		result.finish()
		return result, nil
	}
	result.addCheck(CheckTaskMatches, true, "")
	result.addCheck(CheckLaunchEligible, true, "")
	result.finish()
	return result, nil
}

func (result *PreflightResult) addCheck(name string, passed bool, reason string) {
	result.Checks = append(result.Checks, PreflightCheck{Name: name, Passed: passed, Reason: reason})
}

func (result *PreflightResult) finish() {
	result.Passed = true
	for _, check := range result.Checks {
		if !check.Passed {
			result.Passed = false
			result.Reason = check.Reason
			return
		}
	}
}

func findQueueItem(queue runtimequeue.Queue, id string) (runtimequeue.Item, bool) {
	for _, item := range queue.Items {
		if item.ID == id {
			return item, true
		}
	}
	return runtimequeue.Item{}, false
}

func fallbackValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(missing)"
	}
	return value
}
