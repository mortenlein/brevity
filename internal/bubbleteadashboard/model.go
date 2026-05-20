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
const minimumVisibleListRows = 1
const fullHelpRows = 5
const detailTruncatedIndicator = "  ... details truncated"
const helpTruncatedIndicator = "  ... help truncated"

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
	width           int
	height          int
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
	case tea.WindowSizeMsg:
		model.width = msg.Width
		model.height = msg.Height
		return model, nil
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
	width := model.contentWidth()
	title := truncateValue(fmt.Sprintf("Brevity Runtime Dashboard [read-only] [source: %s]",
		fallback(model.source, "unknown")), width-4)
	return fmt.Sprintf(
		"%s\n%s\n",
		dashboardStyles.title.Render(title),
		dashboardStyles.rule.Render(strings.Repeat("=", width)),
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

func (model Model) selectedRow(text string) string {
	return dashboardStyles.selectedRow.Render(text)
}

func (model Model) unselectedRow(text string) string {
	return text
}

func (model Model) renderRow(selected bool, kind string, label string, warning string) string {
	prefix := fmt.Sprintf("%s %-8s ", rowMarker(selected), kind)
	warningWidth := 0
	if warning != "" {
		warningWidth = 2
	}
	line := prefix + truncateValue(label, model.contentWidth()-len(prefix)-warningWidth) + warning
	if selected {
		return model.selectedRow(line)
	}
	return model.unselectedRow(line)
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
	fmt.Fprintf(&output, "  repo: %s\n", model.renderInlinePath(fallback(state.RepoRoot, "(unknown)"), len("  repo: ")))
	fmt.Fprintf(&output, "%s\n", model.renderLine(fmt.Sprintf("  generated: %s", fallback(state.GeneratedAt, "(unknown)"))))
	fmt.Fprintf(&output, "%s\n", model.renderLine(fmt.Sprintf("  providers: %d total, %d degraded, %d unavailable%s",
		state.Providers.Summary.Total,
		state.Providers.Summary.Degraded,
		state.Providers.Summary.Unavailable,
		renderWarningCount(state.Providers.Summary.Degraded+state.Providers.Summary.Unavailable),
	)))
	fmt.Fprintf(&output, "%s\n", model.renderLine(fmt.Sprintf("  tasks: %d tracked, %d runnable, %d blocked, %d stale, %d provider-gated, %d review%s",
		state.TaskCounts.Tracked,
		state.TaskCounts.Runnable,
		state.TaskCounts.Blocked,
		state.TaskCounts.Stale,
		state.TaskCounts.ProviderGated,
		state.TaskCounts.Review,
		renderWarningCount(state.TaskCounts.Blocked+state.TaskCounts.Stale+state.TaskCounts.ProviderGated),
	)))
	if state.Cleanup != nil && state.Cleanup.Summary != nil {
		summary := state.Cleanup.Summary
		fmt.Fprintf(&output, "%s\n", model.renderLine(fmt.Sprintf("  cleanup: %d candidates, %d require inspection%s",
			summary.TotalCandidates,
			summary.RequiresInspectionCount,
			renderWarningCount(summary.TotalCandidates),
		)))
	} else {
		fmt.Fprintln(&output, "  cleanup: no candidates reported")
	}
	return output.String()
}

func (model Model) renderListAndDetails() string {
	items := dashboard.SelectableItems(model.state)
	selection := model.selection
	selection.Clamp(len(items))
	window := model.selectableListWindow(len(items), selection.SelectedIndex)
	detailsRows := model.visibleDetailsRows(window.end - window.start)
	helpRows := model.visibleHelpRows(window.end-window.start, detailsRows)

	var output strings.Builder
	fmt.Fprintln(&output)
	renderSection(&output, "Selectable List")
	if len(items) == 0 {
		fmt.Fprintln(&output, "  (none)")
	} else {
		if window.truncated {
			fmt.Fprintf(&output, "  showing %d-%d of %d\n", window.start+1, window.end, len(items))
		}
		for index := window.start; index < window.end; index++ {
			item := items[index]
			fmt.Fprintln(&output, model.renderRow(index == selection.SelectedIndex, string(item.Kind), item.Label, itemWarning(item)))
		}
	}

	fmt.Fprintln(&output)
	renderSection(&output, "Details Pane")
	var details strings.Builder
	if selection.ShowDetails {
		model.renderDetails(&details, items, selection.SelectedIndex)
	} else {
		fmt.Fprintln(&details, "  details hidden; press d or enter")
	}
	output.WriteString(truncateRows(details.String(), detailsRows, detailTruncatedIndicator, model.contentWidth()))
	if selection.ShowHelp {
		fmt.Fprintln(&output)
		renderSection(&output, "Help")
		output.WriteString(model.renderHelp(helpRows))
	}
	return output.String()
}

type listWindow struct {
	start     int
	end       int
	truncated bool
}

func (model Model) selectableListWindow(itemCount int, selected int) listWindow {
	if itemCount <= 0 {
		return listWindow{}
	}
	selected = clampInt(selected, 0, itemCount-1)
	visibleRows := model.visibleSelectableRows()
	if visibleRows >= itemCount {
		return listWindow{start: 0, end: itemCount}
	}
	if visibleRows < minimumVisibleListRows {
		visibleRows = minimumVisibleListRows
	}

	start := selected - visibleRows + 1
	if start < 0 {
		start = 0
	}
	maxStart := itemCount - visibleRows
	if start > maxStart {
		start = maxStart
	}
	return listWindow{start: start, end: start + visibleRows, truncated: true}
}

func (model Model) visibleSelectableRows() int {
	if model.height <= 0 {
		return maxInt
	}
	reservedRows := 0
	reservedRows += 2 // header
	reservedRows += 6 // runtime summary
	reservedRows += 2 // selectable list spacing and title
	reservedRows += 4 // details pane, collapsed or minimum visible details
	if model.selection.ShowHelp {
		reservedRows += 7
	}
	if model.lastError != nil {
		reservedRows += 3
	}
	reservedRows += 4 // footer

	visibleRows := model.height - reservedRows
	if visibleRows < minimumVisibleListRows {
		return minimumVisibleListRows
	}
	return visibleRows
}

func (model Model) visibleDetailsRows(visibleListRows int) int {
	return model.remainingRowsAfterList(visibleListRows, model.minimumHelpRows())
}

func (model Model) visibleHelpRows(visibleListRows int, visibleDetailsRows int) int {
	if !model.selection.ShowHelp {
		return 0
	}
	return model.remainingRowsAfterListAndDetails(visibleListRows, visibleDetailsRows)
}

func (model Model) minimumHelpRows() int {
	if model.selection.ShowHelp {
		return fullHelpRows
	}
	return 0
}

func (model Model) remainingRowsAfterList(visibleListRows int, reservedHelpRows int) int {
	if model.height <= 0 {
		return maxInt
	}
	remaining := model.height - model.fixedRowsWithoutDetailsOrHelp() - visibleListRows
	if model.selection.ShowHelp {
		remaining -= 2 // help spacing and section title
		remaining -= reservedHelpRows
	}
	if remaining < 1 {
		return 1
	}
	return remaining
}

func (model Model) remainingRowsAfterListAndDetails(visibleListRows int, visibleDetailsRows int) int {
	if model.height <= 0 {
		return maxInt
	}
	remaining := model.height - model.fixedRowsWithoutDetailsOrHelp() - visibleListRows - visibleDetailsRows
	if model.selection.ShowHelp {
		remaining -= 2 // help spacing and section title
	}
	if remaining < 1 {
		return 1
	}
	return remaining
}

func (model Model) fixedRowsWithoutDetailsOrHelp() int {
	rows := 0
	rows += 2 // header
	rows += 6 // runtime summary
	rows += 2 // selectable list spacing and title
	rows += 2 // details pane spacing and title
	if model.lastError != nil {
		rows += 3
	}
	rows += 4 // footer
	return rows
}

func (model Model) renderHelp(maxRows int) string {
	var output strings.Builder
	for _, line := range []string{
		"  q quit",
		"  j/k or arrows move",
		"  d/enter details",
		"  r refresh",
		"  ? help",
	} {
		fmt.Fprintln(&output, dashboardStyles.help.Render(line))
	}
	return truncateRows(output.String(), maxRows, helpTruncatedIndicator, model.contentWidth())
}

func truncateRows(value string, maxRows int, indicator string, width int) string {
	if maxRows <= 0 {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(value, "\n"), "\n")
	if len(lines) <= maxRows {
		return value
	}
	if maxRows == 1 {
		return truncateValue(indicator, width) + "\n"
	}
	kept := append([]string{}, lines[:maxRows-1]...)
	kept = append(kept, truncateValue(indicator, width))
	return strings.Join(kept, "\n") + "\n"
}

const maxInt = int(^uint(0) >> 1)

func clampInt(value int, minimum int, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func (model Model) renderFooter() string {
	width := model.contentWidth()
	help := truncateValue("  q quit | j/k or arrows move | d/enter details | r refresh | ? help", width)
	refresh := truncateValue(fmt.Sprintf("  refresh: every %s | last success: %s | source: %s",
		model.refreshInterval,
		fallbackRefresh(model.lastRefresh),
		fallback(model.source, "unknown")), width)
	return fmt.Sprintf(
		"\n%s\n%s\n%s\n",
		sectionTitle("Footer"),
		dashboardStyles.footer.Render(help),
		dashboardStyles.footer.Render(refresh),
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

func (model Model) renderDetails(output io.Writer, items []dashboard.SelectionItem, selected int) {
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
		model.detailText(output, "type", "provider")
		model.detailText(output, "name", fallback(item.ProviderName, "(unknown)"))
		model.detailText(output, "status", fallback(health.Status, "unknown")+itemWarning(item))
		model.detailText(output, "updated", fallback(health.UpdatedAt, "(unknown)"))
		model.detailText(output, "note", fallback(health.Note, "(none)"))
		model.detailText(output, "guidance", providerGuidance(health))
	case dashboard.SelectionTask:
		task := item.Task
		model.detailText(output, "type", "task")
		model.detailText(output, "slug", fallback(task.Slug, "(unknown)"))
		model.detailText(output, "state", fallback(firstNonEmpty(task.NormalizedState, task.Status), "(unknown)"))
		model.detailText(output, "branch", fallback(task.Branch, "(unknown)"))
		model.detailPath(output, "worktree", fallback(taskWorktreePath(task), "(unknown)"))
		model.detailText(output, "worktree exists", optionalTaskBool(task.WorktreeExists, task.Worktree))
		model.detailText(output, "provider/profile", fmt.Sprintf("%s / %s", fallback(taskProvider(task), "(unknown)"), fallback(taskProfile(task), "(unknown)")))
		model.detailText(output, "latest run", fallback(taskLatestRun(task), "(none)"))
		if task.Context != nil {
			detail(output, "context", "%d materialized, %d missing", task.Context.MaterializedFileCount, len(task.Context.MissingFiles))
		} else {
			model.detailText(output, "context", "(unknown)")
		}
	case dashboard.SelectionCleanup:
		candidate := item.CleanupCandidate
		model.detailText(output, "type", "cleanup candidate")
		model.detailText(output, "id", fallback(candidate.ID, "(unknown)"))
		model.detailText(output, "severity/category", fmt.Sprintf("%s / %s", fallback(candidate.Severity, "(unknown)"), fallback(candidate.Category, "(unknown)")))
		model.detailPath(output, "path", fallback(candidate.Path, "(none)"))
		model.detailText(output, "branch", fallback(candidate.Branch, "(none)"))
		detail(output, "dirty", "%t", candidate.Dirty)
		model.detailText(output, "removable by execute", optionalBool(candidate.RemovableByExecute))
		model.detailText(output, "destructive if unmerged", optionalBool(candidate.DestructiveIfUnmerged))
		model.renderStringList(output, "dirty reasons", candidate.DirtyReasons)
		model.renderStringList(output, "suggested commands", candidate.SuggestedCommands)
	case dashboard.SelectionAction:
		model.detailText(output, "type", "suggested action")
		model.detailText(output, "action", fallback(item.ActionText, "(none)"))
		model.detailText(output, "guidance", "display-only; no command is executed from this dashboard")
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
