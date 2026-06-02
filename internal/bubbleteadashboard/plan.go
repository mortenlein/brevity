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

	"github.com/mortenlein/brevity/internal/actions"
	"github.com/mortenlein/brevity/internal/contracts"
	runtimeexecution "github.com/mortenlein/brevity/internal/runtime/execution"
	runtimequeue "github.com/mortenlein/brevity/internal/runtime/queue"
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

type executionHandoff struct {
	Slug         string
	Activated    bool
	QueueState   string
	QueueItem    *runtimequeue.Item
	Execution    *runtimeexecution.Record
	Review       *reviewHandoff
	NextStep     string
	Why          string
	Commands     []string
	Actions      []workflowAction
	FutureAction string
}

type reviewHandoff struct {
	Status   string
	NextStep string
	Why      string
	Commands []string
	Actions  []workflowAction
}

type workflowAction struct {
	Key   string
	Label string
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
	tasks       []state.Task
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
	model.reloadPlanningTasks()
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
			if strings.ToLower(msg.String()) == "q" && model.workflowActionAvailable("q") {
				model = model.queueSelectedTask()
				return model, nil
			}
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
		case "a":
			model = model.activateSelectedPromotedTask()
		case "r":
			if model.workflowActionAvailable("r") {
				model = model.runRWorkflowAction()
			}
		case "e":
			if model.workflowActionAvailable("e") {
				model = model.planSelectedTaskExecution()
			}
		case "v":
			if model.workflowActionAvailable("v") {
				model = model.viewSelectedExecution()
			}
		case "w":
			if model.workflowActionAvailable("w") {
				model = model.openReviewWorkspace()
			}
		case "b":
			if model.workflowActionAvailable("b") {
				model.message = "Returned to workflow handoff."
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
	model.reloadPlanningTasks()
	return model
}

func (model *PlanningModel) reloadPlanningTasks() {
	store, err := state.NewStore(model.repoRoot)
	if err != nil {
		return
	}
	tasks, _, err := state.LoadTasks(store)
	if err != nil {
		return
	}
	model.tasks = tasks.Items
}

func (model PlanningModel) activateSelectedPromotedTask() PlanningModel {
	draft, ok := model.selectedDraft()
	if !ok {
		model.message = "Create and promote a task draft before activation."
		return model
	}
	if draft.Status != "promoted" {
		model.message = "Promote the selected task draft before activation."
		return model
	}
	slug := strings.TrimSpace(draft.TaskSlug)
	if slug == "" {
		model.message = "Promoted draft is missing a task slug."
		return model
	}
	if active, _ := model.taskActivation(slug); active {
		model.message = "Task already activated: " + slug
		return model
	}
	store, err := state.NewStore(model.repoRoot)
	if err != nil {
		model.storeError = err
		model.message = "Could not open task store: " + err.Error()
		return model
	}
	if err := model.ensurePromotedTaskSpec(store, draft); err != nil {
		model.storeError = err
		model.message = "Could not prepare task spec: " + err.Error()
		return model
	}
	result, err := actions.TaskActivateService{Store: store, Now: model.now}.Activate(slug)
	if err != nil && !result.Success {
		model.storeError = err
		model.message = "Could not activate task: " + err.Error()
		return model
	}
	payload, parseErr := contracts.ParseTaskActivatePayload(result)
	if parseErr != nil {
		model.storeError = parseErr
		model.message = "Task activated, but activation result could not be read."
	} else {
		model.message = "Task activated: " + payload.Slug
	}
	model.reloadPlanningTasks()
	return model
}

func (model PlanningModel) ensurePromotedTaskSpec(store state.Store, draft PlanningTaskDraft) error {
	config, missing, err := state.LoadConfig(store)
	if err != nil {
		return err
	}
	if missing {
		return fmt.Errorf("Brevity config not found. Run brevity init first")
	}
	slug := strings.TrimSpace(draft.TaskSlug)
	specPath := filepath.Join(config.VaultPath, "tasks", slug+".md")
	if _, err := os.Stat(specPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(specPath), 0o755); err != nil {
		return err
	}
	var spec strings.Builder
	spec.WriteString("# " + draft.Title + "\n\n")
	spec.WriteString("## Description\n\n" + strings.TrimSpace(draft.Description) + "\n\n")
	spec.WriteString("## Acceptance Criteria\n\n" + strings.TrimSpace(draft.AcceptanceCriteria) + "\n\n")
	spec.WriteString("## Validation\n\n" + strings.TrimSpace(draft.Validation) + "\n")
	return os.WriteFile(specPath, []byte(spec.String()), 0o644)
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
		statusSegment{text: "idea -> plan -> task -> execute", compact: "workflow", priority: 0},
		statusSegment{text: "safe actions", compact: "actions", priority: 1},
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
		"active  Materialize the task worktree without queue or execution mutation.",
		"execute Queue and run the task after activation.",
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
		"brevity task activate <slug>",
		"brevity task start <slug>",
		"brevity task status",
		"brevity review",
		"brevity task merge <slug>",
	} {
		output.WriteString(model.line("  " + command))
	}

	output.WriteString("\n")
	renderSection(&output, "Mutation Boundary")
	output.WriteString(model.wrapped("  ", "This workspace may update planning state, task state, activation worktree artifacts, queue items, reservations, and execution-plan records only. It does not launch providers, workers, supervisors, merges, cleanup, retries, or execution runs."))
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
		active, activeTask := model.taskActivation(draft.TaskSlug)
		if active {
			output.WriteString(model.line("  Active:  yes"))
			output.WriteString(model.line("  Branch:  " + activeTask.Branch))
			output.WriteString(model.line("  Worktree:" + " " + activeTask.WorktreePath))
			output.WriteString(model.line("  Prompt:  " + activeTask.PromptPath))
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
			steps = []string{"[a] Activate Task"}
			if active {
				steps = []string{"queue task", "execute task", "review task"}
			}
		}
		for _, step := range steps {
			output.WriteString(model.line("  " + step))
		}
		if draft.Status == "promoted" {
			model.renderSelectedDraftWorkflow(output, draft, active)
			if active {
				model.renderExecutionHandoff(output, draft.TaskSlug, active)
				model.renderReviewHandoff(output, draft.TaskSlug, active)
			}
		}
	}
}

