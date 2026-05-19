package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mortenlein/brevity/internal/commands"
	"github.com/mortenlein/brevity/internal/dashboard"
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

func TestRunWritesHelp(t *testing.T) {
	var stdout bytes.Buffer
	if err := run(&stdout, []string{"--help"}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	wants := []string{
		"dashboard remains read-only",
		`.\brevity.ps1 ... --json`,
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

func TestParseOptionsRejectsTaskRunWithoutExecute(t *testing.T) {
	_, err := parseOptions([]string{"task", "run", "my-task", "--smoke"})
	if err == nil {
		t.Fatal("parseOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "requires --execute") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunTaskRunFailsBeforeClientWithoutExecute(t *testing.T) {
	client := &fakeRuntimeClient{}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskRun, slug: "my-task"})
	if err == nil {
		t.Fatal("runWithOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "requires --execute") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v, want no PowerShell calls", client.calls)
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

func TestRunTaskRunUsesClientAndRendersResult(t *testing.T) {
	client := &fakeRuntimeClient{
		taskRun: []byte(`{"schema":"brevity.command-result.v1","command":"task run","success":true,"severity":"info","suggestedNextActions":["Refresh runtime state."],"payload":{"slug":"my-task","runId":"20260519T170000Z-my-task","provider":"smoke","profile":"smoke","worktreePath":"C:\\repo\\worktrees\\active\\brevity-my-task","promptPath":"C:\\repo\\worktrees\\active\\brevity-my-task\\prompt.md","executionMode":"sync","startedAt":"2026-05-19T17:00:00Z","finishedAt":"2026-05-19T17:00:01Z","exitCode":0,"workerStatus":"succeeded","failureType":null,"logPath":"C:\\repo\\.brevity\\logs\\my-task\\20260519T170000Z-my-task.log"}}`),
	}

	var stdout bytes.Buffer
	err := runWithOptions(&stdout, client, cliOptions{kind: commandTaskRun, slug: "my-task", execute: true, profile: "smoke", smoke: true})
	if err != nil {
		t.Fatalf("runWithOptions returned error: %v", err)
	}

	if len(client.calls) != 1 || client.calls[0] != "task-run:my-task:smoke:true" {
		t.Fatalf("calls = %#v, want task run only", client.calls)
	}

	output := stdout.String()
	for _, want := range []string{
		"Task run: success",
		"slug: my-task",
		"provider: smoke",
		"profile: smoke",
		"workerStatus: succeeded",
		"runId: 20260519T170000Z-my-task",
		"startedAt: 2026-05-19T17:00:00Z",
		"finishedAt: 2026-05-19T17:00:01Z",
		"exitCode: 0",
		"logPath: C:\\repo\\.brevity\\logs\\my-task\\20260519T170000Z-my-task.log",
		"- Refresh runtime state.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunTaskRunReturnsErrorWhenWorkerFails(t *testing.T) {
	client := &fakeRuntimeClient{
		taskRun: []byte(`{"schema":"brevity.command-result.v1","command":"task run","success":false,"severity":"error","errors":[{"code":"worker-exit-failed","message":"Worker failed."}],"suggestedNextActions":["Review the worker log."],"payload":{"slug":"my-task","runId":"run-failed","provider":"codex","profile":"default","startedAt":"2026-05-19T17:00:00Z","finishedAt":"2026-05-19T17:00:01Z","exitCode":1,"workerStatus":"failed","failureType":"worker-exit-failed","logPath":"C:\\repo\\.brevity\\logs\\my-task\\run-failed.log"}}`),
	}

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
		"exitCode: 1",
		"failureType: worker-exit-failed",
		"error: worker-exit-failed: Worker failed.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
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
