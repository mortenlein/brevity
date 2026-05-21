package actions

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/preflight"
	"github.com/mortenlein/brevity/internal/state"
	"github.com/mortenlein/brevity/internal/state/locking"
)

const taskStartCommand = "task start"
const taskStartTargetState = "ready-for-worker"

type TaskStartService struct {
	Store       state.Store
	Now         func() time.Time
	LockOptions locking.Options
}

func (service TaskStartService) Start(slug string) (contracts.CommandResult, error) {
	slug = strings.TrimSpace(slug)
	preflightResult, err := preflight.Run(preflight.Options{
		RepoRoot: service.Store.RepoRoot,
		Action:   preflight.ActionTaskStart,
		Slug:     slug,
		Now:      service.now,
	})
	if err != nil {
		return taskStartError("preflight-error", err.Error(), nil)
	}
	if preflightResult.Status == preflight.StatusBlocked {
		return taskStartError("preflight-blocked", "task start preflight blocked mutation", map[string]any{
			"slug":     slug,
			"blockers": preflightResult.Blockers,
		})
	}

	updatedAt := service.now().UTC().Format(time.RFC3339Nano)
	var oldState string
	startedAt := updatedAt
	update, err := state.UpdateTask(service.Store, slug, state.TaskUpdateOptions{LockOptions: service.LockOptions}, func(rawTask map[string]json.RawMessage) error {
		previous, parseErr := rawTaskToStartTask(rawTask)
		if parseErr != nil {
			return parseErr
		}
		oldState = taskState(previous)
		if !taskStartStateAllowed(oldState) {
			return fmt.Errorf("task state does not permit start: %s", oldState)
		}
		if strings.TrimSpace(previous.StartedAt) != "" {
			startedAt = previous.StartedAt
		}
		rawTask["status"] = mustRawString(taskStartTargetState)
		rawTask["normalizedState"] = mustRawString(taskStartTargetState)
		rawTask["updatedAt"] = mustRawString(updatedAt)
		if _, exists := rawTask["startedAt"]; !exists || strings.TrimSpace(previous.StartedAt) == "" {
			rawTask["startedAt"] = mustRawString(startedAt)
		}
		return nil
	})
	if err != nil {
		code := "task-start-failed"
		if strings.Contains(err.Error(), "locked") || strings.Contains(err.Error(), "lock timeout") {
			code = "task-metadata-locked"
		} else if strings.Contains(err.Error(), "not found") {
			code = "task-not-found"
		} else if strings.Contains(err.Error(), "does not permit start") {
			code = "invalid-task-state"
		} else if strings.Contains(err.Error(), "parse tasks.json") || strings.Contains(err.Error(), "file is empty") {
			code = "task-metadata-invalid"
		}
		return taskStartError(code, err.Error(), map[string]any{"slug": slug, "lockPath": service.Store.LockPath()})
	}
	oldState = taskState(update.Previous)
	newState := taskState(update.Updated)
	message := fmt.Sprintf("Task %s moved from %s to %s. No provider or worker execution occurred.", slug, emptyAsUnknown(oldState), newState)
	return taskStartSuccess(slug, oldState, newState, updatedAt, startedAt, message), nil
}

func (service TaskStartService) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func RenderTaskStartResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseTaskStartPayload(result)
	if err != nil {
		return err
	}
	renderStatusLine(stdout, "Task start", result.Success)
	fmt.Fprintf(stdout, "slug: %s\n", payload.Slug)
	fmt.Fprintf(stdout, "oldState: %s\n", emptyAsUnknown(payload.OldState))
	fmt.Fprintf(stdout, "newState: %s\n", emptyAsUnknown(payload.NewState))
	fmt.Fprintf(stdout, "refreshExpected: %t\n", payload.RefreshExpected)
	fmt.Fprintf(stdout, "providerExecution: false\n")
	fmt.Fprintf(stdout, "workerExecution: false\n")
	if payload.OperatorMessage != "" {
		fmt.Fprintf(stdout, "message: %s\n", payload.OperatorMessage)
	}
	renderMessages(stdout, result)
	return nil
}

func taskStartSuccess(slug string, oldState string, newState string, updatedAt string, startedAt string, message string) contracts.CommandResult {
	payload := contracts.TaskStartPayload{
		Action:          "task-start",
		Slug:            slug,
		OldState:        oldState,
		NewState:        newState,
		UpdatedAt:       updatedAt,
		StartedAt:       startedAt,
		OperatorMessage: message,
		RefreshExpected: true,
		NoExecution:     true,
	}
	raw, _ := json.Marshal(payload)
	return contracts.CommandResult{
		Schema:               contracts.CommandResultSchema,
		Command:              taskStartCommand,
		Success:              true,
		Severity:             "info",
		Warnings:             []contracts.ResultMessage{},
		Errors:               []contracts.ResultMessage{},
		SuggestedNextActions: []string{"refresh-runtime-state"},
		Payload:              raw,
	}
}

func taskStartError(code string, message string, details map[string]any) (contracts.CommandResult, error) {
	raw, _ := json.Marshal(contracts.TaskStartPayload{Action: "task-start", OperatorMessage: message, NoExecution: true})
	result := contracts.CommandResult{
		Schema:   contracts.CommandResultSchema,
		Command:  taskStartCommand,
		Success:  false,
		Severity: "error",
		Warnings: []contracts.ResultMessage{},
		Errors: []contracts.ResultMessage{{
			Code:    code,
			Message: message,
			Details: details,
		}},
		SuggestedNextActions: []string{"Inspect native task start preflight output."},
		Payload:              raw,
	}
	return result, errors.New(message)
}

func rawTaskToStartTask(rawTask map[string]json.RawMessage) (state.Task, error) {
	data, err := json.Marshal(rawTask)
	if err != nil {
		return state.Task{}, err
	}
	var task state.Task
	if err := json.Unmarshal(data, &task); err != nil {
		return state.Task{}, err
	}
	return task, nil
}

func taskState(task state.Task) string {
	return strings.ToLower(strings.TrimSpace(firstNonEmpty(task.NormalizedState, task.Status)))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func taskStartStateAllowed(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "planned", "ready", "ready-for-worker", "queued", "new":
		return true
	default:
		return false
	}
}

func mustRawString(value string) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
