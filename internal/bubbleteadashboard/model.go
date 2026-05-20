package bubbleteadashboard

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/dashboard"
	"github.com/mortenlein/brevity/internal/runtimeclient"
	"golang.org/x/term"
)

const defaultRefreshInterval = 5 * time.Second

type refreshMsg struct {
	state contracts.RuntimeState
	err   error
	at    time.Time
}

type tickMsg time.Time

type Model struct {
	client          runtimeclient.Client
	selection       dashboard.InteractiveModel
	state           contracts.RuntimeState
	hasState        bool
	lastRefresh     string
	lastError       error
	refreshInterval time.Duration
	source          string
}

func NewModel(client runtimeclient.Client, refreshInterval time.Duration) Model {
	return NewModelWithSource(client, refreshInterval, "")
}

func NewModelWithSource(client runtimeclient.Client, refreshInterval time.Duration, source string) Model {
	if refreshInterval <= 0 {
		refreshInterval = defaultRefreshInterval
	}
	if strings.TrimSpace(source) == "" {
		source = inferSource(client)
	}
	return Model{
		client:          client,
		refreshInterval: refreshInterval,
		source:          source,
	}
}

func Run(ctx context.Context, input io.Reader, stdout io.Writer, client runtimeclient.Client, refreshInterval time.Duration) error {
	return RunWithSource(ctx, input, stdout, client, refreshInterval, "")
}

func RunWithSource(ctx context.Context, input io.Reader, stdout io.Writer, client runtimeclient.Client, refreshInterval time.Duration, source string) error {
	if !isTerminalInput(input) {
		return runLineFallback(stdout, input, client, refreshInterval, source)
	}

	program := tea.NewProgram(
		NewModelWithSource(client, refreshInterval, source),
		tea.WithContext(ctx),
		tea.WithInput(input),
		tea.WithOutput(stdout),
	)
	_, err := program.Run()
	return err
}

func isTerminalInput(input io.Reader) bool {
	file, ok := input.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func runLineFallback(stdout io.Writer, input io.Reader, client runtimeclient.Client, refreshInterval time.Duration, source string) error {
	model := NewModelWithSource(client, refreshInterval, source)
	refreshed := model.refreshCmd()().(refreshMsg)
	updated, _ := model.Update(refreshed)
	model = updated.(Model)
	fmt.Fprint(stdout, model.View())
	if refreshed.err != nil {
		return refreshed.err
	}

	if input == nil {
		return nil
	}
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		key := strings.TrimSpace(scanner.Text())
		updated, _ := model.Update(lineKeyMsg(key))
		model = updated.(Model)
		if key == "q" {
			return nil
		}
		fmt.Fprint(stdout, model.View())
	}
	return scanner.Err()
}

func lineKeyMsg(key string) tea.KeyMsg {
	switch strings.ToLower(key) {
	case "":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}

func (model Model) Init() tea.Cmd {
	return tea.Batch(model.refreshCmd(), model.tickCmd())
}

func (model Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return model.updateKey(msg)
	case refreshMsg:
		if msg.err != nil {
			model.lastError = msg.err
			return model, nil
		}
		model.state = msg.state
		model.hasState = true
		model.lastError = nil
		model.lastRefresh = msg.at.Format(time.RFC3339)
		model.selection.Clamp(len(dashboard.SelectableItems(model.state)))
		return model, nil
	case tickMsg:
		return model, tea.Batch(model.refreshCmd(), model.tickCmd())
	default:
		return model, nil
	}
}

func (model Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	itemCount := len(dashboard.SelectableItems(model.state))
	switch msg.String() {
	case "q", "ctrl+c":
		return model, tea.Quit
	case "j", "down":
		model.selection.MoveDown(itemCount)
		return model, nil
	case "k", "up":
		model.selection.MoveUp(itemCount)
		return model, nil
	case "d", "enter":
		model.selection.ToggleDetails()
		return model, nil
	case "?":
		model.selection.ToggleHelp()
		return model, nil
	case "r":
		return model, model.refreshCmd()
	default:
		return model, nil
	}
}

