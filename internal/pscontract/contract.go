package pscontract

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mortenlein/brevity/internal/commands"
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

type ResultMode string

const (
	ResultModeText ResultMode = "text"
	ResultModeJSON ResultMode = "json"
)

type TimeoutCategory string

const (
	TimeoutShort  TimeoutCategory = "short"
	TimeoutNormal TimeoutCategory = "normal"
	TimeoutLong   TimeoutCategory = "long"
)

type ExecutionKind string

const (
	ExecutionKindReadOnly       ExecutionKind = "read-only"
	ExecutionKindStateMutation  ExecutionKind = "state-mutation"
	ExecutionKindWorkerProvider ExecutionKind = "worker-provider"
)

type CommandDescriptor struct {
	ActionID             ActionID
	Label                string
	Command              commands.Command
	Arguments            []string
	Provider             string
	Profile              string
	RequiresWorktree     bool
	ExecutionKind        ExecutionKind
	LongRunning          bool
	StreamsOutput        bool
	Mutating             bool
	Destructive          bool
	ProviderExecuting    bool
	RequiresConfirmation bool
	Enabled              bool
	ResultMode           ResultMode
	TimeoutCategory      TimeoutCategory
	SafetyWarning        string
	DisabledReason       string
	RefreshAfterSuccess  bool
}

func DashboardDescriptors() []CommandDescriptor {
	return []CommandDescriptor{
		{
			ActionID:            ActionRefreshState,
			Label:               "Refresh state",
			Command:             commands.RuntimeState,
			Arguments:           []string{"--json"},
			ExecutionKind:       ExecutionKindReadOnly,
			Enabled:             true,
			ResultMode:          ResultModeJSON,
			TimeoutCategory:     TimeoutShort,
			SafetyWarning:       "Refresh reads runtime state only.",
			DisabledReason:      "",
			RefreshAfterSuccess: false,
		},
		{
			ActionID:            ActionProviderStatus,
			Label:               "Provider status",
			Command:             commands.ProviderStatus,
			ExecutionKind:       ExecutionKindReadOnly,
			Enabled:             true,
			ResultMode:          ResultModeText,
			TimeoutCategory:     TimeoutShort,
			SafetyWarning:       "Provider status reads provider health only.",
			DisabledReason:      "",
			RefreshAfterSuccess: false,
		},
		{
			ActionID:            ActionTaskStatus,
			Label:               "Task status",
			Command:             commands.TaskStatus,
			ExecutionKind:       ExecutionKindReadOnly,
			Enabled:             true,
			ResultMode:          ResultModeText,
			TimeoutCategory:     TimeoutShort,
			SafetyWarning:       "Task status reads task metadata and worktree state only.",
			DisabledReason:      "",
			RefreshAfterSuccess: false,
		},
		{
			ActionID:             ActionStartTask,
			Label:                "Start task",
			Command:              commands.TaskStart,
			ExecutionKind:        ExecutionKindStateMutation,
			Mutating:             true,
			RequiresConfirmation: true,
			Enabled:              false,
			ResultMode:           ResultModeJSON,
			TimeoutCategory:      TimeoutNormal,
			SafetyWarning:        "PowerShell remains authoritative for task state changes.",
			DisabledReason:       "not enabled yet; select a task row to enable Start task",
			RefreshAfterSuccess:  true,
		},
		{
			ActionID:             ActionRunWorker,
			Label:                "Run worker",
			Command:              commands.TaskRun,
			Arguments:            []string{"--execute"},
			RequiresWorktree:     true,
			ExecutionKind:        ExecutionKindWorkerProvider,
			LongRunning:          true,
			StreamsOutput:        true,
			Mutating:             true,
			ProviderExecuting:    true,
			RequiresConfirmation: true,
			Enabled:              false,
			ResultMode:           ResultModeJSON,
			TimeoutCategory:      TimeoutLong,
			SafetyWarning:        "PowerShell remains authoritative for provider and worker execution.",
			DisabledReason:       "future PowerShell action; confirmation required; not enabled yet",
			RefreshAfterSuccess:  true,
		},
		{
			ActionID:             ActionMergeTask,
			Label:                "Merge task",
			Command:              commands.TaskMerge,
			ExecutionKind:        ExecutionKindStateMutation,
			Mutating:             true,
			RequiresConfirmation: true,
			Enabled:              false,
			ResultMode:           ResultModeText,
			TimeoutCategory:      TimeoutNormal,
			SafetyWarning:        "PowerShell remains authoritative for Git branch integration.",
			DisabledReason:       "future PowerShell action; confirmation required; not enabled yet",
			RefreshAfterSuccess:  true,
		},
		{
			ActionID:             ActionCleanupTask,
			Label:                "Cleanup task",
			Command:              commands.TaskCleanup,
			Arguments:            []string{"--force"},
			ExecutionKind:        ExecutionKindStateMutation,
			Mutating:             true,
			Destructive:          true,
			RequiresConfirmation: true,
			Enabled:              false,
			ResultMode:           ResultModeJSON,
			TimeoutCategory:      TimeoutNormal,
			SafetyWarning:        "PowerShell remains authoritative; cleanup may remove worktrees, metadata, or branches.",
			DisabledReason:       "future PowerShell action; confirmation required; not enabled yet",
			RefreshAfterSuccess:  true,
		},
	}
}

