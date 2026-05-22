package actions

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/state"
	"github.com/mortenlein/brevity/internal/state/locking"
)

const taskCleanupPlanSchema = "brevity.task-cleanup-plan.v1"

type TaskCleanupService struct {
	Store       state.Store
	Now         func() time.Time
	LockOptions locking.Options
	Git         GitRunner
}

func (service TaskCleanupService) Plan(slug string) (contracts.CommandResult, error) {
	plan, err := service.buildPlan(slug)
	if err != nil {
		return taskCleanupPlanResult(plan, false, "error", []contracts.ResultMessage{{Code: "cleanup-plan-failed", Message: err.Error()}}, nil), err
	}
	success := len(plan.Blockers) == 0
	severity := "info"
	if !success {
		severity = "error"
	}
	return taskCleanupPlanResult(plan, success, severity, plan.Blockers, plan.Warnings), nil
}

func (service TaskCleanupService) Cleanup(slug string, force bool) (contracts.CommandResult, error) {
	plan, err := service.buildPlan(slug)
	if err != nil {
		result := taskCleanupResult(plan, force, false, []contracts.ResultMessage{{Code: "cleanup-plan-failed", Message: err.Error()}}, nil, nil)
		return result, err
	}
	if !force {
		errs := append([]contracts.ResultMessage{}, plan.Blockers...)
		errs = append(errs, contracts.ResultMessage{Code: "force-required", Message: "task cleanup requires --force"})
		result := taskCleanupResult(plan, force, false, errs, nil, nil)
		return result, fmt.Errorf("task cleanup requires --force")
	}
	if len(plan.Blockers) > 0 {
		result := taskCleanupResult(plan, force, false, plan.Blockers, plan.Warnings, nil)
		return result, fmt.Errorf("task cleanup blocked")
	}

	git := service.git()
	commands := []contracts.GitCommandResult{}
	worktreeRemoved := false
	if plan.WorktreeExists && plan.WorktreeRegistered {
		remove := git.Run(plan.RepoRoot, "worktree", "remove", plan.WorktreePath)
		commands = append(commands, toCommandResult(remove))
		if remove.ExitCode != 0 {
			result := taskCleanupResult(plan, force, false, []contracts.ResultMessage{{Code: "git-worktree-remove-failed", Message: trimGitMessage(remove)}}, plan.Warnings, commands)
			return result, fmt.Errorf("git worktree remove failed")
		}
		_, statErr := os.Stat(plan.WorktreePath)
		worktreeRemoved = os.IsNotExist(statErr)
	}

	branchRemoved := false
	if plan.BranchExists {
		removeBranch := git.Run(plan.RepoRoot, "branch", "-d", plan.Branch)
		commands = append(commands, toCommandResult(removeBranch))
		if removeBranch.ExitCode != 0 {
			result := taskCleanupResultWithState(plan, force, false, branchRemoved, worktreeRemoved, []contracts.ResultMessage{{Code: "git-branch-delete-failed", Message: trimGitMessage(removeBranch)}}, plan.Warnings, commands)
			return result, fmt.Errorf("git branch delete failed")
		}
		branchRemoved = git.Run(plan.RepoRoot, "show-ref", "--verify", "--quiet", "refs/heads/"+plan.Branch).ExitCode != 0
	}

	if _, err := state.RemoveTask(service.Store, plan.Slug, state.TaskUpdateOptions{LockOptions: service.LockOptions}); err != nil {
		result := taskCleanupResultWithState(plan, force, false, branchRemoved, worktreeRemoved, []contracts.ResultMessage{{Code: "task-metadata-remove-failed", Message: err.Error()}}, plan.Warnings, commands)
		return result, err
	}
	return taskCleanupResultWithState(plan, force, true, branchRemoved, worktreeRemoved, nil, plan.Warnings, commands), nil
}

