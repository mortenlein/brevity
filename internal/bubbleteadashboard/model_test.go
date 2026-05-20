package bubbleteadashboard

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mortenlein/brevity/internal/contracts"
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

func TestTruncatePathPreservesSuffix(t *testing.T) {
	path := `C:\dev\repos\active\brevity\worktrees\active\very-long-task-name\runs\latest\worker-output.log`

	got := truncatePath(path, 32)

	if len([]rune(got)) > 32 {
		t.Fatalf("truncatePath length = %d, want <= 32: %q", len([]rune(got)), got)
	}
	if !strings.HasPrefix(got, "...") {
		t.Fatalf("truncatePath = %q, want prefix ...", got)
	}
	if !strings.HasSuffix(got, `latest\worker-output.log`) {
		t.Fatalf("truncatePath = %q, want important suffix", got)
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
		"native | read-only | alerts p:1 t:1 c:1",
		"Runtime Summary",
		"Selectable List",
		"> prov  codex: degraded !",
		"Details Pane",
		"select a row, then press d for details",
		"q quit | j/k move | d details | r refresh | ? help",
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
		if len([]rune(line)) > model.width {
			t.Fatalf("line %d width = %d, want <= %d:\n%s", index+1, len([]rune(line)), model.width, output)
		}
	}
	if !strings.Contains(output, "...") {
		t.Fatalf("narrow view missing truncation marker:\n%s", output)
	}
	if strings.Contains(output, `C:\dev\repos\active\brevity\worktrees\active\very-long-task-name`) {
		t.Fatalf("narrow view contains unshortened long path:\n%s", output)
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

	if !strings.Contains(output, "showing 6-6 of 11") {
		t.Fatalf("small-height view missing one-row window indicator:\n%s", output)
	}
	if !strings.Contains(output, "> task  task-05") {
		t.Fatalf("small-height view missing selected row:\n%s", output)
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

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plainView(output string) string {
	return ansiPattern.ReplaceAllString(output, "")
}

func containsDetail(output string, label string, value string) bool {
	pattern := regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(label) + `:\s+` + regexp.QuoteMeta(value))
	return pattern.FindStringIndex(output) != nil
}
