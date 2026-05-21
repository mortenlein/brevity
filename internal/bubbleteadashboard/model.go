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
	"github.com/mortenlein/brevity/internal/pscontract"
	"github.com/mortenlein/brevity/internal/runtimeclient"
	"golang.org/x/term"
)

const defaultRefreshInterval = 5 * time.Second
const minimumVisibleListRows = 1
const ultraSmallHeightThreshold = 3
const fullHelpRows = 5
const detailTruncatedIndicator = "  ... details truncated"
const helpTruncatedIndicator = "  ... help truncated"
const runtimeSignalsTitle = "Runtime Signals"
const selectedDetailTitle = "Selected Detail"
const emptyRuntimeSignalsTitle = "No runtime signals"
const emptyRuntimeSignalsAuthority = "PowerShell backend is authoritative. This dashboard is read-only."
const emptyRuntimeSignalsRefresh = "Refresh to re-read state."

type refreshMsg struct {
	state contracts.RuntimeState
	err   error
	at    time.Time
}

type tickMsg time.Time

type commandResultMsg struct {
	result pscontract.ExecutionResult
}

type Model struct {
	client          runtimeclient.Client
	commandBridge   DashboardCommandBridge
	selection       dashboard.InteractiveModel
	state           contracts.RuntimeState
	hasState        bool
	paletteOpen     bool
	paletteSelected int
	helpOpen        bool
	actionPreview   *ActionDescriptor
	confirmation    *pscontract.ConfirmationState
	commandResult   *pscontract.ExecutionResult
	width           int
	height          int
	hasWindowSize   bool
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
		commandBridge:   RuntimeClientCommandBridge{Client: client},
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
		updated, cmd := model.Update(lineKeyMsg(key))
		model = updated.(Model)
		if key == "q" {
			return nil
		}
		if cmd != nil {
			updated, _ = model.Update(cmd())
			model = updated.(Model)
		}
		fmt.Fprint(stdout, model.View())
	}
	return scanner.Err()
}

func lineKeyMsg(key string) tea.KeyMsg {
	switch strings.ToLower(key) {
	case "", "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+p":
		return tea.KeyMsg{Type: tea.KeyCtrlP}
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
		model.hasWindowSize = true
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
	case commandResultMsg:
		model.commandResult = &msg.result
		return model, nil
	default:
		return model, nil
	}
}

func (model Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if model.actionPreview != nil {
		switch msg.String() {
		case "esc", "q", "p", "ctrl+p":
			model.actionPreview = nil
			return model, nil
		default:
			return model, nil
		}
	}

	if model.confirmation != nil {
		switch msg.String() {
		case "esc", "q", "n":
			model.confirmation = nil
			return model, nil
		default:
			return model, nil
		}
	}

	if model.commandResult != nil {
		switch msg.String() {
		case "esc", "q":
			model.commandResult = nil
			return model, nil
		default:
			return model, nil
		}
	}

	if model.helpOpen {
		switch msg.String() {
		case "esc", "q", "?":
			model.helpOpen = false
			return model, nil
		default:
			return model, nil
		}
	}

	if model.paletteOpen {
		switch msg.String() {
		case "esc", "q", "p", "ctrl+p":
			model.paletteOpen = false
			return model, nil
		case "j", "down":
			model.movePaletteSelection(1)
			return model, nil
		case "k", "up":
			model.movePaletteSelection(-1)
			return model, nil
		case "enter":
			action := actionDescriptors()[model.clampedPaletteSelection()]
			if !action.Enabled {
				model.paletteOpen = false
				model.actionPreview = &action
				return model, nil
			}
			cmd := model.commandForAction(action)
			if cmd == nil {
				return model, nil
			}
			model.paletteOpen = false
			return model, cmd
		default:
			return model, nil
		}
	}

	itemCount := len(dashboard.SelectableItems(model.state))
	switch msg.String() {
	case "q", "ctrl+c":
		return model, tea.Quit
	case "p", "ctrl+p":
		model.paletteOpen = true
		model.paletteSelected = 0
		return model, nil
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
		model.helpOpen = true
		return model, nil
	case "r":
		return model, model.refreshCmd()
	default:
		return model, nil
	}
}

func (model Model) View() string {
	if model.usesUltraSmallHeightMode() {
		return model.renderUltraSmallHeightView()
	}
	if !model.hasState {
		return model.renderLoadingView()
	}

	var output strings.Builder
	output.WriteString(model.renderHeader())
	output.WriteString(model.renderSummary())
	output.WriteString(model.renderListAndDetails())
	if model.lastError != nil {
		output.WriteString("\n")
		output.WriteString(sectionTitle("Warnings"))
		output.WriteString("\n")
		output.WriteString(model.renderRuntimeErrorLine())
	}
	if model.paletteOpen {
		output.WriteString(model.renderActionPalette(renderedRows(output.String())))
	}
	if model.actionPreview != nil {
		output.WriteString(model.renderActionPreview(renderedRows(output.String())))
	}
	if model.confirmation != nil {
		output.WriteString(model.renderConfirmation(renderedRows(output.String())))
	}
	if model.commandResult != nil {
		output.WriteString(model.renderCommandResult(renderedRows(output.String())))
	}
	if model.helpOpen {
		output.WriteString(model.renderHelpOverlay(renderedRows(output.String())))
	}
	return model.renderWithPinnedFooter(output.String())
}

