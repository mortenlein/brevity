package bubbleteadashboard

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/dashboard"
	"github.com/mortenlein/brevity/internal/preflight"
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

type refreshStartedMsg struct{}

type tickMsg time.Time

type commandResultMsg struct {
	id     int
	result pscontract.ExecutionResult
}

type commandStatus string

const (
	commandIdle      commandStatus = "idle"
	commandQueued    commandStatus = "queued"
	commandPreparing commandStatus = "preparing"
	commandRunning   commandStatus = "running"
	commandStreaming commandStatus = "streaming"
	commandCompleted commandStatus = "completed"
	commandSucceeded commandStatus = "succeeded"
	commandFailed    commandStatus = "failed"
	commandTimedOut  commandStatus = "timed out"
	commandCanceled  commandStatus = "cancelled"
)

type commandRunState struct {
	id        int
	action    ActionDescriptor
	status    commandStatus
	result    *pscontract.ExecutionResult
	startedAt time.Time
	scroll    int
}

type commandActivity struct {
	id          int
	action      string
	status      commandStatus
	completedAt time.Time
	summary     string
	result      *pscontract.ExecutionResult
	showDetails bool
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
	confirmAction   *ActionDescriptor
	confirmArgs     []string
	commandRun      *commandRunState
	activities      []commandActivity
	nextCommandID   int
	width           int
	height          int
	hasWindowSize   bool
	lastRefresh     string
	lastError       error
	polling         bool
	pollingEnabled  bool
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
			msg := cmd()
			updated, followup := model.Update(msg)
			model = updated.(Model)
			if followup != nil {
				updated, _ = model.Update(followup())
				model = updated.(Model)
			}
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
	case refreshStartedMsg:
		if model.polling {
			return model, nil
		}
		model.polling = true
		return model, model.refreshCmd()
	case refreshMsg:
		model.polling = false
		if msg.err != nil {
			model.lastError = msg.err
			return model, nil
		}
		model.state = msg.state
		model.hasState = true
		model.lastError = nil
		model.lastRefresh = msg.at.Format(time.RFC3339)
		model.selection.Clamp(len(model.selectableItems()))
		return model, nil
	case tickMsg:
		cmds := []tea.Cmd{model.tickCmd()}
		if model.pollingEnabled && !model.polling {
			cmds = append(cmds, func() tea.Msg { return refreshStartedMsg{} })
		}
		return model, tea.Batch(cmds...)
	case commandResultMsg:
		shouldRefresh := model.completeCommand(msg.id, msg.result)
		if shouldRefresh && !model.polling {
			return model, func() tea.Msg { return refreshStartedMsg{} }
		}
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
		case "enter":
			action := model.confirmAction
			model.confirmation = nil
			model.confirmAction = nil
			model.confirmArgs = nil
			if action == nil {
				return model, nil
			}
			cmd := model.commandForAction(*action)
			if cmd == nil {
				return model, nil
			}
			model.startCommandRun(*action)
			return model, cmd
		case "esc", "q", "n":
			model.confirmation = nil
			model.confirmAction = nil
			model.confirmArgs = nil
			return model, nil
		default:
			return model, nil
		}
	}

	if model.commandRun != nil {
		switch msg.String() {
		case "esc":
			if model.commandRun.status.isActive() {
				return model, nil
			}
			model.commandRun = nil
			return model, nil
		case "q":
			if model.commandRun.status.isActive() {
				return model, nil
			}
			model.commandRun = nil
			return model, nil
		case "j", "down":
			model.commandRun.scroll++
			return model, nil
		case "k", "up":
			if model.commandRun.scroll > 0 {
				model.commandRun.scroll--
			}
			return model, nil
		case "home":
			model.commandRun.scroll = 0
			return model, nil
		case "end":
			model.commandRun.scroll = maxInt
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
			action := model.actionDescriptors()[model.clampedPaletteSelection()]
			if !action.Enabled {
				model.paletteOpen = false
				model.actionPreview = &action
				return model, nil
			}
			if action.ConfirmationRequired {
				confirmation, ok := model.confirmationForAction(action)
				if !ok {
					model.paletteOpen = false
					model.actionPreview = &action
					return model, nil
				}
				model.paletteOpen = false
				model.confirmation = &confirmation
				model.confirmAction = &action
				if slug, ok := model.selectedStartableTaskSlug(); ok && action.ID == ActionStartTask {
					model.confirmArgs = []string{slug}
				}
				return model, nil
			}
			cmd := model.commandForAction(action)
			if cmd == nil {
				return model, nil
			}
			model.paletteOpen = false
			if action.ID == ActionProviderStatus || action.ID == ActionTaskStatus || action.ID == ActionRunWorker {
				model.startCommandRun(action)
			}
			return model, cmd
		default:
			return model, nil
		}
	}

	itemCount := len(model.selectableItems())
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
		if model.polling {
			return model, nil
		}
		return model, func() tea.Msg { return refreshStartedMsg{} }
	case "l":
		model.pollingEnabled = !model.pollingEnabled
		return model, nil
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
	if model.commandRun != nil {
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
	if model.commandRun != nil && model.commandRun.status == commandRunning {
		segments = append(segments, statusSegment{text: "running", priority: 0})
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
		warningParts := []string{fmt.Sprintf("p:%d", warnings.provider), fmt.Sprintf("t:%d", warnings.task), fmt.Sprintf("c:%d", warnings.cleanup)}
		if warnings.queue > 0 {
			warningParts = append(warningParts, fmt.Sprintf("q:%d", warnings.queue))
		}
		segments = append(segments, statusSegment{text: strings.Join(warningParts, " "), priority: 2})
	}
	if strings.TrimSpace(model.state.GeneratedAt) != "" {
		segments = append(segments, statusSegment{text: "generated " + model.state.GeneratedAt, compact: model.state.GeneratedAt, priority: 3})
	}
	if model.polling {
		segments = append(segments, statusSegment{text: "polling", priority: 2})
	} else if model.pollingEnabled {
		segments = append(segments, statusSegment{text: "live", priority: 3})
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
	queue    int
}

func (counts dashboardWarningCounts) total() int {
	return counts.provider + counts.task + counts.cleanup + counts.queue
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
	if queueWarningCount(state.Queue) > 0 {
		counts.queue = 1
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
		return statusBadge("run", "accent")
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
	if state.Queue != nil {
		fmt.Fprintf(&output, "%s\n", model.renderLine("  queue     "+queueSummary(state.Queue)))
		if state.Queue.Plan != nil {
			fmt.Fprintf(&output, "%s\n", model.renderLine("  plan      "+queuePlanSummary(state.Queue.Plan)))
		}
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
	warnings += queueWarningCount(state.Queue)
	return warnings
}

func queueSummary(queue *contracts.RuntimeQueue) string {
	if queue == nil {
		return "unknown"
	}
	parts := []string{
		fmt.Sprintf("%s file", fallback(queue.State, "unknown")),
		fmt.Sprintf("%d items", queue.TotalItems),
		fmt.Sprintf("%d reserved", queue.ReservedItems),
	}
	statuses := sortedQueueStatuses(queue.CountsByStatus)
	if len(statuses) == 0 {
		parts = append(parts, "0 statuses")
	} else {
		counts := make([]string, 0, len(statuses))
		for _, status := range statuses {
			counts = append(counts, fmt.Sprintf("%s:%d", status, queue.CountsByStatus[status]))
		}
		parts = append(parts, strings.Join(counts, " "))
	}
	parts = append(parts, "oldest "+fallback(queue.OldestQueuedItemAge, "-"))
	if warning := queueWarningText(queue); warning != "" {
		parts = append(parts, warning+renderWarningCount(1))
	} else {
		parts[len(parts)-1] += renderWarningCount(0)
	}
	return strings.Join(parts, " | ")
}

func queuePlanSummary(plan *contracts.QueuePlan) string {
	if plan == nil {
		return "unknown"
	}
	parts := []string{
		fmt.Sprintf("%d runnable", plan.Runnable),
		fmt.Sprintf("%d skipped", plan.Skipped),
		fmt.Sprintf("%d reserved", plan.Reserved),
		"next " + fallback(plan.NextRunnableTask, "-"),
		"skip " + fallback(plan.FirstSkipReason, "-"),
	}
	if warning := queuePlanWarningText(plan); warning != "" {
		parts = append(parts, warning+renderWarningCount(1))
	} else {
		parts[len(parts)-1] += renderWarningCount(0)
	}
	return strings.Join(parts, " | ")
}

func queuePlanWarningText(plan *contracts.QueuePlan) string {
	if plan == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(plan.State)) {
	case "corrupted":
		return "corrupted"
	case "invalid":
		return "invalid"
	default:
		if strings.TrimSpace(plan.Error) != "" {
			return "planning-error"
		}
		return ""
	}
}

func sortedQueueStatuses(counts map[string]int) []string {
	statuses := make([]string, 0, len(counts))
	for status := range counts {
		status = strings.TrimSpace(status)
		if status != "" {
			statuses = append(statuses, status)
		}
	}
	sort.Strings(statuses)
	return statuses
}

func queueWarningCount(queue *contracts.RuntimeQueue) int {
	count := 0
	if queueWarningText(queue) != "" {
		count++
	}
	if queue != nil && queuePlanWarningText(queue.Plan) != "" {
		count++
	}
	return count
}

func queueWarningText(queue *contracts.RuntimeQueue) string {
	if queue == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(queue.State)) {
	case "corrupted":
		return "corrupted"
	case "invalid":
		if queue.UnsupportedFutureVersion {
			return "future-version"
		}
		return "invalid"
	default:
		if strings.TrimSpace(queue.Error) != "" {
			return "warning"
		}
		return ""
	}
}

func (model Model) renderListAndDetails() string {
	if model.usesTwoPaneLayout() {
		return model.renderTwoPaneListAndDetails()
	}
	return model.renderSingleColumnListAndDetails()
}

func (model Model) renderSingleColumnListAndDetails() string {
	items := model.selectableItems()
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
	items := model.selectableItems()
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

func (model Model) selectableItems() []dashboard.SelectionItem {
	items := dashboard.SelectableItems(model.state)
	if len(model.activities) == 0 {
		return items
	}
	activityItems := make([]dashboard.SelectionItem, 0, len(model.activities))
	for _, activity := range model.activities {
		label := fmt.Sprintf("%s: %s", activity.action, activity.status)
		if strings.TrimSpace(activity.summary) != "" {
			label += " - " + activity.summary
		}
		activityItems = append(activityItems, dashboard.SelectionItem{
			Kind:  dashboard.SelectionActivity,
			Label: label,
		})
	}
	out := make([]dashboard.SelectionItem, 0, len(items)+len(activityItems))
	providerEnd := 0
	for providerEnd < len(items) && items[providerEnd].Kind == dashboard.SelectionProvider {
		providerEnd++
	}
	out = append(out, items[:providerEnd]...)
	out = append(out, activityItems...)
	out = append(out, items[providerEnd:]...)
	return out
}

func (model Model) selectedStartableTaskSlug() (string, bool) {
	items := model.selectableItems()
	if len(items) == 0 {
		return "", false
	}
	selected := clampInt(model.selection.SelectedIndex, 0, len(items)-1)
	item := items[selected]
	if item.Kind != dashboard.SelectionTask {
		return "", false
	}
	slug := strings.TrimSpace(item.Task.Slug)
	if slug == "" {
		return "", false
	}
	return slug, true
}

func (model Model) selectedRunnableTask() (contracts.TaskSummary, bool) {
	items := model.selectableItems()
	if len(items) == 0 {
		return contracts.TaskSummary{}, false
	}
	selected := clampInt(model.selection.SelectedIndex, 0, len(items)-1)
	item := items[selected]
	if item.Kind != dashboard.SelectionTask {
		return contracts.TaskSummary{}, false
	}
	task := item.Task
	if strings.TrimSpace(task.Slug) == "" {
		return contracts.TaskSummary{}, false
	}
	state := strings.ToLower(strings.TrimSpace(firstNonEmpty(task.NormalizedState, task.Status)))
	switch state {
	case "ready", "ready-for-worker", "runnable", "queued", "prepared":
		return task, true
	default:
		return contracts.TaskSummary{}, false
	}
}

func (model Model) selectedTask() (contracts.TaskSummary, bool) {
	items := model.selectableItems()
	if len(items) == 0 {
		return contracts.TaskSummary{}, false
	}
	selected := clampInt(model.selection.SelectedIndex, 0, len(items)-1)
	item := items[selected]
	if item.Kind != dashboard.SelectionTask || strings.TrimSpace(item.Task.Slug) == "" {
		return contracts.TaskSummary{}, false
	}
	return item.Task, true
}

func (model Model) selectedTaskSlug() (string, bool) {
	task, ok := model.selectedTask()
	if !ok {
		return "", false
	}
	return strings.TrimSpace(task.Slug), true
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
		"  r refreshes runtime state; l toggles live polling",
		"  PowerShell remains authoritative for Brevity state",
		"  Go owns task start metadata updates and does not start providers/workers",
		"  p opens actions; read-only actions can execute PowerShell commands",
		"  Start task requires a selected task row and confirmation",
		"  Start task changes task state through native Go only",
		"  Run worker loads a native Go execution envelope only",
		"  future worker execution will be long-running; provider launch is disabled",
		"  Merge and Cleanup remain disabled future actions",
		"  command results scroll with up/down or j/k; esc closes finished results",
		"  recent command activity is session-only and read-only",
		"  provider launch is disabled; no worker execution occurs",
		"  executable today: Refresh state, Provider status, Task status, confirmed Start task",
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
	if action.ID == ActionRunWorker {
		return model.renderRunWorkerDryRunPreview(*action, usedRows...)
	}
	lines := []string{
		"  action        " + action.Label,
		"  status        disabled / blocked",
		"  command       " + model.previewCommandShape(*action),
		"  blocked       " + fallback(action.Command.DisabledReason, "descriptor is not enabled"),
		"  confirm       confirmation required before any future execution",
		"  authority     PowerShell is authoritative; Go will not write .brevity",
		"  close         esc, q, or p returns to the dashboard",
	}
	lines = append(lines, model.nativePreflightLines(*action)...)
	return model.renderPanel("Command Preview", lines, helpTruncatedIndicator, usedRows...)
}

func (model Model) renderRunWorkerDryRunPreview(action ActionDescriptor, usedRows ...int) string {
	task, ok := model.selectedRunnableTask()
	slug := "<selected-task-slug>"
	provider := fallback(action.Command.Provider, "(unknown)")
	profile := fallback(action.Command.Profile, "(unknown)")
	promptPath := "(PowerShell will resolve prompt path)"
	worktree := "(PowerShell will resolve task worktree)"
	if ok {
		slug = task.Slug
		provider = fallback(taskProvider(task), provider)
		profile = fallback(taskProfile(task), profile)
		worktree = fallback(taskWorktreePath(task), worktree)
		if task.Context != nil {
			promptPath = fallback(task.Context.Path, promptPath)
		}
	}
	lines := []string{
		"  action        " + action.Label,
		"  task          " + slug,
		"  provider      " + provider + " / " + profile,
		"  command       " + model.previewCommandShape(action),
		"  status        dry-run only; execution is disabled",
		"  boundary      native Go owns planning; worker execution is disabled",
		"  warning       worker/provider execution is not enabled",
		"  execution     no worker/provider launched",
		"  mode          dry-run only",
		"  would use     worktree " + worktree,
		"  would read    prompt path " + promptPath,
		"  would invoke  provider/profile after future confirmation",
		"  would track   activity and command result as long-running output",
		"  enter         does not execute",
		"  close         esc, q, or p returns to the dashboard",
	}
	lines = append(lines, model.nativePreflightLines(action)...)
	return model.renderPanel("Run Worker Dry-Run Preview", lines, helpTruncatedIndicator, usedRows...)
}

func (model Model) previewCommandShape(action ActionDescriptor) string {
	var args []string
	switch action.ID {
	case ActionStartTask:
		if slug, ok := model.selectedStartableTaskSlug(); ok {
			args = []string{slug}
		} else {
			args = []string{"<selected-task-slug>"}
		}
		args = append([]string{"brevity", "task", "start"}, args...)
		return strings.Join(args, " ")
	case ActionRunWorker:
		if task, ok := model.selectedRunnableTask(); ok {
			args = []string{task.Slug}
			if profile := taskProfile(task); profile != "" {
				args = append(args, "--profile", profile)
			}
		} else {
			args = []string{"<selected-task-slug>"}
		}
	}
	invocation, err := pscontract.BuildInvocation(action.Command, `.\\brevity.ps1`, args, true)
	if err != nil {
		return "(unavailable)"
	}
	return invocation.Display()
}

func (model Model) confirmationForAction(action ActionDescriptor) (pscontract.ConfirmationState, bool) {
	confirmation, ok := pscontract.NewConfirmationState(action.Command)
	if !ok {
		return pscontract.ConfirmationState{}, false
	}
	if action.ID == ActionStartTask {
		slug, startable := model.selectedStartableTaskSlug()
		if !startable {
			return pscontract.ConfirmationState{}, false
		}
		confirmation.Prompt = fmt.Sprintf(
			"Start task %s changes task metadata through native Go. No provider or worker is launched.",
			slug,
		)
	}
	if action.ID == ActionRunWorker {
		task, runnable := model.selectedRunnableTask()
		if !runnable {
			return pscontract.ConfirmationState{}, false
		}
		confirmation.Prompt = fmt.Sprintf(
			"Run worker for %s loads the native task-run plan only. No provider or worker is launched.",
			task.Slug,
		)
	}
	return confirmation, true
}

func (model Model) renderConfirmation(usedRows ...int) string {
	if model.confirmation == nil {
		return ""
	}
	state := *model.confirmation
	actionLabel := string(state.ActionID)
	command := "(unavailable)"
	if model.confirmAction != nil {
		actionLabel = model.confirmAction.Label
		if model.confirmAction.ID == ActionStartTask {
			args := append([]string{"brevity", "task", "start"}, model.confirmArgs...)
			command = strings.Join(args, " ")
		} else if model.confirmAction.ID == ActionRunWorker {
			args := []string{"brevity", "task", "run"}
			if task, ok := model.selectedRunnableTask(); ok {
				args = append(args, task.Slug, "--plan")
				if profile := taskProfile(task); profile != "" {
					args = append(args, "--profile", profile)
				}
			}
			command = strings.Join(args, " ")
		} else {
			invocation, err := pscontract.BuildInvocation(model.confirmAction.Command, `.\\brevity.ps1`, model.confirmArgs, false)
			if err == nil {
				command = invocation.Display()
			}
		}
	}
	authority := "PowerShell is authoritative; Go does not mutate task state"
	if model.confirmAction != nil && model.confirmAction.ID == ActionStartTask {
		authority = "native Go task start service"
	}
	if model.confirmAction != nil && model.confirmAction.ID == ActionRunWorker {
		authority = "native Go task-run planning service"
	}
	lines := []string{
		"  action        " + actionLabel,
		"  status        not executable unless enabled by command descriptor",
		"  command       " + command,
		"  prompt        " + state.Prompt,
		"  authority     " + authority,
		"  warning       this changes task state",
	}
	if model.confirmAction != nil {
		lines = append(lines, model.nativePreflightLines(*model.confirmAction)...)
	}
	if state.Strength == pscontract.ConfirmationDestructive {
		lines = append(lines, "  warning       destructive action; cleanup can remove branches or worktrees")
	}
	lines = append(lines, "  confirm       enter confirms")
	lines = append(lines, "  cancel        esc, q, or n cancels")
	return model.renderPanel("Confirm Action", lines, helpTruncatedIndicator, usedRows...)
}

func (model Model) nativePreflightLines(action ActionDescriptor) []string {
	preflightAction, slug, ok := model.preflightTarget(action)
	if !ok {
		return nil
	}
	result, err := preflight.Run(preflight.Options{RepoRoot: model.state.RepoRoot, Action: preflightAction, Slug: slug})
	if err != nil {
		return []string{"  preflight     unavailable: " + err.Error()}
	}
	lines := []string{
		"  preflight     " + string(result.Status) + " / " + string(result.Severity),
		"  native gate   " + result.DryRunSummary,
	}
	if len(result.Blockers) > 0 {
		lines = append(lines, "  blocked       "+result.Blockers[0])
	}
	if len(result.Warnings) > 0 {
		lines = append(lines, "  warning       "+result.Warnings[0])
	}
	if result.ProviderExecution {
		lines = append(lines, "  execution     provider execution would be required; not performed")
	}
	if result.Destructive {
		lines = append(lines, "  destructive   cleanup/branch/worktree removal would be destructive")
	}
	return lines
}

func (model Model) preflightTarget(action ActionDescriptor) (preflight.Action, string, bool) {
	switch action.ID {
	case ActionStartTask:
		slug, ok := model.selectedStartableTaskSlug()
		return preflight.ActionTaskStart, slug, ok
	case ActionRunWorker:
		task, ok := model.selectedRunnableTask()
		return preflight.ActionTaskRun, task.Slug, ok
	case ActionMergeTask:
		task, ok := model.selectedTask()
		return preflight.ActionTaskMerge, task.Slug, ok
	case ActionCleanupTask:
		task, ok := model.selectedTask()
		return preflight.ActionTaskCleanup, task.Slug, ok
	default:
		return "", "", false
	}
}

func (model Model) renderCommandResult(usedRows ...int) string {
	if model.commandRun == nil {
		return ""
	}
	run := *model.commandRun
	lines := []string{
		"  action        " + run.action.Label,
		"  status        " + string(run.status),
	}
	if run.status == commandRunning {
		if run.action.ID == ActionRunWorker {
			lines = append(lines,
				"  message       loading native Go execution envelope",
				"  execution     no worker/provider launched",
				"  boundary      native Go owns plan semantics",
			)
			return model.renderPanel("Run Worker Plan", lines, detailTruncatedIndicator, usedRows...)
		}
		lines = append(lines,
			"  message       running PowerShell command",
			"  cancel        wait for timeout or completion; esc is disabled while running",
		)
		return model.renderPanel("Command Result", lines, detailTruncatedIndicator, usedRows...)
	}
	if run.status.isFutureLongRunningState() && run.result == nil {
		lines = append(lines,
			"  message       long-running worker execution state scaffold",
			"  output        streaming output will appear here when execution exists",
			"  authority     PowerShell remains authoritative",
		)
		return model.renderPanel("Command Result", lines, detailTruncatedIndicator, usedRows...)
	}
	if run.result == nil {
		return model.renderPanel("Command Result", lines, detailTruncatedIndicator, usedRows...)
	}
	result := *run.result
	if run.action.ID == ActionRunWorker {
		return model.renderTaskRunPlanResult(run, result, usedRows...)
	}
	lines = append(lines,
		"  exit code     "+fmt.Sprint(result.ExitCode),
		"  message       "+result.OperatorMessage(),
	)
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
		lines = append(lines, "  follow-up     automatic runtime refresh requested")
	}
	lines = append(lines, "  close         esc or q closes result")
	return model.renderScrollablePanel("Command Result", lines, run.scroll, detailTruncatedIndicator, usedRows...)
}

func (model Model) renderTaskRunPlanResult(run commandRunState, result pscontract.ExecutionResult, usedRows ...int) string {
	lines := []string{
		"  action        " + run.action.Label,
		"  status        " + string(run.status),
		"  exit code     " + fmt.Sprint(result.ExitCode),
		"  execution     no worker/provider launched",
		"  boundary      native Go owns planning; execution is disabled",
	}
	commandResult, err := contracts.ParseCommandResult([]byte(result.Stdout))
	if err != nil {
		lines = append(lines,
			"  result        failed to parse native plan JSON",
			"  error         "+err.Error(),
		)
		if result.Error != "" {
			lines = append(lines, "  command error "+result.Error)
		}
		lines = append(lines, "  close         esc or q closes result")
		return model.renderScrollablePanel("Run Worker Plan Error", lines, run.scroll, detailTruncatedIndicator, usedRows...)
	}
	if !commandResult.Success {
		lines = append(lines,
			"  result        native plan generation failed",
		)
		for _, commandError := range commandResult.Errors {
			lines = append(lines, "  error         "+commandError.DisplayText())
		}
		lines = append(lines, "  close         esc or q closes result")
		return model.renderScrollablePanel("Run Worker Plan Error", lines, run.scroll, detailTruncatedIndicator, usedRows...)
	}
	plan, err := contracts.ParseTaskRunPlanPayload(commandResult)
	if err != nil {
		lines = append(lines,
			"  result        malformed native plan payload",
			"  error         "+err.Error(),
			"  close         esc or q closes result",
		)
		return model.renderScrollablePanel("Run Worker Plan Error", lines, run.scroll, detailTruncatedIndicator, usedRows...)
	}
	lines = append(lines,
		"  task          "+plan.Slug,
		"  provider      "+fallback(plan.Provider, "(unknown)"),
		"  profile       "+fallback(plan.Profile, "(default)"),
		"  worktree      "+fallback(plan.WorktreePath, "(missing)"),
		"  prompt        "+fallback(plan.PromptPath, "(missing)"),
		"  worker        "+fallback(plan.WorkerCommand.Display, plan.WorkerCommand.Command),
		"  kind          "+fallback(plan.ExecutionKind, "(unknown)"),
		"  approval      "+fallback(plan.ApprovalMode, "(unspecified)"),
		"  provider run  "+formatBool(plan.ProviderExecutionWouldOccur),
		"  isolated wt   "+formatBool(plan.IsolatedWorktreeRequired),
		"  dry-run       "+formatBool(plan.DryRunOnly),
		"  no execution  "+formatBool(plan.NoExecutionOccurred),
		"  authority     "+fallback(plan.Authority, "native-go"),
	)
	for _, warning := range append(commandResult.Warnings, plan.Warnings...) {
		lines = append(lines, "  warning       "+warning.DisplayText())
	}
	for _, note := range plan.SafetyNotes {
		lines = append(lines, "  safety        "+note)
	}
	for _, item := range plan.Unsupported {
		lines = append(lines, "  note          "+item)
	}
	lines = append(lines, "  close         esc or q closes result")
	return model.renderScrollablePanel("Run Worker Execution Plan", lines, run.scroll, detailTruncatedIndicator, usedRows...)
}

func (status commandStatus) isActive() bool {
	return status == commandQueued || status == commandPreparing || status == commandRunning || status == commandStreaming
}

func (status commandStatus) isFutureLongRunningState() bool {
	return status == commandQueued || status == commandPreparing || status == commandStreaming || status == commandCompleted || status == commandFailed || status == commandCanceled
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

func (model *Model) completeCommand(id int, result pscontract.ExecutionResult) bool {
	if model.commandRun == nil || model.commandRun.id != id {
		return false
	}
	status := commandFailed
	if result.Canceled {
		status = commandCanceled
	} else if result.TimedOut {
		status = commandTimedOut
	} else if result.Success() {
		status = commandSucceeded
	}
	model.commandRun.status = status
	model.commandRun.result = &result
	model.commandRun.scroll = 0
	model.addActivity(commandActivity{
		id:          id,
		action:      fallback(result.CommandDisplayLabel, model.commandRun.action.Label),
		status:      status,
		completedAt: result.CompletedAt,
		summary:     commandResultSummary(result),
		result:      &result,
	})
	return result.ShouldRefresh()
}

func (model *Model) startCommandRun(action ActionDescriptor) {
	model.nextCommandID++
	model.commandRun = &commandRunState{
		id:        model.nextCommandID,
		action:    action,
		status:    commandRunning,
		startedAt: time.Now(),
	}
	model.addActivity(commandActivity{
		id:      model.nextCommandID,
		action:  action.Label,
		status:  commandRunning,
		summary: "started",
	})
}

func (model *Model) addActivity(activity commandActivity) {
	for index := range model.activities {
		if model.activities[index].id == activity.id {
			model.activities[index] = activity
			model.selection.Clamp(len(model.selectableItems()))
			return
		}
	}
	model.activities = append([]commandActivity{activity}, model.activities...)
	if len(model.activities) > 10 {
		model.activities = model.activities[:10]
	}
	model.selection.Clamp(len(model.selectableItems()))
}

func commandResultSummary(result pscontract.ExecutionResult) string {
	if result.TimedOut {
		return "timeout"
	}
	if result.Canceled {
		return "canceled"
	}
	if strings.TrimSpace(result.Error) != "" {
		return result.Error
	}
	if strings.TrimSpace(result.Stderr) != "" {
		return strings.TrimSpace(result.Stderr)
	}
	if strings.TrimSpace(result.Stdout) != "" {
		return firstOutputLine(result.Stdout)
	}
	return result.OperatorMessage()
}

func firstOutputLine(value string) string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
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

func (model Model) renderScrollablePanel(title string, lines []string, scroll int, indicator string, usedRows ...int) string {
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
	if scroll < 0 {
		scroll = 0
	}
	if scroll > len(lines)-maxRows {
		scroll = len(lines) - maxRows
	}
	if scroll < 0 {
		scroll = 0
	}
	end := scroll + maxRows
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[scroll:end]
	if scroll > 0 && len(visible) > 0 {
		visible[0] = "  ... output above"
	}
	if end < len(lines) && len(visible) > 0 {
		visible[len(visible)-1] = indicator
	}
	if model.commandRun != nil {
		model.commandRun.scroll = scroll
	}
	var output strings.Builder
	fmt.Fprintln(&output)
	renderSection(&output, title)
	var body strings.Builder
	for _, line := range visible {
		fmt.Fprintln(&body, dashboardStyles.help.Render(model.renderLine(line)))
	}
	output.WriteString(body.String())
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
	controls := "up/down or j/k move | r refresh | p actions | d details | q quit | ? help"
	compactControls := "j/k r p d q quit ? help"
	if model.commandRun != nil {
		if model.commandRun.status.isActive() {
			controls = "command running | q/esc wait | read-only"
			compactControls = "running | wait"
		} else {
			controls = "up/down or j/k scroll | home/end | esc close | q close"
			compactControls = "j/k scroll | esc close"
		}
	} else if model.paletteOpen {
		controls = "up/down or j/k choose | enter run/preview | esc close"
		compactControls = "j/k enter esc"
	} else if model.helpOpen {
		controls = "esc or ? closes help | read-only boundary | ? help"
		compactControls = "esc ? help"
	}
	live := "live off"
	if model.pollingEnabled {
		live = "live on"
	}
	if model.polling {
		live = "polling"
	}
	footer := statusLine(width,
		statusSegment{text: controls, compact: compactControls, priority: 0},
		statusSegment{text: source, priority: 1},
		statusSegment{text: "read-only", priority: 1},
		statusSegment{text: live, priority: 3},
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
	for index, action := range model.actionDescriptors() {
		fmt.Fprintln(&output, model.renderPaletteActionRow(action, index == selected))
	}
	if model.shouldRenderActionPaletteHelp(usedRows...) {
		fmt.Fprintln(&output, dashboardStyles.help.Render(model.renderLine("  enter runs read-only/start or loads worker plan | esc closes")))
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
	paletteRowsWithHelp := 1 + 1 + len(model.actionDescriptors()) + 1
	return used+paletteRowsWithHelp+footerRows <= model.height
}

func (model *Model) movePaletteSelection(delta int) {
	actionCount := len(model.actionDescriptors())
	if actionCount == 0 {
		model.paletteSelected = 0
		return
	}
	model.paletteSelected = (model.paletteSelected + delta + actionCount) % actionCount
}

func (model Model) clampedPaletteSelection() int {
	actionCount := len(model.actionDescriptors())
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

func formatBool(value bool) string {
	if value {
		return "yes"
	}
	return "no"
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
		detail(output, "run count", "%d", task.RunCount)
		model.detailText(output, "run flags", taskRunFlags(task))
		if task.Context != nil {
			detail(output, "context", "%d materialized, %d missing", task.Context.MaterializedFileCount, len(task.Context.MissingFiles))
		} else {
			model.detailText(output, "context", "(unknown)")
		}
		model.detailText(output, "type", "task")
	case dashboard.SelectionActivity:
		activity := model.activityForLabel(item.Label)
		model.detailText(output, "activity", fallback(item.Label, "(none)"))
		if activity != nil {
			model.detailText(output, "status", string(activity.status))
			model.detailText(output, "summary", fallback(activity.summary, "(none)"))
			if !activity.completedAt.IsZero() {
				model.detailText(output, "completed", activity.completedAt.Format(time.RFC3339))
			}
			if activity.result != nil {
				model.detailText(output, "exit code", fmt.Sprint(activity.result.ExitCode))
				if !activity.result.StartedAt.IsZero() {
					model.detailText(output, "started", activity.result.StartedAt.Format(time.RFC3339))
				}
				if stdout := firstOutputLine(activity.result.Stdout); stdout != "" {
					model.detailText(output, "stdout", stdout)
				}
				if stderr := firstOutputLine(activity.result.Stderr); stderr != "" {
					model.detailText(output, "stderr", stderr)
				}
			}
			model.detailText(output, "guidance", "session-only command activity; future worker runs remain PowerShell-authoritative")
		} else {
			model.detailText(output, "guidance", "display-only runtime activity signal")
		}
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

func (model Model) activityForLabel(label string) *commandActivity {
	for index := range model.activities {
		activityLabel := fmt.Sprintf("%s: %s", model.activities[index].action, model.activities[index].status)
		if strings.TrimSpace(model.activities[index].summary) != "" {
			activityLabel += " - " + model.activities[index].summary
		}
		if activityLabel == label {
			return &model.activities[index]
		}
	}
	return nil
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

func taskRunFlags(task contracts.TaskSummary) string {
	flags := []string{}
	if task.LatestRunStale {
		flags = append(flags, "stale")
	}
	if task.LatestRunIncomplete {
		flags = append(flags, "incomplete")
	}
	if len(flags) == 0 {
		return "(none)"
	}
	return strings.Join(flags, ", ")
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
