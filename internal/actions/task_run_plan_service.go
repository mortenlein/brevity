package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/preflight"
	"github.com/mortenlein/brevity/internal/providers"
	"github.com/mortenlein/brevity/internal/state"
)

const taskRunPlanPayloadSchema = "brevity.task-run-plan.v1"

type TaskRunPlanService struct {
	Store state.Store
	Now   func() time.Time
	RunID func(string, time.Time) string
}

func (service TaskRunPlanService) Plan(slug string, profile string) (contracts.CommandResult, error) {
	store := service.Store
	if store.RepoRoot == "" {
		var err error
		store, err = state.NewStore("")
		if err != nil {
			return contracts.CommandResult{}, err
		}
	}
	now := service.now()
	tasks, _, err := state.LoadTasks(store)
	if err != nil {
		return contracts.CommandResult{}, err
	}
	task, ok := findPlanTask(tasks, slug)
	if !ok {
		return taskRunPlanError(slug, "task-not-found", "Task not found: "+slug, nil), nil
	}

	preflightResult, err := preflight.Run(preflight.Options{RepoRoot: store.RepoRoot, Action: preflight.ActionTaskRun, Slug: slug, Profile: profile, Now: service.now})
	if err != nil {
		return contracts.CommandResult{}, err
	}

	config, configWarnings := providers.ReadRunConfig(store)
	health, healthMissing, healthErr := loadPlanProviderHealth(store)
	worker, resolverWarnings, resolverBlockers := providers.Resolve(config, task.Provider, task.Profile, profile, health)
	worktreePath := firstPlanNonEmpty(task.WorktreePath, planWorktreePath(task.Worktree))
	promptPath := firstPlanNonEmpty(task.PromptPath, planPromptPath(task.Prompt))
	promptFreshness := planPromptFreshness(promptPath, task.PromptRefreshedAt)
	runID := service.runID(slug, now)
	logRoot := filepath.Join(store.BrevityRoot(), "logs", slug)
	payload := contracts.TaskRunPlanPayload{
		Schema:                      taskRunPlanPayloadSchema,
		Version:                     1,
		Slug:                        task.Key(),
		TaskState:                   firstPlanNonEmpty(task.NormalizedState, task.Status),
		Provider:                    worker.Provider,
		Profile:                     worker.Profile,
		Model:                       worker.Model,
		WorktreePath:                worktreePath,
		PromptPath:                  promptPath,
		PromptStatus:                promptFreshness,
		PromptFreshness:             promptFreshness,
		RunIDPlan:                   runID,
		LogPathPlan:                 filepath.Join(logRoot, runID+".log"),
		StdoutPathPlan:              filepath.Join(logRoot, runID+".out"),
		StderrPathPlan:              filepath.Join(logRoot, runID+".err"),
		ApprovalMode:                "future-confirmation-required",
		ExecutionKind:               "worker-provider",
		ProviderExecutionWouldOccur: true,
		ProviderExecution:           true,
		LongRunning:                 true,
		Streaming:                   false,
		IsolatedWorktreeRequired:    true,
		DryRunOnly:                  true,
		NoExecutionOccurred:         true,
		Authority:                   "native-go",
		GeneratedAt:                 now.UTC().Format(time.RFC3339),
		ExpectedStateMutations:      []string{"append .brevity/runs.jsonl", "update task worker lifecycle", "write worker log files"},
		ExpectedFilesWritten:        []string{filepath.Join(store.BrevityRoot(), "runs.jsonl"), filepath.Join(logRoot, runID+".log"), filepath.Join(logRoot, runID+".out"), filepath.Join(logRoot, runID+".err")},
		SafetyNotes:                 []string{"Plan generation is read-only.", "No worker/provider process was launched.", "No .brevity/runs.jsonl record was written."},
		Unsupported:                 []string{},
		Warnings:                    []contracts.ResultMessage{},
		Blockers:                    []contracts.ResultMessage{},
	}
	payload.Warnings = append(payload.Warnings, configWarnings...)
	payload.Warnings = append(payload.Warnings, resolverWarnings...)
	payload.Blockers = append(payload.Blockers, resolverBlockers...)
	payload.Warnings = append(payload.Warnings, providerHealthLoadMessages(healthMissing, healthErr, false)...)
	payload.Blockers = append(payload.Blockers, providerHealthLoadMessages(healthMissing, healthErr, true)...)
	payload.Blockers = append(payload.Blockers, preflightBlockers(preflightResult)...)
	payload.Warnings = append(payload.Warnings, preflightWarnings(preflightResult)...)
	if promptFreshness == "stale" {
		payload.Warnings = append(payload.Warnings, contracts.ResultMessage{Code: "prompt-stale", Message: "Task prompt appears stale compared with promptRefreshedAt."})
	}
	command, commandBlocker := planWorkerCommand(worker, worktreePath, promptPath)
	payload.WorkerCommand = command
	if commandBlocker != nil {
		payload.Blockers = append(payload.Blockers, *commandBlocker)
	}
	if profile == "" {
		payload.Unsupported = append(payload.Unsupported, "No explicit profile was provided; provider default profile resolution was used.")
	}

	success := len(payload.Blockers) == 0
	severity := "info"
	var errors []contracts.ResultMessage
	if !success {
		severity = "error"
		errors = append(errors, payload.Blockers...)
	} else if len(payload.Warnings) > 0 {
		severity = "warning"
	}
	payloadJSON, _ := json.Marshal(payload)
	return contracts.CommandResult{
		Schema:               contracts.CommandResultSchema,
		Command:              "task run",
		Success:              success,
		Severity:             severity,
		Warnings:             payload.Warnings,
		Errors:               errors,
		SuggestedNextActions: []string{"Review the native execution envelope before enabling future worker execution."},
		Payload:              payloadJSON,
	}, nil
}

