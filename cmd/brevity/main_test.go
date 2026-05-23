package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mortenlein/brevity/internal/commands"
	"github.com/mortenlein/brevity/internal/dashboard"
	runtimeexecution "github.com/mortenlein/brevity/internal/runtime/execution"
	runtimequeue "github.com/mortenlein/brevity/internal/runtime/queue"
	"github.com/mortenlein/brevity/internal/state"
)

type fakeRuntimeClient struct {
	output            []byte
	err               error
	providerSet       []byte
	providerReset     []byte
	doctor            []byte
	contextRefresh    []byte
	taskCleanup       []byte
	taskNew           []byte
	taskRun           []byte
	runtimeInfo       []byte
	taskRuns          []byte
	runsReconcile     []byte
	runsRetention     []byte
	runsCompact       []byte
	cleanupInspect    []byte
	actionErr         error
	calls             []string
	afterRuntimeState func(int)
}

func (client *fakeRuntimeClient) RuntimeStateJSON() ([]byte, error) {
	client.calls = append(client.calls, "runtime-state")
	if client.afterRuntimeState != nil {
		client.afterRuntimeState(len(client.calls))
	}
	return client.output, client.err
}

func (client *fakeRuntimeClient) ProviderSetJSON(provider string, status string) ([]byte, error) {
	client.calls = append(client.calls, "provider-set:"+provider+":"+status)
	return client.providerSet, client.actionErr
}

func (client *fakeRuntimeClient) ProviderResetJSON(provider string) ([]byte, error) {
	client.calls = append(client.calls, "provider-reset:"+provider)
	return client.providerReset, client.actionErr
}

func (client *fakeRuntimeClient) DoctorJSON() ([]byte, error) {
	client.calls = append(client.calls, "doctor")
	return client.doctor, client.actionErr
}

func (client *fakeRuntimeClient) TaskContextRefreshJSON(slug string) ([]byte, error) {
	client.calls = append(client.calls, "context-refresh:"+slug)
	return client.contextRefresh, client.actionErr
}

func (client *fakeRuntimeClient) TaskCleanupJSON(slug string) ([]byte, error) {
	client.calls = append(client.calls, "task-cleanup:"+slug)
	return client.taskCleanup, client.actionErr
}

func (client *fakeRuntimeClient) TaskNewJSON(slug string) ([]byte, error) {
	client.calls = append(client.calls, "task-new:"+slug)
	return client.taskNew, client.actionErr
}

func (client *fakeRuntimeClient) TaskRunJSON(slug string, profile string, smoke bool) ([]byte, error) {
	client.calls = append(client.calls, "task-run:"+slug+":"+profile+":"+boolString(smoke))
	return client.taskRun, client.actionErr
}

func (client *fakeRuntimeClient) TaskRunPlanJSON(slug string, profile string) ([]byte, error) {
	client.calls = append(client.calls, "task-run-plan:"+slug+":"+profile)
	return client.taskRun, client.actionErr
}

func (client *fakeRuntimeClient) TaskRuntimeInfoJSON(slug string) ([]byte, error) {
	client.calls = append(client.calls, "runtime-info:"+slug)
	return client.runtimeInfo, client.actionErr
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func (client *fakeRuntimeClient) TaskRunsJSON(slug string) ([]byte, error) {
	client.calls = append(client.calls, "task-runs:"+slug)
	return client.taskRuns, client.actionErr
}

func (client *fakeRuntimeClient) TaskRunsReconcileJSON() ([]byte, error) {
	client.calls = append(client.calls, "task-runs-reconcile")
	return client.runsReconcile, client.actionErr
}

func (client *fakeRuntimeClient) TaskRunsRetentionJSON() ([]byte, error) {
	client.calls = append(client.calls, "task-runs-retention")
	return client.runsRetention, client.actionErr
}

func (client *fakeRuntimeClient) TaskRunsCompactJSON() ([]byte, error) {
	client.calls = append(client.calls, "task-runs-compact")
	return client.runsCompact, client.actionErr
}

func (client *fakeRuntimeClient) CleanupInspectJSON() ([]byte, error) {
	client.calls = append(client.calls, "cleanup-inspect")
	return client.cleanupInspect, client.actionErr
}

func TestRunWithClientRendersRuntimeState(t *testing.T) {
	client := &fakeRuntimeClient{
		output: []byte(`{"schema":"brevity.runtime-state.v1","repoRoot":"C:\\dev\\repos\\active\\brevity","taskCounts":{"tracked":7}}`),
	}

	var stdout bytes.Buffer
	if err := runWithClient(&stdout, client); err != nil {
		t.Fatalf("runWithClient returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Brevity Runtime Dashboard") {
		t.Fatalf("output missing dashboard title:\n%s", output)
	}
	if !strings.Contains(output, "tracked: 7") {
		t.Fatalf("output missing task count:\n%s", output)
	}
}

func TestTaskPreflightJSONOutput(t *testing.T) {
	root := taskPreflightFixture(t, "alpha", "planned", state.StatusHealthy)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := runWithOptions(&stdout, &fakeRuntimeClient{}, cliOptions{kind: commandTaskPreflight, preflightAction: "task-start", slug: "alpha", json: true}); err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, `"schema":"brevity.task-preflight.v1"`) || !strings.Contains(output, `"status":"allowed"`) {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestTaskPreflightHumanBlockedOutput(t *testing.T) {
	root := taskPreflightFixture(t, "alpha", "planned", state.StatusHealthy)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := runWithOptions(&stdout, &fakeRuntimeClient{}, cliOptions{kind: commandTaskPreflight, preflightAction: "task-start", slug: "missing"}); err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "status: blocked") || !strings.Contains(output, "task does not exist") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestSchedulerReserveNextReservesFirstSelectableItem(t *testing.T) {
	repoRoot := tempRepoWithQueue(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		mainTestQueueItem("first", "alpha", runtimequeue.StatusQueued),
		mainTestQueueItem("second", "beta", runtimequeue.StatusQueued),
	}})
	t.Chdir(repoRoot)

	client := &fakeRuntimeClient{}
	var stdout bytes.Buffer
	if err := runWithOptions(&stdout, client, cliOptions{kind: commandSchedulerReserveNext}); err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}
	output := stdout.String()
	for _, want := range []string{"Reserved scheduler queue item", "id: first", "task: alpha", "reservationId:", "no provider, worker, supervisor, task state, run history, or queue drain"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	queue := readMainTestQueue(t, repoRoot)
	if queue.Items[0].Reservation == nil || queue.Items[1].Reservation != nil {
		t.Fatalf("reservations = %#v %#v, want first only", queue.Items[0].Reservation, queue.Items[1].Reservation)
	}
}

func TestSchedulerReserveNextSkipsReservedAndInvalidItems(t *testing.T) {
	reserved := mainTestQueueItem("reserved", "alpha", runtimequeue.StatusQueued)
	reserved.Reservation = &runtimequeue.Reservation{
		Owner:         "runtime-supervisor",
		ReservedAt:    "2026-05-22T12:00:00Z",
		ReservationID: "res-existing",
	}
	repoRoot := tempRepoWithQueue(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		reserved,
		mainTestQueueItem("invalid", "cancelled-task", runtimequeue.StatusCancelled),
		mainTestQueueItem("selected", "beta", runtimequeue.StatusQueued),
	}})
	t.Chdir(repoRoot)

	var stdout bytes.Buffer
	if err := runWithOptions(&stdout, &fakeRuntimeClient{}, cliOptions{kind: commandSchedulerReserveNext}); err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "id: selected") || !strings.Contains(stdout.String(), "task: beta") {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
	queue := readMainTestQueue(t, repoRoot)
	if queue.Items[2].Reservation == nil {
		t.Fatalf("selected item was not reserved: %#v", queue.Items[2])
	}
	if queue.Items[1].Reservation != nil {
		t.Fatalf("scheduler-invalid item was reserved: %#v", queue.Items[1].Reservation)
	}
}

func TestSchedulerReserveNextFailsSafelyWhenNoSelectableItemExists(t *testing.T) {
	repoRoot := tempRepoWithQueue(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		mainTestQueueItem("cancelled", "alpha", runtimequeue.StatusCancelled),
	}})
	t.Chdir(repoRoot)
	before := readMainTestFile(t, filepath.Join(repoRoot, ".brevity", runtimequeue.FileName))

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, &fakeRuntimeClient{}, cliOptions{kind: commandSchedulerReserveNext})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "no selectable scheduler item") {
		t.Fatalf("unexpected error: %v", err)
	}
	after := readMainTestFile(t, filepath.Join(repoRoot, ".brevity", runtimequeue.FileName))
	if after != before {
		t.Fatalf("queue mutated on no selection\nbefore: %s\nafter: %s", before, after)
	}
}

