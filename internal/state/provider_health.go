package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/state/locking"
)

const ProviderHealthFile = "provider-health.json"

type ProviderStatus string

const (
	StatusHealthy          ProviderStatus = "healthy"
	StatusCapacityDegraded ProviderStatus = "capacity-degraded"
	StatusQuotaConstrained ProviderStatus = "quota-constrained"
	StatusUnavailable      ProviderStatus = "unavailable"
	StatusUnknown          ProviderStatus = "unknown"
)

type ProviderHealth struct {
	Status    ProviderStatus `json:"status"`
	Note      string         `json:"note"`
	UpdatedAt string         `json:"updatedAt"`
}

type ProviderHealthState map[string]ProviderHealth

type ProviderHealthService struct {
	Store       Store
	Now         func() time.Time
	LockOptions locking.Options
}

func SupportedProviderStatuses() []ProviderStatus {
	return []ProviderStatus{StatusHealthy, StatusCapacityDegraded, StatusQuotaConstrained, StatusUnavailable, StatusUnknown}
}

func NormalizeProviderStatus(status string) (ProviderStatus, error) {
	normalized := ProviderStatus(strings.ToLower(strings.TrimSpace(status)))
	for _, supported := range SupportedProviderStatuses() {
		if normalized == supported {
			return normalized, nil
		}
	}
	return "", fmt.Errorf("invalid provider status: %s", status)
}

func DefaultProviderHealthState() ProviderHealthState {
	return ProviderHealthState{
		"codex":   {Status: StatusUnknown, Note: "", UpdatedAt: ""},
		"gemini":  {Status: StatusUnknown, Note: "", UpdatedAt: ""},
		"copilot": {Status: StatusUnknown, Note: "", UpdatedAt: ""},
	}
}

func LoadProviderHealth(store Store) (ProviderHealthState, bool, error) {
	state := ProviderHealthState{}
	missing, err := store.ReadJSON(ProviderHealthFile, &state)
	if err != nil {
		return nil, false, err
	}
	if missing {
		return ProviderHealthState{}, true, nil
	}
	if state == nil {
		state = ProviderHealthState{}
	}
	for name, health := range state {
		status, err := NormalizeProviderStatus(string(health.Status))
		if err != nil || status == "" {
			status = StatusUnknown
		}
		health.Status = status
		state[strings.ToLower(strings.TrimSpace(name))] = health
		if name != strings.ToLower(strings.TrimSpace(name)) {
			delete(state, name)
		}
	}
	return state, false, nil
}

func (health *ProviderHealth) UnmarshalJSON(input []byte) error {
	var raw struct {
		Status    string `json:"status"`
		Note      string `json:"note"`
		Reason    string `json:"reason"`
		UpdatedAt string `json:"updatedAt"`
	}
	if err := json.Unmarshal(input, &raw); err != nil {
		return err
	}
	status := StatusUnknown
	if strings.TrimSpace(raw.Status) != "" {
		parsed, err := NormalizeProviderStatus(raw.Status)
		if err != nil {
			return err
		}
		status = parsed
	}
	health.Status = status
	health.Note = raw.Note
	if health.Note == "" {
		health.Note = raw.Reason
	}
	health.UpdatedAt = raw.UpdatedAt
	return nil
}

func (health ProviderHealth) ToContract() contracts.ProviderHealth {
	return contracts.ProviderHealth{
		Status:    string(health.Status),
		UpdatedAt: health.UpdatedAt,
		Note:      health.Note,
	}
}

func (service ProviderHealthService) List() (ProviderHealthState, bool, error) {
	return LoadProviderHealth(service.Store)
}

func (service ProviderHealthService) Set(provider string, status ProviderStatus, note string) (contracts.CommandResult, error) {
	return service.mutate("provider set", provider, func(current ProviderHealth) ProviderHealth {
		current.Status = status
		current.Note = note
		current.UpdatedAt = service.now().UTC().Format(time.RFC3339Nano)
		return current
	})
}

func (service ProviderHealthService) Reset(provider string) (contracts.CommandResult, error) {
	return service.mutate("provider reset", provider, func(current ProviderHealth) ProviderHealth {
		current.Status = StatusUnknown
		current.Note = "Provider state reset."
		current.UpdatedAt = service.now().UTC().Format(time.RFC3339Nano)
		return current
	})
}

func (service ProviderHealthService) mutate(command string, provider string, change func(ProviderHealth) ProviderHealth) (contracts.CommandResult, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return providerError(command, "missing-provider", "Missing provider name.", nil)
	}
	lockOptions := service.LockOptions
	if lockOptions.Timeout == 0 {
		lockOptions.Timeout = 5 * time.Second
	}
	lock, err := locking.Acquire(service.Store.LockPath(), lockOptions)
	if err != nil {
		return providerError(command, "provider-health-locked", err.Error(), map[string]any{"provider": provider, "lockPath": service.Store.LockPath()})
	}
	defer lock.Release()

	health, missing, err := LoadProviderHealth(service.Store)
	if err != nil {
		return contracts.CommandResult{}, err
	}
	if missing {
		health = DefaultProviderHealthState()
	}
	ensureDefaultProviders(health)
	current, ok := health[provider]
	if !ok {
		return providerError(command, "invalid-provider", fmt.Sprintf("Invalid provider: %s", provider), map[string]any{"provider": provider, "knownProviders": sortedProviderNames(health)})
	}
	previousStatus := current.Status
	next := change(current)
	health[provider] = next
	if err := service.Store.WriteJSON(ProviderHealthFile, health); err != nil {
		return contracts.CommandResult{}, err
	}
	return providerSuccess(command, provider, string(previousStatus), next), nil
}

func (service ProviderHealthService) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func ensureDefaultProviders(health ProviderHealthState) {
	for provider, value := range DefaultProviderHealthState() {
		if _, ok := health[provider]; !ok {
			health[provider] = value
		}
	}
}

func providerSuccess(command string, provider string, previousStatus string, health ProviderHealth) contracts.CommandResult {
	payload := contracts.ProviderActionPayload{
		Provider:       provider,
		PreviousStatus: previousStatus,
		NewStatus:      string(health.Status),
		UpdatedAt:      health.UpdatedAt,
		Note:           health.Note,
	}
	raw, _ := json.Marshal(payload)
	return contracts.CommandResult{
		Schema:               contracts.CommandResultSchema,
		Command:              command,
		Success:              true,
		Severity:             "info",
		Warnings:             []contracts.ResultMessage{},
		Errors:               []contracts.ResultMessage{},
		SuggestedNextActions: []string{"refresh-runtime-state"},
		Payload:              raw,
	}
}

func providerError(command string, code string, message string, details map[string]any) (contracts.CommandResult, error) {
	raw, _ := json.Marshal(contracts.ProviderActionPayload{})
	result := contracts.CommandResult{
		Schema:   contracts.CommandResultSchema,
		Command:  command,
		Success:  false,
		Severity: "error",
		Warnings: []contracts.ResultMessage{},
		Errors: []contracts.ResultMessage{{
			Code:    code,
			Message: message,
			Details: details,
		}},
		SuggestedNextActions: []string{},
		Payload:              raw,
	}
	return result, errors.New(message)
}

func sortedProviderNames(health ProviderHealthState) []string {
	names := make([]string, 0, len(health))
	for name := range health {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
