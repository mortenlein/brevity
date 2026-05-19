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
	contextRefresh []byte
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

func (client *fakeRuntimeClient) TaskContextRefreshJSON(slug string) ([]byte, error) {
	client.calls = append(client.calls, "context-refresh:"+slug)
	return client.contextRefresh, client.actionErr
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

func TestParseOptionsRejectsUnsupportedProviderCommand(t *testing.T) {
	_, err := parseOptions([]string{"provider", "status"})
	if err == nil {
		t.Fatal("parseOptions returned nil error")
	}
	if !strings.Contains(err.Error(), `unsupported provider command "status"`) {
		t.Fatalf("unexpected error: %v", err)
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
