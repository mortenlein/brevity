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
