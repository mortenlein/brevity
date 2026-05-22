package actions

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/preflight"
	"github.com/mortenlein/brevity/internal/state"
	"github.com/mortenlein/brevity/internal/state/locking"
)

const taskNewCommand = "task new"
const taskNewInitialState = "ready-for-worker"

type TaskNewService struct {
	Store       state.Store
	Now         func() time.Time
	LockOptions locking.Options
}

type taskNewConfig struct {
	VaultPath     string `json:"vaultPath"`
	WorktreesRoot string `json:"worktreesRoot"`
}

func (service TaskNewService) Create(slug string) (contracts.CommandResult, error) {
	slug = strings.TrimSpace(slug)
	preflightResult, err := preflight.Run(preflight.Options{
		RepoRoot: service.Store.RepoRoot,
		Action:   preflight.ActionTaskNew,
		Slug:     slug,
		Now:      service.now,
	})
	if err != nil {
		return taskNewError("preflight-error", err.Error(), nil)
	}
	if preflightResult.Status == preflight.StatusBlocked {
		return taskNewError("preflight-blocked", "task new preflight blocked mutation", map[string]any{
			"slug":     slug,
			"blockers": preflightResult.Blockers,
		})
	}

	config := service.readConfig()
	repoName := filepath.Base(service.Store.RepoRoot)
	worktreesRoot := strings.TrimSpace(config.WorktreesRoot)
	if worktreesRoot == "" {
		worktreesRoot = filepath.Join(filepath.Dir(service.Store.RepoRoot), "worktrees", "active")
	}
	worktreePath := filepath.Join(worktreesRoot, repoName+"-"+slug)
	branch := "task/" + slug
	promptPath := filepath.Join(worktreePath, "prompt.md")
	metadataPath := service.Store.Path(state.TasksFile)

	if _, err := os.Stat(worktreePath); err == nil {
		return taskNewError("worktree-already-exists", "Task worktree already exists: "+worktreePath, map[string]any{
			"slug": slug, "worktreePath": worktreePath,
		})
	} else if !errors.Is(err, os.ErrNotExist) {
		return taskNewError("worktree-inspection-failed", err.Error(), map[string]any{"slug": slug, "worktreePath": worktreePath})
	}
	if err := os.MkdirAll(worktreesRoot, 0o755); err != nil {
		return taskNewError("worktrees-root-create-failed", err.Error(), map[string]any{"slug": slug, "worktreesRoot": worktreesRoot})
	}
	if err := runGitWorktreeAdd(service.Store.RepoRoot, worktreePath, branch); err != nil {
		return taskNewError("git-worktree-add-failed", err.Error(), map[string]any{
			"slug": slug, "branch": branch, "worktreePath": worktreePath,
		})
	}

	contextPath := filepath.Join(worktreePath, state.DirectoryName, "context")
	contextFiles, missingContext, err := materializeContext(contextPath, config.VaultPath)
	if err != nil {
		return taskNewError("context-materialize-failed", err.Error(), map[string]any{"slug": slug, "contextPath": contextPath})
	}
	specPath, specContents, err := loadTaskSpec(config.VaultPath, "", slug)
	if err != nil {
		return taskNewError("spec-read-failed", err.Error(), map[string]any{"slug": slug})
	}
	createdAt := service.now().UTC().Format(time.RFC3339Nano)
	if err := writeTextFile(promptPath, renderTaskPrompt(promptContext{
		Slug:            slug,
		State:           taskNewInitialState,
		SpecPath:        specPath,
		SpecContents:    specContents,
		ContextFiles:    contextFiles,
		PromptPath:      promptPath,
		WorktreePath:    worktreePath,
		MissingContext:  missingContext,
		VaultConfigured: strings.TrimSpace(config.VaultPath) != "",
		VaultPath:       config.VaultPath,
	})); err != nil {
		return taskNewError("prompt-write-failed", err.Error(), map[string]any{"slug": slug, "promptPath": promptPath})
	}

	task := state.Task{
		Slug:            slug,
		Branch:          branch,
		WorktreePath:    worktreePath,
		PromptPath:      promptPath,
		SpecPath:        specPath,
		Status:          taskNewInitialState,
		NormalizedState: taskNewInitialState,
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}
	if _, err := state.CreateTask(service.Store, task, state.TaskCreateOptions{LockOptions: service.LockOptions}); err != nil {
		code := "task-new-failed"
		if strings.Contains(err.Error(), "locked") || strings.Contains(err.Error(), "lock timeout") {
			code = "task-metadata-locked"
		} else if strings.Contains(err.Error(), "already exists") {
			code = "task-already-exists"
		} else if strings.Contains(err.Error(), "parse tasks.json") || strings.Contains(err.Error(), "file is empty") {
			code = "task-metadata-invalid"
		}
		return taskNewError(code, err.Error(), map[string]any{"slug": slug, "metadataPath": metadataPath, "lockPath": service.Store.LockPath()})
	}

	return taskNewSuccess(contracts.TaskNewPayload{
		Slug:                slug,
		State:               taskNewInitialState,
		Branch:              branch,
		WorktreePath:        worktreePath,
		PromptPath:          promptPath,
		SpecPath:            specPath,
		MetadataPath:        metadataPath,
		CreatedAt:           createdAt,
		NoProviderExecution: true,
		NoWorkerExecution:   true,
	}), nil
}