func TestSchedulerReserveNextDoesNotExecuteOrMutateTaskState(t *testing.T) {
	repoRoot := tempRepoWithQueue(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		mainTestQueueItem("first", "alpha", runtimequeue.StatusQueued),
	}})
	tasksPath := filepath.Join(repoRoot, ".brevity", state.TasksFile)
	writeMainTestFile(t, tasksPath, `[{"slug":"alpha","status":"ready-for-worker","normalizedState":"ready-for-worker"}]`+"\n")
	t.Chdir(repoRoot)
	beforeTasks := readMainTestFile(t, tasksPath)

	client := &fakeRuntimeClient{}
	var stdout bytes.Buffer
	if err := runWithOptions(&stdout, client, cliOptions{kind: commandSchedulerReserveNext}); err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}
	afterTasks := readMainTestFile(t, tasksPath)
	if afterTasks != beforeTasks {
		t.Fatalf("tasks mutated\nbefore: %s\nafter: %s", beforeTasks, afterTasks)
	}
	if strings.Contains(readMainTestFile(t, filepath.Join(repoRoot, ".brevity", runtimequeue.FileName)), "running") {
		t.Fatal("queue contains running status after reserve-next")
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}
}

func TestSchedulerPlanExecutionCreatesPlannedExecutionFromReservedSelectedItem(t *testing.T) {
	reserved := mainTestReservedQueueItem("reserved", "alpha", "res-alpha")
	repoRoot := tempRepoWithQueue(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{reserved}})
	t.Chdir(repoRoot)

	client := &fakeRuntimeClient{}
	var stdout bytes.Buffer
	if err := runWithOptions(&stdout, client, cliOptions{kind: commandSchedulerPlanExec}); err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}
	output := stdout.String()
	for _, want := range []string{"Planned scheduler execution", "id: reserved", "task: alpha", "reservationId: res-alpha", "executionId:", "no provider, worker, supervisor, task state, run history, or queue drain"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	executions := readMainTestExecutions(t, repoRoot)
	if len(executions.Records) != 1 {
		t.Fatalf("records = %d, want 1", len(executions.Records))
	}
	record := executions.Records[0]
	if record.QueueItemID != "reserved" || record.Task != "alpha" || record.ReservationID != "res-alpha" || record.Status != runtimeexecution.StatusPlanned {
		t.Fatalf("record = %#v", record)
	}
}

func TestSchedulerPlanExecutionFailsSafelyWithNoSelectedItem(t *testing.T) {
	repoRoot := tempRepoWithQueue(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{}})
	t.Chdir(repoRoot)

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, &fakeRuntimeClient{}, cliOptions{kind: commandSchedulerPlanExec})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "no selectable scheduler item") || !strings.Contains(err.Error(), "no eligible runnable queue item") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(repoRoot, ".brevity", runtimeexecution.FileName)); !os.IsNotExist(statErr) {
		t.Fatalf("executions file exists after rejected plan: %v", statErr)
	}
}

func TestSchedulerPlanExecutionRejectsUnreservedSelectedItem(t *testing.T) {
	repoRoot := tempRepoWithQueue(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		mainTestQueueItem("selected", "alpha", runtimequeue.StatusQueued),
	}})
	t.Chdir(repoRoot)

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, &fakeRuntimeClient{}, cliOptions{kind: commandSchedulerPlanExec})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "selected queue item is not reserved") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(repoRoot, ".brevity", runtimeexecution.FileName)); !os.IsNotExist(statErr) {
		t.Fatalf("executions file exists after rejected plan: %v", statErr)
	}
}

func TestSchedulerPlanExecutionRejectsDuplicateExecutionPlanning(t *testing.T) {
	reserved := mainTestReservedQueueItem("reserved", "alpha", "res-alpha")
	repoRoot := tempRepoWithQueue(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{reserved}})
	t.Chdir(repoRoot)

	if err := runWithOptions(&bytes.Buffer{}, &fakeRuntimeClient{}, cliOptions{kind: commandSchedulerPlanExec}); err != nil {
		t.Fatalf("first runWithOptions returned error: %v", err)
	}
	err := runWithOptions(&bytes.Buffer{}, &fakeRuntimeClient{}, cliOptions{kind: commandSchedulerPlanExec})
	if err == nil {
		t.Fatal("second runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "execution already planned") {
		t.Fatalf("unexpected error: %v", err)
	}
	executions := readMainTestExecutions(t, repoRoot)
	if len(executions.Records) != 1 {
		t.Fatalf("records = %d, want 1", len(executions.Records))
	}
}

func TestSchedulerPlanExecutionDoesNotExecuteProvidersMutateTaskStateOrCreateRunHistory(t *testing.T) {
	reserved := mainTestReservedQueueItem("reserved", "alpha", "res-alpha")
	repoRoot := tempRepoWithQueue(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{reserved}})
	tasksPath := filepath.Join(repoRoot, ".brevity", state.TasksFile)
	runsPath := filepath.Join(repoRoot, ".brevity", "runs.jsonl")
	writeMainTestFile(t, tasksPath, `[{"slug":"alpha","status":"ready-for-worker","normalizedState":"ready-for-worker"}]`+"\n")
	t.Chdir(repoRoot)
	beforeTasks := readMainTestFile(t, tasksPath)

	client := &fakeRuntimeClient{}
	var stdout bytes.Buffer
	if err := runWithOptions(&stdout, client, cliOptions{kind: commandSchedulerPlanExec}); err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}
	if afterTasks := readMainTestFile(t, tasksPath); afterTasks != beforeTasks {
		t.Fatalf("tasks mutated\nbefore: %s\nafter: %s", beforeTasks, afterTasks)
	}
	if _, err := os.Stat(runsPath); !os.IsNotExist(err) {
		t.Fatalf("run history exists after plan-execution: %v", err)
	}
	if strings.Contains(readMainTestFile(t, filepath.Join(repoRoot, ".brevity", runtimequeue.FileName)), "running") {
		t.Fatal("queue contains running status after plan-execution")
	}
}

func TestExecutionMarkReadyTransitionsPlannedExecution(t *testing.T) {
	repoRoot := tempRepoWithQueue(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		mainTestReservedQueueItem("queue-1", "alpha", "res-alpha"),
	}})
	writeMainTestExecutions(t, repoRoot, runtimeexecution.Executions{Version: runtimeexecution.Version, Records: []runtimeexecution.Record{
		mainTestExecutionRecord("exec-1", "queue-1", "alpha", "res-alpha", runtimeexecution.StatusPlanned),
	}})
	t.Chdir(repoRoot)

	client := &fakeRuntimeClient{}
	var stdout bytes.Buffer
	if err := runWithOptions(&stdout, client, cliOptions{kind: commandExecutionMarkReady, candidateID: "exec-1"}); err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}
	output := stdout.String()
	for _, want := range []string{"Marked runtime execution ready", "id: exec-1", "task: alpha", "oldStatus: planned", "newStatus: ready", "no provider, worker, supervisor, task state, run history, or queue drain"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	executions := readMainTestExecutions(t, repoRoot)
	if executions.Records[0].Status != runtimeexecution.StatusReady {
		t.Fatalf("status = %q, want ready", executions.Records[0].Status)
	}
}

func TestExecutionMarkReadyRejectsMissingExecutionID(t *testing.T) {
	if _, err := parseOptions([]string{"execution", "mark-ready"}); err == nil {
		t.Fatal("parseOptions returned nil error")
	}
}

func TestExecutionMarkReadyRejectsCancelledExecution(t *testing.T) {
	repoRoot := tempRepoWithQueue(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{}})
	writeMainTestExecutions(t, repoRoot, runtimeexecution.Executions{Version: runtimeexecution.Version, Records: []runtimeexecution.Record{
		mainTestExecutionRecord("exec-1", "queue-1", "alpha", "res-alpha", runtimeexecution.StatusCancelled),
	}})
	t.Chdir(repoRoot)
	before := readMainTestFile(t, filepath.Join(repoRoot, ".brevity", runtimeexecution.FileName))

	err := runWithOptions(&bytes.Buffer{}, &fakeRuntimeClient{}, cliOptions{kind: commandExecutionMarkReady, candidateID: "exec-1"})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "status is cancelled, want planned") {
		t.Fatalf("unexpected error: %v", err)
	}
	if after := readMainTestFile(t, filepath.Join(repoRoot, ".brevity", runtimeexecution.FileName)); after != before {
		t.Fatalf("executions mutated\nbefore: %s\nafter: %s", before, after)
	}
}

