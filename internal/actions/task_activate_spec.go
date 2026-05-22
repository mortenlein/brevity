package actions

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/state"
	"github.com/mortenlein/brevity/internal/state/locking"
)

const taskActivateCommand = "task activate"
const taskSpecCommand = "task spec"

var taskSlugPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type TaskActivateService struct {
	Store       state.Store
	Now         func() time.Time
	LockOptions locking.Options
}

type TaskSpecService struct {
	Store state.Store
}

func (service TaskActivateService) Activate(slug string) (contracts.CommandResult, error) {
	slug = strings.TrimSpace(slug)
	if !taskSlugPattern.MatchString(slug) {
		return taskActivateError("invalid-slug", "slug must contain only letters, numbers, dot, underscore, or dash and start with a letter or number", map[string]any{"slug": slug})
	}
	config, missing, err := state.LoadConfig(service.Store)
	if err != nil {
		return taskActivateError("config-read-failed", err.Error(), nil)
	}
	if missing {
		return taskActivateError("config-missing", "Brevity config not found. Run brevity init first.", map[string]any{"path": service.Store.Path(state.ConfigFile)})
	}
	specPath := filepath.Join(config.VaultPath, "tasks", slug+".md")
	specContents, err := os.ReadFile(specPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return taskActivateError("spec-not-found", "Vault task spec not found: "+slug, map[string]any{"slug": slug, "expectedPath": specPath})
		}
		return taskActivateError("spec-read-failed", err.Error(), map[string]any{"slug": slug, "specPath": specPath})
	}

	repoRoot := service.Store.RepoRoot
	worktreePath := filepath.Join(config.WorktreesRoot, config.ProjectName+"-"+slug)
	branch := "task/" + slug
	promptPath := filepath.Join(worktreePath, "prompt.md")
	metadataPath := service.Store.Path(state.TasksFile)

	if tasks, _, err := state.LoadTasks(service.Store); err == nil {
		for _, task := range tasks.Items {
			if task.Key() == slug && strings.TrimSpace(task.WorktreePath) != "" && pathExists(task.WorktreePath) {
				payload := contracts.TaskActivatePayload{Slug: slug, Branch: firstNonEmpty(task.Branch, branch), WorktreePath: task.WorktreePath, PromptPath: firstNonEmpty(task.PromptPath, promptPath), SpecPath: firstNonEmpty(task.SpecPath, specPath), ContextPath: filepath.Join(task.WorktreePath, state.DirectoryName, "context"), MetadataPath: metadataPath, NoProviderExecution: true, NoWorkerExecution: true}
				return taskActivateSuccess(payload, []contracts.ResultMessage{{Code: "already-active", Message: "Task already has an active worktree."}}), nil
			}
		}
	}
	if pathExists(worktreePath) {
		return taskActivateError("worktree-already-exists", "Task worktree already exists: "+worktreePath, map[string]any{"slug": slug, "worktreePath": worktreePath})
	}
	if exists, err := gitBranchExists(repoRoot, branch); err != nil {
		return taskActivateError("branch-check-failed", err.Error(), map[string]any{"branch": branch})
	} else if exists {
		return taskActivateError("branch-already-exists", "Task branch already exists: "+branch, map[string]any{"slug": slug, "branch": branch})
	}
	if err := os.MkdirAll(config.WorktreesRoot, 0o755); err != nil {
		return taskActivateError("worktrees-root-create-failed", err.Error(), map[string]any{"worktreesRoot": config.WorktreesRoot})
	}
	if err := runGitWorktreeAdd(repoRoot, worktreePath, branch); err != nil {
		return taskActivateError("git-worktree-add-failed", err.Error(), map[string]any{"slug": slug, "branch": branch, "worktreePath": worktreePath})
	}
	contextPath := filepath.Join(worktreePath, state.DirectoryName, "context")
	contextFiles, missingContext, err := materializeContext(contextPath, config.VaultPath)
	if err != nil {
		return taskActivateError("context-materialize-failed", err.Error(), map[string]any{"contextPath": contextPath})
	}
	createdAt := service.now().UTC().Format(time.RFC3339Nano)
	prompt := renderTaskPrompt(promptContext{Slug: slug, State: taskNewInitialState, SpecPath: specPath, SpecContents: strings.TrimSpace(string(specContents)), ContextFiles: contextFiles, PromptPath: promptPath, WorktreePath: worktreePath, MissingContext: missingContext, VaultConfigured: strings.TrimSpace(config.VaultPath) != "", VaultPath: config.VaultPath})
	if err := writeTextFile(promptPath, prompt); err != nil {
		return taskActivateError("prompt-write-failed", err.Error(), map[string]any{"promptPath": promptPath})
	}
	task := state.Task{Slug: slug, Branch: branch, WorktreePath: worktreePath, PromptPath: promptPath, SpecPath: specPath, Status: taskNewInitialState, NormalizedState: taskNewInitialState, CreatedAt: createdAt, UpdatedAt: createdAt, PromptRefreshedAt: createdAt, PromptRefreshStatus: "fresh"}
	if _, err := state.CreateTask(service.Store, task, state.TaskCreateOptions{LockOptions: service.LockOptions}); err != nil {
		return taskActivateError("task-metadata-create-failed", err.Error(), map[string]any{"slug": slug, "metadataPath": metadataPath, "lockPath": service.Store.LockPath()})
	}
	return taskActivateSuccess(contracts.TaskActivatePayload{Slug: slug, Branch: branch, WorktreePath: worktreePath, PromptPath: promptPath, SpecPath: specPath, ContextPath: contextPath, MetadataPath: metadataPath, CreatedAt: createdAt, NoProviderExecution: true, NoWorkerExecution: true}, nil), nil
}