func (model PlanningModel) renderSelectedDraftWorkflow(output *strings.Builder, draft PlanningTaskDraft, active bool) {
	output.WriteString("\n")
	renderSection(output, "Workflow Progress")
	handoff := model.executionHandoff(draft.TaskSlug, active)
	steps := workflowProgressSteps(draft, handoff)
	for _, step := range steps {
		mark := "-"
		if step.done {
			mark = "v"
		} else if step.current {
			mark = ">"
		}
		output.WriteString(model.line("  " + mark + " " + step.label))
	}
}

func (model PlanningModel) renderExecutionHandoff(output *strings.Builder, slug string, active bool) {
	handoff := model.executionHandoff(slug, active)
	output.WriteString("\n")
	renderSection(output, "Execution Handoff")
	output.WriteString(model.line("  Task:"))
	output.WriteString(model.line("  " + handoff.Slug))
	output.WriteString("\n")
	output.WriteString(model.line("  State:"))
	if handoff.Activated {
		output.WriteString(model.line("  Task activated"))
	} else {
		output.WriteString(model.line("  Task not activated"))
	}
	output.WriteString(model.line("  Queue: " + handoffQueueSummary(handoff)))
	output.WriteString(model.line("  Reservation: " + handoffReservationSummary(handoff)))
	output.WriteString(model.line("  Execution: " + handoffExecutionSummary(handoff)))
	output.WriteString("\n")
	output.WriteString(model.line("  Recommended Next Step:"))
	output.WriteString(model.line("  " + handoff.NextStep))
	output.WriteString("\n")
	output.WriteString(model.line("  Why:"))
	output.WriteString(model.wrapped("  ", handoff.Why))
	output.WriteString("\n")
	output.WriteString(model.line("  Commands:"))
	for _, command := range handoff.Commands {
		output.WriteString(model.line("  " + command))
	}
	if len(handoff.Actions) > 0 {
		output.WriteString("\n")
		output.WriteString(model.line("  Actions:"))
		for _, action := range handoff.Actions {
			output.WriteString(model.line("  [" + action.Key + "] " + action.Label))
		}
	} else if handoff.FutureAction != "" {
		output.WriteString("\n")
		output.WriteString(model.line("  Future action:"))
		output.WriteString(model.line("  " + handoff.FutureAction))
	}
}

