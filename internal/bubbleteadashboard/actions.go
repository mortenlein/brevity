package bubbleteadashboard

import (
	"context"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/pscontract"
	"github.com/mortenlein/brevity/internal/runtimeclient"
)

type ActionKind string

const (
	ActionKindReadOnly ActionKind = "read-only"
	ActionKindMutating ActionKind = "mutating"
	ActionKindFuture   ActionKind = "future"
)

type ActionID string

const (
	ActionRefreshState   ActionID = "refresh-state"
	ActionProviderStatus ActionID = "provider-status"
	ActionTaskStatus     ActionID = "task-status"
	ActionStartTask      ActionID = "start-task"
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

func (model Model) commandForAction(action ActionDescriptor) tea.Cmd {
	if !action.Enabled {
		return nil
	}
	switch action.ID {
	case ActionRefreshState:
		return model.refreshCmd()
	case ActionProviderStatus, ActionTaskStatus:
		return model.executeReadOnlyCmd(action)
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
	return func() tea.Msg {
		return commandResultMsg{result: model.dashboardCommandBridge().ExecuteReadOnlyAction(action)}
	}
}

func (model Model) dashboardCommandBridge() DashboardCommandBridge {
	if model.commandBridge != nil {
		return model.commandBridge
	}
	return RuntimeClientCommandBridge{Client: model.client}
}
