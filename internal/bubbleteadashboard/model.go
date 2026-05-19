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
}

func NewModel(client runtimeclient.Client, refreshInterval time.Duration) Model {
	if refreshInterval <= 0 {
		refreshInterval = defaultRefreshInterval
	}
	return Model{
		client:          client,
		refreshInterval: refreshInterval,
	}
}

func Run(ctx context.Context, input io.Reader, stdout io.Writer, client runtimeclient.Client, refreshInterval time.Duration) error {
	if !isTerminalInput(input) {
		return runLineFallback(stdout, input, client, refreshInterval)
	}

	program := tea.NewProgram(
		NewModel(client, refreshInterval),
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

func runLineFallback(stdout io.Writer, input io.Reader, client runtimeclient.Client, refreshInterval time.Duration) error {
	model := NewModel(client, refreshInterval)
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
	case "r":
		return model, model.refreshCmd()
	default:
		return model, nil
	}
}

func (model Model) View() string {
	if !model.hasState {
		output := "Brevity Runtime Dashboard\n=========================\n"
		output += "Mode: experimental Bubble Tea read-only dashboard.\n"
		output += "Loading runtime state...\n"
		if model.lastError != nil {
			output += fmt.Sprintf("Polling error: %v\n", model.lastError)
		}
		output += "\nKeys: q quit, j/k or arrows move, d/enter details, r refresh\n"
		return output
	}

	output := dashboard.RenderInteractiveString(model.state, model.selection)
	output += fmt.Sprintf("\nLast successful refresh: %s\n", fallbackRefresh(model.lastRefresh))
	if model.lastError != nil {
		output += fmt.Sprintf("Polling error: %v\n", model.lastError)
	}
	output += "Keys: q quit, j/k or arrows move, d/enter details, r refresh\n"
	return output
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