func (model PlanningModel) renderReviewHandoff(output *strings.Builder, slug string, active bool) {
	handoff := model.executionHandoff(slug, active)
	if handoff.Review == nil {
		return
	}
	review := handoff.Review
	output.WriteString("\n")
	renderSection(output, "Review Handoff")
	output.WriteString(model.line("  Status:"))
	output.WriteString(model.line("  " + review.Status))
	output.WriteString("\n")
	output.WriteString(model.line("  Recommended Next Step:"))
	output.WriteString(model.line("  " + review.NextStep))
	output.WriteString("\n")
	output.WriteString(model.line("  Why:"))
	output.WriteString(model.wrapped("  ", review.Why))
	output.WriteString("\n")
	output.WriteString(model.line("  Commands:"))
	for _, command := range review.Commands {
		output.WriteString(model.line("  " + command))
	}
	output.WriteString("\n")
	output.WriteString(model.line("  Actions:"))
	for _, action := range review.Actions {
		output.WriteString(model.line("  [" + action.Key + "] " + action.Label))
	}
}

func (model PlanningModel) executionHandoff(slug string, active bool) executionHandoff {
	slug = strings.TrimSpace(slug)
	handoff := executionHandoff{
		Slug:      slug,
		Activated: active,
		NextStep:  "Inspect task activation",
		Why:       "The selected draft does not have an activated task yet.",
		Commands:  []string{"brevity task status"},
	}
	if slug == "" {
		handoff.Slug = "(missing)"
		return handoff
	}
	if !active {
		handoff.Commands = []string{"brevity task activate " + slug, "brevity task status"}
		return handoff
	}

	queueStore, err := runtimequeue.NewStore(model.repoRoot)
	if err != nil {
		handoff.QueueState = "unavailable: " + err.Error()
		handoff.NextStep = "Inspect queue state"
		handoff.Why = "The task is activated, but queue state could not be inspected."
		handoff.Commands = []string{"brevity queue inspect"}
		return handoff
	}
	queueState, missing, err := queueStore.Load()
	if err != nil {
		handoff.QueueState = "invalid: " + err.Error()
		handoff.NextStep = "Inspect queue state"
		handoff.Why = "The task is activated, but runtime queue state is not readable."
		handoff.Commands = []string{"brevity queue inspect"}
		return handoff
	}
	if missing {
		handoff.QueueState = "missing"
	} else {
		handoff.QueueState = "valid"
	}
	handoff.QueueItem = newestQueueItemForTask(queueState.Items, slug)

	executionStore, err := runtimeexecution.NewStore(model.repoRoot)
	if err == nil {
		executions, _, loadErr := executionStore.Load()
		if loadErr == nil {
			handoff.Execution = newestExecutionForTask(executions.Records, slug, handoff.QueueItem)
		}
	}

	return finalizeExecutionHandoff(handoff)
}