func TestExecutionMarkReadyRejectsAlreadyReadyExecution(t *testing.T) {
	repoRoot := tempRepoWithQueue(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{}})
	writeMainTestExecutions(t, repoRoot, runtimeexecution.Executions{Version: runtimeexecution.Version, Records: []runtimeexecution.Record{
		mainTestExecutionRecord("exec-1", "queue-1", "alpha", "res-alpha", runtimeexecution.StatusReady),
	}})
	t.Chdir(repoRoot)
	err := runWithOptions(&bytes.Buffer{}, &fakeRuntimeClient{}, cliOptions{kind: commandExecutionMarkReady, candidateID: "exec-1"})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "status is ready, want planned") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecutionMarkReadyDoesNotMutateQueueState(t *testing.T) {
	repoRoot := tempRepoWithQueue(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		mainTestReservedQueueItem("queue-1", "alpha", "res-alpha"),
	}})
	writeMainTestExecutions(t, repoRoot, runtimeexecution.Executions{Version: runtimeexecution.Version, Records: []runtimeexecution.Record{
		mainTestExecutionRecord("exec-1", "queue-1", "alpha", "res-alpha", runtimeexecution.StatusPlanned),
	}})
	t.Chdir(repoRoot)
	queuePath := filepath.Join(repoRoot, ".brevity", runtimequeue.FileName)
	before := readMainTestFile(t, queuePath)
	if err := runWithOptions(&bytes.Buffer{}, &fakeRuntimeClient{}, cliOptions{kind: commandExecutionMarkReady, candidateID: "exec-1"}); err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}
	if after := readMainTestFile(t, queuePath); after != before {
		t.Fatalf("queue mutated\nbefore: %s\nafter: %s", before, after)
	}
}

func TestExecutionListAndInspectShowReadyStatus(t *testing.T) {
	repoRoot := tempRepoWithQueue(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{}})
	writeMainTestExecutions(t, repoRoot, runtimeexecution.Executions{Version: runtimeexecution.Version, Records: []runtimeexecution.Record{
		mainTestExecutionRecord("exec-1", "queue-1", "alpha", "res-alpha", runtimeexecution.StatusPlanned),
		mainTestExecutionRecord("exec-2", "queue-2", "beta", "res-beta", runtimeexecution.StatusReady),
	}})
	t.Chdir(repoRoot)

	var stdout bytes.Buffer
	if err := runWithOptions(&stdout, &fakeRuntimeClient{}, cliOptions{kind: commandExecutionList}); err != nil {
		t.Fatalf("execution list returned error: %v", err)
	}
	for _, want := range []string{"planned: 1", "ready: 1", "exec-2\tqueue-2\tbeta\tres-beta\tready"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("list output missing %q:\n%s", want, stdout.String())
		}
	}
	stdout.Reset()
	if err := runWithOptions(&stdout, &fakeRuntimeClient{}, cliOptions{kind: commandExecutionInspect}); err != nil {
		t.Fatalf("execution inspect returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "- ready: 1") {
		t.Fatalf("inspect output missing ready count:\n%s", stdout.String())
	}
}

func TestExecutionPreflightReportsReadyReservedExecution(t *testing.T) {
	repoRoot := tempRepoWithQueue(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		mainTestReservedQueueItem("queue-1", "alpha", "res-alpha"),
	}})
	writeMainTestExecutions(t, repoRoot, runtimeexecution.Executions{Version: runtimeexecution.Version, Records: []runtimeexecution.Record{
		mainTestExecutionRecord("exec-1", "queue-1", "alpha", "res-alpha", runtimeexecution.StatusReady),
	}})
	t.Chdir(repoRoot)
	queuePath := filepath.Join(repoRoot, ".brevity", runtimequeue.FileName)
	executionsPath := filepath.Join(repoRoot, ".brevity", runtimeexecution.FileName)
	beforeQueue := readMainTestFile(t, queuePath)
	beforeExecutions := readMainTestFile(t, executionsPath)

	var stdout bytes.Buffer
	if err := runWithOptions(&stdout, &fakeRuntimeClient{}, cliOptions{kind: commandExecutionPreflight, candidateID: "exec-1"}); err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{"EXECUTION PREFLIGHT", "Execution: exec-1", "Task: alpha", "Status: ready", "- execution exists: ok", "- reservation matches: ok", "- task matches: ok", "Result: passed", "no provider, worker, supervisor, task state, run history, or queue drain"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if afterQueue := readMainTestFile(t, queuePath); afterQueue != beforeQueue {
		t.Fatalf("queue mutated\nbefore: %s\nafter: %s", beforeQueue, afterQueue)
	}
	if afterExecutions := readMainTestFile(t, executionsPath); afterExecutions != beforeExecutions {
		t.Fatalf("executions mutated\nbefore: %s\nafter: %s", beforeExecutions, afterExecutions)
	}
}

func TestExecutionPreflightJSONFailsForPlannedExecution(t *testing.T) {
	repoRoot := tempRepoWithQueue(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		mainTestReservedQueueItem("queue-1", "alpha", "res-alpha"),
	}})
	writeMainTestExecutions(t, repoRoot, runtimeexecution.Executions{Version: runtimeexecution.Version, Records: []runtimeexecution.Record{
		mainTestExecutionRecord("exec-1", "queue-1", "alpha", "res-alpha", runtimeexecution.StatusPlanned),
	}})
	t.Chdir(repoRoot)

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, &fakeRuntimeClient{}, cliOptions{kind: commandExecutionPreflight, candidateID: "exec-1", json: true})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "execution preflight failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{`"executionId":"exec-1"`, `"task":"alpha"`, `"status":"planned"`, `"passed":false`, `"name":"status ready"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("json output missing %q:\n%s", want, output)
		}
	}
}

func TestExecutionMarkPlannedRollsReadyBackToPlanned(t *testing.T) {
	repoRoot := tempRepoWithQueue(t, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{}})
	writeMainTestExecutions(t, repoRoot, runtimeexecution.Executions{Version: runtimeexecution.Version, Records: []runtimeexecution.Record{
		mainTestExecutionRecord("exec-1", "queue-1", "alpha", "res-alpha", runtimeexecution.StatusReady),
	}})
	t.Chdir(repoRoot)
	var stdout bytes.Buffer
	if err := runWithOptions(&stdout, &fakeRuntimeClient{}, cliOptions{kind: commandExecutionMarkPlanned, candidateID: "exec-1"}); err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "oldStatus: ready") || !strings.Contains(stdout.String(), "newStatus: planned") {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
	if got := readMainTestExecutions(t, repoRoot).Records[0].Status; got != runtimeexecution.StatusPlanned {
		t.Fatalf("status = %q, want planned", got)
	}
}

