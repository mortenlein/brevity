package bubbleteadashboard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/dashboard"
)

type fakeClient struct {
	output []byte
	err    error
	calls  int
}

func (client *fakeClient) RuntimeStateJSON() ([]byte, error) {
	client.calls++
	return client.output, client.err
}

func (client *fakeClient) DoctorJSON() ([]byte, error) { return nil, nil }
func (client *fakeClient) ProviderSetJSON(provider string, status string) ([]byte, error) {
	return nil, nil
}
func (client *fakeClient) ProviderResetJSON(provider string) ([]byte, error) { return nil, nil }
func (client *fakeClient) TaskContextRefreshJSON(slug string) ([]byte, error) {
	return nil, nil
}
func (client *fakeClient) TaskCleanupJSON(slug string) ([]byte, error) { return nil, nil }
func (client *fakeClient) TaskNewJSON(slug string) ([]byte, error)     { return nil, nil }
func (client *fakeClient) TaskRunJSON(slug string, profile string, smoke bool) ([]byte, error) {
	return nil, nil
}
func (client *fakeClient) TaskRuntimeInfoJSON(slug string) ([]byte, error) { return nil, nil }
func (client *fakeClient) TaskRunsJSON(slug string) ([]byte, error)        { return nil, nil }
func (client *fakeClient) TaskRunsReconcileJSON() ([]byte, error)          { return nil, nil }
func (client *fakeClient) TaskRunsRetentionJSON() ([]byte, error)          { return nil, nil }
func (client *fakeClient) TaskRunsCompactJSON() ([]byte, error)            { return nil, nil }

func TestNewModelDefaultsRefreshInterval(t *testing.T) {
	model := NewModel(&fakeClient{}, 0)

	if model.refreshInterval != defaultRefreshInterval {
		t.Fatalf("refreshInterval = %s, want %s", model.refreshInterval, defaultRefreshInterval)
	}
	if model.hasState {
		t.Fatal("hasState = true, want false")
	}
}

func TestModelNavigationAndDetailsToggle(t *testing.T) {
	model := NewModel(&fakeClient{}, time.Second)
	model.state = bubbleState()
	model.hasState = true

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	model = updated.(Model)
	if model.selection.SelectedIndex != 1 {
		t.Fatalf("SelectedIndex = %d, want 1", model.selection.SelectedIndex)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	model = updated.(Model)
	if model.selection.SelectedIndex != 0 {
		t.Fatalf("SelectedIndex = %d, want 0", model.selection.SelectedIndex)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if !model.selection.ShowDetails {
		t.Fatal("ShowDetails = false, want true")
	}
}

func TestActionPaletteOpensWithShortcut(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleState()
	model.hasState = true

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	model = updated.(Model)

	if cmd != nil {
		t.Fatal("palette shortcut returned a command, want nil")
	}
	if !model.paletteOpen {
		t.Fatal("paletteOpen = false, want true")
	}
	output := plainView(model.View())
	for _, want := range []string{
		"Actions",
		"Start task     read-only preview",
		"Run worker     read-only preview",
		"Merge task     read-only preview",
		"Cleanup task   read-only preview",
		"Refresh state  enabled",
		"enter activate | esc/q/p close",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("palette output missing %q:\n%s", want, output)
		}
	}
}

func TestActionPaletteOpensWithCtrlPAndClosesWithShortcuts(t *testing.T) {
	closeKeys := []tea.KeyMsg{
		{Type: tea.KeyEsc},
		{Type: tea.KeyRunes, Runes: []rune("q")},
		{Type: tea.KeyRunes, Runes: []rune("p")},
		{Type: tea.KeyCtrlP},
	}

	for _, key := range closeKeys {
		t.Run(key.String(), func(t *testing.T) {
			model := NewModelWithSource(&fakeClient{}, time.Second, "native")
			model.state = bubbleState()
			model.hasState = true

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
			model = updated.(Model)
			if !model.paletteOpen {
				t.Fatal("ctrl+p did not open palette")
			}

			updated, cmd := model.Update(key)
			model = updated.(Model)
			if cmd != nil {
				t.Fatalf("close key %q returned command, want nil", key.String())
			}
			if model.paletteOpen {
				t.Fatalf("close key %q left palette open", key.String())
			}
			if strings.Contains(plainView(model.View()), "Start task") {
				t.Fatalf("close key %q left palette visible:\n%s", key.String(), plainView(model.View()))
			}
		})
	}
}

func TestActionPaletteNavigatesWithoutChangingDashboardSelection(t *testing.T) {
	client := &fakeClient{}
	model := NewModelWithSource(client, time.Second, "native")
	model.state = bubbleState()
	model.hasState = true
	model.paletteOpen = true

	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("r")},
		{Type: tea.KeyDown},
		{Type: tea.KeyRunes, Runes: []rune("j")},
	} {
		updated, cmd := model.Update(key)
		model = updated.(Model)
		if cmd != nil {
			t.Fatalf("palette key %q returned command, want nil", key.String())
		}
	}
	if client.calls != 0 {
		t.Fatalf("palette key handling called runtime client %d times, want 0", client.calls)
	}
	if !model.paletteOpen {
		t.Fatal("non-close palette key closed palette")
	}
	if model.selection.SelectedIndex != 0 {
		t.Fatalf("palette navigation changed dashboard selection to %d, want 0", model.selection.SelectedIndex)
	}
	if model.paletteSelected != 2 {
		t.Fatalf("paletteSelected = %d, want 2", model.paletteSelected)
	}
}

func TestActionPaletteEnterOnRefreshPollsRuntimeStateOnce(t *testing.T) {
	client := &fakeClient{
		output: []byte(`{"schema":"brevity.runtime-state.v1","repoRoot":"C:\\repo","taskCounts":{"tracked":3}}`),
	}
	model := NewModelWithSource(client, time.Second, "native")
	model.state = bubbleState()
	model.hasState = true
	model.paletteOpen = true
	model.paletteSelected = 4

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("enter on refresh returned nil command, want refresh command")
	}
	if model.paletteOpen {
		t.Fatal("enter on refresh left palette open")
	}

	msg := cmd()
	if _, ok := msg.(refreshMsg); !ok {
		t.Fatalf("command message = %T, want refreshMsg", msg)
	}
	if client.calls != 1 {
		t.Fatalf("runtime polls = %d, want 1", client.calls)
	}
}

func TestActionPaletteEnterOnDisabledActionDoesNotPollRuntimeState(t *testing.T) {
	client := &fakeClient{}
	model := NewModelWithSource(client, time.Second, "native")
	model.state = bubbleState()
	model.hasState = true
	model.paletteOpen = true
	model.paletteSelected = 0

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("enter on disabled action returned command, want nil")
	}
	if client.calls != 0 {
		t.Fatalf("runtime polls = %d, want 0", client.calls)
	}
	if !model.paletteOpen {
		t.Fatal("enter on disabled action closed palette")
	}
}

func TestActionPaletteRowsStayWithinNarrowWidth(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleState()
	model.hasState = true
	model.paletteOpen = true
	model.width = 32

	output := plainView(model.renderActionPalette())

	for _, want := range []string{"Actions", "read-only", "..."} {
		if !strings.Contains(output, want) {
			t.Fatalf("narrow palette missing %q:\n%s", want, output)
		}
	}
	assertLinesWithinWidth(t, output, model.width)
}

func TestModelRefreshMessageUpdatesState(t *testing.T) {
	model := NewModel(&fakeClient{}, time.Second)
	state := bubbleState()

	updated, _ := model.Update(refreshMsg{state: state, at: time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)})
	model = updated.(Model)

	if !model.hasState {
		t.Fatal("hasState = false, want true")
	}
	if model.state.RepoRoot != `C:\repo` {
		t.Fatalf("RepoRoot = %q, want C:\\repo", model.state.RepoRoot)
	}
	if model.lastRefresh != "2026-05-19T10:00:00Z" {
		t.Fatalf("lastRefresh = %q", model.lastRefresh)
	}
	if !strings.Contains(plainView(model.View()), "Brevity Runtime Dashboard") {
		t.Fatalf("view missing title:\n%s", model.View())
	}
}