func finalizeExecutionHandoff(handoff executionHandoff) executionHandoff {
	if handoff.Execution != nil {
		id := strings.TrimSpace(handoff.Execution.ID)
		switch strings.ToLower(strings.TrimSpace(handoff.Execution.Status)) {
		case runtimeexecution.StatusPlanned:
			handoff.NextStep = "Mark execution ready"
			handoff.Why = "The task has a planned execution record that is not ready for preflight or launch yet."
			handoff.Commands = []string{"brevity execution mark-ready " + id, "brevity execution flow"}
			handoff.Actions = []workflowAction{{Key: "v", Label: "View Execution"}, {Key: "b", Label: "Back"}}
		case runtimeexecution.StatusReady:
			handoff.NextStep = "Preflight and dry-run launch"
			handoff.Why = "The execution is ready; validate it before any real provider launch."
			handoff.Commands = []string{"brevity execution preflight " + id, "brevity execution launch-dry-run " + id, "brevity execution launch " + id}
			handoff.Actions = []workflowAction{{Key: "v", Label: "View Execution"}, {Key: "b", Label: "Back"}}
		case runtimeexecution.StatusLaunching:
			handoff.NextStep = "Inspect active execution"
			handoff.Why = "The execution is launching, so review should wait until the provider run finishes or fails."
			handoff.Commands = []string{"brevity execution inspect", "brevity execution flow"}
			handoff.Actions = []workflowAction{{Key: "v", Label: "View Execution"}, {Key: "b", Label: "Back"}}
		case runtimeexecution.StatusCompleted:
			handoff.NextStep = "Review generated work"
			handoff.Why = "Execution completed successfully and is ready for operator review."
			handoff.Commands = []string{"brevity review " + handoff.Slug, "brevity cmux --review " + handoff.Slug, "brevity task status"}
			handoff.Actions = []workflowAction{{Key: "w", Label: "Open Review Workspace"}, {Key: "v", Label: "View Execution"}, {Key: "b", Label: "Back"}}
			handoff.Review = &reviewHandoff{
				Status:   "Execution completed",
				NextStep: "Review generated work",
				Why:      "Execution completed successfully and is ready for operator review.",
				Commands: []string{"brevity review " + handoff.Slug, "brevity cmux --review " + handoff.Slug},
				Actions:  []workflowAction{{Key: "w", Label: "Open Review Workspace"}, {Key: "v", Label: "View Execution"}, {Key: "b", Label: "Back"}},
			}
		case runtimeexecution.StatusFailed:
			handoff.NextStep = "Inspect execution failure"
			handoff.Why = "Execution failed; inspect the failure details before deciding whether any later retry or manual review is appropriate."
			handoff.Commands = []string{"brevity execution inspect", "brevity review " + handoff.Slug, "brevity execution flow"}
			handoff.Actions = []workflowAction{{Key: "w", Label: "Open Review Workspace"}, {Key: "v", Label: "View Execution"}, {Key: "r", Label: "Review Failure Details"}, {Key: "b", Label: "Back"}}
			handoff.Review = &reviewHandoff{
				Status:   "Execution failed",
				NextStep: "Inspect execution failure",
				Why:      "Execution failed; inspect failure details before retry or merge decisions.",
				Commands: []string{"brevity review " + handoff.Slug, "brevity execution inspect"},
				Actions:  []workflowAction{{Key: "w", Label: "Open Review Workspace"}, {Key: "v", Label: "View Execution"}, {Key: "r", Label: "Review Failure Details"}},
			}
		default:
			handoff.NextStep = "Inspect execution state"
			handoff.Why = "The task has an execution record in a state that needs operator inspection."
			handoff.Commands = []string{"brevity execution inspect", "brevity execution flow"}
			handoff.Actions = []workflowAction{{Key: "v", Label: "View Execution"}, {Key: "b", Label: "Back"}}
		}
		return handoff
	}
	if handoff.QueueItem != nil && handoff.QueueItem.Reservation != nil {
		handoff.NextStep = "Plan execution from reservation"
		handoff.Why = "The task has a reserved queue item but no execution record yet."
		handoff.Commands = []string{"brevity scheduler plan-execution", "brevity execution list"}
		handoff.Actions = []workflowAction{{Key: "e", Label: "Create Execution Plan"}}
		return handoff
	}
	if handoff.QueueItem != nil {
		handoff.NextStep = "Plan and reserve queued task"
		handoff.Why = "The task is queued but not reserved, so the scheduler can select or reserve it next."
		handoff.Commands = []string{"brevity scheduler plan", "brevity scheduler reserve-next"}
		handoff.Actions = []workflowAction{{Key: "r", Label: "Reserve Task"}}
		return handoff
	}
	handoff.NextStep = "Queue this task"
	handoff.Why = "The task is activated but has no queue item yet."
	handoff.Commands = []string{"brevity queue add " + handoff.Slug, "brevity queue inspect", "brevity scheduler plan"}
	handoff.Actions = []workflowAction{{Key: "q", Label: "Queue Task"}}
	return handoff
}

type workflowProgressStep struct {
	label   string
	done    bool
	current bool
}

func workflowProgressSteps(draft PlanningTaskDraft, handoff executionHandoff) []workflowProgressStep {
	executionPlanned := handoff.Execution != nil
	executionCompleted := handoff.Execution != nil && strings.EqualFold(strings.TrimSpace(handoff.Execution.Status), runtimeexecution.StatusCompleted)
	executionFailed := handoff.Execution != nil && strings.EqualFold(strings.TrimSpace(handoff.Execution.Status), runtimeexecution.StatusFailed)
	queued := handoff.QueueItem != nil
	reserved := queued && handoff.QueueItem.Reservation != nil
	steps := []workflowProgressStep{
		{label: "Idea Captured", done: strings.TrimSpace(draft.IdeaID) != ""},
		{label: "Draft Created", done: strings.TrimSpace(draft.ID) != ""},
		{label: "Task Created", done: strings.TrimSpace(draft.TaskSlug) != ""},
		{label: "Task Activated", done: handoff.Activated},
		{label: "Task Queued", done: queued},
		{label: "Task Reserved", done: reserved},
		{label: "Execution Planned", done: executionPlanned},
	}
	if executionCompleted {
		steps = append(steps, workflowProgressStep{label: "Execution Completed", done: true})
	} else if executionFailed {
		steps = append(steps, workflowProgressStep{label: "Execution Failed", done: true})
	}
	for index := range steps {
		if !steps[index].done {
			steps[index].current = true
			break
		}
	}
	if len(steps) > 0 && steps[len(steps)-1].done {
		steps[len(steps)-1].current = true
	}
	return steps
}

