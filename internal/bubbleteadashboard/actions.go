package bubbleteadashboard

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mortenlein/brevity/internal/actions"
	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/pscontract"
	"github.com/mortenlein/brevity/internal/runtimeclient"
	"github.com/mortenlein/brevity/internal/state"
)

type ActionKind string

const (
	ActionKindReadOnly ActionKind = "read-only"
	ActionKindMutating ActionKind = "mutating"
	ActionKindDryRun   ActionKind = "dry-run"
	ActionKindFuture   ActionKind = "future"
)

type ActionID string

const (
	ActionRefreshState   ActionID = "refresh-state"
	ActionProviderStatus ActionID = "provider-status"
	ActionTaskStatus     ActionID = "task-status"
	ActionStartTask      ActionID = "start-task"
	ActionRefreshContext ActionID = "refresh-context"
	ActionRunWorker      ActionID = "run-worker"
	ActionMergeTask      ActionID = "merge-task"
	ActionCleanupTask    ActionID = "cleanup-task"
)

type ActionDescriptor struct {
	ID                   ActionID
	Label                string
	Kind                 ActionKind
	Enabled              bool
	Description          string
	Shortcut             string
	ConfirmationRequired bool
	ExecutesViaBridge    bool
	Command              pscontract.CommandDescriptor
}

// DashboardCommandBridge is the only place where executable TUI actions may
// cross into the authoritative PowerShell/runtime layer. Future mutating
// actions should be added here before they are enabled in the palette.
type DashboardCommandBridge interface {
	RefreshRuntimeState() (contracts.RuntimeState, error)
	ExecuteReadOnlyAction(action ActionDescriptor) pscontract.ExecutionResult
	ExecuteMutatingAction(action ActionDescriptor, commandArgs []string) pscontract.ExecutionResult
	ExecuteTaskStart(slug string, repoRoot string) pscontract.ExecutionResult
	ExecuteContextRefresh(slug string, repoRoot string) pscontract.ExecutionResult
	LoadTaskRunPlan(slug string, profile string, repoRoot string) pscontract.ExecutionResult
	ExecuteTaskRun(slug string, profile string, repoRoot string) pscontract.ExecutionResult
}

type RuntimeClientCommandBridge struct {
	Client runtimeclient.Client
}

func (bridge RuntimeClientCommandBridge) RefreshRuntimeState() (contracts.RuntimeState, error) {
	output, err := bridge.Client.RuntimeStateJSON()
	if err != nil {
		return contracts.RuntimeState{}, err
	}
	return contracts.ParseRuntimeState(output)
}

func (bridge RuntimeClientCommandBridge) ExecuteReadOnlyAction(action ActionDescriptor) pscontract.ExecutionResult {
	return pscontract.PowerShellCommandRunner{ScriptPath: bridge.scriptPath()}.Run(context.Background(), action.Command)
}

func (bridge RuntimeClientCommandBridge) ExecuteMutatingAction(action ActionDescriptor, commandArgs []string) pscontract.ExecutionResult {
	return pscontract.PowerShellCommandRunner{ScriptPath: bridge.scriptPath()}.RunMutating(context.Background(), action.Command, commandArgs)
}

func (bridge RuntimeClientCommandBridge) ExecuteTaskStart(slug string, repoRoot string) pscontract.ExecutionResult {
	started := time.Now()
	store, err := state.NewStore(repoRoot)
	result := pscontract.ExecutionResult{
		ActionID:            pscontract.ActionStartTask,
		CommandDisplayLabel: "Start task",
		StartedAt:           started,
		CompletedAt:         time.Now(),
		RefreshAfter:        true,
	}
	if err != nil {
		result.ExitCode = 1
		result.Error = err.Error()
		return result
	}
	commandResult, runErr := actions.TaskStartService{Store: store}.Start(slug)
	output, marshalErr := json.Marshal(commandResult)
	if marshalErr != nil {
		result.ExitCode = 1
		result.Error = marshalErr.Error()
		return result
	}
	result.Stdout = string(output)
	if runErr != nil || !commandResult.Success {
		result.ExitCode = 1
		if runErr != nil {
			result.Error = runErr.Error()
		}
	}
	result.CompletedAt = time.Now()
	return result
}

func (bridge RuntimeClientCommandBridge) ExecuteContextRefresh(slug string, repoRoot string) pscontract.ExecutionResult {
	started := time.Now()
	store, err := state.NewStore(repoRoot)
	result := pscontract.ExecutionResult{
		ActionID:            pscontract.ActionRefreshContext,
		CommandDisplayLabel: "Refresh context",
		StartedAt:           started,
		CompletedAt:         time.Now(),
		RefreshAfter:        true,
	}
	if err != nil {
		result.ExitCode = 1
		result.Error = err.Error()
		return result
	}
	commandResult, runErr := actions.TaskContextRefreshService{Store: store}.Refresh(slug)
	output, marshalErr := json.Marshal(commandResult)
	if marshalErr != nil {
		result.ExitCode = 1
		result.Error = marshalErr.Error()
		return result
	}
	result.Stdout = string(output)
	if runErr != nil || !commandResult.Success {
		result.ExitCode = 1
		if runErr != nil {
			result.Error = runErr.Error()
		}
	}
	result.CompletedAt = time.Now()
	return result
}