func TestModelStoresWindowSize(t *testing.T) {
	model := NewModel(&fakeClient{}, time.Second)

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 72, Height: 24})
	model = updated.(Model)

	if model.width != 72 {
		t.Fatalf("width = %d, want 72", model.width)
	}
	if model.height != 24 {
		t.Fatalf("height = %d, want 24", model.height)
	}
}

func TestTruncateValue(t *testing.T) {
	got := truncateValue("abcdefghijklmnopqrstuvwxyz", 12)
	if got != "abcdefghi..." {
		t.Fatalf("truncateValue = %q, want %q", got, "abcdefghi...")
	}
}

func TestTruncateValueUsesVisibleWidthForANSI(t *testing.T) {
	styled := ansiStyle("abcdefghijklmnopqrstuvwxyz")

	got := truncateValue(styled, 12)
	plain := plainView(got)

	if plain != "abcdefghi..." {
		t.Fatalf("truncateValue plain text = %q, want %q", plain, "abcdefghi...")
	}
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("truncateValue dropped styling escape sequences: %q", got)
	}
	if visibleWidth(got) != 12 {
		t.Fatalf("truncateValue visible width = %d, want 12: %q", visibleWidth(got), plain)
	}
}

func TestTruncateValueUsesVisibleWidthForUnicode(t *testing.T) {
	tests := []struct {
		name  string
		value string
		width int
	}{
		{name: "norwegian", value: "provider blåbær kø åpen ærlig", width: 18},
		{name: "emoji", value: "task 🚧 wide warning provider", width: 16},
		{name: "box drawing", value: "pane │ separator ═ footer", width: 17},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateValue(tt.value, tt.width)
			if visibleWidth(got) > tt.width {
				t.Fatalf("visible width = %d, want <= %d: %q", visibleWidth(got), tt.width, got)
			}
			if visibleWidth(tt.value) > tt.width && !strings.Contains(got, "...") {
				t.Fatalf("unicode truncation missing ellipsis: %q", got)
			}
		})
	}
}

func TestStyledWarningMarkerSurvivesProtectedTruncation(t *testing.T) {
	value := "provider-with-a-very-long-name: unavailable " + ansiStyle("!")

	got := truncateValuePreservingWarning(value, 24)
	plain := plainView(got)

	if !strings.HasSuffix(plain, " !") {
		t.Fatalf("styled warning marker was not preserved at line end: %q", plain)
	}
	if !strings.Contains(plain, "...") {
		t.Fatalf("styled warning truncation did not ellipsize base text: %q", plain)
	}
	if visibleWidth(got) > 24 {
		t.Fatalf("styled warning visible width = %d, want <= 24: %q", visibleWidth(got), plain)
	}
}

func TestStyledCompactWarningCountSurvivesProtectedTruncation(t *testing.T) {
	value := "alerts-with-a-very-long-prefix " + ansiStyle("!12")

	got := truncateValuePreservingWarning(value, 18)
	plain := plainView(got)

	if !strings.HasSuffix(plain, " !12") {
		t.Fatalf("styled compact warning count was not preserved at line end: %q", plain)
	}
	if visibleWidth(got) > 18 {
		t.Fatalf("styled compact warning visible width = %d, want <= 18: %q", visibleWidth(got), plain)
	}
}

func TestUnicodeWarningMarkersSurviveProtectedTruncation(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		width  int
		suffix string
	}{
		{name: "plain marker", value: "oppgave-æøå-med-emoji-🚧 provider-gated !", width: 24, suffix: " !"},
		{name: "compact count", value: "varsler blåbær-provider 🚧 !12", width: 20, suffix: " !12"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateValuePreservingWarning(tt.value, tt.width)
			if !strings.HasSuffix(plainView(got), tt.suffix) {
				t.Fatalf("unicode warning suffix was not preserved: %q", plainView(got))
			}
			if visibleWidth(got) > tt.width {
				t.Fatalf("visible width = %d, want <= %d: %q", visibleWidth(got), tt.width, plainView(got))
			}
		})
	}
}

func TestTruncatePathPreservesSuffix(t *testing.T) {
	path := `C:\dev\repos\active\brevity\worktrees\active\very-long-task-name\runs\latest\worker-output.log`

	got := truncatePath(path, 32)

	if visibleWidth(got) > 32 {
		t.Fatalf("truncatePath visible width = %d, want <= 32: %q", visibleWidth(got), got)
	}
	if !strings.HasPrefix(got, "...") {
		t.Fatalf("truncatePath = %q, want prefix ...", got)
	}
	if !strings.HasSuffix(got, `latest\worker-output.log`) {
		t.Fatalf("truncatePath = %q, want important suffix", got)
	}
}

func TestTruncatePathUsesVisibleWidthForUnicodeSuffix(t *testing.T) {
	path := `C:\dev\repos\active\brevity\worktrees\active\oppgave-æøå-🚧\runs\latest\worker-output-✅.log`

	got := truncatePath(path, 34)

	if visibleWidth(got) > 34 {
		t.Fatalf("unicode path visible width = %d, want <= 34: %q", visibleWidth(got), got)
	}
	if !strings.HasPrefix(got, "...") {
		t.Fatalf("unicode path = %q, want prefix ...", got)
	}
	if !strings.HasSuffix(got, `worker-output-✅.log`) {
		t.Fatalf("unicode path = %q, want important suffix", got)
	}
}

func TestModelViewRendersOperatorSections(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleState()
	model.hasState = true
	model.lastRefresh = "2026-05-19T10:00:00Z"

	output := plainView(model.View())
	for _, want := range []string{
		"Brevity Runtime Dashboard",
		"native | read-only | alerts !3 p:1 t:1 c:1",
		"Runtime Summary",
		"Selectable List",
		"> prov  codex: degraded !",
		"Details Pane",
		"select a row, then press d for details",
		"q quit | j/k move | d details | p action | r refresh | ? help",
		"native | read-only | 1s refresh",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("view missing %q:\n%s", want, output)
		}
	}
}

func TestModelViewReadableAtNarrowWidth(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleStateWithLongPaths()
	model.hasState = true
	model.width = 48
	model.selection.SelectedIndex = 1
	model.selection.ShowDetails = true

	output := plainView(model.View())

	for index, line := range strings.Split(output, "\n") {
		if visibleWidth(line) > model.width {
			t.Fatalf("line %d visible width = %d, want <= %d:\n%s", index+1, visibleWidth(line), model.width, output)
		}
	}
	if !strings.Contains(output, "...") {
		t.Fatalf("narrow view missing truncation marker:\n%s", output)
	}
	if strings.Contains(output, `C:\dev\repos\active\brevity\worktrees\active\very-long-task-name`) {
		t.Fatalf("narrow view contains unshortened long path:\n%s", output)
	}
}

func TestUnicodeHeavyViewKeepsRowsWithinWidth(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native-æøå-🚧")
	model.state = bubbleStateWithUnicodeText()
	model.hasState = true
	model.width = 44
	model.height = 28
	model.selection.SelectedIndex = 1
	model.selection.ShowDetails = true
	model.lastRefresh = "2026-05-19T10:00:00Z"

	output := plainView(model.View())

	for _, want := range []string{"> task", "!", "...", "state:", "read-only"} {
		if !strings.Contains(output, want) {
			t.Fatalf("unicode-heavy output missing %q:\n%s", want, output)
		}
	}
	assertLinesWithinWidth(t, output, model.width)
}

func TestUnicodeTwoPaneSeparatorAndFooterStayWithinWidth(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "powershell-æøå-🚧")
	model.state = bubbleStateWithUnicodeText()
	model.hasState = true
	model.width = 112
	model.height = 24
	model.selection.SelectedIndex = 1
	model.selection.ShowDetails = true
	model.selection.ShowHelp = true
	model.lastRefresh = "2026-05-19T10:00:00Z"

	output := plainView(model.View())

	if !strings.Contains(output, "Selectable List") || !strings.Contains(output, paneSeparator+"Details Pane") {
		t.Fatalf("unicode two-pane output missing pane separator alignment:\n%s", output)
	}
	if !strings.Contains(output, "> task") {
		t.Fatalf("unicode two-pane output dropped selected marker:\n%s", output)
	}
	if !strings.Contains(output, "? help") || !strings.Contains(output, "powershell") {
		t.Fatalf("unicode footer dropped readable controls/source:\n%s", output)
	}
	assertLinesWithinWidth(t, output, model.width)
}

