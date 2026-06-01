package bubbleteadashboard

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

const productGoalPath = "docs/product-goal.md"

type PlanningModel struct {
	productGoal string
	loadError   error
	repoRoot    string
	width       int
	height      int
	quitting    bool
}

func NewPlanningModel(productGoal string, loadError error) PlanningModel {
	return PlanningModel{productGoal: strings.TrimSpace(productGoal), loadError: loadError, repoRoot: "."}
}

func RunPlan(ctx context.Context, input io.Reader, stdout io.Writer, repoRoot string) error {
	goal, err := loadProductGoal(repoRoot)
	model := NewPlanningModel(goal, err)
	model.repoRoot = fallback(repoRoot, ".")
	if !isPlanningTerminalInput(input) {
		return runPlanningLineFallback(stdout, input, model)
	}
	program := tea.NewProgram(model, tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(stdout))
	_, runErr := program.Run()
	if runErr != nil {
		return runErr
	}
	return nil
}

func loadProductGoal(repoRoot string) (string, error) {
	if strings.TrimSpace(repoRoot) == "" {
		repoRoot = "."
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, productGoalPath))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func isPlanningTerminalInput(input io.Reader) bool {
	file, ok := input.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func runPlanningLineFallback(stdout io.Writer, input io.Reader, model PlanningModel) error {
	fmt.Fprint(stdout, model.View())
	if input == nil {
		return nil
	}
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		key := strings.TrimSpace(strings.ToLower(scanner.Text()))
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		model = updated.(PlanningModel)
		if key == "q" {
			return nil
		}
		fmt.Fprint(stdout, model.View())
	}
	return scanner.Err()
}

func (model PlanningModel) Init() tea.Cmd {
	return nil
}

func (model PlanningModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		model.width = msg.Width
		model.height = msg.Height
	case tea.KeyMsg:
		switch strings.ToLower(msg.String()) {
		case "q", "esc", "ctrl+c":
			model.quitting = true
			return model, tea.Quit
		}
	}
	return model, nil
}

func (model PlanningModel) View() string {
	var output strings.Builder
	width := model.contentWidth()
	output.WriteString(dashboardStyles.title.Render(statusLine(width,
		statusSegment{text: "BREVITY PLAN", priority: 0},
		statusSegment{text: "idea -> plan -> task", compact: "planning", priority: 0},
		statusSegment{text: "read-only workspace", compact: "read-only", priority: 1},
	)) + "\n")

	renderSection(&output, "Product Goal")
	if model.loadError != nil {
		output.WriteString(model.line("  product goal unavailable: " + model.loadError.Error()))
	} else {
		for _, line := range planningGoalLines(model.productGoal, 4) {
			output.WriteString(model.line("  " + line))
		}
	}

	output.WriteString("\n")
	renderSection(&output, "Planning Flow")
	for _, step := range []string{
		"idea    Capture the operator intent before it becomes task machinery.",
		"plan    Shape scope, acceptance, risk, and review notes.",
		"task    Create a focused worktree-ready task.",
		"execute Run the task outside this v1 planning workspace.",
		"review  Inspect the result in Brevity.",
		"merge   Approve and integrate completed work.",
	} {
		output.WriteString(model.line("  " + step))
	}

	output.WriteString("\n")
	renderSection(&output, "Workspace")
	if !planningPlanExists(model.repoRoot) {
		output.WriteString(model.wrapped("  ", "No plan exists yet. Start with the product goal, write the smallest useful plan, then turn it into one reviewable task."))
		output.WriteString(model.wrapped("  ", "A good Brevity plan should make a developer more likely to open Brevity instead of Codex directly."))
	} else {
		output.WriteString(model.line("  plan draft detected; refine it until the next task is obvious"))
	}

	output.WriteString("\n")
	renderSection(&output, "Next Commands")
	for _, command := range []string{
		"brevity task new <slug>",
		"brevity task status",
		"brevity review",
		"brevity task merge <slug>",
	} {
		output.WriteString(model.line("  " + command))
	}

	output.WriteString("\n")
	renderSection(&output, "Mutation Boundary")
	output.WriteString(model.wrapped("  ", "This workspace reads docs/product-goal.md and renders guidance only. It does not mutate task, queue, execution, provider, or run state."))
	output.WriteString(model.footer())
	return output.String()
}

func (model PlanningModel) contentWidth() int {
	if model.width <= 0 {
		return defaultTerminalWidth
	}
	if model.width < minimumTerminalWidth {
		return minimumTerminalWidth
	}
	return model.width
}

func (model PlanningModel) line(value string) string {
	return truncateValuePreservingWarning(value, model.contentWidth()) + "\n"
}

func (model PlanningModel) wrapped(prefix string, value string) string {
	width := model.contentWidth() - visibleWidth(prefix)
	lines := wrapDetailValue(value, width)
	var output strings.Builder
	for _, line := range lines {
		output.WriteString(model.line(prefix + line))
	}
	return output.String()
}

func (model PlanningModel) footer() string {
	return "\n" + dashboardStyles.footer.Render(model.line("  q quit"))
}

func planningGoalLines(markdown string, limit int) []string {
	lines := strings.Split(markdown, "\n")
	result := make([]string, 0, limit)
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if line == "" || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "1.") {
			continue
		}
		result = append(result, line)
		if len(result) >= limit {
			break
		}
	}
	if len(result) == 0 {
		return []string{"Product goal is empty; define the operator outcome before creating tasks."}
	}
	return result
}

func planningPlanExists(repoRoot string) bool {
	for _, path := range []string{
		filepath.Join(repoRoot, ".brevity", "plan.md"),
		filepath.Join(repoRoot, "docs", "plan.md"),
	} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}
