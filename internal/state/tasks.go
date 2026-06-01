package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/state/locking"
)

const TasksFile = "tasks.json"

type Tasks struct {
	Items []Task
}

type Task struct {
	Slug                  string              `json:"slug"`
	ID                    string              `json:"id,omitempty"`
	Description           string              `json:"description,omitempty"`
	Status                string              `json:"status"`
	NormalizedState       string              `json:"normalizedState,omitempty"`
	MetadataStatus        string              `json:"metadataStatus,omitempty"`
	Branch                string              `json:"branch,omitempty"`
	WorktreePath          string              `json:"worktreePath,omitempty"`
	Worktree              *TaskWorktree       `json:"worktree,omitempty"`
	PromptPath            string              `json:"promptPath,omitempty"`
	SpecPath              string              `json:"specPath,omitempty"`
	PromptRefreshedAt     string              `json:"promptRefreshedAt,omitempty"`
	PromptRefreshStatus   string              `json:"promptRefreshStatus,omitempty"`
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
	RunCount              int                 `json:"runCount,omitempty"`
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
	LatestRunStartedAt    string              `json:"latestRunStartedAt,omitempty"`
	LatestRunFinishedAt   string              `json:"latestRunFinishedAt,omitempty"`
	LatestRunFailureType  string              `json:"latestRunFailureType,omitempty"`
	LatestRunIncomplete   bool                `json:"latestRunIncomplete,omitempty"`
	LatestRunStale        bool                `json:"latestRunStale,omitempty"`
	LatestRunAgeMinutes   *float64            `json:"latestRunAgeMinutes,omitempty"`
	LatestRunSource       string              `json:"latestRunSource,omitempty"`
}

type TaskUpdateOptions struct {
	LockOptions locking.Options
}

type TaskCreateOptions struct {
	LockOptions locking.Options
}

type TaskUpdate struct {
	Previous Task
	Updated  Task
}

type TaskMutator func(task map[string]json.RawMessage) error

type TaskCreate struct {
	Created Task
}

type TaskRemove struct {
	Removed Task
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

func UpdateTask(store Store, slug string, options TaskUpdateOptions, mutate TaskMutator) (TaskUpdate, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return TaskUpdate{}, fmt.Errorf("task slug is required")
	}
	if mutate == nil {
		return TaskUpdate{}, fmt.Errorf("task mutator is required")
	}
	lockOptions := options.LockOptions
	if lockOptions.Timeout == 0 {
		lockOptions.Timeout = 5 * time.Second
	}
	lock, err := locking.Acquire(store.LockPath(), lockOptions)
	if err != nil {
		return TaskUpdate{}, fmt.Errorf("task metadata locked: %w", err)
	}
	defer lock.Release()

	rawTasks, err := loadRawTasks(store)
	if err != nil {
		return TaskUpdate{}, err
	}
	index := -1
	for i, rawTask := range rawTasks {
		if rawTaskKey(rawTask) == slug {
			index = i
			break
		}
	}
	if index < 0 {
		return TaskUpdate{}, fmt.Errorf("task not found: %s", slug)
	}

	previous, err := rawTaskToTask(rawTasks[index])
	if err != nil {
		return TaskUpdate{}, fmt.Errorf("parse task %s: %w", slug, err)
	}
	nextRaw := cloneRawTask(rawTasks[index])
	if err := mutate(nextRaw); err != nil {
		return TaskUpdate{}, err
	}
	updated, err := rawTaskToTask(nextRaw)
	if err != nil {
		return TaskUpdate{}, fmt.Errorf("parse updated task %s: %w", slug, err)
	}
	rawTasks[index] = nextRaw
	if err := writeRawTasks(store, rawTasks); err != nil {
		return TaskUpdate{}, err
	}
	return TaskUpdate{Previous: previous, Updated: updated}, nil
}

