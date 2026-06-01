package bubbleteadashboard

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

const productGoalPath = "docs/product-goal.md"
const planningIdeasPath = ".brevity/ideas.json"

type PlanningIdea struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"createdAt"`
	Status      string `json:"status"`
}

type planningIdeaStore struct {
	path string
}

type PlanningModel struct {
	productGoal string
	loadError   error
	storeError  error
	repoRoot    string
	store       planningIdeaStore
	ideas       []PlanningIdea
	selected    int
	inputMode   string
	inputValue  string
	message     string
	now         func() time.Time
	width       int
	height      int
	quitting    bool
}

func NewPlanningModel(productGoal string, loadError error) PlanningModel {
	return PlanningModel{productGoal: strings.TrimSpace(productGoal), loadError: loadError, repoRoot: ".", now: time.Now}
}

func RunPlan(ctx context.Context, input io.Reader, stdout io.Writer, repoRoot string) error {
	goal, err := loadProductGoal(repoRoot)
	model := NewPlanningModel(goal, err)
	model.repoRoot = fallback(repoRoot, ".")
	model.store = newPlanningIdeaStore(model.repoRoot)
	model.ideas, model.storeError = model.store.Load()
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
		line := scanner.Text()
		key := strings.TrimSpace(strings.ToLower(line))
		if model.inputMode != "" {
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			model.inputValue = line
			updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			model = updated.(PlanningModel)
			fmt.Fprint(stdout, model.View())
			continue
		}
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
		if model.inputMode != "" {
			return model.updateInput(msg)
		}
		switch strings.ToLower(msg.String()) {
		case "q", "esc", "ctrl+c":
			model.quitting = true
			return model, tea.Quit
		case "n":
			model.inputMode = "new-title"
			model.inputValue = ""
			model.message = "Enter idea title, then press enter."
		case "enter":
			if len(model.ideas) > 0 {
				model.message = "Inspecting selected idea."
			}
		case "up", "k":
			if model.selected > 0 {
				model.selected--
			}
		case "down", "j":
			if model.selected < len(model.ideas)-1 {
				model.selected++
			}
		case "d":
			model = model.deleteSelectedIdea()
		case "t":
			if len(model.ideas) == 0 {
				model.message = "Capture an idea before creating a task draft."
			} else {
				model.message = "Task draft conversion is not implemented yet."
			}
		}
	}
	return model, nil
}

func (model PlanningModel) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		model.inputMode = ""
		model.inputValue = ""
		model.message = "Idea capture cancelled."
	case tea.KeyEnter:
		if strings.TrimSpace(model.inputValue) == "" {
			model.message = "Idea title is required."
			return model, nil
		}
		idea := PlanningIdea{
			ID:        model.nextIdeaID(),
			Title:     strings.TrimSpace(model.inputValue),
			CreatedAt: model.now().UTC().Format(time.RFC3339),
			Status:    "captured",
		}
		model.ideas = append(model.ideas, idea)
		model.selected = len(model.ideas) - 1
		model.inputMode = ""
		model.inputValue = ""
		if err := model.store.Save(model.ideas); err != nil {
			model.storeError = err
			model.message = "Could not save idea: " + err.Error()
		} else {
			model.message = "Idea captured."
		}
	case tea.KeyBackspace, tea.KeyCtrlH:
		if len(model.inputValue) > 0 {
			runes := []rune(model.inputValue)
			model.inputValue = string(runes[:len(runes)-1])
		}
	case tea.KeyRunes:
		model.inputValue += string(msg.Runes)
	}
	return model, nil
}

func (model PlanningModel) nextIdeaID() string {
	base := "idea-" + model.now().UTC().Format("20060102-150405")
	seen := map[string]struct{}{}
	for _, idea := range model.ideas {
		seen[idea.ID] = struct{}{}
	}
	if _, ok := seen[base]; !ok {
		return base
	}
	for index := 2; ; index++ {
		id := fmt.Sprintf("%s-%d", base, index)
		if _, ok := seen[id]; !ok {
			return id
		}
	}
}

func (model PlanningModel) deleteSelectedIdea() PlanningModel {
	if len(model.ideas) == 0 {
		model.message = "No idea selected."
		return model
	}
	deleted := model.ideas[model.selected]
	model.ideas = append(model.ideas[:model.selected], model.ideas[model.selected+1:]...)
	if model.selected >= len(model.ideas) && model.selected > 0 {
		model.selected--
	}
	if err := model.store.Save(model.ideas); err != nil {
		model.storeError = err
		model.message = "Could not delete idea: " + err.Error()
		return model
	}
	model.message = "Deleted idea: " + deleted.Title
	return model
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
	if model.storeError != nil {
		output.WriteString(model.wrapped("  ", "Planning store unavailable: "+model.storeError.Error()))
	}
	if len(model.ideas) == 0 {
		output.WriteString(model.wrapped("  ", "No ideas captured yet. Press n to capture the smallest useful operator intent before it becomes task machinery."))
	} else {
		renderSection(&output, "Ideas")
		for index, idea := range model.ideas {
			prefix := fmt.Sprintf("  %d. ", index+1)
			if index == model.selected {
				prefix = "  > "
			}
			output.WriteString(model.line(prefix + idea.Title + " [" + idea.Status + "]"))
		}
		output.WriteString("\n")
		model.renderSelectedIdea(&output)
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
	output.WriteString(model.wrapped("  ", "This workspace may update .brevity/ideas.json only. It does not mutate task, queue, execution, provider, or run state."))
	output.WriteString(model.footer())
	return output.String()
}

func (model PlanningModel) renderSelectedIdea(output *strings.Builder) {
	if len(model.ideas) == 0 {
		return
	}
	idea := model.ideas[model.selected]
	renderSection(output, "Selected Idea")
	output.WriteString(model.line("  Title:   " + idea.Title))
	output.WriteString(model.line("  Status:  " + idea.Status))
	output.WriteString(model.line("  Created: " + idea.CreatedAt))
	if strings.TrimSpace(idea.Description) != "" {
		output.WriteString(model.wrapped("  Description: ", idea.Description))
	}
	output.WriteString("\n")
	renderSection(output, "Next Steps")
	for _, step := range []string{
		"[convert to milestone] placeholder",
		"[convert to task draft] placeholder",
		"[edit] placeholder",
	} {
		output.WriteString(model.line("  " + step))
	}
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
	if model.inputMode != "" {
		return "\n" + dashboardStyles.footer.Render(model.line("  title: "+model.inputValue+"  enter save  esc cancel"))
	}
	help := "  n new idea  enter inspect  d delete  t task draft  q quit"
	if model.message != "" {
		help = "  " + model.message + "  |  " + strings.TrimSpace(help)
	}
	return "\n" + dashboardStyles.footer.Render(model.line(help))
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

func newPlanningIdeaStore(repoRoot string) planningIdeaStore {
	if strings.TrimSpace(repoRoot) == "" {
		repoRoot = "."
	}
	return planningIdeaStore{path: filepath.Join(repoRoot, planningIdeasPath)}
}

func (store planningIdeaStore) Load() ([]PlanningIdea, error) {
	data, err := os.ReadFile(store.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ideas []PlanningIdea
	if err := json.Unmarshal(data, &ideas); err != nil {
		return nil, err
	}
	sort.SliceStable(ideas, func(i, j int) bool {
		return ideas[i].CreatedAt < ideas[j].CreatedAt
	})
	return ideas, nil
}

func (store planningIdeaStore) Save(ideas []PlanningIdea) error {
	if err := os.MkdirAll(filepath.Dir(store.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ideas, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(store.path, data, 0o644)
}
