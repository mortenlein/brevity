package commands

import "strings"

type ID string

const (
	DashboardID          ID = "dashboard"
	ProviderSetID        ID = "provider-set"
	ProviderResetID      ID = "provider-reset"
	TaskContextRefreshID ID = "context-refresh"
	DoctorID             ID = "doctor"
	TaskCleanupID        ID = "task-cleanup"
	TaskNewID            ID = "task-new"
	TaskRunID            ID = "task-run"
	TaskRuntimeInfoID    ID = "task-runtime-info"
	TaskRunsID           ID = "task-runs"
	TaskRunsReconcileID  ID = "task-runs-reconcile"
	TaskRunsRetentionID  ID = "task-runs-retention"
	TaskRunsCompactID    ID = "task-runs-compact"
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
		Usage: "brevity doctor",
	}
	ProviderSet = Command{
		ID:    ProviderSetID,
		Words: []string{"provider", "set"},
		Usage: "brevity provider set <provider> <status>",
	}
	ProviderReset = Command{
		ID:    ProviderResetID,
		Words: []string{"provider", "reset"},
		Usage: "brevity provider reset <provider>",
	}
	TaskContextRefresh = Command{
		ID:    TaskContextRefreshID,
		Words: []string{"task", "context", "refresh"},
		Usage: "brevity task context refresh <slug>",
	}
	TaskNew = Command{
		ID:    TaskNewID,
		Words: []string{"task", "new"},
		Usage: "brevity task new <slug>",
	}
	TaskRun = Command{
		ID:    TaskRunID,
		Words: []string{"task", "run"},
		Usage: "brevity task run <slug> --execute [--profile <profile>] [--smoke]",
	}
	TaskRuntimeInfo = Command{
		ID:    TaskRuntimeInfoID,
		Words: []string{"task", "runtime-info"},
		Usage: "brevity task runtime-info <slug>",
	}
	TaskRuns = Command{
		ID:    TaskRunsID,
		Words: []string{"task", "runs"},
		Usage: "brevity task runs <slug>",
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
	TaskCleanup = Command{
		ID:    TaskCleanupID,
		Words: []string{"task", "cleanup"},
		Usage: "brevity task cleanup <slug> --force",
	}
)

var UsageCommands = []Command{
	Dashboard,
	Doctor,
	ProviderSet,
	ProviderReset,
	TaskContextRefresh,
	TaskNew,
	TaskRun,
	TaskRuntimeInfo,
	TaskRuns,
	TaskRunsReconcile,
	TaskRunsRetention,
	TaskRunsCompact,
	TaskCleanup,
}
