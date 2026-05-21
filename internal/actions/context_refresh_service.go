package actions

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/preflight"
	"github.com/mortenlein/brevity/internal/state"
	"github.com/mortenlein/brevity/internal/state/locking"
)

const taskContextRefreshCommand = "task refresh-context"

var managedContextFiles = []string{
	"project.md",
	"architecture.md",
	"decisions.md",
	"current-state.md",
	"roadmap.md",
}

type TaskContextRefreshService struct {
	Store       state.Store
	Now         func() time.Time
	LockOptions locking.Options
}

type brevityConfig struct {
	VaultPath string `json:"vaultPath"`
}

type promptContext struct {
	Slug            string
	State           string
	SpecPath        string
	SpecContents    string
	ContextFiles    []string
	Provider        string
	Profile         string
	PromptPath      string
	WorktreePath    string
	MissingContext  []string
	VaultConfigured bool
	VaultPath       string
}

func (service TaskContextRefreshService) Refresh(slug string) (contracts.CommandResult, error) {
	slug = strings.TrimSpace(slug)
	preflightResult, err := preflight.Run(preflight.Options{
		RepoRoot: service.Store.RepoRoot,
		Action:   preflight.ActionTaskStart,
		Slug:     slug,
		Now:      service.now,
	})
	if err != nil {
		return contextRefreshError("preflight-error", err.Error(), map[string]any{"slug": slug})
	}
	if preflightResult.Status == preflight.StatusBlocked {
		return contextRefreshError("preflight-blocked", "task context refresh preflight blocked mutation", map[string]any{
			"slug":     slug,
			"blockers": preflightResult.Blockers,
		})
	}

	config := service.readConfig()
	generatedAt := service.now().UTC().Format(time.RFC3339Nano)
	var payload contracts.TaskContextRefreshPayload
	update, err := state.UpdateTask(service.Store, slug, state.TaskUpdateOptions{LockOptions: service.LockOptions}, func(rawTask map[string]json.RawMessage) error {
		task, parseErr := rawTaskToStartTask(rawTask)
		if parseErr != nil {
			return parseErr
		}
		worktreePath := strings.TrimSpace(task.WorktreePath)
		promptPath := strings.TrimSpace(task.PromptPath)
		if worktreePath == "" {
			return fmt.Errorf("task metadata is missing worktreePath for: %s", slug)
		}
		if promptPath == "" {
			return fmt.Errorf("task metadata is missing promptPath for: %s", slug)
		}
		if _, statErr := os.Stat(worktreePath); statErr != nil {
			return fmt.Errorf("task worktree unavailable: %w", statErr)
		}

		contextPath := filepath.Join(worktreePath, state.DirectoryName, "context")
		materialized, missing, copyErr := materializeContext(contextPath, config.VaultPath)
		if copyErr != nil {
			return copyErr
		}
		specPath, specContents, specErr := loadTaskSpec(config.VaultPath, task.SpecPath, slug)
		if specErr != nil {
			return specErr
		}
		prompt := renderTaskPrompt(promptContext{
			Slug:            slug,
			State:           firstNonEmpty(task.NormalizedState, task.Status),
			SpecPath:        specPath,
			SpecContents:    specContents,
			ContextFiles:    materialized,
			Provider:        task.Provider,
			Profile:         task.Profile,
			PromptPath:      promptPath,
			WorktreePath:    worktreePath,
			MissingContext:  missing,
			VaultConfigured: strings.TrimSpace(config.VaultPath) != "",
			VaultPath:       config.VaultPath,
		})
		if err := writeTextFile(promptPath, prompt); err != nil {
			return err
		}

		rawTask["promptRefreshedAt"] = rawString(generatedAt)
		rawTask["promptRefreshStatus"] = rawString("fresh")
		rawTask["updatedAt"] = rawString(generatedAt)
		if specPath != "" {
			rawTask["specPath"] = rawString(specPath)
		}
		payload = contracts.TaskContextRefreshPayload{
			Slug:                slug,
			Refreshed:           true,
			ContextPath:         contextPath,
			PromptPath:          promptPath,
			SpecPath:            specPath,
			GeneratedAt:         generatedAt,
			NormalizedState:     firstNonEmpty(task.NormalizedState, task.Status),
			MaterializedFiles:   materialized,
			MissingFiles:        missing,
			PromptRefreshStatus: "fresh",
			NoProviderExecution: true,
			NoWorkerExecution:   true,
		}
		return nil
	})
	if err != nil {
		code := "task-context-refresh-failed"
		if strings.Contains(err.Error(), "locked") || strings.Contains(err.Error(), "lock timeout") {
			code = "task-metadata-locked"
		} else if strings.Contains(err.Error(), "not found") {
			code = "task-not-found"
		}
		return contextRefreshError(code, err.Error(), map[string]any{"slug": slug, "lockPath": service.Store.LockPath()})
	}
	if payload.NormalizedState == "" {
		payload.NormalizedState = firstNonEmpty(update.Updated.NormalizedState, update.Updated.Status)
	}
	return contextRefreshSuccess(payload), nil
}

func (service TaskContextRefreshService) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func (service TaskContextRefreshService) readConfig() brevityConfig {
	var config brevityConfig
	data, err := os.ReadFile(service.Store.Path("config.json"))
	if err != nil {
		return config
	}
	_ = json.Unmarshal(data, &config)
	return config
}