func TestFooterPreservesPrioritySegmentsAtNarrowWidth(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleState()
	model.hasState = true
	model.width = 48
	model.lastRefresh = "2026-05-19T10:00:00Z"

	output := plainView(model.renderFooter())

	for _, want := range []string{"q", "j/k", "d", "r", "? help", "native", "read-only"} {
		if !strings.Contains(output, want) {
			t.Fatalf("narrow footer missing %q:\n%s", want, output)
		}
	}
	for _, dropped := range []string{"1s refresh", "last 2026-05-19T10:00:00Z"} {
		if strings.Contains(output, dropped) {
			t.Fatalf("narrow footer kept lower-priority segment %q:\n%s", dropped, output)
		}
	}
	if strings.Contains(output, "...") {
		t.Fatalf("narrow footer should drop whole low-priority segments before clipping:\n%s", output)
	}
}

func TestFooterDropsRefreshBeforeKeyHints(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "powershell")
	model.width = 36
	model.lastRefresh = "2026-05-19T10:00:00Z"

	output := plainView(model.renderFooter())

	for _, want := range []string{"q", "j/k", "d", "r", "? help"} {
		if !strings.Contains(output, want) {
			t.Fatalf("tight footer dropped key hint %q before refresh text:\n%s", want, output)
		}
	}
	if strings.Contains(output, "refresh") || strings.Contains(output, "last ") {
		t.Fatalf("tight footer kept refresh metadata before key hints:\n%s", output)
	}
}

func TestHeaderReadableAtNarrowWidth(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "powershell")
	model.state = bubbleState()
	model.hasState = true
	model.width = 45

	output := plainView(model.renderHeader())

	for _, want := range []string{"powershell", "read-only", "!3", "p:1 t:1 c:1"} {
		if !strings.Contains(output, want) {
			t.Fatalf("narrow header missing priority text %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "...") {
		t.Fatalf("narrow header should prefer compact text over clipping:\n%s", output)
	}
}

func TestStyledHeaderFooterAndSummaryUseVisibleWidth(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "powershell")
	model.state = bubbleState()
	model.hasState = true
	model.width = 45
	model.lastRefresh = "2026-05-19T10:00:00Z"

	header := statusLine(model.width,
		statusSegment{text: ansiStyle("Brevity Runtime Dashboard"), compact: ansiStyle("Brevity Runtime"), priority: 2},
		statusSegment{text: "powershell", priority: 0},
		statusSegment{text: "read-only", priority: 0},
		statusSegment{text: "alerts " + ansiStyle("!3") + " p:1 t:1 c:1", compact: ansiStyle("!3") + " p:1 t:1 c:1", priority: 0},
	)
	footer := statusLine(model.width,
		statusSegment{text: ansiStyle("q quit | j/k move | d details | p action | r refresh | ? help"), compact: ansiStyle("q j/k d p r ? help"), priority: 0},
		statusSegment{text: "powershell", priority: 1},
		statusSegment{text: "read-only", priority: 1},
		statusSegment{text: "1s refresh", priority: 2},
	)
	summaryLine := model.renderLine("  providers  " + ansiStyle("provider-with-a-long-warning-state") + " " + ansiStyle("!1"))

	for name, output := range map[string]string{
		"header":  header,
		"footer":  footer,
		"summary": summaryLine,
	} {
		if !strings.Contains(output, "\x1b[") {
			t.Fatalf("%s output is not styled: %q", name, output)
		}
		if visibleWidth(strings.TrimSuffix(output, "\n")) > model.width {
			t.Fatalf("%s visible width = %d, want <= %d:\n%s", name, visibleWidth(strings.TrimSuffix(output, "\n")), model.width, plainView(output))
		}
	}
	if !strings.Contains(plainView(header), "!3") {
		t.Fatalf("styled header dropped compact warning count:\n%s", plainView(header))
	}
	if !strings.Contains(plainView(footer), "q") || !strings.Contains(plainView(footer), "read-only") {
		t.Fatalf("styled footer dropped protected status text:\n%s", plainView(footer))
	}
	if !strings.HasSuffix(plainView(summaryLine), " !1") {
		t.Fatalf("styled summary line dropped compact warning count: %q", plainView(summaryLine))
	}
}

func TestStyledSelectedAndWarningRowsPreserveMarkers(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.width = 30

	selected := model.renderRow(true, string(dashboard.SelectionTask), ansiStyle("task-with-a-long-styled-label"), "")
	warning := model.renderRow(false, string(dashboard.SelectionTask), ansiStyle("task-with-a-long-styled-label"), " "+ansiStyle("!"))

	if !strings.HasPrefix(plainView(selected), ">") {
		t.Fatalf("styled selected row dropped selection marker: %q", plainView(selected))
	}
	if !strings.HasSuffix(plainView(warning), " !") {
		t.Fatalf("styled warning row dropped warning marker: %q", plainView(warning))
	}
	if visibleWidth(selected) > model.width || visibleWidth(warning) > model.width {
		t.Fatalf("styled row exceeded width:\nselected %d %q\nwarning %d %q", visibleWidth(selected), plainView(selected), visibleWidth(warning), plainView(warning))
	}
}

func TestModelViewUsesSingleColumnBelowTwoPaneThreshold(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleState()
	model.hasState = true
	model.width = twoPaneWidthThreshold - 1

	output := plainView(model.View())

	if !strings.Contains(output, "Selectable List\n") {
		t.Fatalf("narrow view missing single-column list section:\n%s", output)
	}
	if strings.Contains(output, "Selectable List") && strings.Contains(output, paneSeparator+"Details Pane") {
		t.Fatalf("narrow view appears to use pane separator:\n%s", output)
	}
}

func TestModelViewUsesTwoPaneLayoutAtWideWidth(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleState()
	model.hasState = true
	model.width = twoPaneWidthThreshold

	output := plainView(model.View())

	if !strings.Contains(output, "Selectable List") || !strings.Contains(output, paneSeparator+"Details Pane") {
		t.Fatalf("wide view missing side-by-side pane headings:\n%s", output)
	}
	if !strings.Contains(output, "q quit | j/k move | d details") {
		t.Fatalf("wide view missing footer controls:\n%s", output)
	}
}

func TestModelViewLayoutSnapshotsByWidth(t *testing.T) {
	tests := []struct {
		name        string
		width       int
		wantTwoPane bool
	}{
		{name: "narrow", width: 48},
		{name: "medium", width: 82},
		{name: "wide", width: 120, wantTwoPane: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModelWithSource(&fakeClient{}, time.Second, "native")
			model.state = bubbleState()
			model.hasState = true
			model.width = tt.width
			model.height = 28
			model.lastRefresh = "2026-05-19T10:00:00Z"

			output := plainView(model.View())
			for _, want := range []string{
				"native",
				"read-only",
				"alerts !3 p:1 t:1 c:1",
				"> prov  codex: degraded !",
				"q",
				"j/k",
				"d",
				"r",
				"? help",
			} {
				if !strings.Contains(output, want) {
					t.Fatalf("%s snapshot missing %q:\n%s", tt.name, want, output)
				}
			}
			if tt.width >= 82 && !strings.Contains(output, "Brevity Runtime") {
				t.Fatalf("%s snapshot missing dashboard title:\n%s", tt.name, output)
			}
			if strings.Contains(output, "degraded ! degraded") {
				t.Fatalf("%s snapshot duplicated provider warning wording:\n%s", tt.name, output)
			}
			hasTwoPane := strings.Contains(output, paneSeparator+"Details Pane")
			if hasTwoPane != tt.wantTwoPane {
				t.Fatalf("%s two-pane separator = %t, want %t:\n%s", tt.name, hasTwoPane, tt.wantTwoPane, output)
			}
		})
	}
}

