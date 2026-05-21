package pscontract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const DefaultCommandTimeout = 15 * time.Second

type ProcessRunner interface {
	Run(ctx context.Context, executable string, args []string) (ProcessResult, error)
}

type ProcessResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type ExecProcessRunner struct{}

func (runner ExecProcessRunner) Run(ctx context.Context, executable string, args []string) (ProcessResult, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	result := ProcessResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}
	if err != nil {
		result.ExitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
		return result, err
	}
	return result, nil
}

type PowerShellCommandRunner struct {
	Executable string
	ScriptPath string
	Timeout    time.Duration
	Process    ProcessRunner
}

func (runner PowerShellCommandRunner) Run(ctx context.Context, descriptor CommandDescriptor) ExecutionResult {
	started := time.Now()
	result := ExecutionResult{
		ActionID:            descriptor.ActionID,
		CommandDisplayLabel: descriptor.Label,
		ExitCode:            1,
		StartedAt:           started,
		RefreshAfter:        descriptor.RefreshAfterSuccess,
	}

	if descriptor.Mutating || !descriptor.Enabled {
		result.Error = "action is not an enabled read-only command"
		result.CompletedAt = time.Now()
		return result
	}

	scriptPath := strings.TrimSpace(runner.ScriptPath)
	if scriptPath == "" {
		scriptPath = `.\\brevity.ps1`
	}
	if _, err := os.Stat(scriptPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			result.Error = fmt.Sprintf("PowerShell script not found: %s", scriptPath)
		} else {
			result.Error = fmt.Sprintf("PowerShell script unavailable: %v", err)
		}
		result.CompletedAt = time.Now()
		return result
	}

	invocation, err := BuildInvocation(descriptor, scriptPath, nil, false)
	if err != nil {
		result.Error = err.Error()
		result.CompletedAt = time.Now()
		return result
	}
	if executable := strings.TrimSpace(runner.Executable); executable != "" {
		invocation.Executable = executable
	}

	timeout := runner.Timeout
	if timeout <= 0 {
		timeout = timeoutForCategory(descriptor.TimeoutCategory)
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	process := runner.Process
	if process == nil {
		process = ExecProcessRunner{}
	}

	processResult, err := process.Run(runCtx, invocation.Executable, invocation.ExecArgs())
	result.Stdout = processResult.Stdout
	result.Stderr = processResult.Stderr
	result.ExitCode = processResult.ExitCode
	if err != nil {
		switch {
		case errors.Is(runCtx.Err(), context.DeadlineExceeded):
			result.TimedOut = true
			result.Error = fmt.Sprintf("command exceeded %s timeout", timeout)
		case errors.Is(runCtx.Err(), context.Canceled), errors.Is(ctx.Err(), context.Canceled):
			result.Canceled = true
			result.Error = "command was canceled"
		case errors.Is(err, exec.ErrNotFound):
			result.Error = fmt.Sprintf("PowerShell executable not found: %s", invocation.Executable)
		case strings.TrimSpace(processResult.Stderr) != "":
			result.Error = strings.TrimSpace(processResult.Stderr)
		default:
			result.Error = err.Error()
		}
	}
	if strings.TrimSpace(result.Stdout) == "" && strings.TrimSpace(result.Stderr) == "" && result.Error == "" {
		result.Error = "command returned empty output"
	}
	result.CompletedAt = time.Now()
	return result
}

func timeoutForCategory(category TimeoutCategory) time.Duration {
	switch category {
	case TimeoutLong:
		return 2 * time.Minute
	case TimeoutNormal:
		return 30 * time.Second
	default:
		return DefaultCommandTimeout
	}
}