func CreateTask(store Store, task Task, options TaskCreateOptions) (TaskCreate, error) {
	slug := strings.TrimSpace(task.Key())
	if slug == "" {
		return TaskCreate{}, fmt.Errorf("task slug is required")
	}
	lockOptions := options.LockOptions
	if lockOptions.Timeout == 0 {
		lockOptions.Timeout = 5 * time.Second
	}
	lock, err := locking.Acquire(store.LockPath(), lockOptions)
	if err != nil {
		return TaskCreate{}, fmt.Errorf("task metadata locked: %w", err)
	}
	defer lock.Release()

	rawTasks, err := loadRawTasksAllowMissing(store)
	if err != nil {
		return TaskCreate{}, err
	}
	for _, rawTask := range rawTasks {
		if rawTaskKey(rawTask) == slug {
			return TaskCreate{}, fmt.Errorf("task already exists: %s", slug)
		}
	}
	data, err := json.Marshal(task)
	if err != nil {
		return TaskCreate{}, fmt.Errorf("marshal task %s: %w", slug, err)
	}
	var rawTask map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawTask); err != nil {
		return TaskCreate{}, fmt.Errorf("marshal task %s: %w", slug, err)
	}
	rawTasks = append(rawTasks, rawTask)
	sort.SliceStable(rawTasks, func(i, j int) bool {
		return rawTaskKey(rawTasks[i]) < rawTaskKey(rawTasks[j])
	})
	if err := writeRawTasks(store, rawTasks); err != nil {
		return TaskCreate{}, err
	}
	return TaskCreate{Created: task}, nil
}

func RemoveTask(store Store, slug string, options TaskUpdateOptions) (TaskRemove, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return TaskRemove{}, fmt.Errorf("task slug is required")
	}
	lockOptions := options.LockOptions
	if lockOptions.Timeout == 0 {
		lockOptions.Timeout = 5 * time.Second
	}
	lock, err := locking.Acquire(store.LockPath(), lockOptions)
	if err != nil {
		return TaskRemove{}, fmt.Errorf("task metadata locked: %w", err)
	}
	defer lock.Release()

	rawTasks, err := loadRawTasks(store)
	if err != nil {
		return TaskRemove{}, err
	}
	index := -1
	for i, rawTask := range rawTasks {
		if rawTaskKey(rawTask) == slug {
			index = i
			break
		}
	}
	if index < 0 {
		return TaskRemove{}, fmt.Errorf("task not found: %s", slug)
	}
	removed, err := rawTaskToTask(rawTasks[index])
	if err != nil {
		return TaskRemove{}, fmt.Errorf("parse task %s: %w", slug, err)
	}
	rawTasks = append(rawTasks[:index], rawTasks[index+1:]...)
	if err := writeRawTasks(store, rawTasks); err != nil {
		return TaskRemove{}, err
	}
	return TaskRemove{Removed: removed}, nil
}

func UpdateTaskRunMetadata(store Store, record RunRecord, options TaskUpdateOptions) error {
	_, err := UpdateTask(store, record.Slug, options, func(task map[string]json.RawMessage) error {
		setRaw(task, "workerStatus", record.WorkerStatus)
		setRaw(task, "lastRunId", record.RunID)
		setRaw(task, "lastRunStartedAt", record.StartedAt)
		setRaw(task, "lastRunFinishedAt", record.FinishedAt)
		setRaw(task, "lastExitCode", record.ExitCode)
		setRaw(task, "lastFailureType", record.FailureType)
		setRaw(task, "lastLogPath", record.LogPath)
		setRaw(task, "lastProvider", record.Provider)
		setRaw(task, "lastProfile", record.Profile)
		setRaw(task, "latestRunId", record.RunID)
		setRaw(task, "latestRunLogPath", record.LogPath)
		setRaw(task, "latestRunExitCode", record.ExitCode)
		setRaw(task, "latestRunProvider", record.Provider)
		setRaw(task, "latestRunProfile", record.Profile)
		setRaw(task, "latestRunWorkerStatus", record.WorkerStatus)
		setRaw(task, "latestRunStartedAt", record.StartedAt)
		setRaw(task, "latestRunFinishedAt", record.FinishedAt)
		setRaw(task, "latestRunFailureType", record.FailureType)
		setRaw(task, "updatedAt", record.FinishedAt)
		task["latestRunIncomplete"] = json.RawMessage("false")
		task["latestRunStale"] = json.RawMessage("false")
		var currentRunCount int
		_ = json.Unmarshal(task["runCount"], &currentRunCount)
		setRaw(task, "runCount", currentRunCount+1)
		execution := TaskExecution{
			Status:            record.WorkerStatus,
			LastRunID:         record.RunID,
			LastRunStartedAt:  record.StartedAt,
			LastRunFinishedAt: record.FinishedAt,
			LastExitCode:      record.ExitCode,
			LastFailureType:   record.FailureType,
			LastLogPath:       record.LogPath,
			LastProvider:      record.Provider,
			LastProfile:       record.Profile,
		}
		setRaw(task, "execution", execution)
		return nil
	})
	return err
}

