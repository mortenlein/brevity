package pscontract

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDashboardDescriptorsKeepMutatingActionsDisabled(t *testing.T) {
	var refreshCount int
	for _, descriptor := range DashboardDescriptors() {
		if descriptor.ActionID == ActionRefreshState {
			refreshCount++
			if !descriptor.Enabled || descriptor.Mutating || descriptor.RequiresConfirmation {
				t.Fatalf("refresh descriptor = %#v, want enabled read-only without confirmation", descriptor)
			}
			continue
		}
		if !descriptor.Mutating {
			t.Fatalf("%s Mutating = false, want true", descriptor.ActionID)
		}
		if descriptor.Enabled {
			t.Fatalf("%s Enabled = true, want false", descriptor.ActionID)
		}
		if !descriptor.RequiresConfirmation {
			t.Fatalf("%s RequiresConfirmation = false, want true", descriptor.ActionID)
		}
		if !strings.Contains(descriptor.DisabledReason, "not enabled yet") {
			t.Fatalf("%s DisabledReason = %q, want not-enabled explanation", descriptor.ActionID, descriptor.DisabledReason)
		}
	}
	if refreshCount != 1 {
		t.Fatalf("refresh descriptors = %d, want 1", refreshCount)
	}
}

func TestBuildInvocationUsesDiscreteArgvEntries(t *testing.T) {
	descriptor := findDescriptor(t, ActionRefreshState)

	invocation, err := BuildInvocation(descriptor, `C:\dev path\brevity.ps1`, nil, false)
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	wantArgs := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", `C:\dev path\brevity.ps1`, "runtime", "state", "--json"}
	if invocation.Executable != "powershell.exe" {
		t.Fatalf("Executable = %q, want powershell.exe", invocation.Executable)
	}
	if !reflect.DeepEqual(invocation.ExecArgs(), wantArgs) {
		t.Fatalf("ExecArgs = %#v, want %#v", invocation.ExecArgs(), wantArgs)
	}
}

func TestBuildInvocationKeepsSlugAndPathsAsDiscreteArgs(t *testing.T) {
	descriptor := findDescriptor(t, ActionRunWorker)

	invocation, err := BuildInvocation(descriptor, `.\\brevity.ps1`, []string{"normal-task", "--profile", `C:\profiles\worker path`}, true)
	if err != nil {
		t.Fatalf("BuildInvocation returned error: %v", err)
	}

	want := []string{"task", "run", "normal-task", "--profile", `C:\profiles\worker path`, "--execute", "--json"}
	if !reflect.DeepEqual(invocation.Args, want) {
		t.Fatalf("Args = %#v, want %#v", invocation.Args, want)
	}
}

func TestDisabledMutatingActionDoesNotBuildWithoutExplicitAllow(t *testing.T) {
	descriptor := findDescriptor(t, ActionStartTask)

	if _, err := BuildInvocation(descriptor, `.\\brevity.ps1`, []string{"my-task"}, false); err == nil {
		t.Fatal("BuildInvocation returned nil error for disabled action")
	}

	if _, err := BuildInvocation(descriptor, `.\\brevity.ps1`, []string{"my-task"}, true); err != nil {
		t.Fatalf("BuildInvocation with explicit allow returned error: %v", err)
	}
}

func TestCleanupDescriptorRequiresDestructiveConfirmation(t *testing.T) {
	descriptor := findDescriptor(t, ActionCleanupTask)
	if !descriptor.Destructive || !descriptor.RequiresConfirmation {
		t.Fatalf("cleanup descriptor = %#v, want destructive confirmation", descriptor)
	}

	if _, ok := NewConfirmationState(descriptor); ok {
		t.Fatal("disabled cleanup action entered confirmation, want blocked")
	}

	descriptor.Enabled = true
	confirmation, ok := NewConfirmationState(descriptor)
	if !ok {
		t.Fatal("enabled cleanup action did not create confirmation")
	}
	if confirmation.Strength != ConfirmationDestructive {
		t.Fatalf("confirmation strength = %q, want %q", confirmation.Strength, ConfirmationDestructive)
	}
	if !strings.Contains(confirmation.Prompt, "PowerShell-authoritative") {
		t.Fatalf("confirmation prompt missing authority boundary: %q", confirmation.Prompt)
	}
}

func TestExecutionResultFormatting(t *testing.T) {
	started := time.Date(2026, 5, 21, 8, 0, 0, 0, time.UTC)
	completed := started.Add(time.Second)

	success := ExecutionResult{ActionID: ActionRefreshState, CommandDisplayLabel: "Refresh state", StartedAt: started, CompletedAt: completed}
	if !success.Success() || success.OperatorMessage() != "Refresh state succeeded" {
		t.Fatalf("success result formatted unexpectedly: %#v message=%q", success, success.OperatorMessage())
	}

	failure := ExecutionResult{ActionID: ActionRunWorker, CommandDisplayLabel: "Run worker", ExitCode: 2, Stderr: "worker failed"}
	if failure.Success() || failure.OperatorMessage() != "Run worker failed with exit code 2: worker failed" {
		t.Fatalf("failure result formatted unexpectedly: %#v message=%q", failure, failure.OperatorMessage())
	}

	nonzero := ExecutionResult{ActionID: ActionMergeTask, CommandDisplayLabel: "Merge task", ExitCode: 1}
	if nonzero.OperatorMessage() != "Merge task failed with exit code 1" {
		t.Fatalf("nonzero message = %q", nonzero.OperatorMessage())
	}

	refresh := ExecutionResult{ActionID: ActionCleanupTask, CommandDisplayLabel: "Cleanup task", RefreshAfter: true}
	if !refresh.ShouldRefresh() {
		t.Fatal("successful refresh-after result should request refresh")
	}
	refresh.ExitCode = 1
	if refresh.ShouldRefresh() {
		t.Fatal("failed refresh-after result should not request refresh")
	}
}

func findDescriptor(t *testing.T, id ActionID) CommandDescriptor {
	t.Helper()
	for _, descriptor := range DashboardDescriptors() {
		if descriptor.ActionID == id {
			return descriptor
		}
	}
	t.Fatalf("descriptor %s not found", id)
	return CommandDescriptor{}
}
