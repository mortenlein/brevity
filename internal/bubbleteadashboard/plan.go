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

	"github.com/mortenlein/brevity/internal/state"
)

const productGoalPath = "docs/product-goal.md"
const planningIdeasPath = ".brevity/ideas.json"
const planningTaskDraftsPath = ".brevity/task-drafts.json"

type PlanningIdea struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"createdAt"`
	Status      string `json:"status"`
}

type PlanningTaskDraft struct {
	ID                 string `json:"id"`
	IdeaID             string `json:"ideaId"`
	TaskSlug           string `json:"taskSlug,omitempty"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	Status             string `json:"status"`
	AcceptanceCriteria string `json:"acceptanceCriteria"`
	Validation         string `json:"validation"`
	CreatedAt          string `json:"createdAt"`
	PromotedAt         string `json:"promotedAt,omitempty"`
}

type planningIdeaStore struct {
	path string
}

type planningTaskDraftStore struct {
	path string
}

type PlanningModel struct {
	productGoal string
	loadError   error
	storeError  error
	repoRoot    string
	store       planningIdeaStore
	draftStore  planningTaskDraftStore
	ideas       []PlanningIdea
	drafts      []PlanningTaskDraft
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
	model.draftStore = newPlanningTaskDraftStore(model.repoRoot)
	model.drafts, err = model.draftStore.Load()
	if err != nil && model.storeError == nil {
		model.storeError = err
	}
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
				model = model.createTaskDraftFromSelectedIdea()
			}
		case "p":
			model = model.promoteSelectedTaskDraft()
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

func (model PlanningModel) createTaskDraftFromSelectedIdea() PlanningModel {
	if len(model.ideas) == 0 {
		model.message = "No idea selected."
		return model
	}
	idea := model.ideas[model.selected]
	for _, draft := range model.drafts {
		if draft.IdeaID == idea.ID {
			model.message = "Task draft already exists for selected idea."
			return model
		}
	}
	draft := PlanningTaskDraft{
		ID:                 model.nextTaskDraftID(idea),
		IdeaID:             idea.ID,
		Title:              idea.Title,
		Description:        "Generated from planning idea",
		Status:             "draft",
		AcceptanceCriteria: "[placeholder]",
		Validation:         "[placeholder]",
		CreatedAt:          model.now().UTC().Format(time.RFC3339),
	}
	model.drafts = append(model.drafts, draft)
	if err := model.draftStore.Save(model.drafts); err != nil {
		model.storeError = err
		model.message = "Could not save task draft: " + err.Error()
		return model
	}
	model.message = "Task draft created."
	return model
}

func (model PlanningModel) nextTaskDraftID(idea PlanningIdea) string {
	base := "draft-" + strings.TrimPrefix(idea.ID, "idea-")
	if strings.TrimSpace(base) == "draft-" {
		base = "draft-" + model.now().UTC().Format("20060102-150405")
	}
	seen := map[string]struct{}{}
	for _, draft := range model.drafts {
		seen[draft.ID] = struct{}{}
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

func (model PlanningModel) promoteSelectedTaskDraft() PlanningModel {
	draft, ok := model.selectedDraft()
	if !ok {
		model.message = "Create a task draft before promoting."
		return model
	}
	if draft.Status == "promoted" {
		model.message = "Task draft already promoted."
		return model
	}
	slug := taskSlugFromTitle(draft.Title)
	if slug == "" {
		model.message = "Task draft title cannot produce a task slug."
		return model
	}
	store, err := state.NewStore(model.repoRoot)
	if err != nil {
		model.storeError = err
		model.message = "Could not open task store: " + err.Error()
		return model
	}
	createdAt := firstNonEmptyString(draft.CreatedAt, model.now().UTC().Format(time.RFC3339))
	now := model.now().UTC().Format(time.RFC3339)
	task := state.Task{
		Slug:            slug,
		ID:              slug,
		Description:     draft.Description,
		Status:          "draft",
		NormalizedState: "draft",
		CreatedAt:       createdAt,
		UpdatedAt:       now,
	}
	if _, err := state.CreateTask(store, task, state.TaskCreateOptions{}); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			model.message = "Task already exists for draft: " + slug
		} else {
			model.storeError = err
			model.message = "Could not promote task draft: " + err.Error()
		}
		return model
	}
	for index := range model.drafts {
		if model.drafts[index].ID == draft.ID {
			model.drafts[index].Status = "promoted"
			model.drafts[index].TaskSlug = slug
			model.drafts[index].PromotedAt = now
			break
		}
	}
	if err := model.draftStore.Save(model.drafts); err != nil {
		model.storeError = err
		model.message = "Task created, but draft status could not be saved: " + err.Error()
		return model
	}
	model.message = "Task draft promoted: " + slug
	return model
}

