package contracts

import (
	"encoding/json"
	"errors"
	"fmt"
)

const RuntimeStateSchema = "brevity.runtime-state.v1"

type RuntimeState struct {
	Schema               string         `json:"schema"`
	RepoRoot             string         `json:"repoRoot"`
	GeneratedAt          string         `json:"generatedAt"`
	Providers            Providers      `json:"providers"`
	TaskCounts           TaskCounts     `json:"taskCounts"`
	Tasks                []TaskSummary  `json:"tasks"`
	Cleanup              *Cleanup       `json:"cleanup,omitempty"`
	SuggestedNextActions []string       `json:"suggestedNextActions"`
	Groups               map[string]any `json:"groups"`
	Extras               map[string]any `json:"-"`
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

type TaskCounts struct {
	Tracked       int `json:"tracked"`
	Runnable      int `json:"runnable"`
	Blocked       int `json:"blocked"`
	Stale         int `json:"stale"`
	ProviderGated int `json:"providerGated"`
	Review        int `json:"review"`
}

type TaskSummary struct {
	Slug            string          `json:"slug"`
	Status          string          `json:"status"`
	NormalizedState string          `json:"normalizedState"`
	WorktreePath    string          `json:"worktreePath"`
	Worktree        *TaskWorktree   `json:"worktree,omitempty"`
	Execution       *TaskExecution  `json:"execution,omitempty"`
	LatestRun       json.RawMessage `json:"latestRun,omitempty"`
	Extras          map[string]any  `json:"-"`
}

type TaskWorktree struct {
	Exists bool   `json:"exists"`
	Path   string `json:"path"`
	Branch string `json:"branch"`
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
	ID                string   `json:"id"`
	Severity          string   `json:"severity"`
	Category          string   `json:"category"`
	Path              string   `json:"path"`
	Branch            string   `json:"branch"`
	Dirty             bool     `json:"dirty"`
	DirtyReasons      []string `json:"dirtyReasons"`
	SuggestedCommands []string `json:"suggestedCommands"`
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
