package actions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/providers"
	"github.com/mortenlein/brevity/internal/state"
)

const taskRunExecutionModeNative = "native-go-provider"

type TaskRunExecuteService struct {
	Store  state.Store
	Now    func() time.Time
	RunID  func(string, time.Time) string
	Runner ProviderRunner
}

type ProviderRunner interface {
	Run(context.Context, ProviderRunRequest) ProviderRunResult
}

type ProviderRunRequest struct {
	Command          string
	Arguments        []string
	WorkingDirectory string
	Environment      map[string]string
	Secrets          []string
	StartedAt        time.Time
}

type ProviderRunResult struct {
	StartedAt  time.Time
	FinishedAt time.Time
	Stdout     string
	Stderr     string
	ExitCode   int
	Err        error
	TimedOut   bool
	Canceled   bool
}

type ExecProviderRunner struct {
	Now     func() time.Time
	Timeout time.Duration
}

func (service TaskRunExecuteService) Execute(ctx context.Context, slug string, profile string) (contracts.CommandResult, error) {
	store := service.Store
	if store.RepoRoot == "" {
		var err error
		store, err = state.NewStore("")
		if err != nil {
			return contracts.CommandResult{}, err
		}
	}

	planService := TaskRunPlanService{Store: store, Now: service.Now, RunID: service.RunID}
	planResult, err := planService.Plan(slug, profile)
	if err != nil {
		return contracts.CommandResult{}, err
	}
	plan, err := contracts.ParseTaskRunPlanPayload(planResult)
	if err != nil {
		return contracts.CommandResult{}, err
	}
	if !planResult.Success || len(plan.Blockers) > 0 {
		return taskRunExecutionErrorFromPlan(plan, planResult.Errors, "plan-blocked", "Native task-run plan is blocked; no provider process was launched."), nil
	}

	config, _ := providers.ReadRunConfig(store)
	health, _, _ := loadPlanProviderHealth(store)
	worker, _, _ := providers.Resolve(config, plan.Provider, plan.Profile, profile, health)
	runner := service.Runner
	if runner == nil {
		runner = ExecProviderRunner{Now: service.now}
	}
	secrets := secretValues(worker.Config.Env)
	result := runner.Run(ctx, ProviderRunRequest{
		Command:          plan.WorkerCommand.Command,
		Arguments:        append([]string{}, plan.WorkerCommand.Arguments...),
		WorkingDirectory: plan.WorkerCommand.WorkingDirectory,
		Environment:      cloneStringMap(worker.Config.Env),
		Secrets:          secrets,
		StartedAt:        service.now().UTC(),
	})
	result.Stdout = RedactSecrets(result.Stdout, secrets)
	result.Stderr = RedactSecrets(result.Stderr, secrets)

	logs, logErr := writeRunLogs(plan, result)
	if logErr != nil {
		return taskRunExecutionFailure(plan, result, "log-write-failed", logErr.Error()), nil
	}
	record := state.RunRecord{
		RunID:        plan.RunIDPlan,
		Slug:         plan.Slug,
		Provider:     plan.Provider,
		Profile:      plan.Profile,
		StartedAt:    result.StartedAt.UTC().Format(time.RFC3339Nano),
		FinishedAt:   result.FinishedAt.UTC().Format(time.RFC3339Nano),
		ExitCode:     result.ExitCode,
		WorkerStatus: workerStatus(result),
		FailureType:  failureType(result),
		LogPath:      logs.LogPath,
		StdoutPath:   logs.StdoutPath,
		StderrPath:   logs.StderrPath,
		Summary:      runSummary(result),
		Message:      runMessage(result),
	}
	if err := state.AppendRun(store, record, state.AppendRunOptions{}); err != nil {
		return taskRunExecutionFailure(plan, result, "runs-append-failed", err.Error()), nil
	}
	if err := state.UpdateTaskRunMetadata(store, record, state.TaskUpdateOptions{}); err != nil {
		return taskRunExecutionFailure(plan, result, "task-metadata-update-failed", err.Error()), nil
	}

	payload := executionPayload(plan, record)
	payloadJSON, _ := json.Marshal(payload)
	commandResult := contracts.CommandResult{
		Schema:   contracts.CommandResultSchema,
		Command:  "task run",
		Success:  record.WorkerStatus == "succeeded",
		Severity: "info",
		Warnings: []contracts.ResultMessage{},
		Errors:   []contracts.ResultMessage{},
		Payload:  payloadJSON,
	}
	if !commandResult.Success {
		commandResult.Severity = "error"
		commandResult.Errors = append(commandResult.Errors, contracts.ResultMessage{Code: record.FailureType, Message: record.Message})
	}
	return commandResult, nil
}

