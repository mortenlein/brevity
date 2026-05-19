package dashboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mortenlein/brevity/internal/contracts"
)

type SelectionKind string

const (
	SelectionProvider SelectionKind = "provider"
	SelectionTask     SelectionKind = "task"
	SelectionCleanup  SelectionKind = "cleanup"
	SelectionAction   SelectionKind = "action"
)

type SelectionItem struct {
	Kind             SelectionKind
	Label            string
	ProviderName     string
	ProviderHealth   contracts.ProviderHealth
	Task             contracts.TaskSummary
	CleanupCandidate contracts.CleanupCandidate
	ActionText       string
}

type InteractiveModel struct {
	SelectedIndex int
	ShowDetails   bool
	ShowHelp      bool
}

func (model *InteractiveModel) Clamp(itemCount int) {
	if itemCount <= 0 {
		model.SelectedIndex = 0
		return
	}
	if model.SelectedIndex < 0 {
		model.SelectedIndex = 0
	}
	if model.SelectedIndex >= itemCount {
		model.SelectedIndex = itemCount - 1
	}
}

func (model *InteractiveModel) MoveDown(itemCount int) {
	model.Clamp(itemCount)
	if model.SelectedIndex+1 < itemCount {
		model.SelectedIndex++
	}
}

func (model *InteractiveModel) MoveUp(itemCount int) {
	model.Clamp(itemCount)
	if model.SelectedIndex > 0 {
		model.SelectedIndex--
	}
}

func (model *InteractiveModel) ToggleDetails() {
	model.ShowDetails = !model.ShowDetails
}

func (model *InteractiveModel) ToggleHelp() {
	model.ShowHelp = !model.ShowHelp
}

func SelectableItems(state contracts.RuntimeState) []SelectionItem {
	items := make([]SelectionItem, 0)
	for _, name := range sortedProviderNames(state.Providers.Health) {
		health := state.Providers.Health[name]
		items = append(items, SelectionItem{
			Kind:           SelectionProvider,
			Label:          fmt.Sprintf("%s: %s", name, fallback(health.Status, "unknown")),
			ProviderName:   name,
			ProviderHealth: health,
		})
	}
	for _, task := range state.Tasks {
		label := fallback(task.Slug, "(unknown task)")
		if task.NormalizedState != "" {
			label += ": " + task.NormalizedState
		} else if task.Status != "" {
			label += ": " + task.Status
		}
		items = append(items, SelectionItem{Kind: SelectionTask, Label: label, Task: task})
	}
	if state.Cleanup != nil {
		for _, candidate := range state.Cleanup.OrphanedTaskWorktrees {
			items = append(items, SelectionItem{
				Kind:             SelectionCleanup,
				Label:            cleanupLabel(candidate),
				CleanupCandidate: candidate,
			})
		}
		for _, candidate := range state.Cleanup.OrphanedTaskBranches {
			items = append(items, SelectionItem{
				Kind:             SelectionCleanup,
				Label:            cleanupLabel(candidate),
				CleanupCandidate: candidate,
			})
		}
	}
	for _, action := range state.SuggestedNextActions {
		action = strings.TrimSpace(action)
		if action == "" {
			continue
		}
		items = append(items, SelectionItem{Kind: SelectionAction, Label: action, ActionText: action})
	}
	return items
}

func RenderInteractive(stdout io.Writer, state contracts.RuntimeState, model InteractiveModel) {
	fmt.Fprint(stdout, RenderInteractiveString(state, model))
}

func RenderInteractiveString(state contracts.RuntimeState, model InteractiveModel) string {
	items := SelectableItems(state)
	model.Clamp(len(items))

	var stdout bytes.Buffer
	fmt.Fprintln(&stdout, "Brevity Runtime Dashboard")
	fmt.Fprintln(&stdout, "=========================")
	fmt.Fprintln(&stdout, "Mode: read-only watch; no dashboard input mutates state.")
	fmt.Fprintf(&stdout, "Repo: %s\n", fallback(state.RepoRoot, "(unknown)"))
	fmt.Fprintf(&stdout, "Generated: %s\n", fallback(state.GeneratedAt, "(unknown)"))
	fmt.Fprintln(&stdout, "Line input: type j/k/d/r/?/q, then press Enter. Enter alone toggles details.")

	renderInteractiveProviders(&stdout, state, items, model.SelectedIndex)
	renderInteractiveTasks(&stdout, state, items, model.SelectedIndex)
	if state.Cleanup != nil && state.Cleanup.Summary != nil {
		renderCleanup(&stdout, *state.Cleanup.Summary)
	}
	renderInteractiveCleanupRows(&stdout, items, model.SelectedIndex)
	renderInteractiveActions(&stdout, items, model.SelectedIndex)

	if model.ShowHelp {
		renderInteractiveHelp(&stdout)
	}
	if model.ShowDetails {
		renderInteractiveDetails(&stdout, items, model.SelectedIndex)
	}
	return stdout.String()
}