func planWorkerCommand(worker providers.Resolved, worktreePath string, promptPath string) (contracts.TaskRunWorkerCommand, *contracts.ResultMessage) {
	command := contracts.TaskRunWorkerCommand{Provider: worker.Provider, Command: worker.Config.Command, WorkingDirectory: worktreePath, ExecutionPolicy: worker.Config.ExecutionPolicy}
	for name := range worker.Config.Env {
		command.EnvironmentNames = append(command.EnvironmentNames, name)
	}
	sort.Strings(command.EnvironmentNames)
	switch worker.Provider {
	case "codex":
		args := []string{firstPlanNonEmpty(worker.Config.Mode, "exec"), "-C", worktreePath}
		if sandbox := firstPlanNonEmpty(worker.Config.Sandbox, "workspace-write"); sandbox != "" && sandbox != "none" {
			args = append(args, "-s", sandbox)
		}
		if worker.Model != "" {
			args = append(args, "-m", worker.Model)
		}
		if worker.Config.Profile != "" {
			args = append(args, "-p", worker.Config.Profile)
		}
		args = append(args, promptPath)
		command.Arguments = args
	case "gemini":
		args := []string{}
		if sandbox := worker.Config.Sandbox; sandbox != "" && sandbox != "none" {
			args = append(args, "-s")
		}
		if worker.Model != "" {
			args = append(args, "-m", worker.Model)
		}
		approvalMode := worker.Config.ApprovalMode
		if approvalMode == "" && worker.Config.SkipTrust {
			approvalMode = "yolo"
		}
		if approvalMode != "" {
			args = append(args, "--approval-mode", approvalMode)
		}
		if worker.Config.SkipTrust {
			args = append(args, "--skip-trust")
		}
		args = append(args, "-p", promptPath)
		command.Arguments = args
	case "antigravity":
		args := []string{"--worktree", worktreePath, "--prompt", promptPath}
		if worker.Model != "" {
			args = append(args, "--model", worker.Model)
		}
		command.Arguments = args
	default:
		return command, &contracts.ResultMessage{Code: "unsupported-provider", Message: "Unsupported worker provider: " + worker.Provider}
	}
	if strings.TrimSpace(worktreePath) == "" {
		return command, &contracts.ResultMessage{Code: "missing-worktree-path", Message: "Task metadata is missing worktreePath."}
	}
	if strings.TrimSpace(promptPath) == "" {
		return command, &contracts.ResultMessage{Code: "missing-prompt-path", Message: "Task metadata is missing promptPath."}
	}
	if info, err := os.Stat(promptPath); err != nil || info.IsDir() {
		return command, &contracts.ResultMessage{Code: "prompt-missing", Message: "Task prompt file does not exist."}
	}
	command.Display = formatDisplay(append([]string{command.Command}, command.Arguments...))
	return command, nil
}

