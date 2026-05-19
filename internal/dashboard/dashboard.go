package dashboard

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mortenlein/brevity/internal/contracts"
)

func Render(stdout io.Writer, state contracts.RuntimeState) {
	fmt.Fprint(stdout, RenderString(state))
}

func RenderString(state contracts.RuntimeState) string {
	var stdout bytes.Buffer

	fmt.Fprintln(&stdout, "Brevity Runtime Dashboard")
	fmt.Fprintln(&stdout, "=========================")
	fmt.Fprintf(&stdout, "Repo: %s\n", fallback(state.RepoRoot, "(unknown)"))
	fmt.Fprintf(&stdout, "Generated: %s\n\n", fallback(state.GeneratedAt, "(unknown)"))

	fmt.Fprintln(&stdout, "Providers")
	fmt.Fprintf(
		&stdout,
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
		fmt.Fprintln(&stdout, line)
	}

	fmt.Fprintln(&stdout, "\nTasks")
	fmt.Fprintf(&stdout, "  tracked: %d\n", state.TaskCounts.Tracked)
	fmt.Fprintf(&stdout, "  runnable: %d\n", state.TaskCounts.Runnable)
	fmt.Fprintf(&stdout, "  blocked: %d\n", state.TaskCounts.Blocked)
	fmt.Fprintf(&stdout, "  stale: %d\n", state.TaskCounts.Stale)
	fmt.Fprintf(&stdout, "  provider gated: %d\n", state.TaskCounts.ProviderGated)
	fmt.Fprintf(&stdout, "  review: %d\n", state.TaskCounts.Review)

	if state.Cleanup != nil && state.Cleanup.Summary != nil {
		renderCleanup(&stdout, *state.Cleanup.Summary)
	}

	fmt.Fprintln(&stdout, "\nSuggested Next Actions")
	if len(state.SuggestedNextActions) == 0 {
		fmt.Fprintln(&stdout, "  none")
		return stdout.String()
	}
	for _, action := range state.SuggestedNextActions {
		if strings.TrimSpace(action) == "" {
			continue
		}
		fmt.Fprintf(&stdout, "  - %s\n", action)
	}

	return stdout.String()
}

func renderCleanup(stdout io.Writer, summary contracts.CleanupSummary) {
	fmt.Fprintln(stdout, "\nCleanup")
	fmt.Fprintf(stdout, "  total candidates: %d\n", summary.TotalCandidates)
	fmt.Fprintf(stdout, "  requires inspection: %d\n", summary.RequiresInspectionCount)
	fmt.Fprintf(stdout, "  removable by execute: %d\n", summary.RemovableByExecuteCount)
	fmt.Fprintf(stdout, "  orphaned worktrees: %d\n", summary.OrphanedTaskWorktreeCount)
	fmt.Fprintf(stdout, "  orphaned branches: %d\n", summary.OrphanedTaskBranchCount)
	renderIntMap(stdout, "  by severity", summary.BySeverity)
	renderIntMap(stdout, "  by category", summary.ByCategory)
}

func renderIntMap(stdout io.Writer, label string, values map[string]int) {
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

func sortedProviderNames(values map[string]contracts.ProviderHealth) []string {
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