func (bridge RuntimeClientCommandBridge) LoadTaskRunPlan(slug string, profile string, repoRoot string) pscontract.ExecutionResult {
	started := time.Now()
	output, err := runtimeclient.NewNativeClient(repoRoot).TaskRunPlanJSON(slug, profile)
	result := pscontract.ExecutionResult{
		ActionID:            pscontract.ActionRunWorker,
		CommandDisplayLabel: "Run worker plan",
		StartedAt:           started,
		CompletedAt:         time.Now(),
		Stdout:              string(output),
		ExitCode:            0,
	}
	if err != nil {
		result.ExitCode = 1
		result.Error = err.Error()
	}
	return result
}

func (bridge RuntimeClientCommandBridge) ExecuteTaskRun(slug string, profile string, repoRoot string) pscontract.ExecutionResult {
	started := time.Now()
	output, err := runtimeclient.NewNativeClient(repoRoot).TaskRunJSON(slug, profile, false)
	result := pscontract.ExecutionResult{
		ActionID:            pscontract.ActionRunWorker,
		CommandDisplayLabel: "Run worker",
		StartedAt:           started,
		CompletedAt:         time.Now(),
		Stdout:              string(output),
		ExitCode:            0,
		RefreshAfter:        true,
	}
	if err != nil {
		result.ExitCode = 1
		result.Error = err.Error()
	}
	var commandResult contracts.CommandResult
	if parseErr := json.Unmarshal(output, &commandResult); parseErr == nil && !commandResult.Success {
		result.ExitCode = 1
	}
	return result
}

func (bridge RuntimeClientCommandBridge) scriptPath() string {
	switch client := bridge.Client.(type) {
	case runtimeclient.PowerShellClient:
		return client.ScriptPath
	case *runtimeclient.PowerShellClient:
		return client.ScriptPath
	case runtimeclient.NativeClient:
		if client.RepoRoot != "" {
			return filepath.Join(client.RepoRoot, "brevity.ps1")
		}
	case *runtimeclient.NativeClient:
		if client.RepoRoot != "" {
			return filepath.Join(client.RepoRoot, "brevity.ps1")
		}
	}
	return `.\\brevity.ps1`
}

func actionDescriptors() []ActionDescriptor {
	descriptors := pscontract.DashboardDescriptors()
	actions := make([]ActionDescriptor, 0, len(descriptors))
	for _, descriptor := range descriptors {
		action := ActionDescriptor{
			ID:                   ActionID(descriptor.ActionID),
			Label:                descriptor.Label,
			Kind:                 ActionKindFuture,
			Enabled:              descriptor.Enabled,
			Description:          descriptor.DisabledReason,
			ConfirmationRequired: descriptor.RequiresConfirmation,
			ExecutesViaBridge:    true,
			Command:              descriptor,
		}
		if !descriptor.Mutating && descriptor.Enabled {
			action.Kind = ActionKindReadOnly
			action.Description = "executable read-only"
			if descriptor.ActionID == pscontract.ActionRefreshState {
				action.Description = "enter refreshes state"
				action.Shortcut = "r"
			}
		} else if descriptor.Mutating {
			action.Kind = ActionKindMutating
		}
		actions = append(actions, action)
	}
	return actions
}

func (model Model) actionDescriptors() []ActionDescriptor {
	actions := actionDescriptors()
	startSlug, startable := model.selectedStartableTaskSlug()
	runTask, runnable := model.selectedRunnableTask()
	refreshSlug, refreshable := model.selectedTaskSlug()
	for index := range actions {
		switch actions[index].ID {
		case ActionStartTask:
			if startable {
				actions[index].Enabled = true
				actions[index].Description = "native start confirmation for " + startSlug
				actions[index].Command.Enabled = true
				actions[index].Command.DisabledReason = ""
				actions[index].Command.SafetyWarning = "Go native task start updates task metadata only; no provider or worker execution."
			} else {
				actions[index].Enabled = false
				actions[index].Description = "select a task row to enable"
				actions[index].Command.Enabled = false
				actions[index].Command.DisabledReason = "select a task row with a slug to enable Start task"
			}
		case ActionRefreshContext:
			if refreshable {
				actions[index].Enabled = true
				actions[index].Description = "native prompt/context refresh for " + refreshSlug
				actions[index].Command.Enabled = true
				actions[index].Command.DisabledReason = ""
			} else {
				actions[index].Enabled = false
				actions[index].Description = "select a task row to enable"
				actions[index].Command.Enabled = false
				actions[index].Command.DisabledReason = "select a task row with a slug to enable Refresh context"
			}
		case ActionRunWorker:
			if runnable {
				actions[index].Enabled = true
				actions[index].Kind = ActionKindMutating
				actions[index].Description = "native provider execution for " + runTask.Slug
				actions[index].ConfirmationRequired = true
				actions[index].Command.Arguments = []string{"--execute"}
				actions[index].Command.Provider = taskProvider(runTask)
				actions[index].Command.Profile = taskProfile(runTask)
				actions[index].Command.Enabled = true
				actions[index].Command.DisabledReason = ""
				actions[index].Command.SafetyWarning = "Native Go will execute the provider command shown by the task-run plan."
			} else {
				actions[index].Enabled = false
				actions[index].Description = "plan preview only; select a runnable task row"
				actions[index].Command.Enabled = false
				actions[index].Command.DisabledReason = "plan preview only; select a runnable task row with slug and runnable state"
			}
		}
	}
	return actions
}