func (model Model) usesUltraSmallHeightMode() bool {
	return model.height > 0 && model.height <= ultraSmallHeightThreshold || model.hasWindowSize && model.height <= ultraSmallHeightThreshold
}

func (model Model) renderUltraSmallHeightView() string {
	if model.hasWindowSize && model.height <= 0 {
		return ""
	}

	width := model.contentWidth()
	segments := []statusSegment{
		{text: "Brevity", priority: 0},
	}
	if !model.hasState {
		segments = append(segments, statusSegment{text: "loading", priority: 0})
	}
	if model.lastError != nil || model.hasState && model.warningCounts().total() > 0 {
		segments = append(segments, statusSegment{text: "warning", priority: 0})
	}
	segments = append(segments,
		statusSegment{text: fallback(model.source, "unknown"), priority: 0},
		statusSegment{text: "read-only", priority: 0},
		statusSegment{text: "q quit", priority: 1},
	)

	line := dashboardStyles.title.Render(statusLine(width, segments...))
	return truncateValue(line, width) + "\n"
}

func (model Model) renderLoadingView() string {
	var output strings.Builder
	output.WriteString(model.renderHeader())
	output.WriteString("\n")
	renderSection(&output, "Runtime Summary")
	output.WriteString(model.renderLine("  status     Loading runtime state") + "\n")
	output.WriteString(model.renderLine("  source     "+fallback(model.source, "unknown")+" / read-only") + "\n")
	output.WriteString(model.renderLine("  authority  PowerShell runtime state") + "\n")
	output.WriteString("\n")
	renderSection(&output, runtimeSignalsTitle)
	output.WriteString(model.renderEmptyRuntimeSignals())
	if model.lastError != nil {
		output.WriteString("\n")
		renderSection(&output, "Warnings")
		output.WriteString(model.renderRuntimeErrorLine())
	}
	return model.renderWithPinnedFooter(output.String())
}

func (model Model) renderWithPinnedFooter(body string) string {
	footer := model.renderFooter()
	if model.height <= 0 {
		return body + footer
	}
	paddingRows := model.height - renderedRows(body) - renderedRows(footer)
	if paddingRows > 0 {
		body += strings.Repeat("\n", paddingRows)
	}
	return body + footer
}

func (model Model) renderRuntimeErrorLine() string {
	message := dashboardStyles.error.Render(fmt.Sprintf("polling error  %v", model.lastError))
	return model.renderLine(fmt.Sprintf("  %s %s", warningMarker(), message)) + "\n"
}

func (model Model) renderHeader() string {
	width := model.contentWidth()
	statusText := statusLine(width, model.headerStatusSegments()...)
	return dashboardStyles.title.Render(statusText) + "\n"
}

func (model Model) headerStatusSegments() []statusSegment {
	source := fallback(model.source, "unknown")
	segments := []statusSegment{
		{text: "Brevity", priority: 0},
		{text: source, priority: 0},
		{text: "read-only", priority: 0},
	}

	if !model.hasState {
		if model.lastError != nil {
			segments = append(segments, statusSegment{text: "error", priority: 0})
		} else {
			segments = append(segments, statusSegment{text: "loading", priority: 0})
		}
		return segments
	}

	warnings := model.warningCounts()
	if model.lastError != nil {
		segments = append(segments, statusSegment{text: headerAlertText("error", warnings.total()), compact: headerAlertCompact("error", warnings.total()), priority: 0})
	} else if warnings.total() > 0 {
		segments = append(segments, statusSegment{text: headerAlertText("alerts", warnings.total()), compact: headerAlertCompact("alerts", warnings.total()), priority: 0})
	} else {
		segments = append(segments, statusSegment{text: "ok", priority: 2})
	}

	if warnings.total() > 0 {
		segments = append(segments, statusSegment{text: fmt.Sprintf("p:%d t:%d c:%d", warnings.provider, warnings.task, warnings.cleanup), priority: 2})
	}
	if strings.TrimSpace(model.state.GeneratedAt) != "" {
		segments = append(segments, statusSegment{text: "generated " + model.state.GeneratedAt, compact: model.state.GeneratedAt, priority: 3})
	}
	return segments
}

func headerAlertText(label string, count int) string {
	if count <= 0 {
		return label
	}
	return fmt.Sprintf("%s !%d", label, count)
}

func headerAlertCompact(label string, count int) string {
	if count <= 0 {
		return label
	}
	return fmt.Sprintf("!%d", count)
}

func renderSection(output *strings.Builder, title string) {
	fmt.Fprintln(output, sectionTitle(title))
}

func renderWarningCount(count int) string {
	if count > 0 {
		return fmt.Sprintf(" !%d", count)
	}
	return " ok"
}

func renderWarningText(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return " " + dashboardStyles.warning.Render(text)
}

type dashboardWarningCounts struct {
	provider int
	task     int
	cleanup  int
}

func (counts dashboardWarningCounts) total() int {
	return counts.provider + counts.task + counts.cleanup
}

