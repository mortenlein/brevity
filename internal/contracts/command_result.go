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
	Slug                string   `json:"slug"`
	Refreshed           bool     `json:"refreshed"`
	ContextPath         string   `json:"contextPath"`
	PromptPath          string   `json:"promptPath,omitempty"`
	SpecPath            string   `json:"specPath,omitempty"`
	GeneratedAt         string   `json:"generatedAt"`
	LatestRunID         string   `json:"latestRunId"`
	NormalizedState     string   `json:"normalizedState"`
	MaterializedFiles   []string `json:"materializedFiles,omitempty"`
	MissingFiles        []string `json:"missingFiles,omitempty"`
	PromptRefreshStatus string   `json:"promptRefreshStatus,omitempty"`
	NoProviderExecution bool     `json:"noProviderExecution,omitempty"`
	NoWorkerExecution   bool     `json:"noWorkerExecution,omitempty"`
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
	Slug                string `json:"slug"`
	State               string `json:"state,omitempty"`
	Branch              string `json:"branch"`
	WorktreePath        string `json:"worktreePath"`
	PromptPath          string `json:"promptPath"`
	SpecPath            string `json:"specPath,omitempty"`
	MetadataPath        string `json:"metadataPath"`
	CreatedAt           string `json:"createdAt,omitempty"`
	NoProviderExecution bool   `json:"noProviderExecution,omitempty"`
	NoWorkerExecution   bool   `json:"noWorkerExecution,omitempty"`
}

type TaskStartPayload struct {
	Action          string `json:"action"`
	Slug            string `json:"slug"`
	OldState        string `json:"oldState"`
	NewState        string `json:"newState"`
	UpdatedAt       string `json:"updatedAt"`
	StartedAt       string `json:"startedAt,omitempty"`
	OperatorMessage string `json:"operatorMessage"`
	RefreshExpected bool   `json:"refreshExpected"`
	NoExecution     bool   `json:"noExecution"`
}

type TaskRunExecutionPayload struct {
	Slug          string `json:"slug"`
	RunID         string `json:"runId"`
	Provider      string `json:"provider"`
	Profile       string `json:"profile"`
	WorktreePath  string `json:"worktreePath"`
	PromptPath    string `json:"promptPath"`
	ExecutionMode string `json:"executionMode"`
	StartedAt     string `json:"startedAt"`
	FinishedAt    string `json:"finishedAt"`
	ExitCode      any    `json:"exitCode"`
	WorkerStatus  string `json:"workerStatus"`
	FailureType   string `json:"failureType"`
	LogPath       string `json:"logPath"`
}

type TaskRunPlanPayload struct {
	Schema                      string               `json:"schema,omitempty"`
	Version                     int                  `json:"version,omitempty"`
	Slug                        string               `json:"slug"`
	TaskState                   string               `json:"taskState,omitempty"`
	Provider                    string               `json:"provider"`
	Profile                     string               `json:"profile"`
	Model                       string               `json:"model,omitempty"`
	WorktreePath                string               `json:"worktreePath"`
	PromptPath                  string               `json:"promptPath"`
	PromptStatus                string               `json:"promptStatus,omitempty"`
	PromptFreshness             string               `json:"promptFreshness,omitempty"`
	RunIDPlan                   string               `json:"runIdPlan,omitempty"`
	LogPathPlan                 string               `json:"logPathPlan,omitempty"`
	StdoutPathPlan              string               `json:"stdoutPathPlan,omitempty"`
	StderrPathPlan              string               `json:"stderrPathPlan,omitempty"`
	WorkerCommand               TaskRunWorkerCommand `json:"workerCommand"`
	ApprovalMode                string               `json:"approvalMode"`
	ExecutionKind               string               `json:"executionKind"`
	ProviderExecutionWouldOccur bool                 `json:"providerExecutionWouldOccur"`
	ProviderExecution           bool                 `json:"providerExecution,omitempty"`
	LongRunning                 bool                 `json:"longRunning,omitempty"`
	Streaming                   bool                 `json:"streaming,omitempty"`
	IsolatedWorktreeRequired    bool                 `json:"isolatedWorktreeRequired"`
	DryRunOnly                  bool                 `json:"dryRunOnly"`
	NoExecutionOccurred         bool                 `json:"noExecutionOccurred"`
	Authority                   string               `json:"authority"`
	GeneratedAt                 string               `json:"generatedAt,omitempty"`
	ExpectedStateMutations      []string             `json:"expectedStateMutations,omitempty"`
	ExpectedFilesWritten        []string             `json:"expectedFilesWritten,omitempty"`
	Warnings                    []ResultMessage      `json:"warnings"`
	Blockers                    []ResultMessage      `json:"blockers,omitempty"`
	SafetyNotes                 []string             `json:"safetyNotes"`
	Unsupported                 []string             `json:"unsupported"`
}