func (service TaskCleanupService) buildPlan(slug string) (contracts.TaskCleanupPlanPayload, error) {
	slug = strings.TrimSpace(slug)
	plan := contracts.TaskCleanupPlanPayload{
		Schema:                   taskCleanupPlanSchema,
		Version:                  1,
		Slug:                     slug,
		RepoRoot:                 service.Store.RepoRoot,
		Destructive:              true,
		RequiresForce:            true,
		ExpectedMetadataMutation: "remove selected task metadata record after successful git cleanup",
		Blockers:                 []contracts.ResultMessage{},
		Warnings:                 []contracts.ResultMessage{},
		GeneratedAt:              service.now().UTC().Format(time.RFC3339Nano),
	}
	if slug == "" {
		plan.Blockers = append(plan.Blockers, contracts.ResultMessage{Code: "missing-slug", Message: "task slug is required"})
		return plan, nil
	}
	tasks, missing, err := state.LoadTasks(service.Store)
	if err != nil {
		plan.Blockers = append(plan.Blockers, contracts.ResultMessage{Code: "tasks-json-invalid", Message: err.Error()})
		return plan, nil
	}
	if missing {
		plan.Blockers = append(plan.Blockers, contracts.ResultMessage{Code: "tasks-json-missing", Message: ".brevity/tasks.json is missing"})
		return plan, nil
	}
	task, ok := findMergeTask(tasks, slug)
	if !ok {
		plan.Blockers = append(plan.Blockers, contracts.ResultMessage{Code: "task-not-found", Message: "task not found: " + slug})
		return plan, nil
	}
	plan.Branch = firstNonEmpty(task.Branch, nestedWorktreeBranch(task.Worktree))
	plan.WorktreePath = firstNonEmpty(task.WorktreePath, nestedWorktreePath(task.Worktree))
	if plan.Branch == "" {
		plan.Blockers = append(plan.Blockers, contracts.ResultMessage{Code: "missing-branch", Message: "task metadata is missing branch"})
	}
	if plan.WorktreePath == "" {
		plan.Blockers = append(plan.Blockers, contracts.ResultMessage{Code: "missing-worktree-path", Message: "task metadata is missing worktree path"})
	}
	git := service.git()
	if plan.WorktreePath != "" {
		if _, err := os.Stat(plan.WorktreePath); err == nil {
			plan.WorktreeExists = true
			dirty := git.Run(plan.WorktreePath, "status", "--porcelain")
			plan.Dirty = strings.TrimSpace(dirty.Stdout) != ""
			if dirty.ExitCode != 0 {
				plan.Blockers = append(plan.Blockers, contracts.ResultMessage{Code: "dirty-check-failed", Message: trimGitMessage(dirty)})
			} else if plan.Dirty {
				plan.Blockers = append(plan.Blockers, contracts.ResultMessage{Code: "dirty-worktree", Message: "task worktree has uncommitted changes"})
			}
		} else {
			plan.Warnings = append(plan.Warnings, contracts.ResultMessage{Code: "missing-worktree", Message: "recorded task worktree is missing"})
		}
	}
	if plan.WorktreePath != "" {
		plan.WorktreeRegistered = worktreeRegistered(git, plan.RepoRoot, plan.WorktreePath)
		if plan.WorktreeExists && !plan.WorktreeRegistered {
			plan.Warnings = append(plan.Warnings, contracts.ResultMessage{Code: "unregistered-worktree", Message: "recorded task worktree is not registered with Git"})
		}
	}
	if plan.Branch != "" {
		plan.BranchExists = git.Run(plan.RepoRoot, "show-ref", "--verify", "--quiet", "refs/heads/"+plan.Branch).ExitCode == 0
		if !plan.BranchExists {
			plan.Warnings = append(plan.Warnings, contracts.ResultMessage{Code: "missing-branch", Message: "recorded task branch is already missing"})
		} else {
			merged := git.Run(plan.RepoRoot, "branch", "--merged", "HEAD", "--list", plan.Branch)
			if merged.ExitCode == 0 {
				plan.BranchMergedKnown = true
				plan.BranchMerged = strings.TrimSpace(merged.Stdout) != ""
			}
			if plan.BranchMergedKnown && !plan.BranchMerged {
				plan.Blockers = append(plan.Blockers, contracts.ResultMessage{Code: "unmerged-branch", Message: "task branch is not merged into current HEAD"})
			} else if !plan.BranchMergedKnown {
				plan.Blockers = append(plan.Blockers, contracts.ResultMessage{Code: "branch-merge-state-unknown", Message: "could not determine whether task branch is merged"})
			}
		}
	}
	plan.ExpectedGitCommands = [][]string{}
	if plan.WorktreePath != "" && plan.WorktreeExists && plan.WorktreeRegistered {
		plan.ExpectedGitCommands = append(plan.ExpectedGitCommands, []string{"git", "worktree", "remove", plan.WorktreePath})
	}
	if plan.Branch != "" && plan.BranchExists {
		plan.ExpectedGitCommands = append(plan.ExpectedGitCommands, []string{"git", "branch", "-d", plan.Branch})
	}
	plan.Removable = len(plan.Blockers) == 0
	return plan, nil
}

func RenderTaskCleanupResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseTaskCleanupPayload(result)
	if err != nil {
		return err
	}

	renderStatusLine(stdout, "Task cleanup", result.Success)
	fmt.Fprintf(stdout, "slug: %s\n", payload.Slug)
	fmt.Fprintf(stdout, "worktreeRemoved: %t\n", payload.WorktreeRemoved)
	fmt.Fprintf(stdout, "branchRemoved: %t\n", payload.BranchRemoved)
	fmt.Fprintf(stdout, "metadataRemoved: %t\n", payload.MetadataRemoved)

	for _, warning := range payload.CleanupWarnings {
		fmt.Fprintf(stdout, "cleanupWarning: %s\n", warning)
	}

	renderMessages(stdout, result)
	return nil
}

func RenderTaskCleanupPlanResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseTaskCleanupPlanPayload(result)
	if err != nil {
		return err
	}
	renderStatusLine(stdout, "Task cleanup plan", result.Success)
	fmt.Fprintf(stdout, "slug: %s\nworktreePath: %s\nbranch: %s\n", payload.Slug, payload.WorktreePath, payload.Branch)
	fmt.Fprintf(stdout, "dirty: %t\nbranchMerged: %t\nremovable: %t\ndestructive: %t\nrequiresForce: %t\n", payload.Dirty, payload.BranchMerged, payload.Removable, payload.Destructive, payload.RequiresForce)
	for _, cmd := range payload.ExpectedGitCommands {
		fmt.Fprintf(stdout, "gitCommand: %s\n", strings.Join(cmd, " "))
	}
	renderMessages(stdout, result)
	return nil
}

func taskCleanupPlanResult(payload contracts.TaskCleanupPlanPayload, success bool, severity string, errors []contracts.ResultMessage, warnings []contracts.ResultMessage) contracts.CommandResult {
	raw, _ := json.Marshal(payload)
	return contracts.CommandResult{Schema: contracts.CommandResultSchema, Command: "task cleanup --plan", Success: success, Severity: severity, Warnings: emptyMessages(warnings), Errors: emptyMessages(errors), SuggestedNextActions: cleanupSuggested(success), Payload: raw}
}

func taskCleanupResult(plan contracts.TaskCleanupPlanPayload, force bool, metadataRemoved bool, errors []contracts.ResultMessage, warnings []contracts.ResultMessage, commands []contracts.GitCommandResult) contracts.CommandResult {
	return taskCleanupResultWithState(plan, force, metadataRemoved, false, false, errors, warnings, commands)
}

func taskCleanupResultWithState(plan contracts.TaskCleanupPlanPayload, force bool, metadataRemoved bool, branchRemoved bool, worktreeRemoved bool, errors []contracts.ResultMessage, warnings []contracts.ResultMessage, commands []contracts.GitCommandResult) contracts.CommandResult {
	warningTexts := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		warningTexts = append(warningTexts, warning.DisplayText())
	}
	payload := contracts.TaskCleanupPayload{Slug: plan.Slug, WorktreePath: plan.WorktreePath, Branch: plan.Branch, Plan: &plan, GitCommands: commands, MetadataRemoved: metadataRemoved, BranchRemoved: branchRemoved, WorktreeRemoved: worktreeRemoved, Force: force, CleanupWarnings: warningTexts}
	raw, _ := json.Marshal(payload)
	success := len(errors) == 0
	severity := "info"
	if !success {
		severity = "error"
	} else if len(warnings) > 0 {
		severity = "warning"
	}
	return contracts.CommandResult{Schema: contracts.CommandResultSchema, Command: "task cleanup", Success: success, Severity: severity, Warnings: emptyMessages(warnings), Errors: emptyMessages(errors), SuggestedNextActions: cleanupSuggested(success), Payload: raw}
}

func (service TaskCleanupService) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func (service TaskCleanupService) git() GitRunner {
	if service.Git != nil {
		return service.Git
	}
	return ExecGitRunner{}
}

func worktreeRegistered(git GitRunner, repoRoot string, path string) bool {
	list := git.Run(repoRoot, "worktree", "list", "--porcelain")
	if list.ExitCode != 0 {
		return false
	}
	needle := strings.TrimRight(strings.ReplaceAll(path, "/", "\\"), "\\")
	for _, line := range strings.Split(list.Stdout, "\n") {
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		got := strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(strings.TrimPrefix(line, "worktree ")), "/", "\\"), "\\")
		if strings.EqualFold(got, needle) {
			return true
		}
	}
	return false
}

func cleanupSuggested(success bool) []string {
	if success {
		return []string{"refresh-runtime-state"}
	}
	return []string{"inspect cleanup plan", "resolve cleanup blockers"}
}
