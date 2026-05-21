package bubbleteadashboard

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mortenlein/brevity/internal/contracts"
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
	ActionRefreshState ActionID = "refresh-state"
	ActionStartTask    ActionID = "start-task"
	ActionRunWorker    ActionID = "run-worker"
	ActionMergeTask    ActionID = "merge-task"
	ActionCleanupTask  ActionID = "cleanup-task"
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
}

// DashboardCommandBridge is the only place where executable TUI actions may
// cross into the authoritative PowerShell/runtime layer. Future mutating
// actions should be added here before they are enabled in the palette.
type DashboardCommandBridge interface {
	RefreshRuntimeState() (contracts.RuntimeState, error)
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

func actionDescriptors() []ActionDescriptor {
	return []ActionDescriptor{
		{ID: ActionStartTask, Label: "Start task", Kind: ActionKindFuture, Description: "future via PowerShell", ConfirmationRequired: true, ExecutesViaBridge: true},
		{ID: ActionRunWorker, Label: "Run worker", Kind: ActionKindFuture, Description: "future via PowerShell", ConfirmationRequired: true, ExecutesViaBridge: true},
		{ID: ActionMergeTask, Label: "Merge task", Kind: ActionKindFuture, Description: "future via PowerShell", ConfirmationRequired: true, ExecutesViaBridge: true},
		{ID: ActionCleanupTask, Label: "Cleanup task", Kind: ActionKindFuture, Description: "future via PowerShell", ConfirmationRequired: true, ExecutesViaBridge: true},
		{ID: ActionRefreshState, Label: "Refresh state", Kind: ActionKindReadOnly, Enabled: true, Description: "enter refreshes state", Shortcut: "r", ExecutesViaBridge: true},
	}
}

func (model Model) commandForAction(action ActionDescriptor) tea.Cmd {
	if !action.Enabled {
		return nil
	}
	switch action.ID {
	case ActionRefreshState:
		return model.refreshCmd()
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

func (model Model) dashboardCommandBridge() DashboardCommandBridge {
	if model.commandBridge != nil {
		return model.commandBridge
	}
	return RuntimeClientCommandBridge{Client: model.client}
}
