package state

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mortenlein/brevity/internal/contracts"
)

const TasksFile = "tasks.json"

type Tasks struct {
	Items []Task
}

type Task struct {
	Slug                  string              `json:"slug"`
	ID                    string              `json:"id,omitempty"`
	Status                string              `json:"status"`
	NormalizedState       string              `json:"normalizedState,omitempty"`
	MetadataStatus        string              `json:"metadataStatus,omitempty"`
	Branch                string              `json:"branch,omitempty"`
	WorktreePath          string              `json:"worktreePath,omitempty"`
	Worktree              *TaskWorktree       `json:"worktree,omitempty"`
	PromptPath            string              `json:"promptPath,omitempty"`
	Prompt                *TaskPrompt         `json:"prompt,omitempty"`
	Provider              string              `json:"provider,omitempty"`
	Profile               string              `json:"profile,omitempty"`
	ProviderHealth        string              `json:"providerHealth,omitempty"`
	ProviderGated         bool                `json:"providerGated,omitempty"`
	Context               *TaskRuntimeContext `json:"context,omitempty"`
	Execution             *TaskExecution      `json:"execution,omitempty"`
	WorkerStatus          string              `json:"workerStatus,omitempty"`
	LastRunID             string              `json:"lastRunId,omitempty"`
	LastRunStartedAt      string              `json:"lastRunStartedAt,omitempty"`
	LastRunFinishedAt     string              `json:"lastRunFinishedAt,omitempty"`
	LastExitCode          any                 `json:"lastExitCode,omitempty"`
	LastFailureType       string              `json:"lastFailureType,omitempty"`
	LastLogPath           string              `json:"lastLogPath,omitempty"`
	LastProvider          string              `json:"lastProvider,omitempty"`
	LastProfile           string              `json:"lastProfile,omitempty"`
	CreatedAt             string              `json:"createdAt,omitempty"`
	UpdatedAt             string              `json:"updatedAt,omitempty"`
	StartedAt             string              `json:"startedAt,omitempty"`
	FinishedAt            string              `json:"finishedAt,omitempty"`
	LatestRunID           string              `json:"latestRunId,omitempty"`
	LatestRunLogPath      string              `json:"latestRunLogPath,omitempty"`
	LatestRunExitCode     any                 `json:"latestRunExitCode,omitempty"`
	LatestRunProvider     string              `json:"latestRunProvider,omitempty"`
	LatestRunProfile      string              `json:"latestRunProfile,omitempty"`
	LatestRunWorkerStatus string              `json:"latestRunWorkerStatus,omitempty"`
}

type TaskWorktree struct {
	Exists     bool   `json:"exists"`
	Path       string `json:"path"`
	Branch     string `json:"branch"`
	Registered bool   `json:"registered,omitempty"`
}

type TaskPrompt struct {
	Exists bool   `json:"exists"`
	Path   string `json:"path"`
}

type TaskRuntimeContext struct {
	Exists                bool     `json:"exists"`
	Path                  string   `json:"path"`
	MaterializedFileCount int      `json:"materializedFileCount"`
	MissingFiles          []string `json:"missingFiles"`
}

type TaskExecution struct {
	Status            string `json:"status"`
	LastRunID         string `json:"lastRunId"`
	LastRunStartedAt  string `json:"lastRunStartedAt,omitempty"`
	LastRunFinishedAt string `json:"lastRunFinishedAt,omitempty"`
	LastExitCode      any    `json:"lastExitCode,omitempty"`
	LastFailureType   string `json:"lastFailureType,omitempty"`
	LastLogPath       string `json:"lastLogPath"`
	LastProvider      string `json:"lastProvider,omitempty"`
	LastProfile       string `json:"lastProfile,omitempty"`
}

func LoadTasks(store Store) (Tasks, bool, error) {
	var tasks Tasks
	missing, err := store.ReadJSON(TasksFile, &tasks.Items)
	if err != nil {
		return Tasks{}, false, err
	}
	if missing {
		return Tasks{Items: []Task{}}, true, nil
	}
	if tasks.Items == nil {
		tasks.Items = []Task{}
	}
	if err := tasks.Validate(); err != nil {
		return Tasks{}, false, err
	}
	tasks.Sort()
	return tasks, false, nil
}

func (tasks *Tasks) UnmarshalJSON(input []byte) error {
	return json.Unmarshal(input, &tasks.Items)
}

