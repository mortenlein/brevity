package bubbleteadashboard

import (
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
	if !strings.Contains(model.View(), "Brevity Runtime Dashboard") {
		t.Fatalf("view missing title:\n%s", model.View())
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
			Summary: contracts.ProviderSummary{Total: 1},
			Health: map[string]contracts.ProviderHealth{
				"codex": {Status: "ok"},
			},
		},
		Tasks: []contracts.TaskSummary{
			{Slug: "task-one", Status: "ready"},
		},
		SuggestedNextActions: []string{"Inspect state."},
	}
}