func (model Model) View() string {
	if !model.hasState {
		output := model.renderHeader()
		output += "\nRuntime Summary\n"
		output += "  Loading runtime state...\n"
		if model.lastError != nil {
			output += fmt.Sprintf("  ! polling error: %v\n", model.lastError)
		}
		output += model.renderFooter()
		return output
	}

	output := model.renderHeader()
	output += model.renderSummary()
	output += model.renderListAndDetails()
	if model.lastError != nil {
		output += fmt.Sprintf("\nWarnings\n  ! polling error: %v\n", model.lastError)
	}
	output += model.renderFooter()
	return output
}

func (model Model) renderHeader() string {
	return fmt.Sprintf(
		"Brevity Runtime Dashboard [read-only] [source: %s]\n===============================================\n",
		fallback(model.source, "unknown"),
	)
}

func (model Model) renderSummary() string {
	state := model.state
	var output strings.Builder
	fmt.Fprintln(&output, "\nRuntime Summary")
	fmt.Fprintf(&output, "  repo: %s\n", fallback(state.RepoRoot, "(unknown)"))
	fmt.Fprintf(&output, "  generated: %s\n", fallback(state.GeneratedAt, "(unknown)"))
	fmt.Fprintf(&output, "  providers: %d total, %d degraded, %d unavailable%s\n",
		state.Providers.Summary.Total,
		state.Providers.Summary.Degraded,
		state.Providers.Summary.Unavailable,
		warningSuffix(state.Providers.Summary.Degraded+state.Providers.Summary.Unavailable),
	)
	fmt.Fprintf(&output, "  tasks: %d tracked, %d runnable, %d blocked, %d stale, %d provider-gated, %d review%s\n",
		state.TaskCounts.Tracked,
		state.TaskCounts.Runnable,
		state.TaskCounts.Blocked,
		state.TaskCounts.Stale,
		state.TaskCounts.ProviderGated,
		state.TaskCounts.Review,
		warningSuffix(state.TaskCounts.Blocked+state.TaskCounts.Stale+state.TaskCounts.ProviderGated),
	)
	if state.Cleanup != nil && state.Cleanup.Summary != nil {
		summary := state.Cleanup.Summary
		fmt.Fprintf(&output, "  cleanup: %d candidates, %d require inspection%s\n",
			summary.TotalCandidates,
			summary.RequiresInspectionCount,
			warningSuffix(summary.TotalCandidates),
		)
	} else {
		fmt.Fprintln(&output, "  cleanup: no candidates reported")
	}
	return output.String()
}

func (model Model) renderListAndDetails() string {
	items := dashboard.SelectableItems(model.state)
	selection := model.selection
	selection.Clamp(len(items))

	var output strings.Builder
	fmt.Fprintln(&output, "\nSelectable List")
	if len(items) == 0 {
		fmt.Fprintln(&output, "  (none)")
	} else {
		for index, item := range items {
			marker := " "
			if index == selection.SelectedIndex {
				marker = ">"
			}
			fmt.Fprintf(&output, "%s %-8s %s%s\n", marker, item.Kind, item.Label, itemWarning(item))
		}
	}

	fmt.Fprintln(&output, "\nDetails Pane")
	if selection.ShowDetails {
		renderDetails(&output, items, selection.SelectedIndex)
	} else {
		fmt.Fprintln(&output, "  details hidden; press d or enter")
	}
	if selection.ShowHelp {
		fmt.Fprintln(&output, "\nHelp")
		fmt.Fprintln(&output, "  q quit")
		fmt.Fprintln(&output, "  j/k or arrows move")
		fmt.Fprintln(&output, "  d/enter details")
		fmt.Fprintln(&output, "  r refresh")
		fmt.Fprintln(&output, "  ? help")
	}
	return output.String()
}

func (model Model) renderFooter() string {
	return fmt.Sprintf(
		"\nFooter\n  q quit | j/k or arrows move | d/enter details | r refresh | ? help\n  refresh: every %s | last success: %s | source: %s\n",
		model.refreshInterval,
		fallbackRefresh(model.lastRefresh),
		fallback(model.source, "unknown"),
	)
}

