package main

import (
	"bytes"
	"strings"
	"testing"
)

type fakeRuntimeClient struct {
	output         []byte
	err            error
	providerSet    []byte
	providerReset  []byte
	doctor         []byte
	contextRefresh []byte
	taskCleanup    []byte
	taskNew        []byte
	runtimeInfo    []byte
	taskRuns       []byte
	actionErr      error
	calls          []string
}

func (client *fakeRuntimeClient) RuntimeStateJSON() ([]byte, error) {
	client.calls = append(client.calls, "runtime-state")
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

func (client *fakeRuntimeClient) TaskRuntimeInfoJSON(slug string) ([]byte, error) {
	client.calls = append(client.calls, "runtime-info:"+slug)
	return client.runtimeInfo, client.actionErr
}

func (client *fakeRuntimeClient) TaskRunsJSON(slug string) ([]byte, error) {
	client.calls = append(client.calls, "task-runs:"+slug)
	return client.taskRuns, client.actionErr
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

func TestParseOptionsAcceptsProviderSet(t *testing.T) {
	options, err := parseOptions([]string{"provider", "set", "gemini", "capacity-degraded"})
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

func TestRunWritesHelp(t *testing.T) {
	var stdout bytes.Buffer
	if err := run(&stdout, []string{"--help"}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"dashboard remains read-only",
		`.\brevity.ps1 ... --json`,
		"--once",
		"provider set <provider> <status>",
		"task context refresh <slug>",
		"task new <slug>",
		"task runtime-info <slug>",
		"task runs <slug>",
		"task cleanup <slug> --force",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
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

func TestParseOptionsRejectsTaskNewMissingSlug(t *testing.T) {
	_, err := parseOptions([]string{"task", "new"})
	if err == nil {
		t.Fatal("parseOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "usage: brevity task new <slug>") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseOptionsRejectsUnsupportedProviderCommand(t *testing.T) {
	_, err := parseOptions([]string{"provider", "status"})
	if err == nil {
		t.Fatal("parseOptions returned nil error")
	}
	if !strings.Contains(err.Error(), `unsupported provider command "status"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDoctorUsesClientAndRendersResult(t *testing.T) {
	client := &fakeRuntimeClient{
		doctor: []byte(`{"schema":"brevity.command-result.v1","command":"doctor","success":true,"severity":"info","warnings":[{"code":"orphaned-task-worktrees","message":"Orphaned task worktrees are present.","count":2}],"suggestedNextActions":["Run doctor."],"payload":{"warningCount":1,"errorCount":0,"providers":{"summary":{"total":3,"degraded":1,"unavailable":0}},"branchCounts":{"orphaned":4},"worktreeCounts":{"orphanedTaskWorktrees":2},"lock":{"exists":false,"path":"C:\\repo\\.brevity\\tasks.lock"},"suggestedNextActions":["Run doctor."]}}`),
	}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandDoctor})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}

	if len(client.calls) != 1 || client.calls[0] != "doctor" {
		t.Fatalf("calls = %#v, want doctor only", client.calls)
	}

	output := stdout.String()
	for _, want := range []string{
		"Doctor",
		"warnings: 1",
		"errors: 0",
		"providers: total=3 degraded=1 unavailable=0",
		"orphanedWorktrees: 2",
		"orphanedBranches: 4",
		"lockExists: false",
		"- Run doctor.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunTaskRuntimeInfoUsesClientAndRendersResult(t *testing.T) {
	client := &fakeRuntimeClient{
		runtimeInfo: []byte(`{"schema":"brevity.command-result.v1","command":"task runtime-info","success":true,"severity":"info","payload":{"slug":"my-task","status":"ready-for-worker","normalizedState":"ready-for-worker","worktree":{"exists":true,"path":"C:\\repo\\worktrees\\active\\brevity-my-task"},"context":{"materializedFileCount":3,"missingFiles":["runtime.md"]},"execution":{"status":"succeeded","lastRunId":"run-abc","lastLogPath":"C:\\repo\\.brevity\\logs\\my-task\\run-abc.log"}}}`),
	}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskRuntimeInfo, slug: "my-task"})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}

	if len(client.calls) != 1 || client.calls[0] != "runtime-info:my-task" {
		t.Fatalf("calls = %#v, want runtime info only", client.calls)
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
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunTaskRuntimeInfoReturnsErrorWhenResultFails(t *testing.T) {
	client := &fakeRuntimeClient{
		runtimeInfo: []byte(`{"schema":"brevity.command-result.v1","command":"task runtime-info","success":false,"severity":"error","errors":[{"code":"task-not-found","message":"Task not found.","details":{"slug":"nope"}}],"payload":{}}`),
	}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskRuntimeInfo, slug: "nope"})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "task runtime-info reported success=false") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "error: task-not-found: Task not found.") {
		t.Fatalf("output missing structured error:\n%s", stdout.String())
	}
}

func TestRunTaskRunsUsesClientAndRendersResult(t *testing.T) {
	client := &fakeRuntimeClient{
		taskRuns: []byte(`{"schema":"brevity.command-result.v1","command":"task runs","success":true,"severity":"info","payload":{"slug":"my-task","count":1,"runs":[{"runId":"run-abc","workerStatus":"failed","exitCode":"1","provider":"codex","profile":"default","logPath":"C:\\repo\\.brevity\\logs\\my-task\\run-abc.log"}]}}`),
	}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskRuns, slug: "my-task"})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}

	if len(client.calls) != 1 || client.calls[0] != "task-runs:my-task" {
		t.Fatalf("calls = %#v, want task runs only", client.calls)
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
		"logPath: C:\\repo\\.brevity\\logs\\my-task\\run-abc.log",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunTaskContextRefreshUsesClientAndRendersResult(t *testing.T) {
	client := &fakeRuntimeClient{
		contextRefresh: []byte(`{"schema":"brevity.command-result.v1","command":"task context refresh","success":true,"severity":"info","suggestedNextActions":["Refresh runtime state."],"payload":{"slug":"my-task","refreshed":true,"contextPath":"C:\\repo\\worktrees\\active\\my-task\\.brevity\\context","generatedAt":"2026-05-19T13:00:00Z","normalizedState":"ready-for-worker"}}`),
	}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandContextRefresh, slug: "my-task"})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}

	if len(client.calls) != 1 || client.calls[0] != "context-refresh:my-task" {
		t.Fatalf("calls = %#v, want context refresh only", client.calls)
	}

	output := stdout.String()
	for _, want := range []string{
		"Task context refresh: success",
		"slug: my-task",
		"refreshed: true",
		"contextPath: C:\\repo\\worktrees\\active\\my-task\\.brevity\\context",
		"generatedAt: 2026-05-19T13:00:00Z",
		"normalizedState: ready-for-worker",
		"- Refresh runtime state.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunTaskContextRefreshReturnsErrorWhenResultFails(t *testing.T) {
	client := &fakeRuntimeClient{
		contextRefresh: []byte(`{"schema":"brevity.command-result.v1","command":"task context refresh","success":false,"severity":"error","errors":[{"code":"missing-task","message":"Task not found: nope"}],"payload":{"slug":"nope","refreshed":false,"contextPath":"","generatedAt":"2026-05-19T13:00:00Z"}}`),
	}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandContextRefresh, slug: "nope"})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "task context refresh reported success=false") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "error: missing-task: Task not found: nope") {
		t.Fatalf("output missing structured error:\n%s", stdout.String())
	}
}

func TestRunTaskCleanupUsesClientAndRendersResult(t *testing.T) {
	client := &fakeRuntimeClient{
		taskCleanup: []byte(`{"schema":"brevity.command-result.v1","command":"task cleanup","success":true,"severity":"warning","warnings":[{"message":"Runtime state is stale."}],"suggestedNextActions":["refresh-runtime-state"],"payload":{"slug":"my-task","worktreePath":"C:\\repo\\worktrees\\active\\brevity-my-task","branch":"task/my-task","metadataRemoved":true,"branchRemoved":true,"worktreeRemoved":true,"force":true,"cleanupWarnings":["Runtime state is stale."]}}`),
	}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskCleanup, slug: "my-task", force: true})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}

	if len(client.calls) != 1 || client.calls[0] != "task-cleanup:my-task" {
		t.Fatalf("calls = %#v, want task cleanup only", client.calls)
	}

	output := stdout.String()
	for _, want := range []string{
		"Task cleanup: success",
		"slug: my-task",
		"worktreeRemoved: true",
		"branchRemoved: true",
		"metadataRemoved: true",
		"cleanupWarning: Runtime state is stale.",
		"warning: Runtime state is stale.",
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
	client := &fakeRuntimeClient{
		taskCleanup: []byte(`{"schema":"brevity.command-result.v1","command":"task cleanup","success":false,"severity":"error","errors":[{"code":"task-not-found","message":"Task not found: nope"}],"payload":{"slug":"nope","metadataRemoved":false,"branchRemoved":false,"worktreeRemoved":false,"force":true,"cleanupWarnings":[]}}`),
	}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskCleanup, slug: "nope", force: true})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "task cleanup reported success=false") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "error: task-not-found: Task not found: nope") {
		t.Fatalf("output missing structured error:\n%s", stdout.String())
	}
}

func TestRunTaskNewUsesClientAndRendersResult(t *testing.T) {
	client := &fakeRuntimeClient{
		taskNew: []byte(`{"schema":"brevity.command-result.v1","command":"task new","success":true,"severity":"info","suggestedNextActions":["refresh-runtime-state"],"payload":{"slug":"my-task","branch":"task/my-task","worktreePath":"C:\\repo\\worktrees\\active\\brevity-my-task","promptPath":"C:\\repo\\worktrees\\active\\brevity-my-task\\prompt.md","metadataPath":"C:\\repo\\.brevity\\tasks.json"}}`),
	}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskNew, slug: "my-task"})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}

	if len(client.calls) != 1 || client.calls[0] != "task-new:my-task" {
		t.Fatalf("calls = %#v, want task new only", client.calls)
	}

	output := stdout.String()
	for _, want := range []string{
		"Task new: success",
		"slug: my-task",
		"branch: task/my-task",
		"worktreePath: C:\\repo\\worktrees\\active\\brevity-my-task",
		"promptPath: C:\\repo\\worktrees\\active\\brevity-my-task\\prompt.md",
		"metadataPath: C:\\repo\\.brevity\\tasks.json",
		"- refresh-runtime-state",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunTaskNewReturnsErrorWhenResultFails(t *testing.T) {
	client := &fakeRuntimeClient{
		taskNew: []byte(`{"schema":"brevity.command-result.v1","command":"task new","success":false,"severity":"error","errors":[{"code":"task-already-exists","message":"Task metadata already exists: my-task","details":{"slug":"my-task","metadataPath":"C:\\repo\\.brevity\\tasks.json"}}],"payload":{}}`),
	}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskNew, slug: "my-task"})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "task new reported success=false") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "slug: my-task") {
		t.Fatalf("output missing slug fallback:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "error: task-already-exists: Task metadata already exists: my-task") {
		t.Fatalf("output missing structured error:\n%s", stdout.String())
	}
}

func TestRunProviderSetUsesClientAndRendersResult(t *testing.T) {
	client := &fakeRuntimeClient{
		providerSet: []byte(`{"schema":"brevity.command-result.v1","command":"provider set","success":true,"severity":"info","suggestedNextActions":["refresh-runtime-state"],"payload":{"provider":"gemini","previousStatus":"unknown","newStatus":"capacity-degraded","note":"busy"}}`),
	}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandProviderSet, provider: "gemini", status: "capacity-degraded"})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}

	if len(client.calls) != 1 || client.calls[0] != "provider-set:gemini:capacity-degraded" {
		t.Fatalf("calls = %#v, want provider set only", client.calls)
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

func TestRunProviderResetReturnsErrorWhenResultFails(t *testing.T) {
	client := &fakeRuntimeClient{
		providerReset: []byte(`{"schema":"brevity.command-result.v1","command":"provider reset","success":false,"severity":"error","errors":[{"code":"invalid-provider","message":"Invalid provider: nope"}],"payload":{"provider":"nope","previousStatus":"unknown","newStatus":"unknown"}}`),
	}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandProviderReset, provider: "nope"})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "provider reset reported success=false") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "error: invalid-provider: Invalid provider: nope") {
		t.Fatalf("output missing structured error:\n%s", stdout.String())
	}
}