func (service TaskSpecService) Show(slug string) (contracts.CommandResult, error) {
	slug = strings.TrimSpace(slug)
	if !taskSlugPattern.MatchString(slug) {
		return taskSpecError("invalid-slug", "slug must contain only letters, numbers, dot, underscore, or dash and start with a letter or number", map[string]any{"slug": slug})
	}
	config, missing, err := state.LoadConfig(service.Store)
	if err != nil {
		return taskSpecError("config-read-failed", err.Error(), nil)
	}
	if missing {
		return taskSpecError("config-missing", "Brevity config not found. Run brevity init first.", map[string]any{"path": service.Store.Path(state.ConfigFile)})
	}
	specPath := filepath.Join(config.VaultPath, "tasks", slug+".md")
	data, err := os.ReadFile(specPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return taskSpecError("spec-not-found", "Vault task spec not found: "+slug, map[string]any{"slug": slug, "expectedPath": specPath})
		}
		return taskSpecError("spec-read-failed", err.Error(), map[string]any{"slug": slug, "specPath": specPath})
	}
	payload := contracts.TaskSpecPayload{Slug: slug, SpecPath: specPath, SpecExists: true, Content: string(data), NoMutation: true, NoProviderExecution: true, NoWorkerExecution: true}
	if tasks, _, err := state.LoadTasks(service.Store); err == nil {
		for _, task := range tasks.Items {
			if task.Key() == slug {
				payload.TaskExists = true
				payload.PromptPath = task.PromptPath
				payload.WorktreePath = task.WorktreePath
				payload.PromptExists = pathExists(task.PromptPath)
				break
			}
		}
	}
	raw, _ := json.Marshal(payload)
	return contracts.CommandResult{Schema: contracts.CommandResultSchema, Command: taskSpecCommand, Success: true, Severity: "info", Warnings: []contracts.ResultMessage{}, Errors: []contracts.ResultMessage{}, SuggestedNextActions: []string{"review-task-spec"}, Payload: raw}, nil
}

func (service TaskActivateService) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func gitBranchExists(repoRoot string, branch string) (bool, error) {
	command := exec.Command("git", "-C", repoRoot, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	err := command.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

func taskActivateSuccess(payload contracts.TaskActivatePayload, warnings []contracts.ResultMessage) contracts.CommandResult {
	if warnings == nil {
		warnings = []contracts.ResultMessage{}
	}
	raw, _ := json.Marshal(payload)
	return contracts.CommandResult{Schema: contracts.CommandResultSchema, Command: taskActivateCommand, Success: true, Severity: "info", Warnings: warnings, Errors: []contracts.ResultMessage{}, SuggestedNextActions: []string{"refresh-runtime-state", "review-prompt-artifact"}, Payload: raw}
}

func taskActivateError(code string, message string, details map[string]any) (contracts.CommandResult, error) {
	raw, _ := json.Marshal(contracts.TaskActivatePayload{NoProviderExecution: true, NoWorkerExecution: true})
	result := contracts.CommandResult{Schema: contracts.CommandResultSchema, Command: taskActivateCommand, Success: false, Severity: "error", Warnings: []contracts.ResultMessage{}, Errors: []contracts.ResultMessage{{Code: code, Message: message, Details: details}}, SuggestedNextActions: []string{"Inspect task spec, branch, worktree, and metadata state."}, Payload: raw}
	return result, errors.New(message)
}

func taskSpecError(code string, message string, details map[string]any) (contracts.CommandResult, error) {
	raw, _ := json.Marshal(contracts.TaskSpecPayload{NoMutation: true, NoProviderExecution: true, NoWorkerExecution: true})
	result := contracts.CommandResult{Schema: contracts.CommandResultSchema, Command: taskSpecCommand, Success: false, Severity: "error", Warnings: []contracts.ResultMessage{}, Errors: []contracts.ResultMessage{{Code: code, Message: message, Details: details}}, SuggestedNextActions: []string{"Create the vault task spec or run brevity init --repair."}, Payload: raw}
	return result, errors.New(message)
}

func RenderTaskActivateResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseTaskActivatePayload(result)
	if err != nil {
		return err
	}
	renderStatusLine(stdout, "Task activate", result.Success)
	fmt.Fprintf(stdout, "slug: %s\nbranch: %s\nworktreePath: %s\npromptPath: %s\nspecPath: %s\nmetadataPath: %s\n", payload.Slug, payload.Branch, payload.WorktreePath, payload.PromptPath, payload.SpecPath, payload.MetadataPath)
	renderMessages(stdout, result)
	return nil
}

func RenderTaskSpecResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseTaskSpecPayload(result)
	if err != nil {
		return err
	}
	renderStatusLine(stdout, "Task spec", result.Success)
	fmt.Fprintf(stdout, "task: %s\nspec: %s\n", payload.Slug, payload.SpecPath)
	if payload.PromptPath != "" {
		fmt.Fprintf(stdout, "promptPath: %s\n", payload.PromptPath)
	}
	fmt.Fprintln(stdout)
	fmt.Fprint(stdout, payload.Content)
	if !strings.HasSuffix(payload.Content, "\n") {
		fmt.Fprintln(stdout)
	}
	renderMessages(stdout, result)
	return nil
}
