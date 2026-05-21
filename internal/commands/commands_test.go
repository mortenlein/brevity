package commands

import (
	"reflect"
	"testing"
)

func TestCommandNameAndArgs(t *testing.T) {
	if TaskRunsReconcile.Name() != "task runs reconcile" {
		t.Fatalf("Name = %q, want task runs reconcile", TaskRunsReconcile.Name())
	}

	got := ProviderSet.JSONArgs("gemini", "capacity-degraded")
	want := []string{"provider", "set", "gemini", "capacity-degraded", "--json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSONArgs = %#v, want %#v", got, want)
	}
}

func TestUsageCommandsHaveDistinctIDsAndUsage(t *testing.T) {
	seen := map[ID]bool{}
	for _, command := range UsageCommands {
		if command.ID == "" {
			t.Fatal("command missing ID")
		}
		if seen[command.ID] {
			t.Fatalf("duplicate command ID %q", command.ID)
		}
		seen[command.ID] = true
		if command.Usage == "" {
			t.Fatalf("command %q missing usage", command.ID)
		}
	}
}

func TestDiscoveredPowerShellCommandShapes(t *testing.T) {
	tests := []struct {
		name    string
		command Command
		extra   []string
		want    []string
	}{
		{name: "runtime state json", command: RuntimeState, extra: []string{"--json"}, want: []string{"runtime", "state", "--json"}},
		{name: "task status list", command: TaskStatus, want: []string{"task", "status"}},
		{name: "task start", command: TaskNew, extra: []string{"my-task"}, want: []string{"task", "new", "my-task"}},
		{name: "task run", command: TaskRun, extra: []string{"my-task", "--execute", "--profile", "default", "--smoke"}, want: []string{"task", "run", "my-task", "--execute", "--profile", "default", "--smoke"}},
		{name: "task merge", command: TaskMerge, extra: []string{"my-task"}, want: []string{"task", "merge", "my-task"}},
		{name: "task cleanup", command: TaskCleanup, extra: []string{"my-task", "--force"}, want: []string{"task", "cleanup", "my-task", "--force"}},
		{name: "provider status", command: ProviderStatus, want: []string{"provider", "status"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.command.Args(tt.extra...)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Args = %#v, want %#v", got, tt.want)
			}
		})
	}
}