type TaskRunWorkerCommand struct {
	Provider         string   `json:"provider"`
	Command          string   `json:"command"`
	Arguments        []string `json:"arguments"`
	Display          string   `json:"display"`
	WorkingDirectory string   `json:"workingDirectory"`
	ExecutionPolicy  string   `json:"executionPolicy"`
	EnvironmentNames []string `json:"environmentNames"`
}

type TaskRuntimeInfoPayload struct {
	Slug              string                      `json:"slug"`
	Status            string                      `json:"status"`
	NormalizedState   string                      `json:"normalizedState"`
	TaskExists        bool                        `json:"taskExists"`
	Branch            string                      `json:"branch,omitempty"`
	PromptPath        string                      `json:"promptPath,omitempty"`
	PromptExists      bool                        `json:"promptExists,omitempty"`
	PromptStatus      string                      `json:"promptStatus,omitempty"`
	PromptRefreshedAt string                      `json:"promptRefreshedAt,omitempty"`
	Provider          string                      `json:"provider,omitempty"`
	Profile           string                      `json:"profile,omitempty"`
	RunCount          int                         `json:"runCount"`
	Worktree          TaskRuntimeWorktreePayload  `json:"worktree"`
	Context           TaskRuntimeContextPayload   `json:"context"`
	Execution         TaskRuntimeExecutionPayload `json:"execution"`
	LatestRun         *TaskRunPayload             `json:"latestRun,omitempty"`
	Stale             bool                        `json:"stale"`
	Incomplete        bool                        `json:"incomplete"`
	LogPath           string                      `json:"logPath,omitempty"`
	Interpretation    string                      `json:"interpretation,omitempty"`
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

type TaskRunsPayload struct {
	Slug  string           `json:"slug"`
	Count int              `json:"count"`
	Runs  []TaskRunPayload `json:"runs"`
}

type TaskRunPayload struct {
	RunID         string   `json:"runId"`
	WorkerStatus  string   `json:"workerStatus"`
	ExitCode      any      `json:"exitCode"`
	Provider      string   `json:"provider"`
	Profile       string   `json:"profile"`
	StartedAt     string   `json:"startedAt,omitempty"`
	FinishedAt    string   `json:"finishedAt,omitempty"`
	FailureType   string   `json:"failureType,omitempty"`
	LogPath       string   `json:"logPath"`
	Incomplete    bool     `json:"incomplete,omitempty"`
	Stale         bool     `json:"stale,omitempty"`
	RunAgeMinutes *float64 `json:"runAgeMinutes,omitempty"`
	Source        string   `json:"source,omitempty"`
}

type TaskRunsReconcilePayload struct {
	StaleThresholdMinutes int                          `json:"staleThresholdMinutes"`
	CandidateCount        int                          `json:"candidateCount"`
	Candidates            []TaskRunsReconcileCandidate `json:"candidates"`
}

type TaskRunsReconcileCandidate struct {
	RunID        string `json:"runId"`
	Slug         string `json:"slug"`
	Stale        bool   `json:"stale"`
	Incomplete   bool   `json:"incomplete"`
	WorkerStatus string `json:"workerStatus"`
}

type TaskRunsRetentionPayload struct {
	TotalRecords          int                    `json:"totalRecords"`
	ValidRecords          int                    `json:"validRecords"`
	InvalidRecords        int                    `json:"invalidRecords"`
	IncompleteRecords     int                    `json:"incompleteRecords"`
	StaleRecords          int                    `json:"staleRecords"`
	StaleThresholdMinutes int                    `json:"staleThresholdMinutes"`
	TopTasks              []TaskRunsTopTaskEntry `json:"topTasks"`
}

type TaskRunsTopTaskEntry struct {
	Slug    string `json:"slug"`
	Records int    `json:"records"`
}

type TaskRunsCompactPayload struct {
	RetainedRecordCount           int `json:"retainedRecordCount"`
	CandidateArchiveSummaryCount  int `json:"candidateArchiveSummaryCount"`
	CandidateDiscardCount         int `json:"candidateDiscardCount"`
	PreservedStaleIncompleteCount int `json:"preservedStaleIncompleteCount"`
	PreservedFailedCount          int `json:"preservedFailedCount"`
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

func ParseTaskStartPayload(result CommandResult) (TaskStartPayload, error) {
	if len(result.Payload) == 0 {
		return TaskStartPayload{}, errors.New("task start result missing payload")
	}

	var payload TaskStartPayload
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		return TaskStartPayload{}, fmt.Errorf("invalid task start payload: %w", err)
	}

	return payload, nil
}

func ParseTaskRunExecutionPayload(result CommandResult) (TaskRunExecutionPayload, error) {
	if len(result.Payload) == 0 {
		return TaskRunExecutionPayload{}, errors.New("task run result missing payload")
	}

	var payload TaskRunExecutionPayload
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		return TaskRunExecutionPayload{}, fmt.Errorf("invalid task run payload: %w", err)
	}

	return payload, nil
}

