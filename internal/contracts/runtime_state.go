package contracts

import (
	"encoding/json"
	"errors"
	"fmt"
)

const RuntimeStateSchema = "brevity.runtime-state.v1"

type RuntimeState struct {
	Schema                string            `json:"schema"`
	RepoRoot              string            `json:"repoRoot"`
	GeneratedAt           string            `json:"generatedAt"`
	Providers             Providers         `json:"providers"`
	Queue                 *RuntimeQueue     `json:"queue,omitempty"`
	Executions            *RuntimeExecution `json:"executions,omitempty"`
	TaskCounts            TaskCounts        `json:"taskCounts"`
	Tasks                 []TaskSummary     `json:"tasks"`
	Cleanup               *Cleanup          `json:"cleanup,omitempty"`
	OrphanedTaskWorktrees []WorktreeRecord  `json:"orphanedTaskWorktrees"`
	ActiveWorktreeCount   int               `json:"activeWorktreeCount"`
	ActiveWorktrees       []WorktreeRecord  `json:"activeWorktrees"`
	SuggestedNextActions  []string          `json:"suggestedNextActions"`
	Groups                map[string]any    `json:"groups"`
	Extras                map[string]any    `json:"-"`
}

type Providers struct {
	Summary ProviderSummary            `json:"summary"`
	Health  map[string]ProviderHealth  `json:"health"`
	Extras  map[string]json.RawMessage `json:"-"`
}

type ProviderSummary struct {
	Total       int `json:"total"`
	Degraded    int `json:"degraded"`
	Unavailable int `json:"unavailable"`
}

type ProviderHealth struct {
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt"`
	Note      string `json:"note"`
}

type RuntimeQueue struct {
	Path                     string         `json:"path"`
	State                    string         `json:"state"`
	Version                  int            `json:"version"`
	SupportedVersion         int            `json:"supportedVersion"`
	TotalItems               int            `json:"totalItems"`
	CountsByStatus           map[string]int `json:"countsByStatus"`
	ReservedItems            int            `json:"reservedItems"`
	OldestQueuedItemAge      string         `json:"oldestQueuedItemAge,omitempty"`
	NewestQueuedItemAge      string         `json:"newestQueuedItemAge,omitempty"`
	Plan                     *QueuePlan     `json:"plan,omitempty"`
	DuplicateIDs             []string       `json:"duplicateIds,omitempty"`
	InvalidItems             []string       `json:"invalidItems,omitempty"`
	InvalidReservations      []string       `json:"invalidReservations,omitempty"`
	UnsupportedFutureVersion bool           `json:"unsupportedFutureVersion"`
	Error                    string         `json:"error,omitempty"`
}

type QueuePlan struct {
	State            string `json:"state"`
	Runnable         int    `json:"runnable"`
	Skipped          int    `json:"skipped"`
	Reserved         int    `json:"reserved"`
	NextRunnableTask string `json:"nextRunnableTask,omitempty"`
	FirstSkipReason  string `json:"firstSkipReason,omitempty"`
	Error            string `json:"error,omitempty"`
	ReadOnly         bool   `json:"readOnly"`
}

type RuntimeExecution struct {
	Path                     string         `json:"path"`
	State                    string         `json:"state"`
	Version                  int            `json:"version"`
	SupportedVersion         int            `json:"supportedVersion"`
	TotalExecutions          int            `json:"totalExecutions"`
	CountsByStatus           map[string]int `json:"countsByStatus"`
	NewestExecutionTask      string         `json:"newestExecutionTask,omitempty"`
	NewestExecutionStatus    string         `json:"newestExecutionStatus,omitempty"`
	NewestPlannedTask        string         `json:"newestPlannedTask,omitempty"`
	DuplicateIDs             []string       `json:"duplicateIds,omitempty"`
	InvalidRecords           []string       `json:"invalidRecords,omitempty"`
	UnsupportedFutureVersion bool           `json:"unsupportedFutureVersion"`
	Error                    string         `json:"error,omitempty"`
}

type TaskCounts struct {
	Tracked       int `json:"tracked"`
	Runnable      int `json:"runnable"`
	Blocked       int `json:"blocked"`
	Stale         int `json:"stale"`
	ProviderGated int `json:"providerGated"`
	Review        int `json:"review"`
}

