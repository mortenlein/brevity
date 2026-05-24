package execution

import (
	"strings"
	"testing"

	runtimequeue "github.com/mortenlein/brevity/internal/runtime/queue"
)

func TestPreflightPassesForReadyExecutionWithMatchingReservedQueueItem(t *testing.T) {
	store := testStore(t)
	writeMatchingPreflightFixture(t, store, StatusReady)

	result, err := store.Preflight("exec-1")
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("passed = false, reason = %q", result.Reason)
	}
	if result.ExecutionID != "exec-1" || result.Task != "some-task" || result.Status != StatusReady {
		t.Fatalf("result = %#v", result)
	}
	assertPreflightCheck(t, result, CheckLaunchEligible, true)
}

func TestPreflightFailsForMissingExecution(t *testing.T) {
	store := testStore(t)
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusReady)}})

	result, err := store.Preflight("exec-missing")
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	assertPreflightFailed(t, result, CheckExecutionExists, "execution not found")
}

func TestPreflightFailsForPlannedExecution(t *testing.T) {
	store := testStore(t)
	writeMatchingPreflightFixture(t, store, StatusPlanned)

	result, err := store.Preflight("exec-1")
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	assertPreflightFailed(t, result, CheckStatusReady, "status is planned, want ready")
}

func TestPreflightFailsForCancelledExecution(t *testing.T) {
	store := testStore(t)
	writeMatchingPreflightFixture(t, store, StatusCancelled)

	result, err := store.Preflight("exec-1")
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	assertPreflightFailed(t, result, CheckStatusReady, "status is cancelled, want ready")
}

func TestPreflightFailsForMissingQueueItem(t *testing.T) {
	store := testStore(t)
	writeQueue(t, store, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{}})
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusReady)}})

	result, err := store.Preflight("exec-1")
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	assertPreflightFailed(t, result, CheckQueueItemExists, "queue item not found")
}

func TestPreflightFailsForUnreservedQueueItem(t *testing.T) {
	store := testStore(t)
	writeQueue(t, store, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{queueItemFixture("queue-abc123", "some-task")}})
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusReady)}})

	result, err := store.Preflight("exec-1")
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	assertPreflightFailed(t, result, CheckQueueItemHasReserve, "queue item is not reserved")
}

func TestPreflightFailsForReservationMismatch(t *testing.T) {
	store := testStore(t)
	writeQueue(t, store, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{reservedQueueItemFixture("queue-abc123", "some-task", "res-other")}})
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusReady)}})

	result, err := store.Preflight("exec-1")
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	assertPreflightFailed(t, result, CheckReservationMatches, "does not match execution reservation")
}

func TestPreflightFailsForTaskMismatch(t *testing.T) {
	store := testStore(t)
	writeQueue(t, store, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{reservedQueueItemFixture("queue-abc123", "other-task", "res-abc123")}})
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusReady)}})

	result, err := store.Preflight("exec-1")
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	assertPreflightFailed(t, result, CheckTaskMatches, "does not match execution task")
}

func TestPreflightIsReadOnly(t *testing.T) {
	store := testStore(t)
	writeMatchingPreflightFixture(t, store, StatusReady)
	beforeQueue := readFile(t, store.Queue.QueuePath())
	beforeExecutions := readFile(t, store.Path())

	result, err := store.Preflight("exec-1")
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("passed = false, reason = %q", result.Reason)
	}
	if afterQueue := readFile(t, store.Queue.QueuePath()); afterQueue != beforeQueue {
		t.Fatalf("queue mutated\nbefore: %s\nafter: %s", beforeQueue, afterQueue)
	}
	if afterExecutions := readFile(t, store.Path()); afterExecutions != beforeExecutions {
		t.Fatalf("executions mutated\nbefore: %s\nafter: %s", beforeExecutions, afterExecutions)
	}
}

func writeMatchingPreflightFixture(t *testing.T, store Store, status string) {
	t.Helper()
	writeQueue(t, store, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{reservedQueueItemFixture("queue-abc123", "some-task", "res-abc123")}})
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", status)}})
}

func assertPreflightFailed(t *testing.T, result PreflightResult, checkName string, reasonContains string) {
	t.Helper()
	if result.Passed {
		t.Fatalf("passed = true, want false")
	}
	assertPreflightCheck(t, result, checkName, false)
	if !strings.Contains(result.Reason, reasonContains) {
		t.Fatalf("reason = %q, want containing %q", result.Reason, reasonContains)
	}
}

func assertPreflightCheck(t *testing.T, result PreflightResult, checkName string, passed bool) {
	t.Helper()
	for _, check := range result.Checks {
		if check.Name == checkName {
			if check.Passed != passed {
				t.Fatalf("check %q passed = %v, want %v", checkName, check.Passed, passed)
			}
			return
		}
	}
	t.Fatalf("missing check %q in %#v", checkName, result.Checks)
}