func TestModelViewRenderInvariantsAcrossWidthsSourcesAndModes(t *testing.T) {
	widths := []struct {
		name        string
		width       int
		wantTwoPane bool
	}{
		{name: "narrow", width: 48},
		{name: "medium", width: 82},
		{name: "wide", width: 120, wantTwoPane: true},
	}
	sources := []string{"native", "powershell"}
	modes := []struct {
		name        string
		showDetails bool
		showHelp    bool
		wantDetails string
	}{
		{name: "collapsed-details", wantDetails: "select a row, then press d for details"},
		{name: "expanded-details", showDetails: true, wantDetails: "type:"},
		{name: "help-visible", showDetails: true, showHelp: true, wantDetails: "type:"},
	}

	for _, width := range widths {
		for _, source := range sources {
			for _, mode := range modes {
				t.Run(width.name+"/"+source+"/"+mode.name, func(t *testing.T) {
					model := NewModelWithSource(&fakeClient{}, time.Second, source)
					model.state = bubbleStateWithLongWarningLabels()
					model.hasState = true
					model.width = width.width
					model.height = 34
					model.lastRefresh = "2026-05-19T10:00:00Z"
					model.selection.SelectedIndex = 1
					model.selection.ShowDetails = mode.showDetails
					model.selection.ShowHelp = mode.showHelp

					output := plainView(model.View())

					assertRenderInvariants(t, output, width.width, source, width.wantTwoPane)
					if !strings.Contains(output, mode.wantDetails) {
						t.Fatalf("view missing details mode marker %q:\n%s", mode.wantDetails, output)
					}
					if mode.showHelp && !strings.Contains(output, "Help") {
						t.Fatalf("help-visible view missing help section:\n%s", output)
					}
				})
			}
		}
	}
}

func TestModelLoadingViewRenderInvariantsAcrossWidths(t *testing.T) {
	widths := []struct {
		name  string
		width int
	}{
		{name: "narrow", width: 48},
		{name: "medium", width: 82},
		{name: "wide", width: 120},
	}

	for _, width := range widths {
		t.Run(width.name, func(t *testing.T) {
			model := NewModelWithSource(&fakeClient{}, time.Second, "native")
			model.width = width.width
			model.height = 20

			output := plainView(model.View())

			assertLoadingOrErrorViewInvariants(t, output, width.width, "native")
			if !strings.Contains(output, "Loading runtime state") {
				t.Fatalf("loading view missing loading message:\n%s", output)
			}
		})
	}
}

func TestModelRuntimeErrorViewRenderInvariantsAcrossWidths(t *testing.T) {
	widths := []struct {
		name  string
		width int
	}{
		{name: "narrow", width: 48},
		{name: "medium", width: 82},
		{name: "wide", width: 120},
	}

	for _, width := range widths {
		t.Run(width.name, func(t *testing.T) {
			model := NewModelWithSource(&fakeClient{}, time.Second, "native")
			model.width = width.width
			model.height = 20
			updated, _ := model.Update(refreshMsg{
				err: fmt.Errorf("runtime unavailable"),
				at:  time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC),
			})
			model = updated.(Model)

			output := plainView(model.View())

			assertLoadingOrErrorViewInvariants(t, output, width.width, "native")
			for _, want := range []string{"polling error", "runtime unavailable"} {
				if !strings.Contains(output, want) {
					t.Fatalf("runtime error view missing %q:\n%s", want, output)
				}
			}
		})
	}
}

func TestRunWithSourceFirstPollErrorRendersFallback(t *testing.T) {
	pollErr := errors.New("runtime source unavailable")
	client := &fakeClient{err: pollErr}
	input := strings.NewReader("")
	var stdout bytes.Buffer

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("RunWithSource panicked on first poll error: %v", recovered)
		}
	}()

	err := RunWithSource(context.Background(), input, &stdout, client, time.Second, "native")
	if !errors.Is(err, pollErr) {
		t.Fatalf("RunWithSource error = %v, want %v", err, pollErr)
	}
	if client.calls != 1 {
		t.Fatalf("runtime polls = %d, want 1", client.calls)
	}

	output := plainView(stdout.String())
	assertLoadingOrErrorViewInvariants(t, output, defaultTerminalWidth, "native")
	for _, want := range []string{"Loading runtime state", "polling error", "runtime source unavailable"} {
		if !strings.Contains(output, want) {
			t.Fatalf("RunWithSource fallback output missing %q:\n%s", want, output)
		}
	}
}

func TestSelectableRowWarningSuffixesByKind(t *testing.T) {
	tests := []struct {
		name          string
		state         contracts.RuntimeState
		selected      int
		wantRow       string
		duplicateText []string
	}{
		{
			name:    "degraded provider",
			state:   bubbleStateWithProvider("codex", "degraded"),
			wantRow: "> prov  codex: degraded !",
			duplicateText: []string{
				"degraded ! degraded",
			},
		},
		{
			name:    "unavailable provider",
			state:   bubbleStateWithProvider("codex", "unavailable"),
			wantRow: "> prov  codex: unavailable !",
			duplicateText: []string{
				"unavailable ! unavailable",
			},
		},
		{
			name:    "blocked task",
			state:   bubbleStateWithTaskState("blocked"),
			wantRow: "> task  task-one: blocked !",
			duplicateText: []string{
				"blocked ! blocked",
			},
		},
		{
			name:    "stale task",
			state:   bubbleStateWithTaskState("stale"),
			wantRow: "> task  task-one: stale !",
			duplicateText: []string{
				"stale ! stale",
			},
		},
		{
			name:    "failed task",
			state:   bubbleStateWithTaskState("failed"),
			wantRow: "> task  task-one: failed !",
			duplicateText: []string{
				"failed ! failed",
			},
		},
		{
			name:    "provider-gated task",
			state:   bubbleStateWithTaskState("provider-gated"),
			wantRow: "> task  task-one: provider-gated !",
			duplicateText: []string{
				"provider-gated ! provider-gated",
			},
		},
		{
			name:    "warning cleanup candidate",
			state:   bubbleStateWithCleanupCandidate("warning", "requires-inspection"),
			wantRow: "> clean requires-inspection: cleanup-one !",
			duplicateText: []string{
				"warning ! warning",
				"requires-inspection ! requires-inspection",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModelWithSource(&fakeClient{}, time.Second, "native")
			model.state = tt.state
			model.hasState = true
			model.selection.SelectedIndex = tt.selected

			output := plainView(model.View())
			if !strings.Contains(output, tt.wantRow) {
				t.Fatalf("view missing warning row %q:\n%s", tt.wantRow, output)
			}
			for _, duplicate := range tt.duplicateText {
				if strings.Contains(output, duplicate) {
					t.Fatalf("view duplicated warning wording %q:\n%s", duplicate, output)
				}
			}
		})
	}
}

func TestWidePaneLayoutKeepsSelectedItemVisible(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleStateWithManyTasks(30)
	model.hasState = true
	model.width = 120
	model.height = 18
	model.selection.SelectedIndex = 11

	output := plainView(model.View())

	if !strings.Contains(output, "> task  task-11") {
		t.Fatalf("wide pane layout missing selected item:\n%s", output)
	}
	if !strings.Contains(output, "showing") {
		t.Fatalf("wide pane layout missing scroll indicator:\n%s", output)
	}
}

func TestWidePaneLayoutRendersDetailsAndHelpInRightPane(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleState()
	model.hasState = true
	model.width = 120
	model.height = 32
	model.selection.ShowDetails = true
	model.selection.ShowHelp = true

	output := plainView(model.View())

	for _, want := range []string{
		paneSeparator + "Details Pane",
		paneSeparator + "  type:",
		paneSeparator + "  q quit",
		paneSeparator + "Help",
		"q quit | j/k move | d details",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("wide pane view missing %q:\n%s", want, output)
		}
	}
}

func TestModelViewShortensLongPaths(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleStateWithLongPaths()
	model.hasState = true
	model.width = 60
	model.selection.SelectedIndex = 1
	model.selection.ShowDetails = true

	output := plainView(model.View())

	if !strings.Contains(output, `worktree:`) || !strings.Contains(output, `...rktrees\active\very-long-task-name`) {
		t.Fatalf("view missing shortened path suffix:\n%s", output)
	}
	if strings.Contains(output, `C:\dev\repos\active\brevity\worktrees\active\very-long-task-name`) {
		t.Fatalf("view contains unshortened long path:\n%s", output)
	}
}