func (tasks Tasks) Validate() error {
	seen := map[string]bool{}
	for index, task := range tasks.Items {
		slug := strings.TrimSpace(task.Slug)
		if slug == "" {
			slug = strings.TrimSpace(task.ID)
		}
		if slug == "" {
			return fmt.Errorf("task at index %d has no slug or id", index)
		}
		if seen[slug] {
			return fmt.Errorf("duplicate task slug: %s", slug)
		}
		seen[slug] = true
	}
	return nil
}

func (tasks *Tasks) Sort() {
	sort.SliceStable(tasks.Items, func(i, j int) bool {
		return tasks.Items[i].Key() < tasks.Items[j].Key()
	})
}

func (task Task) Key() string {
	if strings.TrimSpace(task.Slug) != "" {
		return strings.TrimSpace(task.Slug)
	}
	return strings.TrimSpace(task.ID)
}

func (task Task) ToContract() contracts.TaskSummary {
	worktreePath := firstNonEmpty(task.WorktreePath, nestedWorktreePath(task.Worktree))
	branch := firstNonEmpty(task.Branch, nestedWorktreeBranch(task.Worktree))
	promptPath := firstNonEmpty(task.PromptPath, nestedPromptPath(task.Prompt))
	provider := task.Provider
	profile := task.Profile
	if task.Execution != nil {
		provider = firstNonEmpty(provider, task.Execution.LastProvider)
		profile = firstNonEmpty(profile, task.Execution.LastProfile)
	}
	summary := contracts.TaskSummary{
		Slug:                  task.Key(),
		Status:                task.Status,
		NormalizedState:       task.NormalizedState,
		Provider:              provider,
		Profile:               profile,
		Branch:                branch,
		WorktreePath:          worktreePath,
		WorkerStatus:          task.WorkerStatus,
		LastRunID:             task.LastRunID,
		LastExitCode:          task.LastExitCode,
		LastLogPath:           task.LastLogPath,
		LastProvider:          task.LastProvider,
		LastProfile:           task.LastProfile,
		LatestRunID:           task.LatestRunID,
		LatestRunLogPath:      task.LatestRunLogPath,
		LatestRunExitCode:     task.LatestRunExitCode,
		LatestRunProvider:     task.LatestRunProvider,
		LatestRunProfile:      task.LatestRunProfile,
		LatestRunWorkerStatus: task.LatestRunWorkerStatus,
		PromptPath:            promptPath,
		ProviderHealth:        task.ProviderHealth,
		ProviderGated:         task.ProviderGated,
	}
	if task.Worktree != nil {
		exists := task.Worktree.Exists
		summary.WorktreeExists = &exists
		summary.Worktree = &contracts.TaskWorktree{
			Exists: task.Worktree.Exists,
			Path:   task.Worktree.Path,
			Branch: task.Worktree.Branch,
		}
	}
	if task.Context != nil {
		summary.Context = &contracts.TaskRuntimeContext{
			Exists:                task.Context.Exists,
			Path:                  task.Context.Path,
			MaterializedFileCount: task.Context.MaterializedFileCount,
			MissingFiles:          append([]string{}, task.Context.MissingFiles...),
		}
	}
	if task.Execution != nil {
		summary.Execution = &contracts.TaskExecution{
			Status:    task.Execution.Status,
			LastRunID: task.Execution.LastRunID,
			LogPath:   task.Execution.LastLogPath,
		}
		summary.WorkerStatus = firstNonEmpty(summary.WorkerStatus, task.Execution.Status)
		summary.LastRunID = firstNonEmpty(summary.LastRunID, task.Execution.LastRunID)
		summary.LastExitCode = firstNonNil(summary.LastExitCode, task.Execution.LastExitCode)
		summary.LastLogPath = firstNonEmpty(summary.LastLogPath, task.Execution.LastLogPath)
		summary.LastProvider = firstNonEmpty(summary.LastProvider, task.Execution.LastProvider)
		summary.LastProfile = firstNonEmpty(summary.LastProfile, task.Execution.LastProfile)
	}
	return summary
}

func (tasks Tasks) ToContracts() []contracts.TaskSummary {
	summaries := make([]contracts.TaskSummary, 0, len(tasks.Items))
	for _, task := range tasks.Items {
		summaries = append(summaries, task.ToContract())
	}
	return summaries
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func nestedWorktreePath(worktree *TaskWorktree) string {
	if worktree == nil {
		return ""
	}
	return worktree.Path
}

func nestedWorktreeBranch(worktree *TaskWorktree) string {
	if worktree == nil {
		return ""
	}
	return worktree.Branch
}

func nestedPromptPath(prompt *TaskPrompt) string {
	if prompt == nil {
		return ""
	}
	return prompt.Path
}