func (model Model) warningCounts() dashboardWarningCounts {
	if !model.hasState {
		return dashboardWarningCounts{}
	}
	state := model.state
	counts := dashboardWarningCounts{
		provider: state.Providers.Summary.Degraded + state.Providers.Summary.Unavailable,
		task:     state.TaskCounts.Blocked + state.TaskCounts.Stale + state.TaskCounts.ProviderGated + state.TaskCounts.Review,
	}
	if state.Cleanup != nil && state.Cleanup.Summary != nil {
		counts.cleanup = state.Cleanup.Summary.TotalCandidates
	}
	return counts
}

func kindBadge(kind string) string {
	switch kind {
	case string(dashboard.SelectionProvider):
		return statusBadge("prov", "accent")
	case string(dashboard.SelectionTask):
		return statusBadge("task", "")
	case string(dashboard.SelectionActivity):
		return statusBadge("run", "")
	case string(dashboard.SelectionCleanup):
		return statusBadge("clean", "warning")
	case string(dashboard.SelectionAction):
		return statusBadge("next", "")
	default:
		return statusBadge(kind, "")
	}
}

func kindLabel(kind string) string {
	switch kind {
	case string(dashboard.SelectionProvider):
		return "prov"
	case string(dashboard.SelectionTask):
		return "task"
	case string(dashboard.SelectionActivity):
		return "run"
	case string(dashboard.SelectionCleanup):
		return "clean"
	case string(dashboard.SelectionAction):
		return "next"
	default:
		return kind
	}
}

func (model Model) selectedRow(text string) string {
	return dashboardStyles.selectedRow.Render(text)
}

func (model Model) unselectedRow(text string) string {
	return text
}

func (model Model) renderRow(selected bool, kind string, label string, warning string) string {
	primary, meaning := rowPrimaryAndMeaning(kind, label)
	prefix := fmt.Sprintf("%s %-5s ", rowMarker(selected), kindLabel(kind))
	suffix := warning
	available := model.contentWidth() - lipglossWidth(prefix) - lipglossWidth(suffix)
	line := prefix + renderRowBody(primary, meaning, available) + suffix
	if selected {
		return model.selectedRow(line)
	}
	return model.unselectedRow(line)
}

func renderRowBody(primary string, meaning string, width int) string {
	primary = strings.TrimSpace(primary)
	meaning = strings.TrimSpace(meaning)
	if width <= 0 {
		return ""
	}
	if meaning == "" {
		return truncateValue(primary, width)
	}
	if width < 18 {
		return truncateValuePreservingWarning(primary+" "+meaning, width)
	}

	primaryWidth := visibleWidth(primary)
	columnWidth := clampInt(primaryWidth, 7, 16)
	if width < columnWidth+2+1 {
		return truncateValuePreservingWarning(primary+" "+meaning, width)
	}
	meaningWidth := width - columnWidth - 2
	return padRight(primary, columnWidth) + "  " + truncateValuePreservingWarning(meaning, meaningWidth)
}

func rowPrimaryAndMeaning(kind string, label string) (string, string) {
	primary, meaning, ok := strings.Cut(strings.TrimSpace(label), ": ")
	if !ok {
		primary = strings.TrimSpace(label)
	}
	switch kind {
	case string(dashboard.SelectionCleanup):
		return cleanupSignal(primary), meaning
	case string(dashboard.SelectionAction):
		fields := strings.Fields(label)
		if len(fields) == 0 {
			return "(none)", ""
		}
		signal := strings.ToLower(fields[0])
		if !startsWithLetter(signal) {
			return "note", strings.TrimSpace(label)
		}
		return signal, strings.Join(fields[1:], " ")
	default:
		return primary, meaning
	}
}

func startsWithLetter(value string) bool {
	for _, r := range value {
		return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z'
	}
	return false
}

func cleanupSignal(category string) string {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "requires-inspection", "inspect", "needs-inspection":
		return "inspect"
	case "orphan-worktree", "orphaned-worktree":
		return "worktree"
	case "orphan-branch", "orphaned-branch":
		return "branch"
	case "destructive-if-removed", "destructive-if-unmerged":
		return "destruct"
	default:
		return fallback(category, "cleanup")
	}
}

func rowMarker(selected bool) string {
	if selected {
		return ">"
	}
	return " "
}

func detail(output io.Writer, label string, format string, args ...any) {
	fmt.Fprintln(output, detailLineWithWidth(label, fmt.Sprintf(format, args...), detailLabelWidth))
}

func detailText(output io.Writer, label string, value string) {
	fmt.Fprintln(output, detailLineWithWidth(label, value, detailLabelWidth))
}

func detailBreak(output io.Writer) {
	fmt.Fprintln(output)
}