func (service TaskNewService) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func (service TaskNewService) readConfig() taskNewConfig {
	var config taskNewConfig
	data, err := os.ReadFile(service.Store.Path("config.json"))
	if err != nil {
		return config
	}
	_ = json.Unmarshal(data, &config)
	return config
}

func runGitWorktreeAdd(repoRoot string, worktreePath string, branch string) error {
	command := exec.Command("git", "-C", repoRoot, "worktree", "add", worktreePath, "-b", branch)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func RenderTaskNewResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseTaskNewPayload(result)
	if err != nil {
		return err
	}
	applyTaskNewErrorDetails(&payload, result)

	renderStatusLine(stdout, "Task new", result.Success)
	fmt.Fprintf(stdout, "slug: %s\n", payload.Slug)
	if payload.State != "" {
		fmt.Fprintf(stdout, "state: %s\n", payload.State)
	}
	fmt.Fprintf(stdout, "branch: %s\n", payload.Branch)
	fmt.Fprintf(stdout, "worktreePath: %s\n", payload.WorktreePath)
	fmt.Fprintf(stdout, "promptPath: %s\n", payload.PromptPath)
	if payload.SpecPath != "" {
		fmt.Fprintf(stdout, "specPath: %s\n", payload.SpecPath)
	}
	fmt.Fprintf(stdout, "metadataPath: %s\n", payload.MetadataPath)
	fmt.Fprintf(stdout, "providerExecution: false\n")
	fmt.Fprintf(stdout, "workerExecution: false\n")

	renderMessages(stdout, result)
	return nil
}

func taskNewSuccess(payload contracts.TaskNewPayload) contracts.CommandResult {
	raw, _ := json.Marshal(payload)
	return contracts.CommandResult{
		Schema:               contracts.CommandResultSchema,
		Command:              taskNewCommand,
		Success:              true,
		Severity:             "info",
		Warnings:             []contracts.ResultMessage{},
		Errors:               []contracts.ResultMessage{},
		SuggestedNextActions: []string{"refresh-runtime-state", "review-prompt-artifact"},
		Payload:              raw,
	}
}

func taskNewError(code string, message string, details map[string]any) (contracts.CommandResult, error) {
	raw, _ := json.Marshal(contracts.TaskNewPayload{NoProviderExecution: true, NoWorkerExecution: true})
	result := contracts.CommandResult{
		Schema:   contracts.CommandResultSchema,
		Command:  taskNewCommand,
		Success:  false,
		Severity: "error",
		Warnings: []contracts.ResultMessage{},
		Errors: []contracts.ResultMessage{{
			Code:    code,
			Message: message,
			Details: details,
		}},
		SuggestedNextActions: []string{"Inspect native task new preflight output."},
		Payload:              raw,
	}
	return result, errors.New(message)
}

func applyTaskNewErrorDetails(payload *contracts.TaskNewPayload, result contracts.CommandResult) {
	for _, commandError := range result.Errors {
		if commandError.Details == nil {
			continue
		}
		if payload.Slug == "" {
			payload.Slug = detailString(commandError.Details, "slug")
		}
		if payload.Branch == "" {
			payload.Branch = detailString(commandError.Details, "branch")
		}
		if payload.WorktreePath == "" {
			payload.WorktreePath = detailString(commandError.Details, "worktreePath")
		}
		if payload.PromptPath == "" {
			payload.PromptPath = detailString(commandError.Details, "promptPath")
		}
		if payload.MetadataPath == "" {
			payload.MetadataPath = detailString(commandError.Details, "metadataPath")
		}
	}
}

func detailString(details map[string]any, key string) string {
	value, ok := details[key]
	if !ok || value == nil {
		return ""
	}

	text, ok := value.(string)
	if !ok {
		return ""
	}

	return text
}
