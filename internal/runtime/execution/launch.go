package execution

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
)

type LaunchPayload struct {
	ExecutionID string            `json:"executionId"`
	Task        string            `json:"task"`
	Provider    string            `json:"provider"`
	Profile     string            `json:"profile"`
	Worktree    string            `json:"worktree"`
	Prompt      string            `json:"prompt"`
	Command     string            `json:"command"`
	Arguments   []string          `json:"arguments"`
	Argv        []string          `json:"argv"`
	Environment map[string]string `json:"-"`
}

type LaunchResult struct {
	ExecutionID string        `json:"executionId"`
	Task        string        `json:"task"`
	Provider    string        `json:"provider"`
	Profile     string        `json:"profile"`
	Worktree    string        `json:"worktree"`
	Prompt      string        `json:"prompt"`
	Argv        []string      `json:"argv"`
	ExitCode    int           `json:"exitCode"`
	FinalStatus string        `json:"finalStatus"`
	StartedAt   string        `json:"startedAt"`
	FinishedAt  string        `json:"finishedAt"`
	Duration    time.Duration `json:"duration"`
	Error       string        `json:"error,omitempty"`
}

type Launcher struct {
	Store  Store
	Runner ProcessRunner
	Stdout io.Writer
	Stderr io.Writer
	Now    Clock
}

type ProcessRunner interface {
	Run(context.Context, ProcessRequest) ProcessResult
}

type ProcessRequest struct {
	Command          string
	Arguments        []string
	WorkingDirectory string
	Environment      map[string]string
	Stdout           io.Writer
	Stderr           io.Writer
}

type ProcessResult struct {
	ExitCode int
	Err      error
}

type ExecRunner struct{}

func (launcher Launcher) Launch(ctx context.Context, payload LaunchPayload) (LaunchResult, error) {
	if strings.TrimSpace(payload.ExecutionID) == "" {
		return LaunchResult{}, errors.New("execution id is required")
	}
	if strings.TrimSpace(payload.Command) == "" {
		return LaunchResult{}, errors.New("provider command is required")
	}
	stdout := launcher.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := launcher.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	runner := launcher.Runner
	if runner == nil {
		runner = ExecRunner{}
	}

	started := launcher.now().UTC()
	if _, err := launcher.Store.MarkLaunching(payload.ExecutionID); err != nil {
		return LaunchResult{}, err
	}

	result := runner.Run(ctx, ProcessRequest{
		Command:          payload.Command,
		Arguments:        append([]string{}, payload.Arguments...),
		WorkingDirectory: payload.Worktree,
		Environment:      cloneEnvironment(payload.Environment),
		Stdout:           stdout,
		Stderr:           stderr,
	})
	finished := launcher.now().UTC()

	finalStatus := StatusCompleted
	transition := launcher.Store.MarkCompleted
	if result.Err != nil || result.ExitCode != 0 {
		finalStatus = StatusFailed
		transition = launcher.Store.MarkFailed
	}
	if _, err := transition(payload.ExecutionID); err != nil {
		return LaunchResult{}, err
	}

	launchResult := LaunchResult{
		ExecutionID: payload.ExecutionID,
		Task:        payload.Task,
		Provider:    payload.Provider,
		Profile:     payload.Profile,
		Worktree:    payload.Worktree,
		Prompt:      payload.Prompt,
		Argv:        append([]string{}, payload.Argv...),
		ExitCode:    result.ExitCode,
		FinalStatus: finalStatus,
		StartedAt:   started.Format(time.RFC3339Nano),
		FinishedAt:  finished.Format(time.RFC3339Nano),
		Duration:    finished.Sub(started),
	}
	if result.Err != nil {
		launchResult.Error = result.Err.Error()
		return launchResult, result.Err
	}
	return launchResult, nil
}

func (runner ExecRunner) Run(ctx context.Context, request ProcessRequest) ProcessResult {
	command := exec.CommandContext(ctx, request.Command, request.Arguments...)
	command.Dir = request.WorkingDirectory
	command.Env = os.Environ()
	for key, value := range request.Environment {
		command.Env = append(command.Env, key+"="+value)
	}
	command.Stdout = request.Stdout
	if command.Stdout == nil {
		command.Stdout = os.Stdout
	}
	command.Stderr = request.Stderr
	if command.Stderr == nil {
		command.Stderr = os.Stderr
	}
	err := command.Run()
	if err == nil {
		return ProcessResult{ExitCode: 0}
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return ProcessResult{ExitCode: exitErr.ExitCode(), Err: err}
	}
	return ProcessResult{ExitCode: 1, Err: err}
}

func PayloadFromWorkerCommand(executionID string, task string, provider string, profile string, worktree string, prompt string, command actionsWorkerCommand) LaunchPayload {
	argv := append([]string{command.Command}, command.Arguments...)
	return LaunchPayload{
		ExecutionID: executionID,
		Task:        task,
		Provider:    provider,
		Profile:     profile,
		Worktree:    worktree,
		Prompt:      prompt,
		Command:     command.Command,
		Arguments:   append([]string{}, command.Arguments...),
		Argv:        argv,
	}
}

type actionsWorkerCommand = contracts.TaskRunWorkerCommand

func (launcher Launcher) now() time.Time {
	if launcher.Now != nil {
		return launcher.Now()
	}
	if launcher.Store.Now != nil {
		return launcher.Store.Now()
	}
	return time.Now()
}

func cloneEnvironment(values map[string]string) map[string]string {
	clone := map[string]string{}
	for key, value := range values {
		clone[key] = value
	}
	return clone
}