func setRaw(task map[string]json.RawMessage, key string, value any) {
	data, _ := json.Marshal(value)
	task[key] = data
}

func (tasks *Tasks) UnmarshalJSON(input []byte) error {
	return json.Unmarshal(input, &tasks.Items)
}

func loadRawTasks(store Store) ([]map[string]json.RawMessage, error) {
	data, err := os.ReadFile(store.Path(TasksFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf(".brevity/%s is missing", TasksFile)
		}
		return nil, fmt.Errorf("read %s: %w", TasksFile, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("read %s: file is empty", TasksFile)
	}
	var rawTasks []map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawTasks); err != nil {
		return nil, fmt.Errorf("parse %s: %w", TasksFile, err)
	}
	if rawTasks == nil {
		rawTasks = []map[string]json.RawMessage{}
	}
	var typed []Task
	if err := json.Unmarshal(data, &typed); err != nil {
		return nil, fmt.Errorf("parse %s: %w", TasksFile, err)
	}
	if err := (Tasks{Items: typed}).Validate(); err != nil {
		return nil, err
	}
	return rawTasks, nil
}

func loadRawTasksAllowMissing(store Store) ([]map[string]json.RawMessage, error) {
	rawTasks, err := loadRawTasks(store)
	if err == nil {
		return rawTasks, nil
	}
	if strings.Contains(err.Error(), ".brevity/"+TasksFile+" is missing") {
		return []map[string]json.RawMessage{}, nil
	}
	return nil, err
}

func writeRawTasks(store Store, rawTasks []map[string]json.RawMessage) error {
	return store.WriteJSON(TasksFile, rawTasks)
}

func rawTaskKey(rawTask map[string]json.RawMessage) string {
	for _, key := range []string{"slug", "id"} {
		var value string
		if err := json.Unmarshal(rawTask[key], &value); err == nil && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func rawTaskToTask(rawTask map[string]json.RawMessage) (Task, error) {
	data, err := json.Marshal(rawTask)
	if err != nil {
		return Task{}, err
	}
	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return Task{}, err
	}
	return task, nil
}

func cloneRawTask(rawTask map[string]json.RawMessage) map[string]json.RawMessage {
	clone := make(map[string]json.RawMessage, len(rawTask))
	for key, value := range rawTask {
		clone[key] = append(json.RawMessage(nil), value...)
	}
	return clone
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
		RunCount:              task.RunCount,
		LastProvider:          task.LastProvider,
		LastProfile:           task.LastProfile,
		LatestRunID:           task.LatestRunID,
		LatestRunLogPath:      task.LatestRunLogPath,
		LatestRunExitCode:     task.LatestRunExitCode,
		LatestRunProvider:     task.LatestRunProvider,
		LatestRunProfile:      task.LatestRunProfile,
		LatestRunWorkerStatus: task.LatestRunWorkerStatus,
		LatestRunStartedAt:    task.LatestRunStartedAt,
		LatestRunFinishedAt:   task.LatestRunFinishedAt,
		LatestRunFailureType:  task.LatestRunFailureType,
		LatestRunIncomplete:   task.LatestRunIncomplete,
		LatestRunStale:        task.LatestRunStale,
		LatestRunAgeMinutes:   task.LatestRunAgeMinutes,
		LatestRunSource:       task.LatestRunSource,
		PromptPath:            promptPath,
		PromptRefreshedAt:     task.PromptRefreshedAt,
		PromptStatus:          task.PromptRefreshStatus,
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