func TestTaskStartJSONUsesNativeService(t *testing.T) {
	root := taskPreflightFixture(t, "alpha", "planned", state.StatusHealthy)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	client := &fakeRuntimeClient{}
	var stdout bytes.Buffer
	if err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskStart, slug: "alpha", json: true}); err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{`"schema":"brevity.command-result.v1"`, `"command":"task start"`, `"slug":"alpha"`, `"newState":"ready-for-worker"`, `"noExecution":true`} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if len(client.calls) != 0 {
		t.Fatalf("PowerShell/fake client was called: %v", client.calls)
	}
	data, err := os.ReadFile(filepath.Join(root, state.DirectoryName, state.TasksFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"normalizedState": "ready-for-worker"`) {
		t.Fatalf("tasks.json was not updated:\n%s", data)
	}
}

func taskPreflightFixture(t *testing.T, slug string, taskStatus string, providerStatus state.ProviderStatus) string {
	t.Helper()
	root := t.TempDir()
	worktree := filepath.Join(root, "worktrees", "active", slug)
	prompt := filepath.Join(worktree, "prompt.md")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, state.DirectoryName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prompt, []byte("prompt"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := state.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	tasks := []state.Task{{
		Slug:            slug,
		Status:          taskStatus,
		NormalizedState: taskStatus,
		Branch:          "task/" + slug,
		WorktreePath:    worktree,
		PromptPath:      prompt,
		Provider:        "codex",
		Profile:         "default",
	}}
	if err := store.WriteJSON(state.TasksFile, tasks); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteJSON(state.ProviderHealthFile, state.ProviderHealthState{"codex": {Status: providerStatus}}); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestParseOptionsDefaultsToPowerShellSource(t *testing.T) {
	options, err := parseOptions(nil)
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if options.kind != commandDashboard {
		t.Fatalf("kind = %q, want dashboard", options.kind)
	}
	if options.once {
		t.Fatal("once = true, want false")
	}
	if options.jsonSource != "powershell" {
		t.Fatalf("jsonSource = %q, want powershell", options.jsonSource)
	}
}

func TestParseOptionsAcceptsNativeJSONSource(t *testing.T) {
	options, err := parseOptions([]string{"--json-source", "native", "--once"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if options.jsonSource != "native" {
		t.Fatalf("jsonSource = %q, want native", options.jsonSource)
	}
	if !options.once {
		t.Fatal("once = false, want true")
	}
}

func TestParseOptionsAcceptsOnce(t *testing.T) {
	options, err := parseOptions([]string{"--once"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if !options.once {
		t.Fatal("once = false, want true")
	}
}

func TestParseOptionsAcceptsWatchRefresh(t *testing.T) {
	options, err := parseOptions([]string{"--watch", "--refresh", "2s"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if !options.watch {
		t.Fatal("watch = false, want true")
	}
	if options.refresh != 2*time.Second {
		t.Fatalf("refresh = %s, want 2s", options.refresh)
	}
}

func TestParseOptionsAcceptsBubble(t *testing.T) {
	options, err := parseOptions([]string{"--bubble", "--refresh", "2s"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if !options.bubble {
		t.Fatal("bubble = false, want true")
	}
	if options.refresh != 2*time.Second {
		t.Fatalf("refresh = %s, want 2s", options.refresh)
	}
}

func TestParseOptionsAcceptsNoClear(t *testing.T) {
	options, err := parseOptions([]string{"--watch", "--no-clear"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if !options.watch {
		t.Fatal("watch = false, want true")
	}
	if !options.noClear {
		t.Fatal("noClear = false, want true")
	}
}

func TestParseOptionsRejectsBubbleWithWatchOrOnce(t *testing.T) {
	for _, args := range [][]string{
		{"--bubble", "--watch"},
		{"--bubble", "--once"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := parseOptions(args)
			if err == nil {
				t.Fatal("parseOptions returned nil error")
			}
			if !strings.Contains(err.Error(), "--bubble") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseOptionsRejectsInvalidRefresh(t *testing.T) {
	_, err := parseOptions([]string{"--watch", "--refresh", "not-a-duration"})
	if err == nil {
		t.Fatal("parseOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "invalid --refresh value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseOptionsRejectsOnceWithWatch(t *testing.T) {
	_, err := parseOptions([]string{"--once", "--watch"})
	if err == nil {
		t.Fatal("parseOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "--once and --watch cannot be used together") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWatchDashboardRefreshesUntilContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &fakeRuntimeClient{
		output: []byte(`{"schema":"brevity.runtime-state.v1","repoRoot":"C:\\repo","generatedAt":"2026-05-19T10:00:00Z","taskCounts":{"tracked":3}}`),
	}
	client.afterRuntimeState = func(callCount int) {
		if callCount == 2 {
			client.output = []byte(`{"schema":"brevity.runtime-state.v1","repoRoot":"C:\\repo","generatedAt":"2026-05-19T10:00:02Z","taskCounts":{"tracked":3}}`)
			cancel()
		}
	}

	var stdout bytes.Buffer
	err := runWithContextOptions(ctx, &stdout, client, cliOptions{kind: commandDashboard, watch: true, refresh: time.Millisecond})
	if err != nil {
		t.Fatalf("runWithContextOptions returned error: %v", err)
	}
	if len(client.calls) < 2 {
		t.Fatalf("calls = %#v, want at least two runtime state refreshes", client.calls)
	}
	output := stdout.String()
	for _, want := range []string{"Brevity Runtime Dashboard", "Last successful refresh:", "Stopped."} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestWatchDashboardSkipsUnchangedRender(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &fakeRuntimeClient{
		output: []byte(`{"schema":"brevity.runtime-state.v1","repoRoot":"C:\\repo","taskCounts":{"tracked":3}}`),
		afterRuntimeState: func(callCount int) {
			if callCount >= 2 {
				cancel()
			}
		},
	}

	var stdout bytes.Buffer
	err := runWithContextOptions(ctx, &stdout, client, cliOptions{kind: commandDashboard, watch: true, refresh: time.Millisecond})
	if err != nil {
		t.Fatalf("runWithContextOptions returned error: %v", err)
	}

	output := stdout.String()
	if got := strings.Count(output, "Brevity Runtime Dashboard"); got != 1 {
		t.Fatalf("dashboard render count = %d, want 1\n%s", got, output)
	}
	if got := strings.Count(output, "\x1b[H\x1b[2J"); got != 1 {
		t.Fatalf("clear count = %d, want 1\n%s", got, output)
	}
}

func TestWatchDashboardNoClearSkipsClear(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &fakeRuntimeClient{
		output: []byte(`{"schema":"brevity.runtime-state.v1","repoRoot":"C:\\repo","taskCounts":{"tracked":3}}`),
		afterRuntimeState: func(callCount int) {
			cancel()
		},
	}

	var stdout bytes.Buffer
	err := runWithContextOptions(ctx, &stdout, client, cliOptions{kind: commandDashboard, watch: true, noClear: true, refresh: time.Millisecond})
	if err != nil {
		t.Fatalf("runWithContextOptions returned error: %v", err)
	}
	if strings.Contains(stdout.String(), "\x1b[H\x1b[2J") {
		t.Fatalf("output contains clear sequence:\n%s", stdout.String())
	}
}

func TestWatchDashboardRendersChangedState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &fakeRuntimeClient{
		output: []byte(`{"schema":"brevity.runtime-state.v1","repoRoot":"C:\\repo","taskCounts":{"tracked":3}}`),
	}
	client.afterRuntimeState = func(callCount int) {
		if callCount == 2 {
			client.output = []byte(`{"schema":"brevity.runtime-state.v1","repoRoot":"C:\\repo","taskCounts":{"tracked":4}}`)
			return
		}
		if callCount >= 3 {
			cancel()
		}
	}

	var stdout bytes.Buffer
	err := runWithContextOptions(ctx, &stdout, client, cliOptions{kind: commandDashboard, watch: true, refresh: time.Millisecond})
	if err != nil {
		t.Fatalf("runWithContextOptions returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{"tracked: 3", "tracked: 4"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if got := strings.Count(output, "Brevity Runtime Dashboard"); got != 2 {
		t.Fatalf("dashboard render count = %d, want 2\n%s", got, output)
	}
}

func TestWatchDashboardShowsPollingErrorWithoutFailing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &fakeRuntimeClient{
		err: assertErr("runtime unavailable"),
		afterRuntimeState: func(callCount int) {
			cancel()
		},
	}

	var stdout bytes.Buffer
	err := runWithContextOptions(ctx, &stdout, client, cliOptions{kind: commandDashboard, watch: true, refresh: time.Millisecond})
	if err != nil {
		t.Fatalf("runWithContextOptions returned error: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{"Polling error: runtime unavailable", "Last successful refresh: (none)", "Stopped."} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestApplyDashboardInputNavigatesTogglesAndQuits(t *testing.T) {
	model := dashboardModel()

	changed, refreshNow, quit := applyDashboardInput(&model, "j", 2)
	if !changed || refreshNow || quit || model.SelectedIndex != 1 {
		t.Fatalf("j result changed=%t refresh=%t quit=%t selected=%d", changed, refreshNow, quit, model.SelectedIndex)
	}

	changed, refreshNow, quit = applyDashboardInput(&model, "k", 2)
	if !changed || refreshNow || quit || model.SelectedIndex != 0 {
		t.Fatalf("k result changed=%t refresh=%t quit=%t selected=%d", changed, refreshNow, quit, model.SelectedIndex)
	}

	changed, refreshNow, quit = applyDashboardInput(&model, "d", 2)
	if !changed || refreshNow || quit || !model.ShowDetails {
		t.Fatalf("d result changed=%t refresh=%t quit=%t details=%t", changed, refreshNow, quit, model.ShowDetails)
	}

	changed, refreshNow, quit = applyDashboardInput(&model, "?", 2)
	if !changed || refreshNow || quit || !model.ShowHelp {
		t.Fatalf("? result changed=%t refresh=%t quit=%t help=%t", changed, refreshNow, quit, model.ShowHelp)
	}

	changed, refreshNow, quit = applyDashboardInput(&model, "r", 2)
	if changed || !refreshNow || quit {
		t.Fatalf("r result changed=%t refresh=%t quit=%t", changed, refreshNow, quit)
	}

	changed, refreshNow, quit = applyDashboardInput(&model, "q", 2)
	if changed || refreshNow || !quit {
		t.Fatalf("q result changed=%t refresh=%t quit=%t", changed, refreshNow, quit)
	}
}

func dashboardModel() dashboard.InteractiveModel {
	return dashboard.InteractiveModel{}
}

type assertErr string

func (err assertErr) Error() string {
	return string(err)
}

func TestParseOptionsAcceptsProviderSet(t *testing.T) {
	options, err := parseOptions([]string{"provider", "set", "gemini", "capacity-degraded", "--note", "busy"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if options.kind != commandProviderSet {
		t.Fatalf("kind = %q, want provider-set", options.kind)
	}
	if options.provider != "gemini" {
		t.Fatalf("provider = %q, want gemini", options.provider)
	}
	if options.status != "capacity-degraded" {
		t.Fatalf("status = %q, want capacity-degraded", options.status)
	}
	if options.note != "busy" {
		t.Fatalf("note = %q, want busy", options.note)
	}
}

func TestParseOptionsAcceptsProviderStatus(t *testing.T) {
	options, err := parseOptions([]string{"provider", "status"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if options.kind != commandProviderStatus {
		t.Fatalf("kind = %q, want provider-status", options.kind)
	}
}

func TestParseOptionsAcceptsProviderReset(t *testing.T) {
	options, err := parseOptions([]string{"provider", "reset", "gemini"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if options.kind != commandProviderReset {
		t.Fatalf("kind = %q, want provider-reset", options.kind)
	}
	if options.provider != "gemini" {
		t.Fatalf("provider = %q, want gemini", options.provider)
	}
}

func TestParseOptionsAcceptsTaskContextRefresh(t *testing.T) {
	options, err := parseOptions([]string{"task", "context", "refresh", "my-task"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if options.kind != commandContextRefresh {
		t.Fatalf("kind = %q, want context-refresh", options.kind)
	}
	if options.slug != "my-task" {
		t.Fatalf("slug = %q, want my-task", options.slug)
	}
}

func TestParseOptionsAcceptsRuntimeStateJSON(t *testing.T) {
	options, err := parseOptions([]string{"runtime", "state", "--json"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}
	if options.kind != commandRuntimeState || !options.json {
		t.Fatalf("options = %#v, want runtime state json", options)
	}
}

func TestParseOptionsAcceptsCleanupInspectJSON(t *testing.T) {
	options, err := parseOptions([]string{"cleanup", "inspect", "--json"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}
	if options.kind != commandCleanupInspect || !options.json {
		t.Fatalf("options = %#v, want cleanup inspect json", options)
	}
}

func TestParseOptionsAcceptsTaskStatus(t *testing.T) {
	options, err := parseOptions([]string{"task", "status"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}
	if options.kind != commandTaskStatus {
		t.Fatalf("kind = %q, want task-status", options.kind)
	}
}

func TestParseOptionsAcceptsTaskCleanupWithForce(t *testing.T) {
	options, err := parseOptions([]string{"task", "cleanup", "my-task", "--force"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if options.kind != commandTaskCleanup {
		t.Fatalf("kind = %q, want task-cleanup", options.kind)
	}
	if options.slug != "my-task" {
		t.Fatalf("slug = %q, want my-task", options.slug)
	}
	if !options.force {
		t.Fatal("force = false, want true")
	}
}

func TestParseOptionsAcceptsTaskNew(t *testing.T) {
	options, err := parseOptions([]string{"task", "new", "my-task"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if options.kind != commandTaskNew {
		t.Fatalf("kind = %q, want task-new", options.kind)
	}
	if options.slug != "my-task" {
		t.Fatalf("slug = %q, want my-task", options.slug)
	}
}

func TestParseOptionsAcceptsTaskRunExecute(t *testing.T) {
	options, err := parseOptions([]string{"task", "run", "my-task", "--execute", "--profile", "smoke", "--smoke"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if options.kind != commandTaskRun {
		t.Fatalf("kind = %q, want task-run", options.kind)
	}
	if options.slug != "my-task" {
		t.Fatalf("slug = %q, want my-task", options.slug)
	}
	if !options.execute {
		t.Fatal("execute = false, want true")
	}
	if options.profile != "smoke" {
		t.Fatalf("profile = %q, want smoke", options.profile)
	}
	if !options.smoke {
		t.Fatal("smoke = false, want true")
	}
}

func TestParseOptionsAcceptsDoctor(t *testing.T) {
	options, err := parseOptions([]string{"doctor"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if options.kind != commandDoctor {
		t.Fatalf("kind = %q, want doctor", options.kind)
	}
}

func TestParseOptionsAcceptsTaskRuntimeInfo(t *testing.T) {
	options, err := parseOptions([]string{"task", "runtime-info", "my-task"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if options.kind != commandTaskRuntimeInfo {
		t.Fatalf("kind = %q, want task-runtime-info", options.kind)
	}
	if options.slug != "my-task" {
		t.Fatalf("slug = %q, want my-task", options.slug)
	}
}

func TestParseOptionsAcceptsTaskRuntimeInfoJSON(t *testing.T) {
	options, err := parseOptions([]string{"task", "runtime-info", "my-task", "--json"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}
	if options.kind != commandTaskRuntimeInfo || options.slug != "my-task" || !options.json {
		t.Fatalf("options = %#v, want task runtime-info json", options)
	}
}

func TestParseOptionsAcceptsTaskDetailJSON(t *testing.T) {
	options, err := parseOptions([]string{"task", "detail", "my-task", "--json"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}
	if options.kind != commandTaskDetail || options.slug != "my-task" || !options.json {
		t.Fatalf("options = %#v, want task detail json", options)
	}
}

func TestParseOptionsAcceptsTaskRuns(t *testing.T) {
	options, err := parseOptions([]string{"task", "runs", "my-task"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if options.kind != commandTaskRuns {
		t.Fatalf("kind = %q, want task-runs", options.kind)
	}
	if options.slug != "my-task" {
		t.Fatalf("slug = %q, want my-task", options.slug)
	}
}

func TestParseOptionsAcceptsTaskRunsJSON(t *testing.T) {
	options, err := parseOptions([]string{"task", "runs", "my-task", "--json"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}
	if options.kind != commandTaskRuns || options.slug != "my-task" || !options.json {
		t.Fatalf("options = %#v, want task runs json", options)
	}
}

func TestParseOptionsAcceptsTaskRunsMaintenanceDryRun(t *testing.T) {
	cases := []struct {
		name string
		args []string
		kind commandKind
	}{
		{"reconcile", []string{"task", "runs", "reconcile", "--dry-run"}, commandRunsReconcile},
		{"retention", []string{"task", "runs", "retention", "--dry-run"}, commandRunsRetention},
		{"compact", []string{"task", "runs", "compact", "--dry-run"}, commandRunsCompact},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			options, err := parseOptions(tc.args)
			if err != nil {
				t.Fatalf("parseOptions returned error: %v", err)
			}
			if options.kind != tc.kind {
				t.Fatalf("kind = %q, want %q", options.kind, tc.kind)
			}
			if !options.dryRun {
				t.Fatal("dryRun = false, want true")
			}
		})
	}
}

func TestParseOptionsAcceptsHelp(t *testing.T) {
	for _, arg := range []string{"--help", "-h"} {
		t.Run(arg, func(t *testing.T) {
			options, err := parseOptions([]string{arg})
			if err != nil {
				t.Fatalf("parseOptions returned error: %v", err)
			}
			if !options.help {
				t.Fatal("help = false, want true")
			}
		})
	}
}

func TestParseOptionsAcceptsSupportMatrix(t *testing.T) {
	options, err := parseOptions([]string{"support", "matrix", "--json"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}
	if options.kind != commandSupportMatrix || !options.json {
		t.Fatalf("options = %#v", options)
	}
}

func TestRunSupportMatrixJSON(t *testing.T) {
	var stdout bytes.Buffer
	if err := run(&stdout, []string{"support", "matrix", "--json"}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"id": "provider-health"`) {
		t.Fatalf("support matrix JSON missing provider-health: %s", stdout.String())
	}
}

