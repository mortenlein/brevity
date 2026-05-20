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
		output += "\n" + sectionTitle("Runtime Summary") + "\n"
		output += "  Loading runtime state...\n"
		if model.lastError != nil {
			output += fmt.Sprintf("  %s polling error: %v\n", warningMarker(), model.lastError)
		}
		output += model.renderFooter()
		return output
	}

	output := model.renderHeader()
	output += model.renderSummary()
	output += model.renderListAndDetails()
	if model.lastError != nil {
		output += fmt.Sprintf("\n%s\n  %s polling error: %v\n", sectionTitle("Warnings"), warningMarker(), model.lastError)
	}
	output += model.renderFooter()
	return output
}

func (model Model) renderHeader() string {
	return fmt.Sprintf(
		"%s\n%s\n",
		dashboardStyles.title.Render(fmt.Sprintf("Brevity Runtime Dashboard [read-only] [source: %s]",
			fallback(model.source, "unknown"))),
		dashboardStyles.rule.Render("==============================================="),
	)
}

func renderSection(output *strings.Builder, title string) {
	fmt.Fprintln(output, sectionTitle(title))
}

func renderWarningCount(count int) string {
	if count > 0 {
		return " " + warningMarker()
	}
	return ""
}

func renderWarningText(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return " " + dashboardStyles.warning.Render(text)
}

func selectedRow(text string) string {
	return dashboardStyles.selectedRow.Render(text)
}

func unselectedRow(text string) string {
	return text
}

func renderRow(selected bool, kind string, label string, warning string) string {
	line := fmt.Sprintf("%s %-8s %s%s", rowMarker(selected), kind, label, warning)
	if selected {
		return selectedRow(line)
	}
	return unselectedRow(line)
}

func rowMarker(selected bool) string {
	if selected {
		return ">"
	}
	return " "
}

func detail(output io.Writer, label string, format string, args ...any) {
	fmt.Fprintln(output, detailLine(label, fmt.Sprintf(format, args...)))
}

func detailText(output io.Writer, label string, value string) {
	fmt.Fprintln(output, detailLine(label, value))
}

