package execution

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	bstate "github.com/mortenlein/brevity/internal/state"
)

func TestLaunchSuccessfulProviderPath(t *testing.T) {
	store := testStore(t)
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusReady)}})
	runner := &fakeRunner{exitCode: 0, stdout: "hello from provider\n"}
	var stdout bytes.Buffer

	result, err := Launcher{Store: store, Runner: runner, Stdout: &stdout}.Launch(context.Background(), launchPayloadFixture("exec-1"))
	if err != nil {
		t.Fatalf("Launch returned error: %v", err)
	}
	if result.ExitCode != 0 || result.FinalStatus != StatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if stdout.String() != "hello from provider\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	assertExecutionStatus(t, store, "exec-1", StatusCompleted)
	assertLaunchRunHistory(t, store, "exec-1", "queue-1", StatusCompleted, 0, "")
}

func TestLaunchFailureMarksFailed(t *testing.T) {
	store := testStore(t)
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusReady)}})
	launchErr := errors.New("provider binary not found")

	result, err := Launcher{Store: store, Runner: &fakeRunner{exitCode: 1, err: launchErr}}.Launch(context.Background(), launchPayloadFixture("exec-1"))
	if !errors.Is(err, launchErr) {
		t.Fatalf("err = %v, want launch err", err)
	}
	if result.FinalStatus != StatusFailed || result.ExitCode != 1 {
		t.Fatalf("result = %#v", result)
	}
	assertExecutionStatus(t, store, "exec-1", StatusFailed)
	assertLaunchRunHistory(t, store, "exec-1", "queue-1", StatusFailed, 1, "provider binary not found")
}

func TestLaunchNonzeroExitCodeMarksFailed(t *testing.T) {
	store := testStore(t)
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusReady)}})

	result, err := Launcher{Store: store, Runner: &fakeRunner{exitCode: 42}}.Launch(context.Background(), launchPayloadFixture("exec-1"))
	if err != nil {
		t.Fatalf("Launch returned error for nonzero exit without runner error: %v", err)
	}
	if result.FinalStatus != StatusFailed || result.ExitCode != 42 {
		t.Fatalf("result = %#v", result)
	}
	assertExecutionStatus(t, store, "exec-1", StatusFailed)
	assertLaunchRunHistory(t, store, "exec-1", "queue-1", StatusFailed, 42, "nonzero")
}

func TestLaunchTransitionsReadyLaunchingCompleted(t *testing.T) {
	store := testStore(t)
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusReady)}})
	runner := &fakeRunner{exitCode: 0, inspect: func() {
		assertExecutionStatus(t, store, "exec-1", StatusLaunching)
	}}

	if _, err := (Launcher{Store: store, Runner: runner}).Launch(context.Background(), launchPayloadFixture("exec-1")); err != nil {
		t.Fatalf("Launch returned error: %v", err)
	}
	assertExecutionStatus(t, store, "exec-1", StatusCompleted)
}

func TestLaunchPersistsUpdatedAtForTransitions(t *testing.T) {
	store := testStore(t)
	times := []time.Time{
		time.Date(2026, 5, 22, 13, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 22, 13, 0, 5, 0, time.UTC),
	}
	store.Now = func() time.Time {
		if len(times) == 1 {
			return times[0]
		}
		next := times[0]
		times = times[1:]
		return next
	}
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusReady)}})

	if _, err := (Launcher{Store: store, Runner: &fakeRunner{exitCode: 0}}).Launch(context.Background(), launchPayloadFixture("exec-1")); err != nil {
		t.Fatalf("Launch returned error: %v", err)
	}
	loaded, _, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.Records[0].UpdatedAt != "2026-05-22T13:00:05Z" {
		t.Fatalf("updatedAt = %q", loaded.Records[0].UpdatedAt)
	}
}

func TestLaunchUsesArgvStyleExecution(t *testing.T) {
	store := testStore(t)
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusReady)}})
	payload := launchPayloadFixture("exec-1")
	payload.Command = "fake-provider"
	payload.Arguments = []string{"-p", "hello; Remove-Item important", "quoted value"}
	payload.Argv = append([]string{payload.Command}, payload.Arguments...)
	payload.Environment = map[string]string{"SECRET_TOKEN": "value"}
	runner := &fakeRunner{exitCode: 0}

	if _, err := (Launcher{Store: store, Runner: runner}).Launch(context.Background(), payload); err != nil {
		t.Fatalf("Launch returned error: %v", err)
	}
	if runner.request.Command != "fake-provider" {
		t.Fatalf("command = %q", runner.request.Command)
	}
	if !reflect.DeepEqual(runner.request.Arguments, payload.Arguments) {
		t.Fatalf("arguments = %#v, want %#v", runner.request.Arguments, payload.Arguments)
	}
	if runner.request.Environment["SECRET_TOKEN"] != "value" {
		t.Fatalf("environment was not passed to runner")
	}
}