func taskSlugFromTitle(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	var output strings.Builder
	lastDash := false
	for _, r := range title {
		switch {
		case r >= 'a' && r <= 'z':
			output.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			output.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && output.Len() > 0 {
				output.WriteRune('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(output.String(), "-")
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
	model.renderTaskDrafts(&output)

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
	output.WriteString(model.wrapped("  ", "This workspace may update .brevity/ideas.json, .brevity/task-drafts.json, and .brevity/tasks.json only. It does not mutate queue, execution, provider, or run state."))
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
		"[t] Create Task Draft",
		"[edit] placeholder",
	} {
		output.WriteString(model.line("  " + step))
	}
}

func (model PlanningModel) renderTaskDrafts(output *strings.Builder) {
	renderSection(output, "Task Drafts")
	if len(model.drafts) == 0 {
		output.WriteString(model.wrapped("  ", "No task drafts yet. Select an idea and press t to create a structured draft without touching execution state."))
		return
	}
	for index, draft := range model.drafts {
		output.WriteString(model.line(fmt.Sprintf("  %d. %s", index+1, draft.Title)))
		output.WriteString(model.line("     status: " + draft.Status))
	}
	if draft, ok := model.selectedDraft(); ok {
		output.WriteString("\n")
		renderSection(output, "Selected Draft")
		output.WriteString(model.line("  Title:   " + draft.Title))
		output.WriteString(model.line("  Status:  " + draft.Status))
		if draft.TaskSlug != "" {
			output.WriteString(model.line("  Task:    " + draft.TaskSlug))
		}
		output.WriteString(model.line("  Created: " + draft.CreatedAt))
		if draft.PromotedAt != "" {
			output.WriteString(model.line("  Promoted:" + " " + draft.PromotedAt))
		}
		output.WriteString("\n")
		renderSection(output, "Acceptance Criteria")
		output.WriteString(model.wrapped("  ", draft.AcceptanceCriteria))
		output.WriteString("\n")
		renderSection(output, "Validation")
		output.WriteString(model.wrapped("  ", draft.Validation))
		output.WriteString("\n")
		renderSection(output, "Next Steps")
		steps := []string{"[refine] placeholder", "[p] Promote To Task", "[archive] placeholder"}
		if draft.Status == "promoted" {
			steps = []string{"activate task", "execute task", "review task"}
		}
		for _, step := range steps {
			output.WriteString(model.line("  " + step))
		}
	}
}

func (model PlanningModel) selectedDraft() (PlanningTaskDraft, bool) {
	if len(model.drafts) == 0 {
		return PlanningTaskDraft{}, false
	}
	if len(model.ideas) > 0 && model.selected >= 0 && model.selected < len(model.ideas) {
		ideaID := model.ideas[model.selected].ID
		for _, draft := range model.drafts {
			if draft.IdeaID == ideaID {
				return draft, true
			}
		}
	}
	return model.drafts[0], true
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
	help := "  n new idea  enter inspect  d delete  t task draft  p promote  q quit"
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

func newPlanningTaskDraftStore(repoRoot string) planningTaskDraftStore {
	if strings.TrimSpace(repoRoot) == "" {
		repoRoot = "."
	}
	return planningTaskDraftStore{path: filepath.Join(repoRoot, planningTaskDraftsPath)}
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

func (store planningTaskDraftStore) Load() ([]PlanningTaskDraft, error) {
	data, err := os.ReadFile(store.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var drafts []PlanningTaskDraft
	if err := json.Unmarshal(data, &drafts); err != nil {
		return nil, err
	}
	sort.SliceStable(drafts, func(i, j int) bool {
		return drafts[i].CreatedAt < drafts[j].CreatedAt
	})
	return drafts, nil
}

func (store planningTaskDraftStore) Save(drafts []PlanningTaskDraft) error {
	if err := os.MkdirAll(filepath.Dir(store.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(drafts, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(store.path, data, 0o644)
}