func ParseTaskRunPlanPayload(result CommandResult) (TaskRunPlanPayload, error) {
	if len(result.Payload) == 0 {
		return TaskRunPlanPayload{}, errors.New("task run plan result missing payload")
	}

	var payload TaskRunPlanPayload
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		return TaskRunPlanPayload{}, fmt.Errorf("invalid task run plan payload: %w", err)
	}
	if payload.Slug == "" {
		return TaskRunPlanPayload{}, errors.New("invalid task run plan payload: missing slug")
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

func ParseTaskRunsReconcilePayload(result CommandResult) (TaskRunsReconcilePayload, error) {
	if len(result.Payload) == 0 {
		return TaskRunsReconcilePayload{}, errors.New("task runs reconcile result missing payload")
	}

	var payload TaskRunsReconcilePayload
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		return TaskRunsReconcilePayload{}, fmt.Errorf("invalid task runs reconcile payload: %w", err)
	}

	return payload, nil
}

func ParseTaskRunsRetentionPayload(result CommandResult) (TaskRunsRetentionPayload, error) {
	if len(result.Payload) == 0 {
		return TaskRunsRetentionPayload{}, errors.New("task runs retention result missing payload")
	}

	var payload TaskRunsRetentionPayload
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		return TaskRunsRetentionPayload{}, fmt.Errorf("invalid task runs retention payload: %w", err)
	}

	return payload, nil
}

func ParseTaskRunsCompactPayload(result CommandResult) (TaskRunsCompactPayload, error) {
	if len(result.Payload) == 0 {
		return TaskRunsCompactPayload{}, errors.New("task runs compact result missing payload")
	}

	var payload TaskRunsCompactPayload
	if err := json.Unmarshal(result.Payload, &payload); err != nil {
		return TaskRunsCompactPayload{}, fmt.Errorf("invalid task runs compact payload: %w", err)
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