type Invocation struct {
	Executable string
	ScriptPath string
	Args       []string
}

func (invocation Invocation) ExecArgs() []string {
	args := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", invocation.ScriptPath}
	args = append(args, invocation.Args...)
	return args
}

func (invocation Invocation) Display() string {
	parts := append([]string{invocation.Executable}, invocation.ExecArgs()...)
	return strings.Join(parts, " ")
}

func BuildInvocation(descriptor CommandDescriptor, scriptPath string, commandArgs []string, allowDisabled bool) (Invocation, error) {
	if !descriptor.Enabled && !allowDisabled {
		return Invocation{}, fmt.Errorf("action %s is disabled", descriptor.ActionID)
	}
	if descriptor.Command.ID == "" {
		return Invocation{}, errors.New("descriptor is missing PowerShell command")
	}
	if strings.TrimSpace(scriptPath) == "" {
		scriptPath = `.\\brevity.ps1`
	}

	args := descriptor.Command.Args(commandArgs...)
	args = append(args, descriptor.Arguments...)
	if descriptor.ResultMode == ResultModeJSON && !containsArg(args, "--json") {
		args = append(args, "--json")
	}
	return Invocation{
		Executable: "powershell.exe",
		ScriptPath: scriptPath,
		Args:       args,
	}, nil
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

type ExecutionResult struct {
	ActionID            ActionID
	CommandDisplayLabel string
	ExitCode            int
	Stdout              string
	Stderr              string
	TimedOut            bool
	Canceled            bool
	Error               string
	StartedAt           time.Time
	CompletedAt         time.Time
	RefreshAfter        bool
}

func (result ExecutionResult) Success() bool {
	return result.ExitCode == 0
}

func (result ExecutionResult) OperatorMessage() string {
	label := strings.TrimSpace(result.CommandDisplayLabel)
	if label == "" {
		label = string(result.ActionID)
	}
	if result.TimedOut {
		return label + " timed out"
	}
	if result.Canceled {
		return label + " was canceled"
	}
	if message := strings.TrimSpace(result.Error); message != "" {
		return fmt.Sprintf("%s failed: %s", label, message)
	}
	if result.Success() {
		return label + " succeeded"
	}
	if stderr := strings.TrimSpace(result.Stderr); stderr != "" {
		return fmt.Sprintf("%s failed with exit code %d: %s", label, result.ExitCode, stderr)
	}
	return fmt.Sprintf("%s failed with exit code %d", label, result.ExitCode)
}

func (result ExecutionResult) ShouldRefresh() bool {
	return result.Success() && result.RefreshAfter
}

type ConfirmationStrength string

const (
	ConfirmationNone        ConfirmationStrength = "none"
	ConfirmationStandard    ConfirmationStrength = "standard"
	ConfirmationDestructive ConfirmationStrength = "destructive"
)

type ConfirmationState struct {
	ActionID ActionID
	Prompt   string
	Strength ConfirmationStrength
}

func NewConfirmationState(descriptor CommandDescriptor) (ConfirmationState, bool) {
	if !descriptor.Enabled || !descriptor.RequiresConfirmation {
		return ConfirmationState{}, false
	}
	strength := ConfirmationStandard
	if descriptor.Destructive {
		strength = ConfirmationDestructive
	}
	return ConfirmationState{
		ActionID: descriptor.ActionID,
		Strength: strength,
		Prompt: fmt.Sprintf(
			"%s is a PowerShell-authoritative action. Go will not write .brevity or mutate task state directly.",
			descriptor.Label,
		),
	}, true
}