func loadPlanProviderHealth(store state.Store) (state.ProviderHealthState, bool, error) {
	health, missing, err := state.LoadProviderHealth(store)
	if err != nil || missing {
		return state.ProviderHealthState{}, missing, err
	}
	return health, false, nil
}

func providerHealthLoadMessages(missing bool, err error, blockers bool) []contracts.ResultMessage {
	if err != nil {
		if blockers {
			return []contracts.ResultMessage{{Code: "provider-health-error", Message: err.Error()}}
		}
		return nil
	}
	if missing {
		if blockers {
			return nil
		}
		return []contracts.ResultMessage{{Code: "provider-health-missing", Message: ".brevity/provider-health.json is missing; provider readiness is unknown."}}
	}
	return nil
}

func preflightBlockers(result preflight.Result) []contracts.ResultMessage {
	messages := []contracts.ResultMessage{}
	for _, blocker := range result.Blockers {
		messages = append(messages, contracts.ResultMessage{Code: "preflight-blocked", Message: blocker})
	}
	return messages
}

func preflightWarnings(result preflight.Result) []contracts.ResultMessage {
	messages := []contracts.ResultMessage{}
	for _, warning := range result.Warnings {
		messages = append(messages, contracts.ResultMessage{Code: "preflight-warning", Message: warning})
	}
	return messages
}

func taskRunPlanError(slug string, code string, message string, details map[string]any) contracts.CommandResult {
	payload, _ := json.Marshal(contracts.TaskRunPlanPayload{Schema: taskRunPlanPayloadSchema, Version: 1, Slug: slug, Authority: "native-go", DryRunOnly: true, NoExecutionOccurred: true})
	return contracts.CommandResult{Schema: contracts.CommandResultSchema, Command: "task run", Success: false, Severity: "error", Warnings: []contracts.ResultMessage{}, Errors: []contracts.ResultMessage{{Code: code, Message: message, Details: details}}, Payload: payload}
}

func (service TaskRunPlanService) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func (service TaskRunPlanService) runID(slug string, now time.Time) string {
	if service.RunID != nil {
		return service.RunID(slug, now)
	}
	return fmt.Sprintf("%s-%s", slug, now.UTC().Format("20060102T150405Z"))
}

func planPromptFreshness(path string, refreshedAt string) string {
	if strings.TrimSpace(path) == "" {
		return "missing-path"
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "missing"
	}
	refreshed, err := time.Parse(time.RFC3339Nano, refreshedAt)
	if strings.TrimSpace(refreshedAt) == "" || err != nil {
		return "unknown"
	}
	if info.ModTime().After(refreshed.Add(time.Second)) {
		return "stale"
	}
	return "fresh"
}

func findPlanTask(tasks state.Tasks, slug string) (state.Task, bool) {
	for _, task := range tasks.Items {
		if task.Key() == strings.TrimSpace(slug) {
			return task, true
		}
	}
	return state.Task{}, false
}

func firstPlanNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func planWorktreePath(worktree *state.TaskWorktree) string {
	if worktree == nil {
		return ""
	}
	return worktree.Path
}

func planPromptPath(prompt *state.TaskPrompt) string {
	if prompt == nil {
		return ""
	}
	return prompt.Path
}

func formatDisplay(parts []string) string {
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.ContainsAny(part, " \t\"") {
			quoted = append(quoted, `"`+strings.ReplaceAll(part, `"`, `\"`)+`"`)
		} else {
			quoted = append(quoted, part)
		}
	}
	return strings.Join(quoted, " ")
}