type TaskSummary struct {
	Slug                  string              `json:"slug"`
	Status                string              `json:"status"`
	NormalizedState       string              `json:"normalizedState"`
	Provider              string              `json:"provider"`
	ProviderHealth        string              `json:"providerHealth"`
	ProviderGated         bool                `json:"providerGated"`
	Profile               string              `json:"profile"`
	LastProvider          string              `json:"lastProvider"`
	LastProfile           string              `json:"lastProfile"`
	Branch                string              `json:"branch"`
	WorktreePath          string              `json:"worktreePath"`
	WorktreeExists        *bool               `json:"worktreeExists,omitempty"`
	Worktree              *TaskWorktree       `json:"worktree,omitempty"`
	Context               *TaskRuntimeContext `json:"context,omitempty"`
	PromptPath            string              `json:"promptPath"`
	PromptExists          bool                `json:"promptExists,omitempty"`
	PromptStatus          string              `json:"promptStatus,omitempty"`
	PromptRefreshedAt     string              `json:"promptRefreshedAt,omitempty"`
	Execution             *TaskExecution      `json:"execution,omitempty"`
	WorkerStatus          string              `json:"workerStatus"`
	LastRunID             string              `json:"lastRunId"`
	LastExitCode          any                 `json:"lastExitCode"`
	LastLogPath           string              `json:"lastLogPath"`
	RunCount              int                 `json:"runCount"`
	LatestRunID           string              `json:"latestRunId"`
	LatestRunLogPath      string              `json:"latestRunLogPath"`
	LatestRunExitCode     any                 `json:"latestRunExitCode"`
	LatestRunProvider     string              `json:"latestRunProvider"`
	LatestRunProfile      string              `json:"latestRunProfile"`
	LatestRunWorkerStatus string              `json:"latestRunWorkerStatus"`
	LatestRunStartedAt    string              `json:"latestRunStartedAt"`
	LatestRunFinishedAt   string              `json:"latestRunFinishedAt"`
	LatestRunFailureType  string              `json:"latestRunFailureType"`
	LatestRunIncomplete   bool                `json:"latestRunIncomplete"`
	LatestRunStale        bool                `json:"latestRunStale"`
	LatestRunAgeMinutes   *float64            `json:"latestRunAgeMinutes,omitempty"`
	LatestRunSource       string              `json:"latestRunSource"`
	LatestRun             json.RawMessage     `json:"latestRun,omitempty"`
	Extras                map[string]any      `json:"-"`
}

type TaskWorktree struct {
	Exists bool   `json:"exists"`
	Path   string `json:"path"`
	Branch string `json:"branch"`
}

type WorktreeRecord struct {
	Path     string `json:"path"`
	Branch   string `json:"branch"`
	Head     string `json:"head,omitempty"`
	Bare     bool   `json:"bare,omitempty"`
	Detached bool   `json:"detached,omitempty"`
}

type TaskRuntimeContext struct {
	Exists                bool     `json:"exists"`
	Path                  string   `json:"path"`
	MaterializedFileCount int      `json:"materializedFileCount"`
	MissingFiles          []string `json:"missingFiles"`
}

type TaskExecution struct {
	Status    string `json:"status"`
	LastRunID string `json:"lastRunId"`
	LogPath   string `json:"lastLogPath"`
}

type Cleanup struct {
	Summary               *CleanupSummary    `json:"summary,omitempty"`
	OrphanedTaskWorktrees []CleanupCandidate `json:"orphanedTaskWorktrees,omitempty"`
	OrphanedTaskBranches  []CleanupCandidate `json:"orphanedTaskBranches,omitempty"`
	Extras                map[string]any     `json:"-"`
}

type CleanupSummary struct {
	TotalCandidates           int            `json:"totalCandidates"`
	RequiresInspectionCount   int            `json:"requiresInspectionCount"`
	RemovableByExecuteCount   int            `json:"removableByExecuteCount"`
	OrphanedTaskWorktreeCount int            `json:"orphanedTaskWorktreeCount"`
	OrphanedTaskBranchCount   int            `json:"orphanedTaskBranchCount"`
	BySeverity                map[string]int `json:"bySeverity"`
	ByCategory                map[string]int `json:"byCategory"`
}

type CleanupCandidate struct {
	ID                    string   `json:"id"`
	Severity              string   `json:"severity"`
	Category              string   `json:"category"`
	Path                  string   `json:"path"`
	Branch                string   `json:"branch"`
	Dirty                 bool     `json:"dirty"`
	DirtyReasons          []string `json:"dirtyReasons"`
	SuggestedCommands     []string `json:"suggestedCommands"`
	RemovableByExecute    *bool    `json:"removableByExecute,omitempty"`
	DestructiveIfUnmerged *bool    `json:"destructiveIfUnmerged,omitempty"`
}

func ParseRuntimeState(input []byte) (RuntimeState, error) {
	var state RuntimeState
	if err := json.Unmarshal(input, &state); err != nil {
		return RuntimeState{}, fmt.Errorf("invalid runtime state JSON: %w", err)
	}

	if state.Schema != RuntimeStateSchema {
		if state.Schema == "" {
			return RuntimeState{}, errors.New("unsupported runtime state schema: missing schema")
		}
		return RuntimeState{}, fmt.Errorf("unsupported runtime state schema: %s", state.Schema)
	}

	return state, nil
}
