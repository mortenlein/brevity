package pscontract

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"
)

type fakeProcessRunner struct {
	result     ProcessResult
	err        error
	executable string
	args       []string
}

func (runner *fakeProcessRunner) Run(ctx context.Context, executable string, args []string) (ProcessResult, error) {
	runner.executable = executable
	runner.args = append([]string{}, args...)
	return runner.result, runner.err
}

func TestPowerShellCommandRunnerExecutesReadOnlyDescriptorWithArgv(t *testing.T) {
	script := tempScript(t)
	process := &fakeProcessRunner{result: ProcessResult{Stdout: "provider ok", ExitCode: 0}}
	descriptor := findDescriptor(t, ActionProviderStatus)

	result := PowerShellCommandRunner{
		Executable: "pwsh-test.exe",
		ScriptPath: script,
		Process:    process,
	}.Run(context.Background(), descriptor)

	if !result.Success() || result.Stdout != "provider ok" {
		t.Fatalf("result = %#v, want success with stdout", result)
	}
	wantArgs := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "provider", "status"}
	if process.executable != "pwsh-test.exe" {
		t.Fatalf("executable = %q", process.executable)
	}
	if !reflect.DeepEqual(process.args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", process.args, wantArgs)
	}
}

func TestPowerShellCommandRunnerBlocksMutatingDescriptor(t *testing.T) {
	result := PowerShellCommandRunner{ScriptPath: tempScript(t)}.Run(context.Background(), findDescriptor(t, ActionRunWorker))
	if result.Success() || result.Error != "action is not an enabled read-only command" {
		t.Fatalf("result = %#v, want blocked read-only boundary", result)
	}
}

func TestPowerShellCommandRunnerReportsMissingScript(t *testing.T) {
	result := PowerShellCommandRunner{ScriptPath: "missing-brevity.ps1"}.Run(context.Background(), findDescriptor(t, ActionTaskStatus))
	if result.Success() || result.Error == "" {
		t.Fatalf("result = %#v, want missing script error", result)
	}
}

func TestPowerShellCommandRunnerReportsExecutableFailureAndStderr(t *testing.T) {
	process := &fakeProcessRunner{
		result: ProcessResult{Stderr: "no powershell", ExitCode: 1},
		err:    errors.New("exec failed"),
	}
	result := PowerShellCommandRunner{ScriptPath: tempScript(t), Process: process}.Run(context.Background(), findDescriptor(t, ActionTaskStatus))
	if result.Success() || result.Error != "no powershell" || result.Stderr != "no powershell" {
		t.Fatalf("result = %#v, want stderr failure", result)
	}
}

func TestPowerShellCommandRunnerReportsTimeout(t *testing.T) {
	process := fakeProcessRunnerFunc(func(ctx context.Context, executable string, args []string) (ProcessResult, error) {
		<-ctx.Done()
		return ProcessResult{ExitCode: 1}, ctx.Err()
	})
	result := PowerShellCommandRunner{ScriptPath: tempScript(t), Timeout: time.Nanosecond, Process: process}.Run(context.Background(), findDescriptor(t, ActionProviderStatus))
	if !result.TimedOut || result.Error == "" {
		t.Fatalf("result = %#v, want timeout", result)
	}
}

type fakeProcessRunnerFunc func(context.Context, string, []string) (ProcessResult, error)

func (fn fakeProcessRunnerFunc) Run(ctx context.Context, executable string, args []string) (ProcessResult, error) {
	return fn(ctx, executable, args)
}

func tempScript(t *testing.T) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "brevity-*.ps1")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return file.Name()
}
