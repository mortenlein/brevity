package preflight

import (
	"fmt"
	"io"
	"strings"
)

const Schema = "brevity.task-preflight.v1"

type Action string
type Status string
type Severity string

const (
	ActionTaskNew     Action = "task-new"
	ActionTaskStart   Action = "task-start"
	ActionTaskRun     Action = "task-run"
	ActionTaskMerge   Action = "task-merge"
	ActionTaskCleanup Action = "task-cleanup"

	StatusAllowed Status = "allowed"
	StatusBlocked Status = "blocked"
	StatusWarn    Status = "warn"

	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

type Check struct {
	ID       string   `json:"id"`
	Status   Status   `json:"status"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

type Result struct {
	Schema               string   `json:"schema"`
	Action               Action   `json:"action"`
	TargetSlug           string   `json:"targetSlug"`
	Status               Status   `json:"status"`
	Severity             Severity `json:"severity"`
	Checks               []Check  `json:"checks"`
	Blockers             []string `json:"blockers"`
	Warnings             []string `json:"warnings"`
	DryRunSummary        string   `json:"dryRunSummary"`
	ExpectedMutations    []string `json:"expectedMutations"`
	Destructive          bool     `json:"destructive"`
	ProviderExecution    bool     `json:"providerExecution"`
	RequiresConfirmation bool     `json:"requiresConfirmation"`
	SuggestedNextAction  string   `json:"suggestedNextAction"`
}

func NewResult(action Action, slug string) Result {
	return Result{
		Schema:            Schema,
		Action:            action,
		TargetSlug:        strings.TrimSpace(slug),
		Status:            StatusAllowed,
		Severity:          SeverityInfo,
		Checks:            []Check{},
		Blockers:          []string{},
		Warnings:          []string{},
		ExpectedMutations: []string{},
	}
}

func (result *Result) AddCheck(id string, status Status, severity Severity, message string) {
	check := Check{ID: id, Status: status, Severity: severity, Message: strings.TrimSpace(message)}
	result.Checks = append(result.Checks, check)
	switch severity {
	case SeverityError:
		result.Blockers = append(result.Blockers, check.Message)
	case SeverityWarn:
		result.Warnings = append(result.Warnings, check.Message)
	}
	result.recompute()
}

func (result *Result) recompute() {
	result.Status = StatusAllowed
	result.Severity = SeverityInfo
	if len(result.Warnings) > 0 {
		result.Status = StatusWarn
		result.Severity = SeverityWarn
	}
	if len(result.Blockers) > 0 {
		result.Status = StatusBlocked
		result.Severity = SeverityError
	}
	switch result.Status {
	case StatusBlocked:
		result.SuggestedNextAction = "Resolve blockers, then rerun preflight."
	case StatusWarn:
		if result.Action == ActionTaskStart {
			result.SuggestedNextAction = "Review warnings before executing the native Go mutation."
		} else {
			result.SuggestedNextAction = "Review warnings before executing the PowerShell-owned mutation."
		}
	default:
		if result.Action == ActionTaskStart {
			result.SuggestedNextAction = "Mutation may proceed through native Go task start."
		} else {
			result.SuggestedNextAction = "Mutation may proceed through the current PowerShell authority path."
		}
	}
}

func RenderHuman(output io.Writer, result Result) error {
	fmt.Fprintf(output, "Task mutation preflight: %s\n", result.Action)
	fmt.Fprintf(output, "slug: %s\n", emptyAs(result.TargetSlug, "(none)"))
	fmt.Fprintf(output, "status: %s\n", result.Status)
	fmt.Fprintf(output, "severity: %s\n", result.Severity)
	fmt.Fprintf(output, "destructive: %t\n", result.Destructive)
	fmt.Fprintf(output, "providerExecution: %t\n", result.ProviderExecution)
	fmt.Fprintf(output, "requiresConfirmation: %t\n", result.RequiresConfirmation)
	if result.DryRunSummary != "" {
		fmt.Fprintf(output, "summary: %s\n", result.DryRunSummary)
	}
	fmt.Fprintln(output)
	fmt.Fprintln(output, "Checks:")
	for _, check := range result.Checks {
		fmt.Fprintf(output, "- [%s] %s: %s\n", check.Severity, check.ID, check.Message)
	}
	if len(result.ExpectedMutations) > 0 {
		fmt.Fprintln(output)
		fmt.Fprintln(output, "Expected mutations if executed later:")
		for _, mutation := range result.ExpectedMutations {
			fmt.Fprintf(output, "- %s\n", mutation)
		}
	}
	fmt.Fprintln(output)
	fmt.Fprintf(output, "suggestedNextAction: %s\n", result.SuggestedNextAction)
	return nil
}

func emptyAs(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