func TestLaunchLockingRejectsConcurrentMutation(t *testing.T) {
	store := testStore(t)
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusReady)}})
	lock, err := store.acquireLock()
	if err != nil {
		t.Fatalf("acquireLock returned error: %v", err)
	}
	defer lock.Release()

	launchStore := store
	launchStore.Now = nil
	_, err = Launcher{Store: launchStore, Runner: &fakeRunner{exitCode: 0}}.Launch(context.Background(), launchPayloadFixture("exec-1"))
	if err == nil {
		t.Fatal("Launch succeeded while execution lock was held")
	}
	assertExecutionStatus(t, store, "exec-1", StatusReady)
}

func TestExecRunnerStreamsOutputAndCapturesExitCode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	result := ExecRunner{}.Run(context.Background(), ProcessRequest{
		Command: os.Args[0],
		Arguments: []string{
			"-test.run=TestHelperProviderProcess",
			"--",
			"stream-ok",
		},
		Environment: map[string]string{"GO_WANT_HELPER_PROCESS": "1"},
		Stdout:      &stdout,
		Stderr:      &stderr,
	})
	if result.ExitCode != 0 || result.Err != nil {
		t.Fatalf("result = %#v", result)
	}
	if stdout.String() != "provider stdout\n" || stderr.String() != "provider stderr\n" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestExecRunnerMissingBinaryFails(t *testing.T) {
	result := ExecRunner{}.Run(context.Background(), ProcessRequest{Command: filepath.Join(t.TempDir(), "missing-provider")})
	if result.ExitCode == 0 || result.Err == nil {
		t.Fatalf("result = %#v, want launch failure", result)
	}
}

func TestHelperProviderProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	if len(os.Args) > 0 && os.Args[len(os.Args)-1] == "stream-ok" {
		_, _ = os.Stdout.WriteString("provider stdout\n")
		_, _ = os.Stderr.WriteString("provider stderr\n")
		os.Exit(0)
	}
	os.Exit(3)
}

type fakeRunner struct {
	exitCode int
	err      error
	stdout   string
	stderr   string
	request  ProcessRequest
	inspect  func()
}

func (runner *fakeRunner) Run(_ context.Context, request ProcessRequest) ProcessResult {
	runner.request = request
	if runner.inspect != nil {
		runner.inspect()
	}
	if strings.TrimSpace(runner.stdout) != "" {
		_, _ = request.Stdout.Write([]byte(runner.stdout))
	}
	if strings.TrimSpace(runner.stderr) != "" {
		_, _ = request.Stderr.Write([]byte(runner.stderr))
	}
	return ProcessResult{ExitCode: runner.exitCode, Err: runner.err}
}

func launchPayloadFixture(id string) LaunchPayload {
	return LaunchPayload{
		ExecutionID: id,
		QueueItemID: "queue-1",
		Task:        "some-task",
		Provider:    "codex",
		Profile:     "codex-balanced",
		Worktree:    ".",
		Prompt:      "prompt.md",
		Command:     "provider",
		Arguments:   []string{"exec", "prompt.md"},
		Argv:        []string{"provider", "exec", "prompt.md"},
	}
}

func assertLaunchRunHistory(t *testing.T, store Store, executionID string, queueItemID string, status string, exitCode int, errorContains string) {
	t.Helper()
	history, missing, err := bstate.LoadRuns(store.Store, time.Now().UTC())
	if err != nil {
		t.Fatalf("LoadRuns returned error: %v", err)
	}
	if missing || len(history.Items) != 1 {
		t.Fatalf("history missing=%v items=%#v, want one run", missing, history.Items)
	}
	run := history.Items[0]
	if run.ExecutionID != executionID || run.RunID != executionID || run.QueueItemID != queueItemID {
		t.Fatalf("run ids = %#v", run)
	}
	if run.Slug != "some-task" || run.Provider != "codex" || run.Profile != "codex-balanced" {
		t.Fatalf("run metadata = %#v", run)
	}
	if !reflect.DeepEqual(run.CommandArgv, []string{"provider", "exec", "prompt.md"}) {
		t.Fatalf("command argv = %#v", run.CommandArgv)
	}
	if run.CompletedAt == "" || run.FinishedAt == "" || run.StartedAt == "" {
		t.Fatalf("run timestamps = %#v", run)
	}
	if run.ExitCode != float64(exitCode) {
		t.Fatalf("exitCode = %#v, want %d", run.ExitCode, exitCode)
	}
	if run.FinalExecutionStatus != status {
		t.Fatalf("finalExecutionStatus = %q, want %q", run.FinalExecutionStatus, status)
	}
	if errorContains != "" && !strings.Contains(run.Summary, errorContains) {
		t.Fatalf("summary = %q, want containing %q", run.Summary, errorContains)
	}
}

func assertExecutionStatus(t *testing.T, store Store, id string, status string) {
	t.Helper()
	loaded, _, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	for _, record := range loaded.Records {
		if record.ID == id {
			if record.Status != status {
				t.Fatalf("status = %q, want %q", record.Status, status)
			}
			return
		}
	}
	t.Fatalf("execution not found: %s", id)
}