func (model Model) renderSummary() string {
	state := model.state
	var output strings.Builder
	fmt.Fprintln(&output)
	renderSection(&output, "Runtime Summary")
	fmt.Fprintf(&output, "  repo: %s\n", fallback(state.RepoRoot, "(unknown)"))
	fmt.Fprintf(&output, "  generated: %s\n", fallback(state.GeneratedAt, "(unknown)"))
	fmt.Fprintf(&output, "  providers: %d total, %d degraded, %d unavailable%s\n",
		state.Providers.Summary.Total,
		state.Providers.Summary.Degraded,
		state.Providers.Summary.Unavailable,
		renderWarningCount(state.Providers.Summary.Degraded+state.Providers.Summary.Unavailable),
	)
	fmt.Fprintf(&output, "  tasks: %d tracked, %d runnable, %d blocked, %d stale, %d provider-gated, %d review%s\n",
		state.TaskCounts.Tracked,
		state.TaskCounts.Runnable,
		state.TaskCounts.Blocked,
		state.TaskCounts.Stale,
		state.TaskCounts.ProviderGated,
		state.TaskCounts.Review,
		renderWarningCount(state.TaskCounts.Blocked+state.TaskCounts.Stale+state.TaskCounts.ProviderGated),
	)
	if state.Cleanup != nil && state.Cleanup.Summary != nil {
		summary := state.Cleanup.Summary
		fmt.Fprintf(&output, "  cleanup: %d candidates, %d require inspection%s\n",
			summary.TotalCandidates,
			summary.RequiresInspectionCount,
			renderWarningCount(summary.TotalCandidates),
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
	fmt.Fprintln(&output)
	renderSection(&output, "Selectable List")
	if len(items) == 0 {
		fmt.Fprintln(&output, "  (none)")
	} else {
		for index, item := range items {
			fmt.Fprintln(&output, renderRow(index == selection.SelectedIndex, string(item.Kind), item.Label, itemWarning(item)))
		}
	}

	fmt.Fprintln(&output)
	renderSection(&output, "Details Pane")
	if selection.ShowDetails {
		renderDetails(&output, items, selection.SelectedIndex)
	} else {
		fmt.Fprintln(&output, "  details hidden; press d or enter")
	}
	if selection.ShowHelp {
		fmt.Fprintln(&output)
		renderSection(&output, "Help")
		fmt.Fprintln(&output, dashboardStyles.help.Render("  q quit"))
		fmt.Fprintln(&output, dashboardStyles.help.Render("  j/k or arrows move"))
		fmt.Fprintln(&output, dashboardStyles.help.Render("  d/enter details"))
		fmt.Fprintln(&output, dashboardStyles.help.Render("  r refresh"))
		fmt.Fprintln(&output, dashboardStyles.help.Render("  ? help"))
	}
	return output.String()
}

func (model Model) renderFooter() string {
	return fmt.Sprintf(
		"\n%s\n%s\n%s\n",
		sectionTitle("Footer"),
		dashboardStyles.footer.Render("  q quit | j/k or arrows move | d/enter details | r refresh | ? help"),
		dashboardStyles.footer.Render(fmt.Sprintf("  refresh: every %s | last success: %s | source: %s",
			model.refreshInterval,
			fallbackRefresh(model.lastRefresh),
			fallback(model.source, "unknown"))),
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
		return " " + warningMarker()
	}
	return ""
}

func itemWarning(item dashboard.SelectionItem) string {
	switch item.Kind {
	case dashboard.SelectionProvider:
		status := strings.ToLower(strings.TrimSpace(item.ProviderHealth.Status))
		if status == "degraded" || status == "capacity-degraded" || status == "unavailable" || status == "down" || status == "offline" {
			return renderWarningText("!")
		}
	case dashboard.SelectionTask:
		state := strings.ToLower(strings.TrimSpace(item.Task.NormalizedState))
		if state == "" {
			state = strings.ToLower(strings.TrimSpace(item.Task.Status))
		}
		if state == "blocked" || state == "stale" || state == "provider-gated" || state == "review" || state == "needs-review" {
			return renderWarningText("!")
		}
	case dashboard.SelectionCleanup:
		return renderWarningText("!")
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
		detailText(output, "type", "provider")
		detailText(output, "name", fallback(item.ProviderName, "(unknown)"))
		detailText(output, "status", fallback(health.Status, "unknown")+itemWarning(item))
		detailText(output, "updated", fallback(health.UpdatedAt, "(unknown)"))
		detailText(output, "note", fallback(health.Note, "(none)"))
		detailText(output, "guidance", providerGuidance(health))
	case dashboard.SelectionTask:
		task := item.Task
		detailText(output, "type", "task")
		detailText(output, "slug", fallback(task.Slug, "(unknown)"))
		detailText(output, "state", fallback(firstNonEmpty(task.NormalizedState, task.Status), "(unknown)"))
		detailText(output, "branch", fallback(task.Branch, "(unknown)"))
		detailText(output, "worktree", fallback(taskWorktreePath(task), "(unknown)"))
		detailText(output, "worktree exists", optionalTaskBool(task.WorktreeExists, task.Worktree))
		detailText(output, "provider/profile", fmt.Sprintf("%s / %s", fallback(taskProvider(task), "(unknown)"), fallback(taskProfile(task), "(unknown)")))
		detailText(output, "latest run", fallback(taskLatestRun(task), "(none)"))
		if task.Context != nil {
			detail(output, "context", "%d materialized, %d missing", task.Context.MaterializedFileCount, len(task.Context.MissingFiles))
		} else {
			detailText(output, "context", "(unknown)")
		}
	case dashboard.SelectionCleanup:
		candidate := item.CleanupCandidate
		detailText(output, "type", "cleanup candidate")
		detailText(output, "id", fallback(candidate.ID, "(unknown)"))
		detailText(output, "severity/category", fmt.Sprintf("%s / %s", fallback(candidate.Severity, "(unknown)"), fallback(candidate.Category, "(unknown)")))
		detailText(output, "path", fallback(candidate.Path, "(none)"))
		detailText(output, "branch", fallback(candidate.Branch, "(none)"))
		detail(output, "dirty", "%t", candidate.Dirty)
		detailText(output, "removable by execute", optionalBool(candidate.RemovableByExecute))
		detailText(output, "destructive if unmerged", optionalBool(candidate.DestructiveIfUnmerged))
		renderStringList(output, "dirty reasons", candidate.DirtyReasons)
		renderStringList(output, "suggested commands", candidate.SuggestedCommands)
	case dashboard.SelectionAction:
		detailText(output, "type", "suggested action")
		detailText(output, "action", fallback(item.ActionText, "(none)"))
		detailText(output, "guidance", "display-only; no command is executed from this dashboard")
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
