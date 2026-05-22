package actions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/state"
	"github.com/mortenlein/brevity/internal/state/locking"
)

const taskMergePlanSchema = "brevity.task-merge-plan.v1"

type TaskMergeService struct {
	Store       state.Store
	Now         func() time.Time
	LockOptions locking.Options
	Git         GitRunner
}

type GitRunner interface {
	Run(dir string, args ...string) GitRunResult
}

type GitRunResult struct {
	Args     []string
	Dir      string
	ExitCode int
	Stdout   string
	Stderr   string
	Err      error
}

type ExecGitRunner struct{}

func (ExecGitRunner) Run(dir string, args ...string) GitRunResult {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}
	return GitRunResult{Args: append([]string{}, args...), Dir: dir, ExitCode: exitCode, Stdout: stdout.String(), Stderr: stderr.String(), Err: err}
}

func (service TaskMergeService) Plan(slug string) (contracts.CommandResult, error) {
	payload, err := service.buildPlan(slug)
	if err != nil {
		return taskMergePlanResult(payload, false, "error", []contracts.ResultMessage{{Code: "merge-plan-failed", Message: err.Error()}}, nil), err
	}
	success := len(payload.Blockers) == 0
	severity := "info"
	if !success {
		severity = "error"
	}
	return taskMergePlanResult(payload, success, severity, payload.Blockers, payload.Warnings), nil
}

func (service TaskMergeService) Merge(slug string) (contracts.CommandResult, error) {
	plan, err := service.buildPlan(slug)
	if err != nil {
		result := taskMergeResult(plan, false, []contracts.ResultMessage{{Code: "merge-plan-failed", Message: err.Error()}}, nil, nil)
		return result, err
	}
	if len(plan.Blockers) > 0 {
		result := taskMergeResult(plan, false, plan.Blockers, plan.Warnings, nil)
		return result, fmt.Errorf("task merge blocked")
	}

	git := service.git()
	checkout := git.Run(plan.RepoRoot, "checkout", plan.TargetBranch)
	commands := []contracts.GitCommandResult{toCommandResult(checkout)}
	if checkout.ExitCode != 0 {
		result := taskMergeResultWithCommands(plan, false, []contracts.ResultMessage{{Code: "git-checkout-failed", Message: trimGitMessage(checkout)}}, plan.Warnings, commands, false, "", "")
		return result, fmt.Errorf("git checkout failed")
	}
	merge := git.Run(plan.RepoRoot, "merge", plan.SourceBranch)
	commands = append(commands, toCommandResult(merge))
	if merge.ExitCode != 0 {
		result := taskMergeResultWithCommands(plan, false, []contracts.ResultMessage{{Code: "git-merge-failed", Message: trimGitMessage(merge)}}, plan.Warnings, commands, mergeConflictLikely(merge), "", "")
		return result, fmt.Errorf("git merge failed")
	}

	updatedAt := service.now().UTC().Format(time.RFC3339Nano)
	var previousState, newState string
	update, err := state.UpdateTask(service.Store, plan.Slug, state.TaskUpdateOptions{LockOptions: service.LockOptions}, func(rawTask map[string]json.RawMessage) error {
		var task state.Task
		data, _ := json.Marshal(rawTask)
		if err := json.Unmarshal(data, &task); err != nil {
			return err
		}
		previousState = taskState(task)
		rawTask["status"] = mustRawString("merged")
		rawTask["normalizedState"] = mustRawString("merged")
		rawTask["updatedAt"] = mustRawString(updatedAt)
		return nil
	})
	if err != nil {
		result := taskMergeResultWithCommands(plan, false, []contracts.ResultMessage{{Code: "task-metadata-update-failed", Message: err.Error()}}, plan.Warnings, commands, false, previousState, "")
		return result, err
	}
	newState = taskState(update.Updated)
	return taskMergeResultWithCommands(plan, true, nil, plan.Warnings, commands, false, previousState, newState), nil
}