func TestRunWritesHelp(t *testing.T) {
	var stdout bytes.Buffer
	if err := run(&stdout, []string{"--help"}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	wants := []string{
		"dashboard remains read-only",
		"native Go where implemented",
		"--once",
	}
	for _, command := range commands.UsageCommands {
		wants = append(wants, command.Usage)
	}
	for _, want := range wants {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
}

func TestRunCmuxHelp(t *testing.T) {
	for _, args := range [][]string{
		{"cmux", "--help"},
		{"cmux", "-h"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout bytes.Buffer
			if err := run(&stdout, args); err != nil {
				t.Fatalf("run returned error: %v", err)
			}
			output := stdout.String()
			for _, want := range []string{
				"brevity cmux",
				"[read-only]",
				"--limit",
				"--section",
				"--task",
				"--state",
				"--output",
				"text",
				"markdown",
				"json",
				"brevity.cmux-report.v1",
				"Examples:",
			} {
				if !strings.Contains(output, want) {
					t.Errorf("cmux help missing %q;\noutput:\n%s", want, output)
				}
			}
			// Must not show the generic dashboard help.
			if strings.Contains(output, "--once") {
				t.Error("cmux help must not contain --once (generic dashboard flag)")
			}
		})
	}
}

func TestParseOptionsRejectsUnknownFlag(t *testing.T) {
	_, err := parseOptions([]string{"--unknown"})
	if err == nil {
		t.Fatal("parseOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "flag provided but not defined: -unknown") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseOptionsRejectsInvalidTaskContextRefresh(t *testing.T) {
	for _, args := range [][]string{
		{"task"},
		{"task", "context"},
		{"task", "context", "refresh"},
		{"task", "context", "status", "my-task"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := parseOptions(args)
			if err == nil {
				t.Fatal("parseOptions returned nil error")
			}
			if !strings.Contains(err.Error(), "task") && !strings.Contains(err.Error(), "usage") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseOptionsRejectsTaskCleanupWithoutForce(t *testing.T) {
	_, err := parseOptions([]string{"task", "cleanup", "my-task"})
	if err == nil {
		t.Fatal("parseOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "requires --force") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseOptionsRejectsTaskRunsMaintenanceWithoutDryRun(t *testing.T) {
	for _, args := range [][]string{
		{"task", "runs", "reconcile"},
		{"task", "runs", "retention"},
		{"task", "runs", "compact"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := parseOptions(args)
			if err == nil {
				t.Fatal("parseOptions returned nil error")
			}
			if !strings.Contains(err.Error(), "requires --dry-run") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseOptionsRejectsTaskNewMissingSlug(t *testing.T) {
	_, err := parseOptions([]string{"task", "new"})
	if err == nil {
		t.Fatal("parseOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "usage: "+commands.TaskNew.Usage) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseOptionsRejectsTaskRunWithoutPlanOrExecute(t *testing.T) {
	_, err := parseOptions([]string{"task", "run", "my-task", "--smoke"})
	if err == nil {
		t.Fatal("parseOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "requires --plan or --execute") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunTaskRunFailsBeforeClientWithoutPlanOrExecute(t *testing.T) {
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskRun, slug: "my-task"})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "requires --plan or --execute") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell calls", client.calls)
	}
}

func TestParseOptionsRejectsUnsupportedProviderCommand(t *testing.T) {
	_, err := parseOptions([]string{"provider", "docs"})
	if err == nil {
		t.Fatal("parseOptions returned nil error")
	}
	if !strings.Contains(err.Error(), `unsupported provider command "docs"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDoctorUsesNativeDiagnostics(t *testing.T) {
	repoRoot := tempRepoWithTasks(t, `[]`)
	if err := os.WriteFile(filepath.Join(repoRoot, ".brevity", "config.json"), []byte(`{"vaultPath":"","worktreesRoot":""}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile config returned error: %v", err)
	}
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandDoctor})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}

	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}

	output := stdout.String()
	for _, want := range []string{
		"Doctor",
		"repo: " + repoRoot,
		"checks:",
		"git-executable",
		"tasks-readable",
		"provider-health-readable",
		"runs-readable",
		"suggestedNextActions:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunDoctorJSONUsesNativeDiagnostics(t *testing.T) {
	repoRoot := tempRepoWithTasks(t, `[]`)
	if err := os.WriteFile(filepath.Join(repoRoot, ".brevity", "config.json"), []byte(`{"vaultPath":"","worktreesRoot":""}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile config returned error: %v", err)
	}
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandDoctor, json: true})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}
	output := stdout.String()
	for _, want := range []string{
		`"command":"doctor"`,
		`"schema":"brevity.doctor.v1"`,
		`"errors":[]`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunTaskRuntimeInfoUsesNativeStateAndRendersResult(t *testing.T) {
	repoRoot := tempRepoWithTasksAndRuns(t,
		`[{"slug":"my-task","status":"ready-for-worker","normalizedState":"ready-for-worker","worktreePath":"C:\\repo\\worktrees\\active\\brevity-my-task","worktree":{"exists":true,"path":"C:\\repo\\worktrees\\active\\brevity-my-task"},"context":{"materializedFileCount":3,"missingFiles":["runtime.md"]}}]`,
		`{"slug":"my-task","runId":"run-abc","workerStatus":"succeeded","startedAt":"2026-05-19T09:00:00Z","finishedAt":"2026-05-19T09:01:00Z","exitCode":0,"provider":"codex","profile":"default","logPath":"C:\\repo\\.brevity\\logs\\my-task\\run-abc.log"}`+"\n")
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskRuntimeInfo, slug: "my-task"})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}

	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}

	output := stdout.String()
	for _, want := range []string{
		"Task runtime-info",
		"slug: my-task",
		"status: ready-for-worker",
		"normalizedState: ready-for-worker",
		"worktreeExists: true",
		"worktreePath: C:\\repo\\worktrees\\active\\brevity-my-task",
		"contextFileCount: 3",
		"contextMissingCount: 1",
		"executionStatus: succeeded",
		"latestRunId: run-abc",
		"runCount: 1",
		"logPath: C:\\repo\\.brevity\\logs\\my-task\\run-abc.log",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunTaskDetailUsesNativeStateAndRendersResult(t *testing.T) {
	repoRoot := tempRepoWithTasksAndRuns(t,
		`[{"slug":"my-task","status":"ready-for-worker","normalizedState":"ready-for-worker","branch":"task/my-task","promptPath":"C:\\repo\\worktrees\\active\\brevity-my-task\\prompt.md"}]`,
		`{"slug":"my-task","runId":"run-abc","workerStatus":"succeeded","startedAt":"2026-05-19T09:00:00Z","finishedAt":"2026-05-19T09:01:00Z","exitCode":0,"provider":"codex","profile":"default","logPath":"C:\\repo\\.brevity\\logs\\my-task\\run-abc.log"}`+"\n")
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskDetail, slug: "my-task"})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}
	for _, want := range []string{
		"Task runtime-info",
		"slug: my-task",
		"branch: task/my-task",
		"promptPath: C:\\repo\\worktrees\\active\\brevity-my-task\\prompt.md",
		"latestRunId: run-abc",
		"interpretation: Latest run completed successfully.",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunTaskRuntimeInfoReturnsErrorWhenResultFails(t *testing.T) {
	repoRoot := tempRepoWithTasks(t, `[]`)
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskRuntimeInfo, slug: "nope"})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "task runtime-info reported success=false") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "error: task-not-found: Task not found: nope") {
		t.Fatalf("output missing structured error:\n%s", stdout.String())
	}
}

func TestRunTaskRunsUsesNativeStateAndRendersResult(t *testing.T) {
	repoRoot := tempRepoWithTasksAndRuns(t,
		`[{"slug":"my-task","status":"ready-for-worker"}]`,
		`{"slug":"my-task","runId":"run-abc","workerStatus":"failed","startedAt":"2026-05-19T09:00:00Z","finishedAt":"2026-05-19T09:01:00Z","exitCode":1,"failureType":"worker-exit-failed","provider":"codex","profile":"default","logPath":"C:\\repo\\.brevity\\logs\\my-task\\run-abc.log"}`+"\n")
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskRuns, slug: "my-task"})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}

	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}

	output := stdout.String()
	for _, want := range []string{
		"Task runs",
		"slug: my-task",
		"count: 1",
		"- runId: run-abc",
		"status: failed",
		"exitCode: 1",
		"provider: codex",
		"profile: default",
		"startedAt: 2026-05-19T09:00:00Z",
		"finishedAt: 2026-05-19T09:01:00Z",
		"failureType: worker-exit-failed",
		"logPath: C:\\repo\\.brevity\\logs\\my-task\\run-abc.log",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunTaskRunsMaintenanceUsesClientAndRendersResults(t *testing.T) {
	cases := []struct {
		name       string
		kind       commandKind
		json       []byte
		call       string
		wantOutput []string
	}{
		{
			name:       "reconcile",
			kind:       commandRunsReconcile,
			json:       []byte(`{"schema":"brevity.command-result.v1","command":"task runs reconcile","success":true,"severity":"info","payload":{"staleThresholdMinutes":30,"candidateCount":1,"candidates":[{"runId":"run-abc","slug":"my-task","stale":true,"incomplete":true,"workerStatus":"running"}]}}`),
			call:       "task-runs-reconcile",
			wantOutput: []string{"Task runs reconcile", "candidateCount: 1", "staleThresholdMinutes: 30", "- run-abc slug=my-task stale=true incomplete=true status=running"},
		},
		{
			name:       "retention",
			kind:       commandRunsRetention,
			json:       []byte(`{"schema":"brevity.command-result.v1","command":"task runs retention","success":true,"severity":"info","payload":{"totalRecords":5,"validRecords":4,"invalidRecords":1,"incompleteRecords":2,"staleRecords":1,"staleThresholdMinutes":30,"topTasks":[{"slug":"my-task","records":3}]}}`),
			call:       "task-runs-retention",
			wantOutput: []string{"Task runs retention", "totalRecords: 5", "validRecords: 4", "invalidRecords: 1", "staleRecords: 1", "incompleteRecords: 2", "- my-task: 3"},
		},
		{
			name:       "compact",
			kind:       commandRunsCompact,
			json:       []byte(`{"schema":"brevity.command-result.v1","command":"task runs compact","success":true,"severity":"info","payload":{"retainedRecordCount":4,"candidateArchiveSummaryCount":2,"candidateDiscardCount":1,"preservedStaleIncompleteCount":1,"preservedFailedCount":1}}`),
			call:       "task-runs-compact",
			wantOutput: []string{"Task runs compact", "retainedRecords: 4", "archiveSummaryCandidates: 2", "discardCandidates: 1", "preservedFailedRecords: 1", "preservedStaleIncompleteRecords: 1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeRuntimeClient{}
			switch tc.kind {
			case commandRunsReconcile:
				client.runsReconcile = tc.json
			case commandRunsRetention:
				client.runsRetention = tc.json
			case commandRunsCompact:
				client.runsCompact = tc.json
			}

			var stdout bytes.Buffer
			err := runWithOptions(&stdout, client, cliOptions{kind: tc.kind, dryRun: true})
			if err != nil {
				t.Fatalf("runWithOptions returned error: %v", err)
			}
			if len(client.calls) != 1 || client.calls[0] != tc.call {
				t.Fatalf("calls = %#v, want %s only", client.calls, tc.call)
			}
			output := stdout.String()
			for _, want := range tc.wantOutput {
				if !strings.Contains(output, want) {
					t.Fatalf("output missing %q:\n%s", want, output)
				}
			}
		})
	}
}

func TestRunTaskRunsMaintenanceFailsBeforeClientWithoutDryRun(t *testing.T) {
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandRunsReconcile})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "requires --dry-run") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell calls", client.calls)
	}
}

func TestRunTaskContextRefreshUsesClientAndRendersResult(t *testing.T) {
	root := taskContextRefreshFixture(t, "my-task")
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err = runWithOptions(&stdout, client, cliOptions{kind: commandContextRefresh, slug: "my-task"})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}

	if len(client.calls) != 0 {
		t.Fatalf("PowerShell/fake client was called: %v", client.calls)
	}

	output := stdout.String()
	for _, want := range []string{
		"Task context refresh: success",
		"slug: my-task",
		"refreshed: true",
		"promptPath:",
		"normalizedState: ready-for-worker",
		"promptRefreshStatus: fresh",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	prompt, err := os.ReadFile(filepath.Join(root, "worktree", "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prompt), "Implement fixture behavior.") {
		t.Fatalf("prompt was not materialized from vault spec:\n%s", prompt)
	}
}

func TestRunTaskContextRefreshReturnsErrorWhenResultFails(t *testing.T) {
	root := taskContextRefreshFixture(t, "my-task")
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err = runWithOptions(&stdout, client, cliOptions{kind: commandContextRefresh, slug: "nope"})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "preflight blocked") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "error: preflight-blocked") {
		t.Fatalf("output missing structured error:\n%s", stdout.String())
	}
}

func taskContextRefreshFixture(t *testing.T, slug string) string {
	t.Helper()
	root := t.TempDir()
	worktree := filepath.Join(root, "worktree")
	vault := filepath.Join(root, "vault")
	for _, dir := range []string{filepath.Join(root, ".brevity"), worktree, filepath.Join(vault, "tasks")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeMainTestFile(t, filepath.Join(root, ".brevity", "config.json"), `{"vaultPath":"`+jsonPath(vault)+`"}`+"\n")
	writeMainTestFile(t, filepath.Join(root, ".brevity", "provider-health.json"), `{"codex":{"status":"healthy"}}`+"\n")
	writeMainTestFile(t, filepath.Join(vault, "project.md"), "# Project\nFixture project.\n")
	writeMainTestFile(t, filepath.Join(vault, "architecture.md"), "# Architecture\nFixture architecture.\n")
	writeMainTestFile(t, filepath.Join(vault, "tasks", slug+".md"), "# Goal\nImplement fixture behavior.\n")
	tasks := fmt.Sprintf(`[{"slug":%q,"status":"ready-for-worker","normalizedState":"ready-for-worker","worktreePath":%q,"promptPath":%q,"provider":"codex"}]`, slug, worktree, filepath.Join(worktree, "prompt.md"))
	writeMainTestFile(t, filepath.Join(root, ".brevity", "tasks.json"), tasks+"\n")
	return root
}

func writeMainTestFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readMainTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func tempRepoWithQueue(t *testing.T, queue runtimequeue.Queue) string {
	t.Helper()
	repoRoot := t.TempDir()
	brevityRoot := filepath.Join(repoRoot, ".brevity")
	if err := os.MkdirAll(brevityRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	store, err := state.NewStore(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteJSON(runtimequeue.FileName, queue); err != nil {
		t.Fatal(err)
	}
	return repoRoot
}

func readMainTestQueue(t *testing.T, repoRoot string) runtimequeue.Queue {
	t.Helper()
	store, err := runtimequeue.NewStore(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	queue, _, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return queue
}

func readMainTestExecutions(t *testing.T, repoRoot string) runtimeexecution.Executions {
	t.Helper()
	store, err := runtimeexecution.NewStore(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	executions, _, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return executions
}

func writeMainTestExecutions(t *testing.T, repoRoot string, executions runtimeexecution.Executions) {
	t.Helper()
	store, err := state.NewStore(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteJSON(runtimeexecution.FileName, executions); err != nil {
		t.Fatal(err)
	}
}

func mainTestQueueItem(id string, task string, status string) runtimequeue.Item {
	return runtimequeue.Item{
		ID:        id,
		Task:      task,
		Provider:  "codex",
		Profile:   "default",
		Status:    status,
		CreatedAt: "2026-05-22T10:00:00Z",
		UpdatedAt: "2026-05-22T10:00:00Z",
	}
}

func mainTestReservedQueueItem(id string, task string, reservationID string) runtimequeue.Item {
	item := mainTestQueueItem(id, task, runtimequeue.StatusQueued)
	item.Reservation = &runtimequeue.Reservation{
		Owner:         "runtime-supervisor",
		ReservedAt:    "2026-05-22T11:00:00Z",
		ReservationID: reservationID,
	}
	return item
}

func mainTestExecutionRecord(id string, queueItemID string, task string, reservationID string, status string) runtimeexecution.Record {
	return runtimeexecution.Record{
		ID:            id,
		QueueItemID:   queueItemID,
		Task:          task,
		ReservationID: reservationID,
		Status:        status,
		CreatedAt:     "2026-05-22T12:00:00Z",
		UpdatedAt:     "2026-05-22T12:00:00Z",
	}
}

func jsonPath(path string) string {
	return strings.ReplaceAll(path, `\`, `\\`)
}

func TestRunTaskCleanupUsesClientAndRendersResult(t *testing.T) {
	repoRoot, worktreePath := tempGitRepoForTaskCleanup(t, "my-task")
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskCleanup, slug: "my-task", force: true})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}

	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell calls", client.calls)
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists after cleanup: %v", err)
	}
	command := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/task/my-task")
	command.Dir = repoRoot
	if command.Run() == nil {
		t.Fatal("task branch still exists after cleanup")
	}

	output := stdout.String()
	for _, want := range []string{
		"Task cleanup: success",
		"slug: my-task",
		"worktreeRemoved: true",
		"branchRemoved: true",
		"metadataRemoved: true",
		"- refresh-runtime-state",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunTaskCleanupFailsBeforeClientWithoutForce(t *testing.T) {
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskCleanup, slug: "my-task"})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "requires --force") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell calls", client.calls)
	}
}

func TestRunTaskCleanupReturnsErrorWhenResultFails(t *testing.T) {
	repoRoot := tempGitRepoForTaskNew(t)
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskCleanup, slug: "nope", force: true})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "task cleanup blocked") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "error: task-not-found: task not found: nope") {
		t.Fatalf("output missing structured error:\n%s", stdout.String())
	}
}

func TestRunTaskNewUsesNativeServiceAndRendersResult(t *testing.T) {
	repoRoot := tempGitRepoForTaskNew(t)
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskNew, slug: "my-task"})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}

	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}

	output := stdout.String()
	for _, want := range []string{
		"Task new: success",
		"slug: my-task",
		"state: ready-for-worker",
		"branch: task/my-task",
		"worktreePath:",
		"promptPath:",
		"metadataPath:",
		"providerExecution: false",
		"workerExecution: false",
		"- refresh-runtime-state",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunTaskNewReturnsErrorWhenDuplicate(t *testing.T) {
	repoRoot := tempGitRepoForTaskNew(t)
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	if err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskNew, slug: "my-task"}); err != nil {
		t.Fatalf("first task new returned error: %v", err)
	}
	stdout.Reset()
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskNew, slug: "my-task"})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "task new preflight blocked mutation") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "slug: my-task") {
		t.Fatalf("output missing slug fallback:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "error: preflight-blocked: task new preflight blocked mutation") {
		t.Fatalf("output missing structured error:\n%s", stdout.String())
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}
}

func TestRunTaskNewDisposableFixtureUpdatesStatusAndRuntimeState(t *testing.T) {
	repoRoot := tempGitRepoForTaskNew(t)
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskNew, slug: "fixture-new-task", json: true})
	if err != nil {
		t.Fatalf("task new returned error: %v\n%s", err, stdout.String())
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, ".brevity", "tasks.json"))
	if err != nil {
		t.Fatalf("ReadFile tasks returned error: %v", err)
	}
	for _, want := range []string{`"slug": "fixture-new-task"`, `"status": "ready-for-worker"`, `"branch": "task/fixture-new-task"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("tasks.json missing %q:\n%s", want, string(data))
		}
	}

	stdout.Reset()
	if err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskStatus}); err != nil {
		t.Fatalf("task status returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "fixture-new-task") {
		t.Fatalf("task status missing created task:\n%s", stdout.String())
	}

	stdout.Reset()
	if err := runWithOptions(&stdout, client, cliOptions{kind: commandRuntimeState}); err != nil {
		t.Fatalf("runtime state returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "fixture-new-task") {
		t.Fatalf("runtime state missing created task:\n%s", stdout.String())
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}
}

func TestRunTaskRunUsesNativeExecutionAndRendersResult(t *testing.T) {
	repoRoot := tempRepoWithFakeProvider(t, 0)
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskRun, slug: "my-task", execute: true, smoke: true})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v\n%s", err, stdout.String())
	}

	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}

	output := stdout.String()
	for _, want := range []string{
		"Task run: success",
		"slug: my-task",
		"provider: antigravity",
		"profile: default",
		"workerStatus: succeeded",
		"exitCode: 0",
		".brevity",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunTaskRunReturnsErrorWhenWorkerFails(t *testing.T) {
	repoRoot := tempRepoWithFakeProvider(t, 7)
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskRun, slug: "my-task", execute: true})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "task run reported success=false") {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		"Task run: failure",
		"workerStatus: failed",
		"exitCode: 7",
		"failureType: provider-exit-nonzero",
		"error: provider-exit-nonzero:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunProviderSetUsesNativeStateAndRendersResult(t *testing.T) {
	repoRoot := tempRepoWithProviderHealth(t, `{"gemini":{"status":"unknown","note":"","updatedAt":""}}`)
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandProviderSet, provider: "gemini", status: "capacity-degraded", note: "busy"})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}

	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}

	output := stdout.String()
	for _, want := range []string{
		"Provider action: success",
		"provider: gemini",
		"previousStatus: unknown",
		"newStatus: capacity-degraded",
		"note: busy",
		"- refresh-runtime-state",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunProviderStatusUsesNativeState(t *testing.T) {
	repoRoot := tempRepoWithProviderHealth(t, `{"codex":{"status":"healthy","note":"ok","updatedAt":"2026-05-21T10:00:00Z"}}`)
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandProviderStatus})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}
	for _, want := range []string{"Provider health", "Providers: 1 total, 0 degraded, 0 unavailable", "codex\thealthy\t2026-05-21T10:00:00Z\tok"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunTaskStatusUsesNativeState(t *testing.T) {
	repoRoot := tempRepoWithTasks(t, `[
		{"slug":"my-task","status":"ready-for-worker","normalizedState":"ready-for-worker","branch":"task/my-task","worktreePath":"C:\\repo\\worktrees\\active\\my-task"}
	]`)
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskStatus})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}
	for _, want := range []string{"Task status", "Tasks: 1 tracked", "my-task\tready-for-worker\ttask/my-task\tC:\\repo\\worktrees\\active\\my-task"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunRuntimeStateJSONUsesNativeState(t *testing.T) {
	repoRoot := tempRepoWithTasks(t, `[{"slug":"my-task","status":"blocked"}]`)
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandRuntimeState, json: true})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}
	output := stdout.String()
	for _, want := range []string{`"schema":"brevity.runtime-state.v1"`, `"tracked":1`, `"slug":"my-task"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunCleanupInspectUsesNativeState(t *testing.T) {
	repoRoot := tempRepoWithTasks(t, `[{"slug":"missing","status":"ready-for-worker","branch":"task/missing","worktreePath":"C:\\repo\\missing"}]`)
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandCleanupInspect})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell client calls", client.calls)
	}
	for _, want := range []string{"Cleanup inspection", "No cleanup executed.", "missing-worktree:missing", "inspect task metadata"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunProviderResetReturnsErrorWhenNativeProviderUnknown(t *testing.T) {
	repoRoot := tempRepoWithProviderHealth(t, `{"codex":{"status":"unknown"}}`)
	t.Chdir(repoRoot)
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandProviderReset, provider: "nope"})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "Invalid provider: nope") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "error: invalid-provider: Invalid provider: nope") {
		t.Fatalf("output missing structured error:\n%s", stdout.String())
	}
}

func tempRepoWithProviderHealth(t *testing.T, health string) string {
	t.Helper()
	repoRoot := t.TempDir()
	brevityRoot := filepath.Join(repoRoot, ".brevity")
	if err := os.MkdirAll(brevityRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brevityRoot, "provider-health.json"), []byte(health+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	return repoRoot
}

func tempGitRepoForTaskNew(t *testing.T) string {
	t.Helper()
	raw := t.TempDir()
	// On Windows, t.TempDir returns 8.3 short-name paths (e.g. MORTEN~1),
	// but git resolves worktree paths to their long-name canonical form.
	// EvalSymlinks normalises the path so worktreeRegistered comparisons succeed.
	repoRoot, err := filepath.EvalSymlinks(raw)
	if err != nil {
		t.Fatalf("EvalSymlinks on temp dir: %v", err)
	}
	runTestCommand(t, repoRoot, "git", "init")
	runTestCommand(t, repoRoot, "git", "config", "user.email", "brevity@example.test")
	runTestCommand(t, repoRoot, "git", "config", "user.name", "Brevity Test")
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatalf("WriteFile README returned error: %v", err)
	}
	runTestCommand(t, repoRoot, "git", "add", "README.md")
	runTestCommand(t, repoRoot, "git", "commit", "-m", "initial")
	brevityRoot := filepath.Join(repoRoot, ".brevity")
	if err := os.MkdirAll(brevityRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brevityRoot, "tasks.json"), []byte("[]\n"), 0o644); err != nil {
		t.Fatalf("WriteFile tasks returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brevityRoot, "provider-health.json"), []byte(`{"codex":{"status":"healthy"}}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile provider health returned error: %v", err)
	}
	config := fmt.Sprintf(`{"vaultPath":"","worktreesRoot":%q}`, filepath.Join(repoRoot, "worktrees", "active"))
	if err := os.WriteFile(filepath.Join(brevityRoot, "config.json"), []byte(config+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile config returned error: %v", err)
	}
	return repoRoot
}