func TestModelViewRendersDetailsAndHelp(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "powershell")
	model.state = bubbleState()
	model.hasState = true

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	model = updated.(Model)

	output := plainView(model.View())
	for _, want := range []string{
		"Help",
		"? help",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("view missing %q:\n%s", want, output)
		}
	}
	for _, detail := range []struct {
		label string
		value string
	}{
		{"type", "provider"},
		{"name", "codex"},
		{"status", "degraded !"},
		{"guidance", "provider is degraded"},
	} {
		if !containsDetail(output, detail.label, detail.value) {
			t.Fatalf("view missing detail %q=%q:\n%s", detail.label, detail.value, output)
		}
	}
}

func TestModelViewRendersTaskCleanupAndActionDetails(t *testing.T) {
	tests := []struct {
		name     string
		selected int
		want     []string
		details  map[string]string
	}{
		{
			name:     "task",
			selected: 1,
			details:  map[string]string{"type": "task", "slug": "task-one", "state": "blocked", "branch": "task/task-one"},
		},
		{
			name:     "cleanup",
			selected: 2,
			details:  map[string]string{"type": "cleanup candidate", "id": "orphan-branch:task-old", "severity/category": "warning / requires-inspection"},
		},
		{
			name:     "action",
			selected: 3,
			want:     []string{"display-only; no command is executed"},
			details:  map[string]string{"type": "suggested action", "action": "Inspect state."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModelWithSource(&fakeClient{}, time.Second, "native")
			model.state = bubbleState()
			model.hasState = true
			model.selection.SelectedIndex = tt.selected
			model.selection.ShowDetails = true

			output := plainView(model.View())
			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Fatalf("view missing %q:\n%s", want, output)
				}
			}
			for label, value := range tt.details {
				if !containsDetail(output, label, value) {
					t.Fatalf("view missing detail %q=%q:\n%s", label, value, output)
				}
			}
		})
	}
}

func TestProviderDetailWarningsStayReadable(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		want      map[string]string
		duplicate string
	}{
		{
			name:   "degraded",
			status: "degraded",
			want: map[string]string{
				"status": "degraded !",
				"note":   "capacity limited",
			},
			duplicate: "degraded ! degraded",
		},
		{
			name:   "unavailable",
			status: "unavailable",
			want: map[string]string{
				"status": "unavailable !",
				"note":   "capacity limited",
			},
			duplicate: "unavailable ! unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := bubbleStateWithProvider("codex", tt.status)
			state.Providers.Health["codex"] = contracts.ProviderHealth{
				Status:    tt.status,
				UpdatedAt: "2026-05-20T10:00:00Z",
				Note:      "capacity limited",
			}
			output := detailPaneView(state, 0)

			for label, value := range tt.want {
				if !containsDetail(output, label, value) {
					t.Fatalf("provider detail missing %q=%q:\n%s", label, value, output)
				}
			}
			if strings.Contains(output, tt.duplicate) {
				t.Fatalf("provider detail duplicated warning wording %q:\n%s", tt.duplicate, output)
			}
		})
	}
}

func TestTaskDetailWarningsStayReadable(t *testing.T) {
	tests := []struct {
		state     string
		wantState string
		duplicate string
	}{
		{state: "failed", wantState: "failed !", duplicate: "failed ! failed"},
		{state: "blocked", wantState: "blocked !", duplicate: "blocked ! blocked"},
		{state: "stale", wantState: "stale !", duplicate: "stale ! stale"},
		{state: "provider-gated", wantState: "provider-gated !", duplicate: "provider-gated ! provider-gated"},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			output := detailPaneView(bubbleStateWithTaskState(tt.state), 0)

			for _, detail := range []struct {
				label string
				value string
			}{
				{"type", "task"},
				{"slug", "task-one"},
				{"state", tt.wantState},
			} {
				if !containsDetail(output, detail.label, detail.value) {
					t.Fatalf("task detail missing %q=%q:\n%s", detail.label, detail.value, output)
				}
			}
			if strings.Contains(output, tt.duplicate) {
				t.Fatalf("task detail duplicated state wording %q:\n%s", tt.duplicate, output)
			}
		})
	}
}

func TestCleanupDetailWarningsStayReadable(t *testing.T) {
	state := bubbleStateWithCleanupCandidate("warning", "destructive-if-removed")
	state.Cleanup.OrphanedTaskBranches[0].SuggestedCommands = []string{
		`git branch --delete task/cleanup-one`,
		`.\brevity.ps1 task cleanup cleanup-one --force`,
	}

	output := detailPaneView(state, 0)

	for _, detail := range []struct {
		label string
		value string
	}{
		{"type", "cleanup candidate"},
		{"id", "cleanup-one"},
		{"severity/category", "warning / destructive-if-removed !"},
	} {
		if !containsDetail(output, detail.label, detail.value) {
			t.Fatalf("cleanup detail missing %q=%q:\n%s", detail.label, detail.value, output)
		}
	}
	for _, want := range []string{
		`suggested commands:`,
		`- git branch --delete task/cleanup-one`,
		`- .\brevity.ps1 task cleanup cleanup-one --force`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("cleanup detail missing %q:\n%s", want, output)
		}
	}
	for _, duplicate := range []string{
		"warning ! warning",
		"destructive-if-removed ! destructive-if-removed",
	} {
		if strings.Contains(output, duplicate) {
			t.Fatalf("cleanup detail duplicated warning wording %q:\n%s", duplicate, output)
		}
	}
}

func TestTwoPaneDetailTruncationPreservesWarningMarkers(t *testing.T) {
	tests := []struct {
		name     string
		state    contracts.RuntimeState
		selected int
		label    string
		value    string
	}{
		{
			name:  "unavailable provider",
			state: bubbleStateWithProvider("codex", "unavailable"),
			label: "status",
			value: "unavailable !",
		},
		{
			name:  "failed task",
			state: bubbleStateWithTaskState("failed"),
			label: "state",
			value: "failed !",
		},
		{
			name:  "stale task",
			state: bubbleStateWithTaskState("stale"),
			label: "state",
			value: "stale !",
		},
		{
			name:  "blocked task",
			state: bubbleStateWithTaskState("blocked"),
			label: "state",
			value: "blocked !",
		},
		{
			name:  "cleanup warning",
			state: bubbleStateWithCleanupCandidate("warning", "destructive-if-removed"),
			label: "severity/category",
			value: "warning / destructive-if-removed !",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := detailPaneViewWithSize(tt.state, tt.selected, twoPaneWidthThreshold, 24)

			if !containsDetail(output, tt.label, tt.value) {
				t.Fatalf("two-pane detail truncation hid warning marker or context %q=%q:\n%s", tt.label, tt.value, output)
			}
			assertLinesWithinWidth(t, output, twoPaneWidthThreshold)
		})
	}
}

func TestNarrowWidthTruncationPreservesSelectionAndWarnings(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleStateWithLongWarningLabels()
	model.hasState = true
	model.width = 36
	model.selection.SelectedIndex = 1
	model.selection.ShowDetails = true

	output := plainView(model.View())

	for _, want := range []string{
		"> task",
		"!",
		"...",
		"state:",
		"provider... !",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("narrow output missing intentional marker %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "....") {
		t.Fatalf("narrow output has awkward truncation marker:\n%s", output)
	}
	assertLinesWithinWidth(t, output, model.width)
}

func TestCleanupDetailTruncationPreservesDestructiveContextBeforeCommands(t *testing.T) {
	destructive := true
	removable := false
	state := bubbleStateWithCleanupCandidate("warning", "destructive-if-removed-with-a-very-long-tail")
	state.Cleanup.OrphanedTaskBranches[0].DestructiveIfUnmerged = &destructive
	state.Cleanup.OrphanedTaskBranches[0].RemovableByExecute = &removable
	state.Cleanup.OrphanedTaskBranches[0].SuggestedCommands = []string{
		`.\brevity.ps1 task cleanup cleanup-one --force`,
		`git branch --delete task/cleanup-one`,
	}

	output := detailPaneViewWithSize(state, 0, 52, 40)

	for _, want := range []string{
		"warning / destructive-if... !",
		"destructive if unmerged: true",
		`suggested commands:`,
		`- .\brevity.ps1 task cleanup cleanup-one --force`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("cleanup detail truncation missing %q:\n%s", want, output)
		}
	}
	assertLinesWithinWidth(t, output, 52)
}