func (model PlanningModel) workflowActionAvailable(key string) bool {
	draft, active, ok := model.selectedPromotedActivation()
	if !ok || !active {
		return false
	}
	handoff := model.executionHandoff(draft.TaskSlug, active)
	for _, action := range handoff.Actions {
		if strings.EqualFold(action.Key, key) {
			return true
		}
	}
	return false
}

func (model PlanningModel) selectedPromotedActivation() (PlanningTaskDraft, bool, bool) {
	draft, ok := model.selectedDraft()
	if !ok || draft.Status != "promoted" {
		return PlanningTaskDraft{}, false, false
	}
	active, _ := model.taskActivation(draft.TaskSlug)
	return draft, active, true
}

func (model PlanningModel) queueSelectedTask() PlanningModel {
	draft, active, ok := model.selectedPromotedActivation()
	if !ok || !active {
		model.message = "No activated task is available to queue."
		return model
	}
	slug := strings.TrimSpace(draft.TaskSlug)
	store, err := runtimequeue.NewStore(model.repoRoot)
	if err != nil {
		model.message = "Could not open queue: " + err.Error()
		return model
	}
	item, err := store.Add(slug)
	if err != nil {
		model.message = "Could not queue task: " + err.Error()
		return model
	}
	model.message = "Queued task: " + slug + " (" + item.ID + ")"
	return model
}

func (model PlanningModel) reserveSelectedTask() PlanningModel {
	draft, active, ok := model.selectedPromotedActivation()
	if !ok || !active {
		model.message = "No queued task is available to reserve."
		return model
	}
	handoff := model.executionHandoff(draft.TaskSlug, active)
	if handoff.QueueItem == nil {
		model.message = "Queue the task before reserving it."
		return model
	}
	store, err := runtimequeue.NewStore(model.repoRoot)
	if err != nil {
		model.message = "Could not open queue: " + err.Error()
		return model
	}
	item, err := store.Reserve(handoff.QueueItem.ID)
	if err != nil {
		model.message = "Could not reserve task: " + err.Error()
		return model
	}
	reservationID := ""
	if item.Reservation != nil {
		reservationID = item.Reservation.ReservationID
	}
	model.message = "Reserved task: " + item.Task + " (" + reservationID + ")"
	return model
}

func (model PlanningModel) runRWorkflowAction() PlanningModel {
	draft, active, ok := model.selectedPromotedActivation()
	if !ok || !active {
		model.message = "No workflow action is available."
		return model
	}
	handoff := model.executionHandoff(draft.TaskSlug, active)
	for _, action := range handoff.Actions {
		if !strings.EqualFold(action.Key, "r") {
			continue
		}
		if strings.EqualFold(action.Label, "Review Failure Details") {
			return model.reviewFailureDetails()
		}
		return model.reserveSelectedTask()
	}
	model.message = "No workflow action is available."
	return model
}

func (model PlanningModel) reviewFailureDetails() PlanningModel {
	draft, active, ok := model.selectedPromotedActivation()
	if !ok || !active {
		model.message = "No failed execution is available to review."
		return model
	}
	handoff := model.executionHandoff(draft.TaskSlug, active)
	if handoff.Execution == nil || !strings.EqualFold(handoff.Execution.Status, runtimeexecution.StatusFailed) {
		model.message = "No failed execution is available to review."
		return model
	}
	model.message = "Review failure details for execution " + handoff.Execution.ID + "."
	return model
}

func (model PlanningModel) planSelectedTaskExecution() PlanningModel {
	draft, active, ok := model.selectedPromotedActivation()
	if !ok || !active {
		model.message = "No reserved task is available for execution planning."
		return model
	}
	handoff := model.executionHandoff(draft.TaskSlug, active)
	if handoff.QueueItem == nil || handoff.QueueItem.Reservation == nil {
		model.message = "Reserve the task before creating an execution plan."
		return model
	}
	store, err := runtimeexecution.NewStore(model.repoRoot)
	if err != nil {
		model.message = "Could not open execution store: " + err.Error()
		return model
	}
	record, err := store.PlanFromReservation(handoff.QueueItem.ID)
	if err != nil {
		model.message = "Could not create execution plan: " + err.Error()
		return model
	}
	model.message = "Execution planned: " + record.ID + " for " + record.Task
	return model
}