func (model Model) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		output, err := model.client.RuntimeStateJSON()
		if err != nil {
			return refreshMsg{err: err, at: time.Now()}
		}
		state, err := contracts.ParseRuntimeState(output)
		if err != nil {
			return refreshMsg{err: err, at: time.Now()}
		}
		return refreshMsg{state: state, at: time.Now()}
	}
}

func (model Model) tickCmd() tea.Cmd {
	return tea.Tick(model.refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func fallbackRefresh(value string) string {
	if value == "" {
		return "(none)"
	}
	return value
}

func inferSource(client runtimeclient.Client) string {
	switch client.(type) {
	case runtimeclient.NativeClient, *runtimeclient.NativeClient:
		return "native"
	case runtimeclient.PowerShellClient, *runtimeclient.PowerShellClient:
		return "powershell"
	default:
		return "unknown"
	}
}

func fallback(value string, fallbackValue string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallbackValue
	}
	return value
}

func warningSuffix(count int) string {
	if count > 0 {
		return " !"
	}
	return ""
}

func itemWarning(item dashboard.SelectionItem) string {
	switch item.Kind {
	case dashboard.SelectionProvider:
		status := strings.ToLower(strings.TrimSpace(item.ProviderHealth.Status))
		if status == "degraded" || status == "capacity-degraded" || status == "unavailable" || status == "down" || status == "offline" {
			return " !"
		}
	case dashboard.SelectionTask:
		state := strings.ToLower(strings.TrimSpace(item.Task.NormalizedState))
		if state == "" {
			state = strings.ToLower(strings.TrimSpace(item.Task.Status))
		}
		if state == "blocked" || state == "stale" || state == "provider-gated" || state == "review" || state == "needs-review" {
			return " !"
		}
	case dashboard.SelectionCleanup:
		return " !"
	}
	return ""
}

func renderDetails(output io.Writer, items []dashboard.SelectionItem, selected int) {
	if len(items) == 0 {
		fmt.Fprintln(output, "  no selectable item")
		return
	}
	if selected < 0 || selected >= len(items) {
		selected = 0
	}
	item := items[selected]
	switch item.Kind {
	case dashboard.SelectionProvider:
		health := item.ProviderHealth
		fmt.Fprintln(output, "  type: provider")
		fmt.Fprintf(output, "  name: %s\n", fallback(item.ProviderName, "(unknown)"))
		fmt.Fprintf(output, "  status: %s%s\n", fallback(health.Status, "unknown"), itemWarning(item))
		fmt.Fprintf(output, "  updated: %s\n", fallback(health.UpdatedAt, "(unknown)"))
		fmt.Fprintf(output, "  note: %s\n", fallback(health.Note, "(none)"))
		fmt.Fprintf(output, "  guidance: %s\n", providerGuidance(health))
	case dashboard.SelectionTask:
		task := item.Task
		fmt.Fprintln(output, "  type: task")
		fmt.Fprintf(output, "  slug: %s\n", fallback(task.Slug, "(unknown)"))
		fmt.Fprintf(output, "  state: %s\n", fallback(firstNonEmpty(task.NormalizedState, task.Status), "(unknown)"))
		fmt.Fprintf(output, "  branch: %s\n", fallback(task.Branch, "(unknown)"))
		fmt.Fprintf(output, "  worktree: %s\n", fallback(taskWorktreePath(task), "(unknown)"))
		fmt.Fprintf(output, "  worktree exists: %s\n", optionalTaskBool(task.WorktreeExists, task.Worktree))
		fmt.Fprintf(output, "  provider/profile: %s / %s\n", fallback(taskProvider(task), "(unknown)"), fallback(taskProfile(task), "(unknown)"))
		fmt.Fprintf(output, "  latest run: %s\n", fallback(taskLatestRun(task), "(none)"))
		if task.Context != nil {
			fmt.Fprintf(output, "  context: %d materialized, %d missing\n", task.Context.MaterializedFileCount, len(task.Context.MissingFiles))
		} else {
			fmt.Fprintln(output, "  context: (unknown)")
		}
	case dashboard.SelectionCleanup:
		candidate := item.CleanupCandidate
		fmt.Fprintln(output, "  type: cleanup candidate")
		fmt.Fprintf(output, "  id: %s\n", fallback(candidate.ID, "(unknown)"))
		fmt.Fprintf(output, "  severity/category: %s / %s\n", fallback(candidate.Severity, "(unknown)"), fallback(candidate.Category, "(unknown)"))
		fmt.Fprintf(output, "  path: %s\n", fallback(candidate.Path, "(none)"))
		fmt.Fprintf(output, "  branch: %s\n", fallback(candidate.Branch, "(none)"))
		fmt.Fprintf(output, "  dirty: %t\n", candidate.Dirty)
		fmt.Fprintf(output, "  removable by execute: %s\n", optionalBool(candidate.RemovableByExecute))
		fmt.Fprintf(output, "  destructive if unmerged: %s\n", optionalBool(candidate.DestructiveIfUnmerged))
		renderStringList(output, "dirty reasons", candidate.DirtyReasons)
		renderStringList(output, "suggested commands", candidate.SuggestedCommands)
	case dashboard.SelectionAction:
		fmt.Fprintln(output, "  type: suggested action")
		fmt.Fprintf(output, "  action: %s\n", fallback(item.ActionText, "(none)"))
		fmt.Fprintln(output, "  guidance: display-only; no command is executed from this dashboard")
	}
}

func providerGuidance(health contracts.ProviderHealth) string {
	switch strings.ToLower(strings.TrimSpace(health.Status)) {
	case "degraded", "capacity-degraded":
		return "provider is degraded; treat worker readiness with caution"
	case "unavailable", "down", "offline":
		return "provider is unavailable; PowerShell remains the authority for changes"
	default:
		return "(none)"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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

func optionalTaskBool(value *bool, worktree *contracts.TaskWorktree) string {
	if value != nil {
		return fmt.Sprint(*value)
	}
	if worktree != nil {
		return fmt.Sprint(worktree.Exists)
	}
	return "(unknown)"
}

func taskProvider(task contracts.TaskSummary) string {
	return firstNonEmpty(task.Provider, task.LatestRunProvider, task.LastProvider)
}

func taskProfile(task contracts.TaskSummary) string {
	return firstNonEmpty(task.Profile, task.LatestRunProfile, task.LastProfile)
}

func taskLatestRun(task contracts.TaskSummary) string {
	parts := make([]string, 0, 4)
	if id := firstNonEmpty(task.LatestRunID, task.LastRunID); id != "" {
		parts = append(parts, "id="+id)
	}
	if status := firstNonEmpty(task.LatestRunWorkerStatus, task.WorkerStatus); status != "" {
		parts = append(parts, "status="+status)
	}
	if task.LatestRunExitCode != nil {
		parts = append(parts, "exit="+fmt.Sprint(task.LatestRunExitCode))
	} else if task.LastExitCode != nil {
		parts = append(parts, "exit="+fmt.Sprint(task.LastExitCode))
	}
	if logPath := firstNonEmpty(task.LatestRunLogPath, task.LastLogPath); logPath != "" {
		parts = append(parts, "log="+logPath)
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	if task.Execution != nil {
		if task.Execution.LastRunID != "" {
			parts = append(parts, "id="+task.Execution.LastRunID)
		}
		if task.Execution.Status != "" {
			parts = append(parts, "status="+task.Execution.Status)
		}
		if task.Execution.LogPath != "" {
			parts = append(parts, "log="+task.Execution.LogPath)
		}
	}
	return strings.Join(parts, " ")
}

func optionalBool(value *bool) string {
	if value == nil {
		return "(unknown)"
	}
	return fmt.Sprint(*value)
}

func renderStringList(output io.Writer, label string, values []string) {
	if len(values) == 0 {
		fmt.Fprintf(output, "  %s: (none)\n", label)
		return
	}
	fmt.Fprintf(output, "  %s:\n", label)
	for _, value := range values {
		fmt.Fprintf(output, "    - %s\n", value)
	}
}