func (model Model) commandForAction(action ActionDescriptor) tea.Cmd {
	if !action.Enabled {
		return nil
	}
	switch action.ID {
	case ActionRefreshState:
		return model.refreshCmd()
	case ActionProviderStatus, ActionTaskStatus:
		if model.commandRun != nil && model.commandRun.status == commandRunning {
			return nil
		}
		return model.executeReadOnlyCmd(action)
	case ActionStartTask:
		if model.commandRun != nil && model.commandRun.status == commandRunning {
			return nil
		}
		slug, ok := model.selectedStartableTaskSlug()
		if !ok {
			return nil
		}
		return model.executeTaskStartCmd(slug)
	case ActionRefreshContext:
		if model.commandRun != nil && model.commandRun.status == commandRunning {
			return nil
		}
		slug, ok := model.selectedTaskSlug()
		if !ok {
			return nil
		}
		return model.executeContextRefreshCmd(slug)
	case ActionRunWorker:
		if model.commandRun != nil && model.commandRun.status == commandRunning {
			return nil
		}
		task, ok := model.selectedRunnableTask()
		if !ok {
			return nil
		}
		return model.executeTaskRunCmd(task.Slug, taskProfile(task))
	default:
		return nil
	}
}

func (model Model) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		state, err := model.dashboardCommandBridge().RefreshRuntimeState()
		if err != nil {
			return refreshMsg{err: err, at: time.Now()}
		}
		return refreshMsg{state: state, at: time.Now()}
	}
}

func (model Model) executeReadOnlyCmd(action ActionDescriptor) tea.Cmd {
	runID := model.nextCommandID + 1
	return func() tea.Msg {
		return commandResultMsg{id: runID, result: model.dashboardCommandBridge().ExecuteReadOnlyAction(action)}
	}
}

func (model Model) executeMutatingCmd(action ActionDescriptor, commandArgs []string) tea.Cmd {
	runID := model.nextCommandID + 1
	args := append([]string{}, commandArgs...)
	return func() tea.Msg {
		return commandResultMsg{id: runID, result: model.dashboardCommandBridge().ExecuteMutatingAction(action, args)}
	}
}

func (model Model) executeTaskStartCmd(slug string) tea.Cmd {
	runID := model.nextCommandID + 1
	repoRoot := model.state.RepoRoot
	return func() tea.Msg {
		return commandResultMsg{id: runID, result: model.dashboardCommandBridge().ExecuteTaskStart(slug, repoRoot)}
	}
}

func (model Model) executeContextRefreshCmd(slug string) tea.Cmd {
	runID := model.nextCommandID + 1
	repoRoot := model.state.RepoRoot
	return func() tea.Msg {
		return commandResultMsg{id: runID, result: model.dashboardCommandBridge().ExecuteContextRefresh(slug, repoRoot)}
	}
}

func (model Model) loadTaskRunPlanCmd(slug string, profile string) tea.Cmd {
	runID := model.nextCommandID + 1
	repoRoot := model.state.RepoRoot
	return func() tea.Msg {
		return commandResultMsg{id: runID, result: model.dashboardCommandBridge().LoadTaskRunPlan(slug, profile, repoRoot)}
	}
}

func (model Model) executeTaskRunCmd(slug string, profile string) tea.Cmd {
	runID := model.nextCommandID + 1
	repoRoot := model.state.RepoRoot
	return func() tea.Msg {
		return commandResultMsg{id: runID, result: model.dashboardCommandBridge().ExecuteTaskRun(slug, profile, repoRoot)}
	}
}

func (model Model) dashboardCommandBridge() DashboardCommandBridge {
	if model.commandBridge != nil {
		return model.commandBridge
	}
	return RuntimeClientCommandBridge{Client: model.client}
}