func renderInteractiveProviders(stdout io.Writer, state contracts.RuntimeState, items []SelectionItem, selected int) {
	fmt.Fprintln(stdout, "\nProviders")
	fmt.Fprintf(
		stdout,
		"  total: %d, degraded: %d, unavailable: %d\n",
		state.Providers.Summary.Total,
		state.Providers.Summary.Degraded,
		state.Providers.Summary.Unavailable,
	)
	for index, item := range items {
		if item.Kind != SelectionProvider {
			continue
		}
		health := item.ProviderHealth
		line := fmt.Sprintf("%s %s: %s", selectionMarker(index, selected), item.ProviderName, fallback(health.Status, "unknown"))
		if health.UpdatedAt != "" {
			line += " (" + health.UpdatedAt + ")"
		}
		if health.Note != "" {
			line += " - " + health.Note
		}
		fmt.Fprintln(stdout, line)
	}
}

func renderInteractiveTasks(stdout io.Writer, state contracts.RuntimeState, items []SelectionItem, selected int) {
	fmt.Fprintln(stdout, "\nTasks")
	fmt.Fprintf(stdout, "  tracked: %d\n", state.TaskCounts.Tracked)
	fmt.Fprintf(stdout, "  runnable: %d\n", state.TaskCounts.Runnable)
	fmt.Fprintf(stdout, "  blocked: %d\n", state.TaskCounts.Blocked)
	fmt.Fprintf(stdout, "  stale: %d\n", state.TaskCounts.Stale)
	fmt.Fprintf(stdout, "  provider gated: %d\n", state.TaskCounts.ProviderGated)
	fmt.Fprintf(stdout, "  review: %d\n", state.TaskCounts.Review)

	rendered := false
	for index, item := range items {
		if item.Kind != SelectionTask {
			continue
		}
		fmt.Fprintf(stdout, "%s %s\n", selectionMarker(index, selected), item.Label)
		rendered = true
	}
	if !rendered {
		fmt.Fprintln(stdout, "  none")
	}
}

func renderInteractiveCleanupRows(stdout io.Writer, items []SelectionItem, selected int) {
	rendered := false
	for index, item := range items {
		if item.Kind != SelectionCleanup {
			continue
		}
		if !rendered {
			fmt.Fprintln(stdout, "  candidates:")
			rendered = true
		}
		fmt.Fprintf(stdout, "%s %s\n", selectionMarker(index, selected), item.Label)
	}
}

func renderInteractiveActions(stdout io.Writer, items []SelectionItem, selected int) {
	fmt.Fprintln(stdout, "\nSuggested Next Actions")
	rendered := false
	for index, item := range items {
		if item.Kind != SelectionAction {
			continue
		}
		fmt.Fprintf(stdout, "%s %s\n", selectionMarker(index, selected), item.Label)
		rendered = true
	}
	if !rendered {
		fmt.Fprintln(stdout, "  none")
	}
}

func renderInteractiveHelp(stdout io.Writer) {
	fmt.Fprintln(stdout, "\nHelp")
	fmt.Fprintln(stdout, "  This dashboard is read-only; it displays runtime state and guidance only.")
	fmt.Fprintln(stdout, "  j then Enter: move selection down")
	fmt.Fprintln(stdout, "  k then Enter: move selection up")
	fmt.Fprintln(stdout, "  d then Enter: show/hide details")
	fmt.Fprintln(stdout, "  Enter alone: show/hide details")
	fmt.Fprintln(stdout, "  r then Enter: refresh now")
	fmt.Fprintln(stdout, "  ? then Enter: show/hide help")
	fmt.Fprintln(stdout, "  q then Enter: quit")
	fmt.Fprintln(stdout, "  Input is line-oriented for now; raw key input is deferred.")
}