func TestCleanupDetailTruncatesLongSuggestedCommandClearly(t *testing.T) {
	state := bubbleStateWithCleanupCandidate("warning", "destructive-if-removed")
	state.Cleanup.OrphanedTaskBranches[0].SuggestedCommands = []string{
		`.\brevity.ps1 task cleanup cleanup-one --force --with-an-extra-long-operator-note`,
	}

	output := detailPaneViewWithSize(state, 0, 42, 40)

	if !strings.Contains(output, `- .\brevity.ps1 task cleanup cleanu...`) {
		t.Fatalf("long suggested command was not clearly ellipsized:\n%s", output)
	}
	if !strings.Contains(output, "warning / dest... !") {
		t.Fatalf("long command output lost destructive warning context:\n%s", output)
	}
	assertLinesWithinWidth(t, output, 42)
}

func TestHeaderFooterTruncationParityKeepsWarningSourceReadOnly(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleState()
	model.hasState = true
	model.width = 51
	model.lastRefresh = "2026-05-19T10:00:00Z"

	header := plainView(model.renderHeader())
	footer := plainView(model.renderFooter())

	for _, want := range []string{"native", "read-only", "!3", "p:1 t:1 c:1"} {
		if !strings.Contains(header, want) {
			t.Fatalf("narrow header dropped priority segment %q:\n%s", want, header)
		}
	}
	for _, want := range []string{"q", "j/k", "d", "r", "? help", "native", "read-only"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("narrow footer dropped priority segment %q:\n%s", want, footer)
		}
	}
	for _, dropped := range []string{"Brevity Runtime Dashboard", "last 2026-05-19T10:00:00Z", "1s refresh"} {
		if strings.Contains(header, dropped) || strings.Contains(footer, dropped) {
			t.Fatalf("narrow chrome kept lower-priority metadata %q:\nheader: %s\nfooter: %s", dropped, header, footer)
		}
	}
	assertLinesWithinWidth(t, header, model.width)
	assertLinesWithinWidth(t, footer, model.width)
}

func TestHeaderCompactWarningCountReadableAcrossWidths(t *testing.T) {
	tests := []struct {
		name      string
		width     int
		wantTitle bool
	}{
		{name: "narrow", width: 51},
		{name: "medium", width: 82, wantTitle: true},
		{name: "wide", width: 120, wantTitle: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModelWithSource(&fakeClient{}, time.Second, "native")
			model.state = bubbleState()
			model.hasState = true
			model.width = tt.width

			header := plainView(model.renderHeader())
			for _, want := range []string{"native", "read-only", "!3", "p:1 t:1 c:1"} {
				if !strings.Contains(header, want) {
					t.Fatalf("%s header missing %q:\n%s", tt.name, want, header)
				}
			}
			if tt.wantTitle && !strings.Contains(header, "Brevity Runtime") {
				t.Fatalf("%s header lost richer status title:\n%s", tt.name, header)
			}
			if !tt.wantTitle && strings.Contains(header, "Brevity Runtime Dashboard") {
				t.Fatalf("%s header kept lower-priority title before compact warning metadata:\n%s", tt.name, header)
			}
			for _, duplicate := range []string{"alerts alerts", "warning warning", "!3 !3"} {
				if strings.Contains(header, duplicate) {
					t.Fatalf("%s header duplicated warning wording %q:\n%s", tt.name, duplicate, header)
				}
			}
			assertLinesWithinWidth(t, header, tt.width)
		})
	}
}

func TestSummaryWarningCountsReadableAcrossWidths(t *testing.T) {
	tests := []struct {
		name  string
		width int
		want  []string
	}{
		{
			name:  "narrow",
			width: 46,
			want:  []string{"providers", "degraded", "!1", "tasks", "bl", "!1", "cleanup", "candidates", "!1"},
		},
		{
			name:  "medium",
			width: 82,
			want:  []string{"providers  1 total, 1 degraded, 0 unavailable !1", "tasks      1 tracked, 0 runnable, 1 blocked, 0 stale, 0 gated, 0 review !1", "cleanup    1 candidates, 1 inspect !1"},
		},
		{
			name:  "wide",
			width: 120,
			want:  []string{"providers  1 total, 1 degraded, 0 unavailable !1", "tasks      1 tracked, 0 runnable, 1 blocked, 0 stale, 0 gated, 0 review !1", "cleanup    1 candidates, 1 inspect !1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModelWithSource(&fakeClient{}, time.Second, "native")
			model.state = bubbleState()
			model.hasState = true
			model.width = tt.width

			summary := plainView(model.renderSummary())
			for _, want := range tt.want {
				if !strings.Contains(summary, want) {
					t.Fatalf("%s summary missing %q:\n%s", tt.name, want, summary)
				}
			}
			for _, duplicate := range []string{"warning ! warning", "degraded ! degraded", "blocked ! blocked"} {
				if strings.Contains(summary, duplicate) {
					t.Fatalf("%s summary duplicated warning wording %q:\n%s", tt.name, duplicate, summary)
				}
			}
			if strings.Count(summary, "!1") < 3 {
				t.Fatalf("%s summary did not keep provider/task/cleanup warning counts readable:\n%s", tt.name, summary)
			}
			assertLinesWithinWidth(t, summary, tt.width)
		})
	}
}

func TestFooterCompactParityDropsRefreshMetadataFirst(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleState()
	model.hasState = true
	model.width = 48
	model.lastRefresh = "2026-05-19T10:00:00Z"

	footer := plainView(model.renderFooter())
	for _, want := range []string{"q", "j/k", "d", "r", "? help", "native", "read-only"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("compact footer missing %q:\n%s", want, footer)
		}
	}
	for _, dropped := range []string{"1s refresh", "last 2026-05-19T10:00:00Z"} {
		if strings.Contains(footer, dropped) {
			t.Fatalf("compact footer kept lower-priority metadata %q:\n%s", dropped, footer)
		}
	}
	assertLinesWithinWidth(t, footer, model.width)
}

func TestDetailsPaneSelectionAndCollapsedTextStayIntentional(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleState()
	model.hasState = true
	model.selection.SelectedIndex = 1
	model.selection.ShowDetails = true

	output := plainView(model.View())
	if !containsDetail(output, "type", "task") || !containsDetail(output, "slug", "task-one") {
		t.Fatalf("details pane did not render selected task details:\n%s", output)
	}

	model.selection.ShowDetails = false
	output = plainView(model.View())
	if !strings.Contains(output, "select a row, then press d for details") {
		t.Fatalf("collapsed details text is not intentional:\n%s", output)
	}
	if strings.Contains(output, "no details selected") {
		t.Fatalf("collapsed details text should not look like an empty selection:\n%s", output)
	}

	empty := NewModelWithSource(&fakeClient{}, time.Second, "native")
	empty.state = contracts.RuntimeState{Schema: contracts.RuntimeStateSchema}
	empty.hasState = true
	empty.selection.ShowDetails = true
	output = plainView(empty.View())
	if !strings.Contains(output, "no details selected") {
		t.Fatalf("empty selected-details text is not intentional:\n%s", output)
	}
}

func TestSelectableListKeepsSelectedItemVisible(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleStateWithManyTasks(30)
	model.hasState = true
	model.height = 23
	model.selection.SelectedIndex = 11

	output := plainView(model.View())

	if !strings.Contains(output, "> task  task-11") {
		t.Fatalf("selected item is not visible:\n%s", output)
	}
	if !strings.Contains(output, "showing 5-12 of 33") {
		t.Fatalf("view missing expected scroll indicator:\n%s", output)
	}
}

func TestSelectableListWindowMovesAsSelectionChanges(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleStateWithManyTasks(30)
	model.hasState = true
	model.height = 23

	before := plainView(model.View())
	if !strings.Contains(before, "showing 1-8 of 33") {
		t.Fatalf("initial window did not start at first item:\n%s", before)
	}

	for i := 0; i < 12; i++ {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		model = updated.(Model)
	}

	after := plainView(model.View())
	if !strings.Contains(after, "showing 6-13 of 33") {
		t.Fatalf("window did not move with selection:\n%s", after)
	}
	if !strings.Contains(after, "> task  task-12") {
		t.Fatalf("selected item is not visible after movement:\n%s", after)
	}
}

