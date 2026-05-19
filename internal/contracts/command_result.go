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