func renderInteractiveDetails(stdout io.Writer, items []SelectionItem, selected int) {
	fmt.Fprintln(stdout, "\nDetails")
	if len(items) == 0 {
		fmt.Fprintln(stdout, "  no selectable item")
		return
	}
	if selected < 0 || selected >= len(items) {
		selected = 0
	}
	item := items[selected]
	switch item.Kind {
	case SelectionProvider:
		fmt.Fprintf(stdout, "  type: provider\n")
		fmt.Fprintf(stdout, "  name: %s\n", fallback(item.ProviderName, "(unknown)"))
		fmt.Fprintf(stdout, "  status: %s\n", fallback(item.ProviderHealth.Status, "unknown"))
		fmt.Fprintf(stdout, "  updatedAt: %s\n", fallback(item.ProviderHealth.UpdatedAt, "(unknown)"))
		fmt.Fprintf(stdout, "  note: %s\n", fallback(item.ProviderHealth.Note, "(none)"))
		fmt.Fprintf(stdout, "  hint: %s\n", providerHint(item.ProviderHealth))
	case SelectionTask:
		task := item.Task
		fmt.Fprintf(stdout, "  type: task\n")
		fmt.Fprintf(stdout, "  slug: %s\n", fallback(task.Slug, "(unknown)"))
		fmt.Fprintf(stdout, "  status: %s\n", fallback(task.Status, "(unknown)"))
		fmt.Fprintf(stdout, "  normalizedState: %s\n", fallback(task.NormalizedState, "(unknown)"))
		fmt.Fprintf(stdout, "  worktreePath: %s\n", fallback(taskWorktreePath(task), "(unknown)"))
		fmt.Fprintf(stdout, "  worktreeExists: %s\n", taskWorktreeExists(task))
		fmt.Fprintf(stdout, "  contextMaterialized: %s\n", taskContextMaterialized(task))
		fmt.Fprintf(stdout, "  contextMissing: %s\n", taskContextMissing(task))
		fmt.Fprintf(stdout, "  latestRun: %s\n", fallback(taskLatestRun(task), "(none)"))
		fmt.Fprintf(stdout, "  provider: %s\n", fallback(taskProvider(task), "(unknown)"))
		fmt.Fprintf(stdout, "  profile: %s\n", fallback(taskProfile(task), "(unknown)"))
	case SelectionCleanup:
		candidate := item.CleanupCandidate
		fmt.Fprintf(stdout, "  type: cleanup candidate\n")
		fmt.Fprintf(stdout, "  id: %s\n", fallback(candidate.ID, "(unknown)"))
		fmt.Fprintf(stdout, "  severity: %s\n", fallback(candidate.Severity, "(unknown)"))
		fmt.Fprintf(stdout, "  category: %s\n", fallback(candidate.Category, "(unknown)"))
		fmt.Fprintf(stdout, "  path: %s\n", fallback(candidate.Path, "(none)"))
		fmt.Fprintf(stdout, "  branch: %s\n", fallback(candidate.Branch, "(none)"))
		fmt.Fprintf(stdout, "  dirty: %t\n", candidate.Dirty)
		fmt.Fprintf(stdout, "  removableByExecute: %s\n", optionalBool(candidate.RemovableByExecute))
		fmt.Fprintf(stdout, "  destructiveIfUnmerged: %s\n", optionalBool(candidate.DestructiveIfUnmerged))
		renderStringList(stdout, "dirtyReasons", candidate.DirtyReasons)
		renderStringList(stdout, "suggestedCommands", candidate.SuggestedCommands)
	case SelectionAction:
		fmt.Fprintf(stdout, "  type: suggested action\n")
		fmt.Fprintf(stdout, "  action: %s\n", fallback(item.ActionText, "(none)"))
		fmt.Fprintf(stdout, "  guidance: read-only guidance; no action is executed by this dashboard.\n")
	}
}

func selectionMarker(index int, selected int) string {
	if index == selected {
		return ">"
	}
	return " "
}

func cleanupLabel(candidate contracts.CleanupCandidate) string {
	name := fallback(candidate.ID, candidate.Branch)
	name = fallback(name, candidate.Path)
	return fmt.Sprintf("%s: %s", fallback(candidate.Category, "cleanup"), name)
}

