package commands

import "strings"

type ID string

const (
	DashboardID          ID = "dashboard"
	RuntimeStateID       ID = "runtime-state"
	ProviderStatusID     ID = "provider-status"
	ProviderSetID        ID = "provider-set"
	ProviderResetID      ID = "provider-reset"
	InitID               ID = "init"
	TaskContextRefreshID ID = "context-refresh"
	TaskStatusID         ID = "task-status"
	DoctorID             ID = "doctor"
	TaskCleanupID        ID = "task-cleanup"
	TaskMergeID          ID = "task-merge"
	TaskPreflightID      ID = "task-preflight"
	TaskNewID            ID = "task-new"
	TaskActivateID       ID = "task-activate"
	TaskSpecID           ID = "task-spec"
	TaskStartID          ID = "task-start"
	TaskRunID            ID = "task-run"
	TaskRuntimeInfoID    ID = "task-runtime-info"
	TaskDetailID         ID = "task-detail"
	TaskRunsID           ID = "task-runs"
	TaskRunsReconcileID  ID = "task-runs-reconcile"
	TaskRunsRetentionID  ID = "task-runs-retention"
	TaskRunsCompactID    ID = "task-runs-compact"
	RunsInspectID        ID = "runs-inspect"
	RunsCompactID        ID = "runs-compact"
	CleanupInspectID     ID = "cleanup-inspect"
	CleanupPlanID        ID = "cleanup-plan"
	CleanupExecuteID     ID = "cleanup-execute"
	SupportMatrixID      ID = "support-matrix"
)

type Command struct {
	ID    ID
	Words []string
	Usage string
}

func (command Command) Name() string {
	return strings.Join(command.Words, " ")
}

func (command Command) Args(extra ...string) []string {
	args := append([]string{}, command.Words...)
	args = append(args, extra...)
	return args
}

func (command Command) JSONArgs(extra ...string) []string {
	args := command.Args(extra...)
	return append(args, "--json")
}