func TestSelectableListSmallHeightDoesNotPanic(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleStateWithManyTasks(8)
	model.hasState = true
	model.height = 1
	model.selection.SelectedIndex = 5

	output := plainView(model.View())

	for _, want := range []string{"Brevity", "warning", "native", "read-only", "q quit"} {
		if !strings.Contains(output, want) {
			t.Fatalf("ultra-small-height view missing %q:\n%s", want, output)
		}
	}
	for _, clipped := range []string{"Selectable List", "Details Pane", "showing 6-6 of 11", "> task  task-05"} {
		if strings.Contains(output, clipped) {
			t.Fatalf("ultra-small-height view rendered normal layout fragment %q:\n%s", clipped, output)
		}
	}
	assertLinesWithinWidth(t, output, model.contentWidth())
}

func TestUltraSmallHeightLoadingAndErrorViewsStayMinimal(t *testing.T) {
	tests := []struct {
		name      string
		height    int
		hasState  bool
		lastError error
		want      []string
	}{
		{name: "loading height one", height: 1, want: []string{"Brevity", "loading", "native", "read-only", "q quit"}},
		{name: "loading error height two", height: 2, lastError: errors.New("runtime unavailable"), want: []string{"Brevity", "loading", "warning", "native", "read-only", "q quit"}},
		{name: "state warning height three", height: 3, hasState: true, want: []string{"Brevity", "warning", "native", "read-only", "q quit"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModelWithSource(&fakeClient{}, time.Second, "native")
			model.height = tt.height
			model.lastError = tt.lastError
			if tt.hasState {
				model.state = bubbleState()
				model.hasState = true
			}

			output := plainView(model.View())
			lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
			if len(lines) > tt.height {
				t.Fatalf("ultra-small-height rows = %d, want <= %d:\n%s", len(lines), tt.height, output)
			}
			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Fatalf("ultra-small-height view missing %q:\n%s", want, output)
				}
			}
			for _, clipped := range []string{"Runtime Summary", "Selectable List", "Details Pane", "Warnings"} {
				if strings.Contains(output, clipped) {
					t.Fatalf("ultra-small-height view rendered normal layout fragment %q:\n%s", clipped, output)
				}
			}
			assertLinesWithinWidth(t, output, model.contentWidth())
		})
	}
}

func TestUltraSmallHeightZeroFromTerminalDoesNotPanic(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleState()
	model.hasState = true

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 0})
	model = updated.(Model)

	output := model.View()
	if output != "" {
		t.Fatalf("zero-height terminal rendered output %q, want empty", plainView(output))
	}
}

func TestSelectableListScrollIndicatorAppearsWhenTruncated(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleStateWithManyTasks(20)
	model.hasState = true
	model.height = 22

	output := plainView(model.View())

	if !strings.Contains(output, "showing 1-7 of 23") {
		t.Fatalf("view missing scroll indicator:\n%s", output)
	}
}

func TestDetailsTruncateAtSmallHeight(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleState()
	model.hasState = true
	model.height = 19
	model.selection.SelectedIndex = 2
	model.selection.ShowDetails = true

	output := plainView(model.View())

	if !strings.Contains(output, "... details truncated") {
		t.Fatalf("small-height details missing truncation indicator:\n%s", output)
	}
	if !strings.Contains(output, "q quit | j/k move | d details") {
		t.Fatalf("small-height view missing footer controls:\n%s", output)
	}
	if !strings.Contains(output, "> clean requires-inspection: orphan-branch:task-old") {
		t.Fatalf("small-height view missing selected list row:\n%s", output)
	}
}

func TestDetailsRenderFullyAtNormalHeight(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleState()
	model.hasState = true
	model.height = 40
	model.selection.SelectedIndex = 2
	model.selection.ShowDetails = true

	output := plainView(model.View())

	if strings.Contains(output, "... details truncated") {
		t.Fatalf("normal-height details were truncated:\n%s", output)
	}
	for _, detail := range []struct {
		label string
		value string
	}{
		{"type", "cleanup candidate"},
		{"dirty reasons", "(none)"},
		{"suggested commands", "(none)"},
	} {
		if !containsDetail(output, detail.label, detail.value) {
			t.Fatalf("normal-height details missing %q=%q:\n%s", detail.label, detail.value, output)
		}
	}
}

func TestFooterPinsToBottomWhenHeightAllows(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleState()
	model.hasState = true
	model.width = 82
	model.height = 34
	model.lastRefresh = "2026-05-19T10:00:00Z"

	output := plainView(model.View())
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")

	if len(lines) != model.height {
		t.Fatalf("rendered rows = %d, want terminal height %d:\n%s", len(lines), model.height, output)
	}
	if !strings.Contains(lines[len(lines)-1], "q quit | j/k move | d details | p action | r refresh | ? help") {
		t.Fatalf("footer was not anchored on final row:\n%s", output)
	}
	if lines[len(lines)-2] != "" {
		t.Fatalf("expected quiet padding row above pinned footer, got %q:\n%s", lines[len(lines)-2], output)
	}
}

func TestLoadingFooterPinsToBottomWhenHeightAllows(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.width = 72
	model.height = 18

	output := plainView(model.View())
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")

	if len(lines) != model.height {
		t.Fatalf("loading rendered rows = %d, want terminal height %d:\n%s", len(lines), model.height, output)
	}
	if !strings.Contains(lines[len(lines)-1], "q") || !strings.Contains(lines[len(lines)-1], "p") {
		t.Fatalf("loading footer was not anchored on final row:\n%s", output)
	}
}

func TestHelpRendersAtSmallHeight(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleStateWithManyTasks(8)
	model.hasState = true
	model.height = 20
	model.selection.ShowHelp = true

	output := plainView(model.View())

	if !strings.Contains(output, "Help") || !strings.Contains(output, "? help") {
		t.Fatalf("small-height help missing compact help text:\n%s", output)
	}
	if !strings.Contains(output, "q quit | j/k move | d details") {
		t.Fatalf("small-height help view missing footer controls:\n%s", output)
	}
	if !strings.Contains(output, "> prov  codex") {
		t.Fatalf("small-height help view missing selected row:\n%s", output)
	}
}

func TestModelRefreshCommandReadsRuntimeState(t *testing.T) {
	client := &fakeClient{
		output: []byte(`{"schema":"brevity.runtime-state.v1","repoRoot":"C:\\repo","taskCounts":{"tracked":2}}`),
	}
	model := NewModel(client, time.Second)

	msg := model.refreshCmd()()
	refresh, ok := msg.(refreshMsg)
	if !ok {
		t.Fatalf("message type = %T, want refreshMsg", msg)
	}
	if refresh.err != nil {
		t.Fatalf("refresh error = %v", refresh.err)
	}
	if refresh.state.TaskCounts.Tracked != 2 {
		t.Fatalf("tracked = %d, want 2", refresh.state.TaskCounts.Tracked)
	}
	if client.calls != 1 {
		t.Fatalf("calls = %d, want 1", client.calls)
	}
}

func bubbleState() contracts.RuntimeState {
	return contracts.RuntimeState{
		Schema:   contracts.RuntimeStateSchema,
		RepoRoot: `C:\repo`,
		Providers: contracts.Providers{
			Summary: contracts.ProviderSummary{Total: 1, Degraded: 1},
			Health: map[string]contracts.ProviderHealth{
				"codex": {Status: "degraded", Note: "capacity limited"},
			},
		},
		TaskCounts: contracts.TaskCounts{Tracked: 1, Blocked: 1},
		Tasks: []contracts.TaskSummary{
			{Slug: "task-one", Status: "blocked", Branch: "task/task-one"},
		},
		Cleanup: &contracts.Cleanup{
			Summary: &contracts.CleanupSummary{TotalCandidates: 1, RequiresInspectionCount: 1},
			OrphanedTaskBranches: []contracts.CleanupCandidate{
				{ID: "orphan-branch:task-old", Severity: "warning", Category: "requires-inspection", Branch: "task/old"},
			},
		},
		SuggestedNextActions: []string{"Inspect state."},
	}
}

