package contracts

import (
	"encoding/json"
	"errors"
	"fmt"
)

const CommandResultSchema = "brevity.command-result.v1"

type CommandResult struct {
	Schema               string          `json:"schema"`
	Command              string          `json:"command"`
	Success              bool            `json:"success"`
	Severity             string          `json:"severity"`
	Warnings             []ResultMessage `json:"warnings"`
	Errors               []ResultMessage `json:"errors"`
	SuggestedNextActions []string        `json:"suggestedNextActions"`
	Payload              json.RawMessage `json:"payload"`
}

type ResultMessage struct {
	Code    string         `json:"code,omitempty"`
	Message string         `json:"message,omitempty"`
	Count   int            `json:"count,omitempty"`
	Details map[string]any `json:"details,omitempty"`
	Text    string         `json:"-"`
}

func (message *ResultMessage) UnmarshalJSON(input []byte) error {
	var text string
	if err := json.Unmarshal(input, &text); err == nil {
		message.Text = text
		message.Message = text
		return nil
	}

	type objectMessage ResultMessage
	var parsed objectMessage
	if err := json.Unmarshal(input, &parsed); err != nil {
		return err
	}

	*message = ResultMessage(parsed)
	return nil
}

func (message ResultMessage) DisplayText() string {
	if message.Message != "" {
		if message.Code != "" {
			return fmt.Sprintf("%s: %s", message.Code, message.Message)
		}
		return message.Message
	}
	if message.Text != "" {
		return message.Text
	}
	if message.Code != "" {
		return message.Code
	}
	return "(no message)"
}

type ProviderActionPayload struct {
	Provider       string `json:"provider"`
	PreviousStatus string `json:"previousStatus"`
	NewStatus      string `json:"newStatus"`
	UpdatedAt      string `json:"updatedAt"`
	Note           string `json:"note"`
}

type TaskContextRefreshPayload struct {
	Slug            string `json:"slug"`
	Refreshed       bool   `json:"refreshed"`
	ContextPath     string `json:"contextPath"`
	GeneratedAt     string `json:"generatedAt"`
	LatestRunID     string `json:"latestRunId"`
	NormalizedState string `json:"normalizedState"`
}

type TaskCleanupPayload struct {
	Slug            string   `json:"slug"`
	WorktreePath    string   `json:"worktreePath"`
	Branch          string   `json:"branch"`
	MetadataRemoved bool     `json:"metadataRemoved"`
	BranchRemoved   bool     `json:"branchRemoved"`
	WorktreeRemoved bool     `json:"worktreeRemoved"`
	Force           bool     `json:"force"`
	CleanupWarnings []string `json:"cleanupWarnings"`
}

type TaskNewPayload struct {
	Slug         string `json:"slug"`
	Branch       string `json:"branch"`
	WorktreePath string `json:"worktreePath"`
	PromptPath   string `json:"promptPath"`
	MetadataPath string `json:"metadataPath"`
}

type TaskRuntimeInfoPayload struct {
	Slug            string                      `json:"slug"`
	Status          string                      `json:"status"`
	NormalizedState string                      `json:"normalizedState"`
	TaskExists      bool                        `json:"taskExists"`
	Worktree        TaskRuntimeWorktreePayload  `json:"worktree"`
	Context         TaskRuntimeContextPayload   `json:"context"`
	Execution       TaskRuntimeExecutionPayload `json:"execution"`
}

type TaskRuntimeWorktreePayload struct {
	Exists bool   `json:"exists"`
	Path   string `json:"path"`
}

type TaskRuntimeContextPayload struct {
	MaterializedFileCount int      `json:"materializedFileCount"`
	MissingFiles          []string `json:"missingFiles"`
}

type TaskRuntimeExecutionPayload struct {
	Status      string `json:"status"`
	LastRunID   string `json:"lastRunId"`
	LastLogPath string `json:"lastLogPath"`
}

type TaskRunsPayload struct {
	Slug  string           `json:"slug"`
	Count int              `json:"count"`
	Runs  []TaskRunPayload `json:"runs"`
}

type TaskRunPayload struct {
	RunID        string `json:"runId"`
	WorkerStatus string `json:"workerStatus"`
	ExitCode     any    `json:"exitCode"`
	Provider     string `json:"provider"`
	Profile      string `json:"profile"`
	LogPath      string `json:"logPath"`
}