var (
	Dashboard = Command{
		ID:    DashboardID,
		Usage: "brevity [--once]",
	}
	Doctor = Command{
		ID:    DoctorID,
		Words: []string{"doctor"},
		Usage: "brevity doctor [--json]",
	}
	RuntimeState = Command{
		ID:    RuntimeStateID,
		Words: []string{"runtime", "state"},
		Usage: "brevity runtime state --json",
	}
	ProviderSet = Command{
		ID:    ProviderSetID,
		Words: []string{"provider", "set"},
		Usage: "brevity provider set <provider> <status> [--note <note>]",
	}
	ProviderStatus = Command{
		ID:    ProviderStatusID,
		Words: []string{"provider", "status"},
		Usage: "brevity provider status",
	}
	ProviderReset = Command{
		ID:    ProviderResetID,
		Words: []string{"provider", "reset"},
		Usage: "brevity provider reset <provider>",
	}
	Init = Command{
		ID:    InitID,
		Words: []string{"init"},
		Usage: "brevity init [--repair] [--json]",
	}
	TaskContextRefresh = Command{
		ID:    TaskContextRefreshID,
		Words: []string{"task", "refresh-context"},
		Usage: "brevity task refresh-context <slug> [--json]",
	}
	TaskNew = Command{
		ID:    TaskNewID,
		Words: []string{"task", "new"},
		Usage: "brevity task new <slug>",
	}
	TaskActivate = Command{
		ID:    TaskActivateID,
		Words: []string{"task", "activate"},
		Usage: "brevity task activate <slug> [--json]",
	}
	TaskSpec = Command{
		ID:    TaskSpecID,
		Words: []string{"task", "spec"},
		Usage: "brevity task spec <slug> [--json]",
	}
	TaskStart = Command{
		ID:    TaskStartID,
		Words: []string{"task", "start"},
		Usage: "brevity task start <slug>",
	}
	TaskStatus = Command{
		ID:    TaskStatusID,
		Words: []string{"task", "status"},
		Usage: "brevity task status",
	}
	TaskRun = Command{
		ID:    TaskRunID,
		Words: []string{"task", "run"},
		Usage: "brevity task run <slug> --execute [--profile <profile>] [--smoke]",
	}
	TaskRuntimeInfo = Command{
		ID:    TaskRuntimeInfoID,
		Words: []string{"task", "runtime-info"},
		Usage: "brevity task runtime-info <slug> [--json]",
	}
	TaskDetail = Command{
		ID:    TaskDetailID,
		Words: []string{"task", "detail"},
		Usage: "brevity task detail <slug> [--json]",
	}
	TaskRuns = Command{
		ID:    TaskRunsID,
		Words: []string{"task", "runs"},
		Usage: "brevity task runs <slug> [--json]",
	}
	TaskRunsReconcile = Command{
		ID:    TaskRunsReconcileID,
		Words: []string{"task", "runs", "reconcile"},
		Usage: "brevity task runs reconcile --dry-run",
	}
	TaskRunsRetention = Command{
		ID:    TaskRunsRetentionID,
		Words: []string{"task", "runs", "retention"},
		Usage: "brevity task runs retention --dry-run",
	}
	TaskRunsCompact = Command{
		ID:    TaskRunsCompactID,
		Words: []string{"task", "runs", "compact"},
		Usage: "brevity task runs compact --dry-run",
	}
	RunsInspect = Command{
		ID:    RunsInspectID,
		Words: []string{"runs", "inspect"},
		Usage: "brevity runs inspect [--json]",
	}
	RunsCompact = Command{
		ID:    RunsCompactID,
		Words: []string{"runs", "compact"},
		Usage: "brevity runs compact --plan|--force [--json]",
	}
	TaskCleanup = Command{
		ID:    TaskCleanupID,
		Words: []string{"task", "cleanup"},
		Usage: "brevity task cleanup <slug> [--plan] [--force] [--json]",
	}
	CleanupInspect = Command{
		ID:    CleanupInspectID,
		Words: []string{"cleanup", "inspect"},
		Usage: "brevity cleanup inspect [--json]",
	}
	CleanupPlan = Command{
		ID:    CleanupPlanID,
		Words: []string{"cleanup", "plan"},
		Usage: "brevity cleanup plan <candidate-id>|--all [--json]",
	}
	CleanupExecute = Command{
		ID:    CleanupExecuteID,
		Words: []string{"cleanup", "execute"},
		Usage: "brevity cleanup execute <candidate-id>|--all --force [--json]",
	}
	SupportMatrix = Command{
		ID:    SupportMatrixID,
		Words: []string{"support", "matrix"},
		Usage: "brevity support matrix [--json]",
	}
	TaskMerge = Command{
		ID:    TaskMergeID,
		Words: []string{"task", "merge"},
		Usage: "brevity task merge <slug> [--plan] [--json]",
	}
	TaskPreflight = Command{
		ID:    TaskPreflightID,
		Words: []string{"task", "preflight"},
		Usage: "brevity task preflight <new|start|run|merge|cleanup> <slug> [--json]",
	}
)

var UsageCommands = []Command{
	Dashboard,
	RuntimeState,
	Doctor,
	ProviderStatus,
	ProviderSet,
	ProviderReset,
	Init,
	TaskContextRefresh,
	TaskStatus,
	TaskNew,
	TaskActivate,
	TaskSpec,
	TaskStart,
	TaskRun,
	TaskPreflight,
	TaskRuntimeInfo,
	TaskDetail,
	TaskRuns,
	TaskRunsReconcile,
	TaskRunsRetention,
	TaskRunsCompact,
	RunsInspect,
	RunsCompact,
	CleanupInspect,
	CleanupPlan,
	CleanupExecute,
	TaskCleanup,
	TaskMerge,
	SupportMatrix,
}