func bubbleStateWithProvider(name string, status string) contracts.RuntimeState {
	return contracts.RuntimeState{
		Schema:   contracts.RuntimeStateSchema,
		RepoRoot: `C:\repo`,
		Providers: contracts.Providers{
			Summary: contracts.ProviderSummary{Total: 1, Degraded: 1},
			Health: map[string]contracts.ProviderHealth{
				name: {Status: status},
			},
		},
	}
}

func bubbleStateWithTaskState(state string) contracts.RuntimeState {
	return contracts.RuntimeState{
		Schema:     contracts.RuntimeStateSchema,
		RepoRoot:   `C:\repo`,
		TaskCounts: contracts.TaskCounts{Tracked: 1},
		Tasks: []contracts.TaskSummary{
			{Slug: "task-one", Status: state, Branch: "task/task-one"},
		},
	}
}

func bubbleStateWithCleanupCandidate(severity string, category string) contracts.RuntimeState {
	return contracts.RuntimeState{
		Schema:   contracts.RuntimeStateSchema,
		RepoRoot: `C:\repo`,
		Cleanup: &contracts.Cleanup{
			Summary: &contracts.CleanupSummary{TotalCandidates: 1, RequiresInspectionCount: 1},
			OrphanedTaskBranches: []contracts.CleanupCandidate{
				{ID: "cleanup-one", Severity: severity, Category: category, Branch: "task/cleanup-one"},
			},
		},
	}
}

func bubbleStateWithLongPaths() contracts.RuntimeState {
	state := bubbleState()
	state.RepoRoot = `C:\dev\repos\active\brevity\with\an\unusually\deep\workspace\path`
	state.Tasks[0].WorktreePath = `C:\dev\repos\active\brevity\worktrees\active\very-long-task-name`
	state.Tasks[0].LatestRunLogPath = `C:\dev\repos\active\brevity\.brevity\runs\task-one\20260520-120000\worker-output.log`
	state.Cleanup.OrphanedTaskBranches[0].Path = `C:\dev\repos\active\brevity\worktrees\completed\very-long-task-name`
	state.Cleanup.OrphanedTaskBranches[0].SuggestedCommands = []string{
		`git -C C:\dev\repos\active\brevity\worktrees\completed\very-long-task-name status --short`,
	}
	return state
}

func bubbleStateWithManyTasks(taskCount int) contracts.RuntimeState {
	state := bubbleState()
	state.TaskCounts.Tracked = taskCount
	state.Tasks = make([]contracts.TaskSummary, 0, taskCount)
	for index := 1; index <= taskCount; index++ {
		state.Tasks = append(state.Tasks, contracts.TaskSummary{
			Slug:   fmt.Sprintf("task-%02d", index),
			Status: "ready",
			Branch: fmt.Sprintf("task/task-%02d", index),
		})
	}
	return state
}

func bubbleStateWithLongWarningLabels() contracts.RuntimeState {
	state := bubbleState()
	state.Providers.Health = map[string]contracts.ProviderHealth{
		"codex-provider-with-a-long-display-name": {Status: "unavailable"},
	}
	state.Tasks = []contracts.TaskSummary{
		{
			Slug:            "task-with-a-long-label-that-must-truncate",
			NormalizedState: "provider-gated",
			Branch:          "task/task-with-a-long-label-that-must-truncate",
		},
	}
	state.Cleanup.OrphanedTaskBranches = []contracts.CleanupCandidate{
		{
			ID:       "cleanup-candidate-with-long-id",
			Severity: "warning",
			Category: "destructive-if-removed-with-extra-context",
			Branch:   "task/cleanup-candidate-with-long-id",
		},
	}
	state.SuggestedNextActions = []string{"Review native orphaned task cleanup findings before cleanup."}
	return state
}

func bubbleStateWithUnicodeText() contracts.RuntimeState {
	state := bubbleState()
	state.RepoRoot = `C:\dev\repos\active\brevity\arbeid-æøå\prosjekt-🚧`
	state.Providers.Health = map[string]contracts.ProviderHealth{
		"codex-æøå-🚧-provider-with-a-long-display-name": {
			Status:    "degraded",
			Note:      "kapasitet lav: blåbær kø åpen │ check",
			UpdatedAt: "2026-05-19T10:00:00Z",
		},
	}
	state.Tasks = []contracts.TaskSummary{
		{
			Slug:              "oppgave-æøå-🚧-with-a-long-label-that-must-truncate",
			NormalizedState:   "provider-gated",
			Branch:            "task/oppgave-æøå-🚧-with-a-long-label-that-must-truncate",
			WorktreePath:      `C:\dev\repos\active\brevity\worktrees\active\oppgave-æøå-🚧-with-a-long-label-that-must-truncate`,
			LatestRunLogPath:  `C:\dev\repos\active\brevity\.brevity\runs\oppgave-æøå-🚧\latest\worker-output-✅.log`,
			LatestRunProvider: "codex-æøå-🚧",
			LatestRunProfile:  "norsk-åpen",
		},
	}
	state.Cleanup.OrphanedTaskBranches = []contracts.CleanupCandidate{
		{
			ID:       "opprydding-æøå-🚧-candidate-with-long-id",
			Severity: "warning",
			Category: "destructive-if-removed-│-requires-review",
			Branch:   "task/opprydding-æøå-🚧",
			Path:     `C:\dev\repos\active\brevity\worktrees\completed\opprydding-æøå-🚧`,
			SuggestedCommands: []string{
				`git status --short C:\dev\repos\active\brevity\worktrees\completed\opprydding-æøå-🚧`,
			},
		},
	}
	state.SuggestedNextActions = []string{"Review 🚧 unicode-heavy task values before cleanup."}
	return state
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func ansiStyle(value string) string {
	return "\x1b[33m" + value + "\x1b[0m"
}

func plainView(output string) string {
	return ansiPattern.ReplaceAllString(output, "")
}

func containsDetail(output string, label string, value string) bool {
	pattern := regexp.MustCompile(`(?m)\s+` + regexp.QuoteMeta(label) + `:\s+` + regexp.QuoteMeta(value))
	return pattern.FindStringIndex(output) != nil
}

func detailPaneView(state contracts.RuntimeState, selected int) string {
	return detailPaneViewWithSize(state, selected, defaultTerminalWidth, 0)
}

func detailPaneViewWithSize(state contracts.RuntimeState, selected int, width int, height int) string {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = state
	model.hasState = true
	model.width = width
	model.height = height
	model.selection.SelectedIndex = selected
	model.selection.ShowDetails = true
	return plainView(model.View())
}

func assertLinesWithinWidth(t *testing.T, output string, width int) {
	t.Helper()
	for index, line := range strings.Split(output, "\n") {
		if visibleWidth(line) > width {
			t.Fatalf("line %d visible width = %d, want <= %d:\n%s", index+1, visibleWidth(line), width, output)
		}
	}
}

func assertRenderInvariants(t *testing.T, output string, width int, source string, wantTwoPane bool) {
	t.Helper()
	assertLinesWithinWidth(t, output, width)

	for _, want := range []string{
		">",
		"!",
		"!3",
		source,
		"read-only",
		"q",
		"j/k",
		"d",
		"r",
		"? help",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("render invariant missing %q:\n%s", want, output)
		}
	}

	hasTwoPane := strings.Contains(output, paneSeparator+"Details Pane")
	if hasTwoPane != wantTwoPane {
		t.Fatalf("two-pane separator = %t, want %t:\n%s", hasTwoPane, wantTwoPane, output)
	}
	if !wantTwoPane && strings.Contains(output, paneSeparator+"Details Pane") {
		t.Fatalf("single-column view rendered two-pane separator:\n%s", output)
	}
}

func assertLoadingOrErrorViewInvariants(t *testing.T, output string, width int, source string) {
	t.Helper()
	assertLinesWithinWidth(t, output, width)

	for _, want := range []string{
		"Runtime Summary",
		source,
		"read-only",
		"q",
		"j/k",
		"d",
		"r",
		"? help",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("loading/error render invariant missing %q:\n%s", want, output)
		}
	}
}