func (model Model) renderSummary() string {
	state := model.state
	var output strings.Builder
	fmt.Fprintln(&output)
	renderSection(&output, "Runtime Summary")
	fmt.Fprintf(&output, "  repo      %s\n", dashboardStyles.detailPrimary.Render(model.renderInlinePath(fallback(state.RepoRoot, "(unknown)"), len("  repo      "))))
	fmt.Fprintf(&output, "%s\n", model.renderLine("  generated "+dashboardStyles.headerMeta.Render(fallback(state.GeneratedAt, "(unknown)"))))
	fmt.Fprintf(&output, "%s\n", model.renderLine(fmt.Sprintf("  status    %s | %d runnable | %s%s",
		providerStatusSummary(state.Providers.Summary),
		state.TaskCounts.Runnable,
		cleanupCandidateSummary(state),
		renderWarningCount(runtimeSummaryWarningCount(state)),
	)))
	fmt.Fprintf(&output, "%s\n", model.renderLine(fmt.Sprintf("  tasks     %d tracked | %d runnable | %d blocked | %d stale | %d gated | %d review%s",
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
		fmt.Fprintf(&output, "%s\n", model.renderLine(fmt.Sprintf("  cleanup   %d candidates | %d inspect%s",
			summary.TotalCandidates,
			summary.RequiresInspectionCount,
			renderWarningCount(summary.TotalCandidates),
		)))
	} else {
		fmt.Fprintln(&output, "  cleanup   none")
	}
	return output.String()
}

func providerStatusSummary(summary contracts.ProviderSummary) string {
	if summary.Degraded == 0 && summary.Unavailable == 0 {
		return fmt.Sprintf("%d providers ok", summary.Total)
	}
	return fmt.Sprintf("%d providers | %d degraded | %d unavailable", summary.Total, summary.Degraded, summary.Unavailable)
}

func cleanupCandidateSummary(state contracts.RuntimeState) string {
	if state.Cleanup == nil || state.Cleanup.Summary == nil {
		return "no cleanup data"
	}
	count := state.Cleanup.Summary.TotalCandidates
	if count == 1 {
		return "1 cleanup candidate"
	}
	return fmt.Sprintf("%d cleanup candidates", count)
}

func runtimeSummaryWarningCount(state contracts.RuntimeState) int {
	warnings := state.Providers.Summary.Degraded + state.Providers.Summary.Unavailable
	warnings += state.TaskCounts.Blocked + state.TaskCounts.Stale + state.TaskCounts.ProviderGated
	if state.Cleanup != nil && state.Cleanup.Summary != nil {
		warnings += state.Cleanup.Summary.TotalCandidates
	}
	return warnings
}

func (model Model) renderListAndDetails() string {
	if model.usesTwoPaneLayout() {
		return model.renderTwoPaneListAndDetails()
	}
	return model.renderSingleColumnListAndDetails()
}

func (model Model) renderSingleColumnListAndDetails() string {
	items := dashboard.SelectableItems(model.state)
	selection := model.selection
	selection.Clamp(len(items))
	window := model.selectableListWindow(len(items), selection.SelectedIndex)
	detailsRows := model.visibleDetailsRows(window.end - window.start)
	helpRows := model.visibleHelpRows(window.end-window.start, detailsRows)

	var output strings.Builder
	fmt.Fprintln(&output)
	renderSection(&output, runtimeSignalsTitle)
	if len(items) == 0 {
		output.WriteString(model.renderEmptyRuntimeSignals())
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
	renderSection(&output, selectedDetailTitle)
	var details strings.Builder
	if selection.ShowDetails {
		model.renderDetails(&details, items, selection.SelectedIndex)
	} else {
		fmt.Fprintln(&details, model.renderLine("  select a row, then press d for details"))
	}
	output.WriteString(truncateRows(details.String(), detailsRows, detailTruncatedIndicator, model.contentWidth()))
	if selection.ShowHelp {
		fmt.Fprintln(&output)
		renderSection(&output, "Help")
		output.WriteString(model.renderHelp(helpRows))
	}
	return output.String()
}

func (model Model) renderTwoPaneListAndDetails() string {
	items := dashboard.SelectableItems(model.state)
	selection := model.selection
	selection.Clamp(len(items))

	leftWidth, rightWidth := model.paneWidths()
	leftModel := model.withContentWidth(leftWidth)
	rightModel := model.withContentWidth(rightWidth)
	paneRows := model.visibleTwoPaneRows()
	listRows := paneRows - 2
	if listRows < minimumVisibleListRows {
		listRows = minimumVisibleListRows
	}
	window := model.selectableListWindowForRows(len(items), selection.SelectedIndex, listRows)

	var left strings.Builder
	fmt.Fprintln(&left, paneTitle(runtimeSignalsTitle))
	if len(items) == 0 {
		left.WriteString(leftModel.renderEmptyRuntimeSignals())
	} else {
		if window.truncated {
			fmt.Fprintf(&left, "  showing %d-%d of %d\n", window.start+1, window.end, len(items))
		}
		for index := window.start; index < window.end; index++ {
			item := items[index]
			fmt.Fprintln(&left, leftModel.renderRow(index == selection.SelectedIndex, string(item.Kind), item.Label, itemWarning(item)))
		}
	}

	var right strings.Builder
	fmt.Fprintln(&right, paneTitle(selectedDetailTitle))
	var details strings.Builder
	if selection.ShowDetails {
		rightModel.renderDetails(&details, items, selection.SelectedIndex)
	} else {
		fmt.Fprintln(&details, rightModel.renderLine("  select a row, then press d for details"))
	}
	detailsRows := paneRows - 2
	if selection.ShowHelp {
		detailsRows -= 2 + fullHelpRows
	}
	if detailsRows < 1 {
		detailsRows = 1
	}
	right.WriteString(truncateRows(details.String(), detailsRows, detailTruncatedIndicator, rightWidth))
	if selection.ShowHelp {
		fmt.Fprintln(&right)
		renderSection(&right, "Help")
		helpRows := paneRows - 2 - detailsRows - 2
		if helpRows < 1 {
			helpRows = 1
		}
		right.WriteString(rightModel.renderHelp(helpRows))
	}

	return "\n" + joinPaneLines(left.String(), right.String(), leftWidth, rightWidth)
}

func (model Model) renderEmptyRuntimeSignals() string {
	var output strings.Builder
	for _, line := range []string{
		"  " + emptyRuntimeSignalsTitle,
		"  " + emptyRuntimeSignalsAuthority,
		"  " + emptyRuntimeSignalsRefresh,
	} {
		fmt.Fprintln(&output, model.renderLine(line))
	}
	return output.String()
}

func (model Model) paneWidths() (int, int) {
	width := model.contentWidth()
	separatorWidth := visibleWidth(paneSeparator)
	available := width - separatorWidth
	leftWidth := available * 42 / 100
	if leftWidth < 36 {
		leftWidth = 36
	}
	rightWidth := available - leftWidth
	if rightWidth < 36 {
		rightWidth = 36
		leftWidth = available - rightWidth
	}
	return leftWidth, rightWidth
}

func (model Model) visibleTwoPaneRows() int {
	if model.height <= 0 {
		return maxInt
	}
	rows := model.height
	rows -= 1 // header
	rows -= 6 // runtime summary
	if model.lastError != nil {
		rows -= 3
	}
	rows -= 2 // footer
	if rows < minimumVisibleListRows+2 {
		return minimumVisibleListRows + 2
	}
	return rows
}

func joinPaneLines(left string, right string, leftWidth int, rightWidth int) string {
	leftLines := strings.Split(strings.TrimSuffix(left, "\n"), "\n")
	rightLines := strings.Split(strings.TrimSuffix(right, "\n"), "\n")
	lineCount := len(leftLines)
	if len(rightLines) > lineCount {
		lineCount = len(rightLines)
	}
	var output strings.Builder
	for index := 0; index < lineCount; index++ {
		leftLine := ""
		if index < len(leftLines) {
			leftLine = leftLines[index]
		}
		rightLine := ""
		if index < len(rightLines) {
			rightLine = rightLines[index]
		}
		fmt.Fprintf(&output, "%s%s%s\n", padRight(leftLine, leftWidth), paneSeparator, truncateValue(rightLine, rightWidth))
	}
	return output.String()
}

type listWindow struct {
	start     int
	end       int
	truncated bool
}

func (model Model) selectableListWindow(itemCount int, selected int) listWindow {
	return model.selectableListWindowForRows(itemCount, selected, model.visibleSelectableRows())
}

func (model Model) selectableListWindowForRows(itemCount int, selected int, visibleRows int) listWindow {
	if itemCount <= 0 {
		return listWindow{}
	}
	selected = clampInt(selected, 0, itemCount-1)
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
	reservedRows += 1 // header
	reservedRows += 6 // runtime summary
	reservedRows += 2 // selectable list spacing and title
	reservedRows += 4 // details pane, collapsed or minimum visible details
	if model.selection.ShowHelp {
		reservedRows += 7
	}
	if model.lastError != nil {
		reservedRows += 3
	}
	reservedRows += 2 // footer

	visibleRows := model.height - reservedRows
	if visibleRows < minimumVisibleListRows {
		return minimumVisibleListRows
	}
	return visibleRows
}

func (model Model) visibleDetailsRows(visibleListRows int) int {
	rows := model.remainingRowsAfterList(visibleListRows, model.minimumHelpRows())
	if model.height > 0 && rows > 1 {
		rows--
	}
	return rows
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
	rows += 1 // header
	rows += 6 // runtime summary
	rows += 2 // selectable list spacing and title
	rows += 2 // details pane spacing and title
	if model.lastError != nil {
		rows += 3
	}
	rows += 2 // footer
	return rows
}

func (model Model) renderHelp(maxRows int) string {
	var output strings.Builder
	for _, line := range []string{
		"  up/down or j/k move   r refresh",
		"  p actions   d details   ? help",
		"  q quit   esc closes actions",
	} {
		fmt.Fprintln(&output, dashboardStyles.help.Render(line))
	}
	return truncateRows(output.String(), maxRows, helpTruncatedIndicator, model.contentWidth())
}

func (model Model) renderHelpOverlay(usedRows ...int) string {
	if model.height > 0 && model.height <= ultraSmallHeightThreshold {
		return ""
	}

	maxRows := maxInt
	if model.height > 0 {
		used := 0
		if len(usedRows) > 0 {
			used = usedRows[0]
		}
		maxRows = model.height - used - renderedRows(model.renderFooter()) - 2
		if maxRows < 1 {
			maxRows = 1
		}
	}

	var output strings.Builder
	fmt.Fprintln(&output)
	renderSection(&output, "Help")
	lines := []string{
		"  navigate with up/down or j/k; d toggles selected details",
		"  r refreshes runtime state through the command bridge",
		"  p opens actions; read-only actions can execute PowerShell commands",
		"  mutating actions are disabled and show a blocked command preview",
		"  PowerShell remains authoritative for Brevity state",
		"  Go does not write .brevity or start providers/workers",
		"  executable today: Refresh state, Provider status, Task status",
		"  esc, q, p, or ? closes panels without running commands",
	}
	var body strings.Builder
	for _, line := range lines {
		fmt.Fprintln(&body, dashboardStyles.help.Render(model.renderLine(line)))
	}
	output.WriteString(truncateRows(body.String(), maxRows, helpTruncatedIndicator, model.contentWidth()))
	return output.String()
}

func (model Model) renderActionPreview(usedRows ...int) string {
	action := model.actionPreview
	if action == nil {
		return ""
	}
	lines := []string{
		"  action        " + action.Label,
		"  status        disabled / future",
		"  command       " + model.previewCommandShape(*action),
		"  blocked       " + fallback(action.Command.DisabledReason, "descriptor is not enabled"),
		"  confirm       confirmation required before any future execution",
		"  authority     PowerShell is authoritative; Go will not write .brevity",
		"  close         esc, q, or p returns to the dashboard",
	}
	return model.renderPanel("Command Preview", lines, helpTruncatedIndicator, usedRows...)
}

func (model Model) previewCommandShape(action ActionDescriptor) string {
	invocation, err := pscontract.BuildInvocation(action.Command, `.\\brevity.ps1`, nil, true)
	if err != nil {
		return "(unavailable)"
	}
	return invocation.Display()
}

func (model Model) renderConfirmation(usedRows ...int) string {
	if model.confirmation == nil {
		return ""
	}
	state := *model.confirmation
	lines := []string{
		"  action        " + string(state.ActionID),
		"  status        not executable unless enabled by command descriptor",
		"  prompt        " + state.Prompt,
		"  authority     PowerShell is authoritative; Go does not mutate task state",
	}
	if state.Strength == pscontract.ConfirmationDestructive {
		lines = append(lines, "  warning       destructive action; cleanup can remove branches or worktrees")
	}
	lines = append(lines, "  cancel        esc, q, or n cancels")
	return model.renderPanel("Confirm Action", lines, helpTruncatedIndicator, usedRows...)
}

func (model Model) renderCommandResult(usedRows ...int) string {
	if model.commandResult == nil {
		return ""
	}
	result := *model.commandResult
	status := "success"
	if !result.Success() {
		status = "failure"
	}
	lines := []string{
		"  action        " + fallback(result.CommandDisplayLabel, string(result.ActionID)),
		"  status        " + status,
		"  exit code     " + fmt.Sprint(result.ExitCode),
		"  message       " + result.OperatorMessage(),
	}
	if result.TimedOut {
		lines = append(lines, "  timeout       command exceeded its read-only timeout")
	}
	if result.Canceled {
		lines = append(lines, "  canceled      command was canceled")
	}
	if stdout := strings.TrimSpace(result.Stdout); stdout != "" {
		lines = append(lines, commandOutputLines("stdout", stdout)...)
	} else if result.Success() {
		lines = append(lines, "  stdout        (empty)")
	}
	if stderr := strings.TrimSpace(result.Stderr); stderr != "" {
		lines = append(lines, commandOutputLines("stderr", stderr)...)
	}
	if result.Error != "" && strings.TrimSpace(result.Stderr) == "" {
		lines = append(lines, "  error         "+result.Error)
	}
	if result.ShouldRefresh() {
		lines = append(lines, "  follow-up     press r to refresh runtime state")
	}
	lines = append(lines, "  close         esc or q closes result")
	return model.renderPanel("Command Result", lines, detailTruncatedIndicator, usedRows...)
}

func commandOutputLines(label string, value string) []string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	output := make([]string, 0, len(lines))
	for index, line := range lines {
		line = strings.TrimRight(line, "\r")
		if index == 0 {
			output = append(output, fmt.Sprintf("  %-12s  %s", label, fallback(line, "(empty)")))
			continue
		}
		output = append(output, "                "+line)
	}
	return output
}

func (model Model) renderPanel(title string, lines []string, indicator string, usedRows ...int) string {
	if model.height > 0 && model.height <= ultraSmallHeightThreshold {
		return ""
	}
	maxRows := maxInt
	if model.height > 0 {
		used := 0
		if len(usedRows) > 0 {
			used = usedRows[0]
		}
		maxRows = model.height - used - renderedRows(model.renderFooter()) - 2
		if maxRows < 1 {
			maxRows = 1
		}
	}
	var output strings.Builder
	fmt.Fprintln(&output)
	renderSection(&output, title)
	var body strings.Builder
	for _, line := range lines {
		fmt.Fprintln(&body, dashboardStyles.help.Render(model.renderLine(line)))
	}
	output.WriteString(truncateRows(body.String(), maxRows, indicator, model.contentWidth()))
	return output.String()
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
	source := fallback(model.source, "unknown")
	footer := statusLine(width,
		statusSegment{text: "up/down or j/k move | r refresh | p actions | d details | q quit | ? help", compact: "j/k r p d q quit ? help", priority: 0},
		statusSegment{text: source, priority: 1},
		statusSegment{text: "read-only", priority: 1},
		statusSegment{text: fmt.Sprintf("%s refresh", model.refreshInterval), compact: fmt.Sprintf("%s refresh", model.refreshInterval), priority: 2},
		statusSegment{text: "last " + fallbackRefresh(model.lastRefresh), priority: 3},
	)
	return "\n" + dashboardStyles.footer.Render(footer) + "\n"
}

func (model Model) renderActionPalette(usedRows ...int) string {
	var output strings.Builder
	fmt.Fprintln(&output)
	renderSection(&output, "Actions")
	selected := model.clampedPaletteSelection()
	for index, action := range actionDescriptors() {
		fmt.Fprintln(&output, model.renderPaletteActionRow(action, index == selected))
	}
	if model.shouldRenderActionPaletteHelp(usedRows...) {
		fmt.Fprintln(&output, dashboardStyles.help.Render(model.renderLine("  enter runs read-only actions | esc closes | p toggles")))
	}
	return output.String()
}

func (model Model) renderPaletteActionRow(action ActionDescriptor, selected bool) string {
	cursor := " "
	if selected {
		cursor = ">"
	}
	status := action.Description
	if !action.Enabled {
		status = dashboardStyles.disabledAction.Render(status)
	} else {
		status = dashboardStyles.enabledAction.Render(status)
	}

	width := model.contentWidth()
	prefix := fmt.Sprintf("%s ", cursor)
	labelWidth := 16
	if width < 42 {
		labelWidth = 14
	}
	statusWidth := width - visibleWidth(prefix) - labelWidth - 2
	if statusWidth < 6 {
		statusWidth = 6
	}
	line := prefix + padRight(action.Label, labelWidth) + "  " + truncateValue(status, statusWidth)
	line = model.renderLine(line)
	if selected {
		return model.selectedRow(line)
	}
	return line
}

func (model Model) shouldRenderActionPaletteHelp(usedRows ...int) bool {
	if model.height <= 0 {
		return true
	}
	used := 0
	if len(usedRows) > 0 {
		used = usedRows[0]
	}
	footerRows := renderedRows(model.renderFooter())
	paletteRowsWithHelp := 1 + 1 + len(actionDescriptors()) + 1
	return used+paletteRowsWithHelp+footerRows <= model.height
}

func (model *Model) movePaletteSelection(delta int) {
	actionCount := len(actionDescriptors())
	if actionCount == 0 {
		model.paletteSelected = 0
		return
	}
	model.paletteSelected = (model.paletteSelected + delta + actionCount) % actionCount
}

func (model Model) clampedPaletteSelection() int {
	actionCount := len(actionDescriptors())
	if actionCount == 0 {
		return 0
	}
	return clampInt(model.paletteSelected, 0, actionCount-1)
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
		return warningMarkerSuffix("warning")
	}
	return ""
}

func warningMarkerSuffix(severity string) string {
	return " " + warningMarker()
}

func itemWarning(item dashboard.SelectionItem) string {
	switch item.Kind {
	case dashboard.SelectionProvider:
		status := strings.ToLower(strings.TrimSpace(item.ProviderHealth.Status))
		if status == "degraded" || status == "capacity-degraded" || status == "quota" || status == "quota-exhausted" || status == "rate-limited" || status == "unavailable" || status == "down" || status == "offline" {
			if status == "unavailable" || status == "down" || status == "offline" {
				return warningMarkerSuffix("error")
			}
			return warningMarkerSuffix("warning")
		}
	case dashboard.SelectionTask:
		state := strings.ToLower(strings.TrimSpace(item.Task.NormalizedState))
		if state == "" {
			state = strings.ToLower(strings.TrimSpace(item.Task.Status))
		}
		if state == "blocked" || state == "stale" || state == "failed" || state == "provider-gated" || state == "review" || state == "needs-review" {
			return warningMarkerSuffix("warning")
		}
	case dashboard.SelectionCleanup:
		severity := strings.ToLower(strings.TrimSpace(item.CleanupCandidate.Severity))
		if severity == "" {
			severity = "cleanup"
		}
		if severity == "error" || severity == "critical" || severity == "destructive" {
			return warningMarkerSuffix("error")
		}
		return warningMarkerSuffix("warning")
	}
	return ""
}

func (model Model) renderDetails(output io.Writer, items []dashboard.SelectionItem, selected int) {
	if len(items) == 0 {
		fmt.Fprintln(output, "  no details selected")
		return
	}
	if selected < 0 || selected >= len(items) {
		selected = 0
	}
	item := items[selected]
	switch item.Kind {
	case dashboard.SelectionProvider:
		health := item.ProviderHealth
		model.detailText(output, "status", fallback(health.Status, "unknown")+itemWarning(item))
		model.detailText(output, "provider", fallback(item.ProviderName, "(unknown)"))
		model.detailText(output, "reason", fallback(health.Note, "(none)"))
		model.detailText(output, "action", providerGuidance(health))
		model.detailText(output, "task impact", providerTaskImpact(health))
		model.detailText(output, "switch helps", providerSwitchHelps(health))
		detailBreak(output)
		model.detailText(output, "updated", fallback(health.UpdatedAt, "(unknown)"))
		model.detailText(output, "type", "provider")
	case dashboard.SelectionTask:
		task := item.Task
		model.detailText(output, "state", fallback(firstNonEmpty(task.NormalizedState, task.Status), "(unknown)")+itemWarning(item))
		model.detailText(output, "task", fallback(task.Slug, "(unknown)"))
		model.detailText(output, "action", taskGuidance(task))
		model.detailText(output, "provider", fmt.Sprintf("%s / %s", fallback(taskProvider(task), "(unknown)"), fallback(taskProfile(task), "(unknown)")))
		detailBreak(output)
		model.detailText(output, "branch", fallback(task.Branch, "(unknown)"))
		model.detailPath(output, "worktree", fallback(taskWorktreePath(task), "(unknown)"))
		model.detailText(output, "worktree exists", optionalTaskBool(task.WorktreeExists, task.Worktree))
		model.detailText(output, "latest run", fallback(taskLatestRun(task), "(none)"))
		if task.Context != nil {
			detail(output, "context", "%d materialized, %d missing", task.Context.MaterializedFileCount, len(task.Context.MissingFiles))
		} else {
			model.detailText(output, "context", "(unknown)")
		}
		model.detailText(output, "type", "task")
	case dashboard.SelectionActivity:
		model.detailText(output, "activity", fallback(item.Label, "(none)"))
		model.detailText(output, "guidance", "display-only runtime activity signal")
		detailBreak(output)
		model.detailText(output, "type", "run/activity")
	case dashboard.SelectionCleanup:
		candidate := item.CleanupCandidate
		model.detailText(output, "risk", fmt.Sprintf("%s / %s%s", fallback(candidate.Severity, "(unknown)"), fallback(candidate.Category, "(unknown)"), itemWarning(item)))
		model.detailPath(output, "path", fallback(candidate.Path, "(none)"))
		model.detailText(output, "action", cleanupGuidance(candidate))
		detailBreak(output)
		model.detailText(output, "id", fallback(candidate.ID, "(unknown)"))
		model.detailText(output, "branch", fallback(candidate.Branch, "(none)"))
		detail(output, "dirty", "%t", candidate.Dirty)
		model.detailText(output, "removable", optionalBool(candidate.RemovableByExecute))
		model.detailText(output, "destructive", optionalBool(candidate.DestructiveIfUnmerged))
		model.renderStringList(output, "dirty reasons", candidate.DirtyReasons)
		model.renderStringList(output, "suggested commands", candidate.SuggestedCommands)
		model.detailText(output, "type", "cleanup candidate")
	case dashboard.SelectionAction:
		model.detailText(output, "action", fallback(item.ActionText, "(none)"))
		model.detailText(output, "guidance", "display-only; no command is executed from this dashboard")
		detailBreak(output)
		model.detailText(output, "type", "suggested action")
	}
}

func providerGuidance(health contracts.ProviderHealth) string {
	switch strings.ToLower(strings.TrimSpace(health.Status)) {
	case "degraded", "capacity-degraded":
		return "provider is degraded; treat worker readiness with caution"
	case "quota", "quota-exhausted", "quota-constrained", "rate-limited":
		return "provider quota is constrained; choose another profile before starting work"
	case "unavailable", "down", "offline":
		return "provider is unavailable; PowerShell remains the authority for changes"
	case "unknown", "":
		return "provider health is unknown; refresh state before assuming readiness"
	default:
		return "(none)"
	}
}

func providerTaskImpact(health contracts.ProviderHealth) string {
	switch strings.ToLower(strings.TrimSpace(health.Status)) {
	case "degraded", "capacity-degraded", "quota", "quota-exhausted", "quota-constrained", "rate-limited", "unavailable", "down", "offline":
		return "affects task execution"
	case "unknown", "":
		return "task execution readiness unknown"
	default:
		return "no known task execution impact"
	}
}

func providerSwitchHelps(health contracts.ProviderHealth) string {
	switch strings.ToLower(strings.TrimSpace(health.Status)) {
	case "degraded", "capacity-degraded", "quota", "quota-exhausted", "quota-constrained", "rate-limited", "unavailable", "down", "offline":
		return "yes, if another healthy provider/profile is available"
	case "unknown", "":
		return "maybe; refresh or inspect provider health first"
	default:
		return "not indicated by current state"
	}
}

func taskGuidance(task contracts.TaskSummary) string {
	switch strings.ToLower(strings.TrimSpace(firstNonEmpty(task.NormalizedState, task.Status))) {
	case "blocked":
		return "inspect blocker context before rerun"
	case "failed":
		return "inspect latest run log before retry"
	case "stale":
		return "refresh runtime state before acting"
	case "provider-gated":
		return "check provider health before rerun"
	case "review":
		return "review branch and worktree state"
	default:
		return "(none)"
	}
}

func cleanupGuidance(candidate contracts.CleanupCandidate) string {
	category := strings.ToLower(strings.TrimSpace(candidate.Category))
	if strings.Contains(category, "orphan") || strings.Contains(category, "inspection") || strings.Contains(category, "destructive") || candidate.Dirty || optionalBool(candidate.DestructiveIfUnmerged) == "true" {
		return "inspect before cleanup"
	}
	if optionalBool(candidate.RemovableByExecute) == "true" {
		return "eligible for cleanup after review"
	}
	return "review cleanup candidate"
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
