package bubbleteadashboard

import (
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
		"Brevity Runtime Dashboard [read-only] [source: native]",
		"Runtime Summary",
		"Selectable List",
		"> provider codex: degraded !",
		"Details Pane",
		"details hidden; press d or enter",
		"Footer",
		"q quit | j/k or arrows move | d/enter details | r refresh | ? help",
		"refresh: every 1s | last success: 2026-05-19T10:00:00Z | source: native",
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

func TestModelViewShortensLongPaths(t *testing.T) {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleStateWithLongPaths()
	model.hasState = true
	model.width = 60
	model.selection.SelectedIndex = 1
	model.selection.ShowDetails = true

	output := plainView(model.View())

	if !strings.Contains(output, `worktree: ...`) || !strings.Contains(output, `very-long-task-name`) {
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
		"type: provider",
		"name: codex",
		"status: degraded !",
		"guidance: provider is degraded",
		"Help",
		"? help",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("view missing %q:\n%s", want, output)
		}
	}
}

func TestModelViewRendersTaskCleanupAndActionDetails(t *testing.T) {
	tests := []struct {
		name     string
		selected int
		want     []string
	}{
		{
			name:     "task",
			selected: 1,
			want:     []string{"type: task", "slug: task-one", "state: blocked", "branch: task/task-one"},
		},
		{
			name:     "cleanup",
			selected: 2,
			want:     []string{"type: cleanup candidate", "id: orphan-branch:task-old", "severity/category: warning / requires-inspection"},
		},
		{
			name:     "action",
			selected: 3,
			want:     []string{"type: suggested action", "action: Inspect state.", "display-only; no command is executed"},
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
		})
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

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plainView(output string) string {
	return ansiPattern.ReplaceAllString(output, "")
}
