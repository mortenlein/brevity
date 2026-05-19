package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

const runtimeStateSchema = "brevity.runtime-state.v1"

type runtimeState struct {
	Schema               string         `json:"schema"`
	RepoRoot             string         `json:"repoRoot"`
	GeneratedAt          string         `json:"generatedAt"`
	Providers            providers      `json:"providers"`
	TaskCounts           taskCounts     `json:"taskCounts"`
	Cleanup              *cleanup       `json:"cleanup,omitempty"`
	SuggestedNextActions []string       `json:"suggestedNextActions"`
	Groups               map[string]any `json:"groups"`
	Extras               map[string]any `json:"-"`
}

type providers struct {
	Summary providerSummary            `json:"summary"`
	Health  map[string]providerHealth  `json:"health"`
	Extras  map[string]json.RawMessage `json:"-"`
}

type providerSummary struct {
	Total       int `json:"total"`
	Degraded    int `json:"degraded"`
	Unavailable int `json:"unavailable"`
}

type providerHealth struct {
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt"`
	Note      string `json:"note"`
}

type taskCounts struct {
	Tracked       int `json:"tracked"`
	Runnable      int `json:"runnable"`
	Blocked       int `json:"blocked"`
	Stale         int `json:"stale"`
	ProviderGated int `json:"providerGated"`
	Review        int `json:"review"`
}

type cleanup struct {
	Summary *cleanupSummary `json:"summary,omitempty"`
}

type cleanupSummary struct {
	TotalCandidates           int            `json:"totalCandidates"`
	RequiresInspectionCount   int            `json:"requiresInspectionCount"`
	RemovableByExecuteCount   int            `json:"removableByExecuteCount"`
	OrphanedTaskWorktreeCount int            `json:"orphanedTaskWorktreeCount"`
	OrphanedTaskBranchCount   int            `json:"orphanedTaskBranchCount"`
	BySeverity                map[string]int `json:"bySeverity"`
	ByCategory                map[string]int `json:"byCategory"`
}

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "brevity-go:", err)
		os.Exit(1)
	}
}

func run(stdout *os.File) error {
	output, err := loadRuntimeStateJSON()
	if err != nil {
		return err
	}

	state, err := parseRuntimeState(output)
	if err != nil {
		return err
	}

	renderDashboard(stdout, state)
	return nil
}

func loadRuntimeStateJSON() ([]byte, error) {
	cmd := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-ExecutionPolicy",
		"Bypass",
		"-File",
		".\\brevity.ps1",
		"runtime",
		"state",
		"--json",
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("PowerShell runtime state command failed: %s", message)
	}

	if len(bytes.TrimSpace(output)) == 0 {
		return nil, errors.New("PowerShell runtime state command returned empty output")
	}

	return output, nil
}

func parseRuntimeState(input []byte) (runtimeState, error) {
	var state runtimeState
	if err := json.Unmarshal(input, &state); err != nil {
		return runtimeState{}, fmt.Errorf("invalid runtime state JSON: %w", err)
	}

	if state.Schema != runtimeStateSchema {
		if state.Schema == "" {
			return runtimeState{}, errors.New("unsupported runtime state schema: missing schema")
		}
		return runtimeState{}, fmt.Errorf("unsupported runtime state schema: %s", state.Schema)
	}

	return state, nil
}

func renderDashboard(stdout *os.File, state runtimeState) {
	fmt.Fprintln(stdout, "Brevity Runtime Dashboard")
	fmt.Fprintln(stdout, "=========================")
	fmt.Fprintf(stdout, "Repo: %s\n", fallback(state.RepoRoot, "(unknown)"))
	fmt.Fprintf(stdout, "Generated: %s\n\n", fallback(state.GeneratedAt, "(unknown)"))

	fmt.Fprintln(stdout, "Providers")
	fmt.Fprintf(
		stdout,
		"  total: %d, degraded: %d, unavailable: %d\n",
		state.Providers.Summary.Total,
		state.Providers.Summary.Degraded,
		state.Providers.Summary.Unavailable,
	)
	for _, name := range sortedProviderNames(state.Providers.Health) {
		health := state.Providers.Health[name]
		line := fmt.Sprintf("  %s: %s", name, fallback(health.Status, "unknown"))
		if health.UpdatedAt != "" {
			line += " (" + health.UpdatedAt + ")"
		}
		if health.Note != "" {
			line += " - " + health.Note
		}
		fmt.Fprintln(stdout, line)
	}

	fmt.Fprintln(stdout, "\nTasks")
	fmt.Fprintf(stdout, "  tracked: %d\n", state.TaskCounts.Tracked)
	fmt.Fprintf(stdout, "  runnable: %d\n", state.TaskCounts.Runnable)
	fmt.Fprintf(stdout, "  blocked: %d\n", state.TaskCounts.Blocked)
	fmt.Fprintf(stdout, "  stale: %d\n", state.TaskCounts.Stale)
	fmt.Fprintf(stdout, "  provider gated: %d\n", state.TaskCounts.ProviderGated)
	fmt.Fprintf(stdout, "  review: %d\n", state.TaskCounts.Review)

	if state.Cleanup != nil && state.Cleanup.Summary != nil {
		renderCleanup(stdout, *state.Cleanup.Summary)
	}

	fmt.Fprintln(stdout, "\nSuggested Next Actions")
	if len(state.SuggestedNextActions) == 0 {
		fmt.Fprintln(stdout, "  none")
		return
	}
	for _, action := range state.SuggestedNextActions {
		if strings.TrimSpace(action) == "" {
			continue
		}
		fmt.Fprintf(stdout, "  - %s\n", action)
	}
}

func renderCleanup(stdout *os.File, summary cleanupSummary) {
	fmt.Fprintln(stdout, "\nCleanup")
	fmt.Fprintf(stdout, "  total candidates: %d\n", summary.TotalCandidates)
	fmt.Fprintf(stdout, "  requires inspection: %d\n", summary.RequiresInspectionCount)
	fmt.Fprintf(stdout, "  removable by execute: %d\n", summary.RemovableByExecuteCount)
	fmt.Fprintf(stdout, "  orphaned worktrees: %d\n", summary.OrphanedTaskWorktreeCount)
	fmt.Fprintf(stdout, "  orphaned branches: %d\n", summary.OrphanedTaskBranchCount)
	renderIntMap(stdout, "  by severity", summary.BySeverity)
	renderIntMap(stdout, "  by category", summary.ByCategory)
}

func renderIntMap(stdout *os.File, label string, values map[string]int) {
	if len(values) == 0 {
		return
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, values[key]))
	}
	fmt.Fprintf(stdout, "%s: %s\n", label, strings.Join(parts, ", "))
}

func sortedProviderNames(values map[string]providerHealth) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func fallback(value string, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}