func (model PlanningModel) viewSelectedExecution() PlanningModel {
	draft, active, ok := model.selectedPromotedActivation()
	if !ok || !active {
		model.message = "No execution is available to view."
		return model
	}
	handoff := model.executionHandoff(draft.TaskSlug, active)
	if handoff.Execution == nil {
		model.message = "No execution is available to view."
		return model
	}
	model.message = "Execution " + handoff.Execution.ID + " is " + handoff.Execution.Status + "."
	return model
}

func (model PlanningModel) openReviewWorkspace() PlanningModel {
	draft, active, ok := model.selectedPromotedActivation()
	if !ok || !active {
		model.message = "No completed execution is available for review."
		return model
	}
	handoff := model.executionHandoff(draft.TaskSlug, active)
	if handoff.Review == nil || handoff.Execution == nil {
		model.message = "Review workspace opens after execution handoff."
		return model
	}
	model.message = "Open review workspace: brevity review " + strings.TrimSpace(draft.TaskSlug)
	return model
}

func newestQueueItemForTask(items []runtimequeue.Item, slug string) *runtimequeue.Item {
	var selected *runtimequeue.Item
	for index := range items {
		if strings.TrimSpace(items[index].Task) != slug {
			continue
		}
		if selected == nil || strings.TrimSpace(items[index].CreatedAt) >= strings.TrimSpace(selected.CreatedAt) {
			item := items[index]
			selected = &item
		}
	}
	return selected
}

func newestExecutionForTask(records []runtimeexecution.Record, slug string, item *runtimequeue.Item) *runtimeexecution.Record {
	queueItemID := ""
	if item != nil {
		queueItemID = strings.TrimSpace(item.ID)
	}
	var selected *runtimeexecution.Record
	for index := range records {
		record := records[index]
		if strings.TrimSpace(record.Task) != slug {
			continue
		}
		if queueItemID != "" && strings.TrimSpace(record.QueueItemID) != queueItemID {
			continue
		}
		if selected == nil || strings.TrimSpace(record.CreatedAt) >= strings.TrimSpace(selected.CreatedAt) {
			candidate := record
			selected = &candidate
		}
	}
	return selected
}

func handoffQueueSummary(handoff executionHandoff) string {
	if handoff.QueueItem == nil {
		queueState := strings.TrimSpace(handoff.QueueState)
		if queueState != "" && queueState != "valid" && queueState != "missing" {
			return handoff.QueueState
		}
		return "not queued"
	}
	return strings.TrimSpace(handoff.QueueItem.ID) + " (" + strings.TrimSpace(handoff.QueueItem.Status) + ")"
}

func handoffReservationSummary(handoff executionHandoff) string {
	if handoff.QueueItem == nil || handoff.QueueItem.Reservation == nil {
		return "none"
	}
	return strings.TrimSpace(handoff.QueueItem.Reservation.ReservationID) + " by " + strings.TrimSpace(handoff.QueueItem.Reservation.Owner)
}

func handoffExecutionSummary(handoff executionHandoff) string {
	if handoff.Execution == nil {
		return "none"
	}
	return strings.TrimSpace(handoff.Execution.ID) + " (" + strings.TrimSpace(handoff.Execution.Status) + ")"
}

func (model PlanningModel) taskActivation(slug string) (bool, state.Task) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return false, state.Task{}
	}
	for _, task := range model.tasks {
		if task.Key() == slug && strings.TrimSpace(task.WorktreePath) != "" && pathExistsLocal(task.WorktreePath) {
			return true, task
		}
	}
	return false, state.Task{}
}

func pathExistsLocal(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
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
	if draft, active, ok := model.selectedPromotedActivation(); ok && active {
		handoff := model.executionHandoff(draft.TaskSlug, active)
		if len(handoff.Actions) > 0 {
			parts := []string{}
			for _, action := range handoff.Actions {
				parts = append(parts, action.Key+" "+strings.ToLower(action.Label))
			}
			help = "  " + strings.Join(parts, "  ") + "  esc quit"
		}
	}
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
