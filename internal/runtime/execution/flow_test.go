package execution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimequeue "github.com/mortenlein/brevity/internal/runtime/queue"
)

func TestFlowEmptyStateSuggestsQueueAdd(t *testing.T) {
	plan := testFlowPlan(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{}}, Executions{Version: Version, Records: []Record{}})
	assertFlowCommand(t, plan, "brevity queue add <task>")
}

func TestFlowQueuedItemSuggestsReserveNext(t *testing.T) {
	plan := testFlowPlan(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{queueItemFixture("queue-1", "some-task")}}, Executions{Version: Version, Records: []Record{}})
	assertFlowCommand(t, plan, "brevity scheduler reserve-next")
}

func TestFlowReservedItemSuggestsPlanExecution(t *testing.T) {
	plan := testFlowPlan(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{reservedQueueItemFixture("queue-1", "some-task", "res-1")}}, Executions{Version: Version, Records: []Record{}})
	assertFlowCommand(t, plan, "brevity scheduler plan-execution")
}

func TestFlowPlannedExecutionSuggestsMarkReady(t *testing.T) {
	plan := testFlowPlan(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{}}, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusPlanned)}})
	assertFlowCommand(t, plan, "brevity execution mark-ready exec-1")
}

func TestFlowReadyExecutionSuggestsPreflightDryRunAndLaunch(t *testing.T) {
	plan := testFlowPlan(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{}}, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusReady)}})
	assertFlowCommand(t, plan, "brevity execution preflight exec-1")
	assertFlowCommand(t, plan, "brevity execution launch-dry-run exec-1")
	assertFlowCommand(t, plan, "brevity execution launch exec-1")
}

func TestFlowFailedExecutionReportsFailedState(t *testing.T) {
	plan := testFlowPlan(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{}}, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusFailed)}})
	if plan.NextState != StatusFailed || !strings.Contains(plan.Message, "failed execution") {
		t.Fatalf("plan = %#v, want failed guidance", plan)
	}
	assertFlowCommand(t, plan, "brevity execution inspect")
}

func TestFlowCompletedExecutionReportsCompletedState(t *testing.T) {
	plan := testFlowPlan(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{}}, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusCompleted)}})
	if plan.NextState != StatusCompleted || plan.Completed == nil {
		t.Fatalf("plan = %#v, want completed guidance", plan)
	}
	assertFlowCommand(t, plan, "brevity queue inspect")
}

func TestFlowDoesNotMutateQueueOrExecutionFiles(t *testing.T) {
	store := testStore(t)
	writeQueue(t, store, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{reservedQueueItemFixture("queue-1", "some-task", "res-1")}})
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusReady)}})
	beforeQueue := readFile(t, store.Queue.QueuePath())
	beforeExecutions := readFile(t, store.Path())

	plan := FlowPlanner{Queue: store.Queue, Execution: store}.Plan()
	if plan.NextState != StatusReady {
		t.Fatalf("next state = %q, want ready", plan.NextState)
	}
	if after := readFile(t, store.Queue.QueuePath()); after != beforeQueue {
		t.Fatalf("queue mutated\nbefore: %s\nafter: %s", beforeQueue, after)
	}
	if after := readFile(t, store.Path()); after != beforeExecutions {
		t.Fatalf("executions mutated\nbefore: %s\nafter: %s", beforeExecutions, after)
	}
	if matches, err := filepath.Glob(filepath.Join(store.Store.BrevityRoot(), "*.lock")); err != nil || len(matches) != 0 {
		t.Fatalf("lock files = %#v err=%v, want none", matches, err)
	}
}

func testFlowPlan(t *testing.T, queue runtimequeue.Queue, executions Executions) FlowPlan {
	t.Helper()
	store := testStore(t)
	writeQueue(t, store, queue)
	writeExecutions(t, store, executions)
	return FlowPlanner{Queue: store.Queue, Execution: store}.Plan()
}

func assertFlowCommand(t *testing.T, plan FlowPlan, command string) {
	t.Helper()
	for _, candidate := range plan.Commands {
		if candidate == command {
			return
		}
	}
	t.Fatalf("commands = %#v, missing %q", plan.Commands, command)
}

func TestFlowMissingExecutionFileRemainsReadOnly(t *testing.T) {
	store := testStore(t)
	writeQueue(t, store, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{queueItemFixture("queue-1", "some-task")}})
	plan := FlowPlanner{Queue: store.Queue, Execution: store}.Plan()
	if plan.ExecutionState != "missing" {
		t.Fatalf("execution state = %q, want missing", plan.ExecutionState)
	}
	if _, err := os.Stat(store.Path()); !os.IsNotExist(err) {
		t.Fatalf("executions file created by flow: %v", err)
	}
}