func (runner ExecProviderRunner) Run(ctx context.Context, request ProviderRunRequest) ProviderRunResult {
	started := request.StartedAt
	if started.IsZero() {
		started = runner.now().UTC()
	}
	if runner.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, runner.Timeout)
		defer cancel()
	}
	command := exec.CommandContext(ctx, request.Command, request.Arguments...)
	command.Dir = request.WorkingDirectory
	command.Env = os.Environ()
	for key, value := range request.Environment {
		command.Env = append(command.Env, key+"="+value)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	finished := runner.now().UTC()
	exitCode := 0
	if err != nil {
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	return ProviderRunResult{
		StartedAt:  started,
		FinishedAt: finished,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		ExitCode:   exitCode,
		Err:        err,
		TimedOut:   errors.Is(ctx.Err(), context.DeadlineExceeded),
		Canceled:   errors.Is(ctx.Err(), context.Canceled) && !errors.Is(ctx.Err(), context.DeadlineExceeded),
	}
}

func (runner ExecProviderRunner) now() time.Time {
	if runner.Now != nil {
		return runner.Now()
	}
	return time.Now()
}

type runLogPaths struct {
	LogPath    string
	StdoutPath string
	StderrPath string
}

func writeRunLogs(plan contracts.TaskRunPlanPayload, result ProviderRunResult) (runLogPaths, error) {
	paths := runLogPaths{LogPath: plan.LogPathPlan, StdoutPath: plan.StdoutPathPlan, StderrPath: plan.StderrPathPlan}
	for _, path := range []string{paths.LogPath, paths.StdoutPath, paths.StderrPath} {
		if strings.TrimSpace(path) == "" {
			return paths, fmt.Errorf("run log path is missing")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return paths, fmt.Errorf("create log directory: %w", err)
		}
	}
	if err := os.WriteFile(paths.StdoutPath, []byte(result.Stdout), 0o644); err != nil {
		return paths, fmt.Errorf("write stdout log: %w", err)
	}
	if err := os.WriteFile(paths.StderrPath, []byte(result.Stderr), 0o644); err != nil {
		return paths, fmt.Errorf("write stderr log: %w", err)
	}
	summary := fmt.Sprintf("runId: %s\nstartedAt: %s\nfinishedAt: %s\nexitCode: %d\nworkerStatus: %s\n\n[stdout]\n%s\n[stderr]\n%s", plan.RunIDPlan, result.StartedAt.UTC().Format(time.RFC3339Nano), result.FinishedAt.UTC().Format(time.RFC3339Nano), result.ExitCode, workerStatus(result), result.Stdout, result.Stderr)
	if err := os.WriteFile(paths.LogPath, []byte(summary), 0o644); err != nil {
		return paths, fmt.Errorf("write combined log: %w", err)
	}
	return paths, nil
}

func RedactSecrets(value string, secrets []string) string {
	for _, secret := range secrets {
		if strings.TrimSpace(secret) != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}

func secretValues(values map[string]string) []string {
	secrets := []string{}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			secrets = append(secrets, value)
		}
	}
	return secrets
}

func cloneStringMap(values map[string]string) map[string]string {
	clone := map[string]string{}
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func (service TaskRunExecuteService) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func workerStatus(result ProviderRunResult) string {
	if result.ExitCode == 0 && result.Err == nil {
		return "succeeded"
	}
	return "failed"
}

func failureType(result ProviderRunResult) string {
	if workerStatus(result) == "succeeded" {
		return ""
	}
	if result.TimedOut {
		return "timeout"
	}
	if result.Canceled {
		return "canceled"
	}
	return "provider-exit-nonzero"
}

func runSummary(result ProviderRunResult) string {
	if workerStatus(result) == "succeeded" {
		return "Provider command completed successfully."
	}
	return "Provider command failed."
}

func runMessage(result ProviderRunResult) string {
	if result.Err != nil {
		return result.Err.Error()
	}
	return runSummary(result)
}

func executionPayload(plan contracts.TaskRunPlanPayload, record state.RunRecord) contracts.TaskRunExecutionPayload {
	return contracts.TaskRunExecutionPayload{
		Slug:          record.Slug,
		RunID:         record.RunID,
		Provider:      record.Provider,
		Profile:       record.Profile,
		WorktreePath:  plan.WorktreePath,
		PromptPath:    plan.PromptPath,
		ExecutionMode: taskRunExecutionModeNative,
		StartedAt:     record.StartedAt,
		FinishedAt:    record.FinishedAt,
		ExitCode:      record.ExitCode,
		WorkerStatus:  record.WorkerStatus,
		FailureType:   record.FailureType,
		LogPath:       record.LogPath,
	}
}

func taskRunExecutionErrorFromPlan(plan contracts.TaskRunPlanPayload, errors []contracts.ResultMessage, code string, message string) contracts.CommandResult {
	payloadJSON, _ := json.Marshal(contracts.TaskRunExecutionPayload{Slug: plan.Slug, RunID: plan.RunIDPlan, Provider: plan.Provider, Profile: plan.Profile, WorktreePath: plan.WorktreePath, PromptPath: plan.PromptPath, ExecutionMode: taskRunExecutionModeNative, WorkerStatus: "blocked", FailureType: code, LogPath: plan.LogPathPlan})
	if len(errors) == 0 {
		errors = []contracts.ResultMessage{{Code: code, Message: message}}
	}
	return contracts.CommandResult{Schema: contracts.CommandResultSchema, Command: "task run", Success: false, Severity: "error", Warnings: []contracts.ResultMessage{}, Errors: errors, Payload: payloadJSON}
}

func taskRunExecutionFailure(plan contracts.TaskRunPlanPayload, result ProviderRunResult, code string, message string) contracts.CommandResult {
	record := state.RunRecord{RunID: plan.RunIDPlan, Slug: plan.Slug, Provider: plan.Provider, Profile: plan.Profile, StartedAt: result.StartedAt.UTC().Format(time.RFC3339Nano), FinishedAt: result.FinishedAt.UTC().Format(time.RFC3339Nano), ExitCode: result.ExitCode, WorkerStatus: "failed", FailureType: code, LogPath: plan.LogPathPlan}
	payloadJSON, _ := json.Marshal(executionPayload(plan, record))
	return contracts.CommandResult{Schema: contracts.CommandResultSchema, Command: "task run", Success: false, Severity: "error", Warnings: []contracts.ResultMessage{}, Errors: []contracts.ResultMessage{{Code: code, Message: message}}, Payload: payloadJSON}
}