type DoctorPayload struct {
	WarningCount         int                  `json:"warningCount"`
	ErrorCount           int                  `json:"errorCount"`
	Providers            DoctorProviders      `json:"providers"`
	BranchCounts         DoctorBranchCounts   `json:"branchCounts"`
	WorktreeCounts       DoctorWorktreeCounts `json:"worktreeCounts"`
	Lock                 DoctorLock           `json:"lock"`
	SuggestedNextActions []string             `json:"suggestedNextActions"`
}

type DoctorProviders struct {
	Summary ProviderSummary `json:"summary"`
}

type DoctorBranchCounts struct {
	Orphaned int `json:"orphaned"`
}

type DoctorWorktreeCounts struct {
	OrphanedTaskWorktrees int `json:"orphanedTaskWorktrees"`
}

type DoctorLock struct {
	Exists bool   `json:"exists"`
	Path   string `json:"path"`
}

func ParseCommandResult(input []byte) (CommandResult, error) {
	var result CommandResult
	if err := json.Unmarshal(input, &result); err != nil {
		return CommandResult{}, fmt.Errorf("invalid command result JSON: %w", err)
	}

	if result.Schema != CommandResultSchema {
		if result.Schema == "" {
			return CommandResult{}, errors.New("unsupported command result schema: missing schema")
		}
		return CommandResult{}, fmt.Errorf("unsupported command result schema: %s", result.Schema)
	}

	return result, nil
}

func ParseProviderActionPayload(result CommandResult) (ProviderActionPayload, error) {
	if len(result.Payload) == 0 {
		return ProviderActionPayload{}, errors.New("provider action result missing payload")
	}

	var payload ProviderActionPayload
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		return ProviderActionPayload{}, fmt.Errorf("invalid provider action payload: %w", err)
	}

	return payload, nil
}

func ParseTaskContextRefreshPayload(result CommandResult) (TaskContextRefreshPayload, error) {
	if len(result.Payload) == 0 {
		return TaskContextRefreshPayload{}, errors.New("task context refresh result missing payload")
	}

	var payload TaskContextRefreshPayload
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		return TaskContextRefreshPayload{}, fmt.Errorf("invalid task context refresh payload: %w", err)
	}

	return payload, nil
}

func ParseTaskCleanupPayload(result CommandResult) (TaskCleanupPayload, error) {
	if len(result.Payload) == 0 {
		return TaskCleanupPayload{}, errors.New("task cleanup result missing payload")
	}

	var payload TaskCleanupPayload
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		return TaskCleanupPayload{}, fmt.Errorf("invalid task cleanup payload: %w", err)
	}

	return payload, nil
}

func ParseTaskNewPayload(result CommandResult) (TaskNewPayload, error) {
	if len(result.Payload) == 0 {
		return TaskNewPayload{}, errors.New("task new result missing payload")
	}

	var payload TaskNewPayload
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		return TaskNewPayload{}, fmt.Errorf("invalid task new payload: %w", err)
	}

	return payload, nil
}

func ParseTaskRuntimeInfoPayload(result CommandResult) (TaskRuntimeInfoPayload, error) {
	if len(result.Payload) == 0 {
		return TaskRuntimeInfoPayload{}, errors.New("task runtime-info result missing payload")
	}

	var payload TaskRuntimeInfoPayload
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		return TaskRuntimeInfoPayload{}, fmt.Errorf("invalid task runtime-info payload: %w", err)
	}

	return payload, nil
}

func ParseTaskRunsPayload(result CommandResult) (TaskRunsPayload, error) {
	if len(result.Payload) == 0 {
		return TaskRunsPayload{}, errors.New("task runs result missing payload")
	}

	var payload TaskRunsPayload
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		return TaskRunsPayload{}, fmt.Errorf("invalid task runs payload: %w", err)
	}

	return payload, nil
}

func ParseDoctorPayload(result CommandResult) (DoctorPayload, error) {
	if len(result.Payload) == 0 {
		return DoctorPayload{}, errors.New("doctor result missing payload")
	}

	var payload DoctorPayload
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		return DoctorPayload{}, fmt.Errorf("invalid doctor payload: %w", err)
	}

	return payload, nil
}