func tempGitRepoForTaskCleanup(t *testing.T, slug string) (string, string) {
	t.Helper()
	repoRoot := tempGitRepoForTaskNew(t)
	worktreePath := filepath.Join(repoRoot, "worktrees", "active", "brevity-"+slug)
	branch := "task/" + slug
	runTestCommand(t, repoRoot, "git", "worktree", "add", worktreePath, "-b", branch)
	tasks := fmt.Sprintf(`[{"slug":%q,"status":"merged","normalizedState":"merged","branch":%q,"worktreePath":%q}]`, slug, branch, worktreePath)
	if err := os.WriteFile(filepath.Join(repoRoot, ".brevity", "tasks.json"), []byte(tasks+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile tasks returned error: %v", err)
	}
	return repoRoot, worktreePath
}

func runTestCommand(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	command := exec.Command(name, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, string(output))
	}
}

func tempRepoWithTasks(t *testing.T, tasks string) string {
	t.Helper()
	repoRoot := t.TempDir()
	brevityRoot := filepath.Join(repoRoot, ".brevity")
	if err := os.MkdirAll(brevityRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brevityRoot, "tasks.json"), []byte(tasks+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile tasks returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brevityRoot, "provider-health.json"), []byte(`{"codex":{"status":"healthy"}}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile provider health returned error: %v", err)
	}
	return repoRoot
}

func tempRepoWithTasksAndRuns(t *testing.T, tasks string, runs string) string {
	t.Helper()
	repoRoot := tempRepoWithTasks(t, tasks)
	if err := os.WriteFile(filepath.Join(repoRoot, ".brevity", "runs.jsonl"), []byte(runs), 0o644); err != nil {
		t.Fatalf("WriteFile runs returned error: %v", err)
	}
	return repoRoot
}

func tempRepoWithFakeProvider(t *testing.T, exitCode int) string {
	t.Helper()
	repoRoot := tempRepoWithTasks(t, `[
		{"slug":"my-task","status":"ready-for-worker","normalizedState":"ready-for-worker","provider":"antigravity","worktreePath":"","promptPath":""}
	]`)
	worktree := filepath.Join(repoRoot, "worktrees", "active", "my task")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatalf("MkdirAll worktree returned error: %v", err)
	}
	promptPath := filepath.Join(worktree, "prompt.md")
	if err := os.WriteFile(promptPath, []byte("fake prompt\n"), 0o644); err != nil {
		t.Fatalf("WriteFile prompt returned error: %v", err)
	}
	tasks := fmt.Sprintf(`[{"slug":"my-task","status":"ready-for-worker","normalizedState":"ready-for-worker","provider":"antigravity","branch":"task/my-task","worktreePath":%q,"promptPath":%q}]`, worktree, promptPath)
	if err := os.WriteFile(filepath.Join(repoRoot, ".brevity", "tasks.json"), []byte(tasks+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile tasks returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".brevity", "provider-health.json"), []byte(`{"antigravity":{"status":"healthy"}}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile provider health returned error: %v", err)
	}
	fakeProvider := filepath.Join(repoRoot, "fake-provider.cmd")
	script := fmt.Sprintf("@echo off\r\necho fake stdout\r\necho fake stderr 1>&2\r\nexit /b %d\r\n", exitCode)
	if err := os.WriteFile(fakeProvider, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile fake provider returned error: %v", err)
	}
	config := fmt.Sprintf(`{"defaultProvider":"antigravity","providers":{"antigravity":{"command":%q}}}`, fakeProvider)
	if err := os.WriteFile(filepath.Join(repoRoot, ".brevity", "config.json"), []byte(config+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile config returned error: %v", err)
	}
	return repoRoot
}