func (service TaskMergeService) buildPlan(slug string) (contracts.TaskMergePlanPayload, error) {
	slug = strings.TrimSpace(slug)
	now := service.now().UTC().Format(time.RFC3339Nano)
	payload := contracts.TaskMergePlanPayload{
		Schema:                taskMergePlanSchema,
		Version:               1,
		Slug:                  slug,
		RepoRoot:              service.Store.RepoRoot,
		ExpectedStateMutation: "set task status and normalizedState to merged on successful git merge",
		ExpectedGitCommands:   [][]string{{"git", "checkout", "<targetBranch>"}, {"git", "merge", "<sourceBranch>"}},
		Blockers:              []contracts.ResultMessage{},
		Warnings:              []contracts.ResultMessage{},
		Destructive:           false,
		CleanupRequired:       true,
		GeneratedAt:           now,
	}
	if slug == "" {
		payload.Blockers = append(payload.Blockers, contracts.ResultMessage{Code: "missing-slug", Message: "task slug is required"})
		return payload, nil
	}
	tasks, missing, err := state.LoadTasks(service.Store)
	if err != nil {
		payload.Blockers = append(payload.Blockers, contracts.ResultMessage{Code: "tasks-json-invalid", Message: err.Error()})
		return payload, nil
	}
	if missing {
		payload.Blockers = append(payload.Blockers, contracts.ResultMessage{Code: "tasks-json-missing", Message: ".brevity/tasks.json is missing"})
		return payload, nil
	}
	task, ok := findMergeTask(tasks, slug)
	if !ok {
		payload.Blockers = append(payload.Blockers, contracts.ResultMessage{Code: "task-not-found", Message: "task not found: " + slug})
		return payload, nil
	}
	payload.SourceBranch = firstNonEmpty(task.Branch, nestedWorktreeBranch(task.Worktree))
	payload.WorktreePath = firstNonEmpty(task.WorktreePath, nestedWorktreePath(task.Worktree))
	if payload.SourceBranch == "" {
		payload.Blockers = append(payload.Blockers, contracts.ResultMessage{Code: "missing-source-branch", Message: "task metadata is missing branch"})
	}
	if payload.WorktreePath == "" {
		payload.Blockers = append(payload.Blockers, contracts.ResultMessage{Code: "missing-worktree", Message: "task metadata is missing worktree path"})
	} else if _, err := os.Stat(payload.WorktreePath); err != nil {
		payload.Blockers = append(payload.Blockers, contracts.ResultMessage{Code: "missing-worktree", Message: "task worktree is missing"})
	}
	git := service.git()
	if current := git.Run(payload.RepoRoot, "branch", "--show-current"); current.ExitCode == 0 {
		payload.TargetBranch = strings.TrimSpace(current.Stdout)
	}
	if payload.TargetBranch == "" {
		payload.TargetBranch = "main"
		payload.Warnings = append(payload.Warnings, contracts.ResultMessage{Code: "target-branch-defaulted", Message: "could not inspect current branch; defaulting target branch to main"})
	}
	payload.ExpectedGitCommands = [][]string{{"git", "checkout", payload.TargetBranch}, {"git", "merge", payload.SourceBranch}}
	if payload.SourceBranch != "" && git.Run(payload.RepoRoot, "rev-parse", "--verify", payload.SourceBranch).ExitCode != 0 {
		payload.Blockers = append(payload.Blockers, contracts.ResultMessage{Code: "missing-source-branch", Message: "source branch does not exist: " + payload.SourceBranch})
	}
	if payload.TargetBranch != "" && git.Run(payload.RepoRoot, "rev-parse", "--verify", payload.TargetBranch).ExitCode != 0 {
		payload.Blockers = append(payload.Blockers, contracts.ResultMessage{Code: "missing-target-branch", Message: "target branch does not exist: " + payload.TargetBranch})
	}
	if payload.WorktreePath != "" {
		if _, statErr := os.Stat(payload.WorktreePath); statErr != nil {
			return payload, nil
		}
		dirty := git.Run(payload.WorktreePath, "status", "--porcelain")
		payload.Dirty = strings.TrimSpace(dirty.Stdout) != ""
		if dirty.ExitCode != 0 {
			payload.Blockers = append(payload.Blockers, contracts.ResultMessage{Code: "dirty-check-failed", Message: trimGitMessage(dirty)})
		} else if payload.Dirty {
			payload.Blockers = append(payload.Blockers, contracts.ResultMessage{Code: "dirty-worktree", Message: "task worktree has uncommitted changes"})
		}
	}
	if payload.SourceBranch != "" && payload.TargetBranch != "" {
		if ahead := git.Run(payload.RepoRoot, "rev-list", "--left-right", "--count", payload.TargetBranch+"..."+payload.SourceBranch); ahead.ExitCode == 0 {
			payload.AheadBehind = strings.TrimSpace(ahead.Stdout)
		}
		if base := git.Run(payload.RepoRoot, "merge-base", payload.TargetBranch, payload.SourceBranch); base.ExitCode != 0 {
			payload.Warnings = append(payload.Warnings, contracts.ResultMessage{Code: "merge-base-unavailable", Message: "could not compute merge base; conflict risk is unknown"})
		}
	}
	return payload, nil
}

func RenderTaskMergePlanResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseTaskMergePlanPayload(result)
	if err != nil {
		return err
	}
	renderStatusLine(stdout, "Task merge plan", result.Success)
	fmt.Fprintf(stdout, "slug: %s\nsourceBranch: %s\ntargetBranch: %s\nworktreePath: %s\ndirty: %t\n", payload.Slug, payload.SourceBranch, payload.TargetBranch, payload.WorktreePath, payload.Dirty)
	fmt.Fprintf(stdout, "destructive: %t\ncleanupRequiredAfterMerge: %t\n", payload.Destructive, payload.CleanupRequired)
	for _, cmd := range payload.ExpectedGitCommands {
		fmt.Fprintf(stdout, "gitCommand: %s\n", strings.Join(cmd, " "))
	}
	renderMessages(stdout, result)
	return nil
}

func RenderTaskMergeResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseTaskMergePayload(result)
	if err != nil {
		return err
	}
	renderStatusLine(stdout, "Task merge", result.Success)
	fmt.Fprintf(stdout, "slug: %s\nsourceBranch: %s\ntargetBranch: %s\nmetadataUpdated: %t\n", payload.Slug, payload.SourceBranch, payload.TargetBranch, payload.MetadataUpdated)
	fmt.Fprintf(stdout, "cleanupExecuted: %t\nbranchRemoved: %t\nworktreeRemoved: %t\n", payload.CleanupExecuted, payload.BranchRemoved, payload.WorktreeRemoved)
	if payload.ConflictDetected {
		fmt.Fprintln(stdout, "conflictDetected: true")
	}
	renderMessages(stdout, result)
	return nil
}

func taskMergePlanResult(payload contracts.TaskMergePlanPayload, success bool, severity string, errors []contracts.ResultMessage, warnings []contracts.ResultMessage) contracts.CommandResult {
	raw, _ := json.Marshal(payload)
	return contracts.CommandResult{Schema: contracts.CommandResultSchema, Command: "task merge --plan", Success: success, Severity: severity, Warnings: emptyMessages(warnings), Errors: emptyMessages(errors), SuggestedNextActions: mergeSuggested(success), Payload: raw}
}

func taskMergeResult(plan contracts.TaskMergePlanPayload, success bool, errors []contracts.ResultMessage, warnings []contracts.ResultMessage, commands []contracts.GitCommandResult) contracts.CommandResult {
	return taskMergeResultWithCommands(plan, success, errors, warnings, commands, false, "", "")
}

func taskMergeResultWithCommands(plan contracts.TaskMergePlanPayload, success bool, errors []contracts.ResultMessage, warnings []contracts.ResultMessage, commands []contracts.GitCommandResult, conflict bool, previousState string, newState string) contracts.CommandResult {
	payload := contracts.TaskMergePayload{Slug: plan.Slug, SourceBranch: plan.SourceBranch, TargetBranch: plan.TargetBranch, PreviousState: previousState, NewState: newState, UpdatedAt: plan.GeneratedAt, Plan: plan, GitCommands: commands, MetadataUpdated: success, BranchRemoved: false, WorktreeRemoved: false, CleanupExecuted: false, ConflictDetected: conflict}
	raw, _ := json.Marshal(payload)
	severity := "info"
	if !success {
		severity = "error"
	}
	return contracts.CommandResult{Schema: contracts.CommandResultSchema, Command: "task merge", Success: success, Severity: severity, Warnings: emptyMessages(warnings), Errors: emptyMessages(errors), SuggestedNextActions: mergeSuggested(success), Payload: raw}
}

func (service TaskMergeService) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func (service TaskMergeService) git() GitRunner {
	if service.Git != nil {
		return service.Git
	}
	return ExecGitRunner{}
}

func findMergeTask(tasks state.Tasks, slug string) (state.Task, bool) {
	for _, task := range tasks.Items {
		if task.Key() == slug {
			return task, true
		}
	}
	return state.Task{}, false
}

func nestedWorktreePath(worktree *state.TaskWorktree) string {
	if worktree == nil {
		return ""
	}
	return worktree.Path
}

func nestedWorktreeBranch(worktree *state.TaskWorktree) string {
	if worktree == nil {
		return ""
	}
	return worktree.Branch
}

func toCommandResult(result GitRunResult) contracts.GitCommandResult {
	return contracts.GitCommandResult{Command: "git", Args: result.Args, Dir: result.Dir, ExitCode: result.ExitCode, Stdout: strings.TrimSpace(result.Stdout), Stderr: strings.TrimSpace(result.Stderr)}
}

func trimGitMessage(result GitRunResult) string {
	message := strings.TrimSpace(result.Stderr)
	if message == "" {
		message = strings.TrimSpace(result.Stdout)
	}
	if message == "" && result.Err != nil {
		message = result.Err.Error()
	}
	return message
}

func mergeConflictLikely(result GitRunResult) bool {
	text := strings.ToLower(result.Stdout + "\n" + result.Stderr)
	return strings.Contains(text, "conflict") || strings.Contains(text, "automatic merge failed")
}

func emptyMessages(messages []contracts.ResultMessage) []contracts.ResultMessage {
	if messages == nil {
		return []contracts.ResultMessage{}
	}
	return messages
}

func mergeSuggested(success bool) []string {
	if success {
		return []string{"refresh-runtime-state", "run cleanup explicitly when ready"}
	}
	return []string{"inspect merge plan", "resolve git or task metadata blockers"}
}