func taskWorktreePath(task contracts.TaskSummary) string {
	if task.WorktreePath != "" {
		return task.WorktreePath
	}
	if task.Worktree != nil {
		return task.Worktree.Path
	}
	return ""
}

func taskLatestRun(task contracts.TaskSummary) string {
	parts := make([]string, 0, 4)
	if task.LatestRunID != "" {
		parts = append(parts, "id="+task.LatestRunID)
	} else if task.LastRunID != "" {
		parts = append(parts, "id="+task.LastRunID)
	}
	if task.LatestRunWorkerStatus != "" {
		parts = append(parts, "status="+task.LatestRunWorkerStatus)
	} else if task.WorkerStatus != "" {
		parts = append(parts, "status="+task.WorkerStatus)
	}
	if task.LatestRunExitCode != nil {
		parts = append(parts, "exit="+fmt.Sprint(task.LatestRunExitCode))
	} else if task.LastExitCode != nil {
		parts = append(parts, "exit="+fmt.Sprint(task.LastExitCode))
	}
	if task.LatestRunLogPath != "" {
		parts = append(parts, "log="+task.LatestRunLogPath)
	} else if task.LastLogPath != "" {
		parts = append(parts, "log="+task.LastLogPath)
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	if task.Execution != nil {
		parts := make([]string, 0, 3)
		if task.Execution.LastRunID != "" {
			parts = append(parts, "id="+task.Execution.LastRunID)
		}
		if task.Execution.Status != "" {
			parts = append(parts, "status="+task.Execution.Status)
		}
		if task.Execution.LogPath != "" {
			parts = append(parts, "log="+task.Execution.LogPath)
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
	}
	if len(task.LatestRun) == 0 {
		return ""
	}
	var values map[string]any
	if err := json.Unmarshal(task.LatestRun, &values); err != nil {
		return string(task.LatestRun)
	}
	parts = make([]string, 0, 4)
	for _, key := range []string{"runId", "workerStatus", "status", "exitCode", "logPath"} {
		if value, ok := values[key]; ok {
			parts = append(parts, fmt.Sprintf("%s=%v", key, value))
		}
	}
	if len(parts) == 0 {
		return string(task.LatestRun)
	}
	return strings.Join(parts, " ")
}

func providerHint(health contracts.ProviderHealth) string {
	status := strings.ToLower(strings.TrimSpace(health.Status))
	switch status {
	case "degraded", "capacity-degraded":
		return "provider is degraded; expect slower or less reliable worker starts"
	case "unavailable", "down", "offline":
		return "provider is unavailable; choose another healthy profile before starting work"
	default:
		return "(none)"
	}
}

func taskWorktreeExists(task contracts.TaskSummary) string {
	if task.WorktreeExists != nil {
		return fmt.Sprint(*task.WorktreeExists)
	}
	if task.Worktree != nil {
		return fmt.Sprint(task.Worktree.Exists)
	}
	return "(unknown)"
}

func taskContextMaterialized(task contracts.TaskSummary) string {
	if task.Context == nil {
		return "(unknown)"
	}
	return fmt.Sprint(task.Context.MaterializedFileCount)
}

func taskContextMissing(task contracts.TaskSummary) string {
	if task.Context == nil {
		return "(unknown)"
	}
	return fmt.Sprint(len(task.Context.MissingFiles))
}

func taskProvider(task contracts.TaskSummary) string {
	if task.Provider != "" {
		return task.Provider
	}
	if task.LatestRunProvider != "" {
		return task.LatestRunProvider
	}
	return task.LastProvider
}

func taskProfile(task contracts.TaskSummary) string {
	if task.Profile != "" {
		return task.Profile
	}
	if task.LatestRunProfile != "" {
		return task.LatestRunProfile
	}
	return task.LastProfile
}

func optionalBool(value *bool) string {
	if value == nil {
		return "(unknown)"
	}
	return fmt.Sprint(*value)
}

func renderStringList(stdout io.Writer, label string, values []string) {
	if len(values) == 0 {
		fmt.Fprintf(stdout, "  %s: (none)\n", label)
		return
	}
	fmt.Fprintf(stdout, "  %s:\n", label)
	for _, value := range values {
		fmt.Fprintf(stdout, "    - %s\n", value)
	}
}