func materializeContext(contextPath string, vaultPath string) ([]string, []string, error) {
	if err := os.MkdirAll(contextPath, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create context directory: %w", err)
	}
	materialized := []string{}
	missing := []string{}
	for _, name := range managedContextFiles {
		source := ""
		if strings.TrimSpace(vaultPath) != "" {
			source = filepath.Join(vaultPath, name)
		}
		target := filepath.Join(contextPath, name)
		data, err := os.ReadFile(source)
		if err != nil {
			if source == "" || errors.Is(err, os.ErrNotExist) {
				missing = append(missing, name)
				_ = os.Remove(target)
				continue
			}
			return nil, nil, fmt.Errorf("read vault context %s: %w", name, err)
		}
		if err := writeBytesFile(target, data); err != nil {
			return nil, nil, err
		}
		materialized = append(materialized, name)
	}
	sort.Strings(materialized)
	sort.Strings(missing)
	return materialized, missing, nil
}

func loadTaskSpec(vaultPath string, recordedSpecPath string, slug string) (string, string, error) {
	candidates := []string{}
	if strings.TrimSpace(recordedSpecPath) != "" {
		candidates = append(candidates, recordedSpecPath)
	}
	if strings.TrimSpace(vaultPath) != "" {
		candidates = append(candidates, filepath.Join(vaultPath, "tasks", slug+".md"))
	}
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err == nil {
			return candidate, strings.TrimSpace(string(data)), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("read task spec: %w", err)
		}
	}
	return "", "", nil
}

func renderTaskPrompt(context promptContext) string {
	lines := []string{
		"Read AGENTS.md first.",
		"",
		"You are a bounded implementation worker in this Brevity task worktree.",
		"",
		"# Task",
		"",
		"Slug: " + context.Slug,
	}
	if context.State != "" {
		lines = append(lines, "State: "+context.State)
	}
	lines = append(lines, "", "# Task Spec", "")
	if context.SpecContents == "" {
		lines = append(lines,
			"No vault task spec was materialized for this task.",
			"Use the task slug and repository instructions only. Do not invent unrelated scope.",
		)
	} else {
		if context.SpecPath != "" {
			lines = append(lines, "Source: "+context.SpecPath, "")
		}
		lines = append(lines, context.SpecContents)
	}
	lines = append(lines,
		"",
		"# Local Context",
		"",
		"Brevity materializes selected durable project memory into this worktree before worker execution.",
		"Read local context files from `.brevity\\context\\` when they exist.",
		"Do not read external vault paths directly; the vault is durable memory, and this worktree is the bounded execution context.",
	)
	if len(context.ContextFiles) > 0 {
		lines = append(lines, "", "Materialized context files:")
		for _, name := range context.ContextFiles {
			lines = append(lines, "- .brevity\\context\\"+name)
		}
	}
	if len(context.MissingContext) > 0 {
		lines = append(lines, "", "Missing optional context files:")
		for _, name := range context.MissingContext {
			lines = append(lines, "- "+name)
		}
	}
	lines = append(lines,
		"",
		"# Runtime Metadata",
		"",
		"Prompt path: "+context.PromptPath,
		"Worktree path: "+context.WorktreePath,
	)
	if context.Provider != "" {
		lines = append(lines, "Provider hint: "+context.Provider)
	}
	if context.Profile != "" {
		lines = append(lines, "Profile hint: "+context.Profile)
	}
	lines = append(lines,
		"",
		"# Constraints",
		"",
		"- Keep changes small and focused on this task.",
		"- Stay inside this task worktree.",
		"- Do not merge branches.",
		"- Do not clean up or remove worktrees.",
		"- Do not launch providers or workers from this prompt materialization step.",
		"- Do not add package managers, dependencies, generated projects, or web apps unless the task explicitly requires it.",
		"- Prefer straightforward PowerShell and existing repository patterns.",
		"",
		"# Acceptance Checks",
		"",
		"- The requested behavior is implemented.",
		"- Relevant local checks have been run, or any checks that could not be run are called out.",
		"- The final summary names changed files and verification performed.",
		"",
		"# Worker Behavior",
		"",
		"- Inspect only the context needed to complete the task.",
		"- Make the patch directly.",
		"- Preserve unrelated user or repository changes.",
		"- Stop after patch and concise summary.",
	)
	return strings.Join(lines, "\n") + "\n"
}

func writeTextFile(path string, contents string) error {
	return writeBytesFile(path, []byte(contents))
}

func writeBytesFile(path string, contents []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if _, err := temp.Write(contents); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	return nil
}

func contextRefreshSuccess(payload contracts.TaskContextRefreshPayload) contracts.CommandResult {
	raw, _ := json.Marshal(payload)
	return contracts.CommandResult{
		Schema:               contracts.CommandResultSchema,
		Command:              taskContextRefreshCommand,
		Success:              true,
		Severity:             "info",
		Warnings:             []contracts.ResultMessage{},
		Errors:               []contracts.ResultMessage{},
		SuggestedNextActions: []string{"refresh-runtime-state", "review-prompt-artifact"},
		Payload:              raw,
	}
}

func contextRefreshError(code string, message string, details map[string]any) (contracts.CommandResult, error) {
	raw, _ := json.Marshal(contracts.TaskContextRefreshPayload{PromptRefreshStatus: "failed", NoProviderExecution: true, NoWorkerExecution: true})
	result := contracts.CommandResult{
		Schema:   contracts.CommandResultSchema,
		Command:  taskContextRefreshCommand,
		Success:  false,
		Severity: "error",
		Warnings: []contracts.ResultMessage{},
		Errors: []contracts.ResultMessage{{
			Code:    code,
			Message: message,
			Details: details,
		}},
		SuggestedNextActions: []string{"Inspect task metadata, worktree path, and vault configuration."},
		Payload:              raw,
	}
	return result, errors.New(message)
}

func rawString(value string) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
