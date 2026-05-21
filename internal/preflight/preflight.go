package preflight

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	nativecleanup "github.com/mortenlein/brevity/internal/cleanup"
	"github.com/mortenlein/brevity/internal/state"
)

type Options struct {
	RepoRoot string
	Action   Action
	Slug     string
	Profile  string
	Now      func() time.Time
}

var slugPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func Run(options Options) (Result, error) {
	store, err := state.NewStore(options.RepoRoot)
	if err != nil {
		return Result{}, err
	}
	now := time.Now
	if options.Now != nil {
		now = options.Now
	}
	result := NewResult(options.Action, options.Slug)
	applyActionMetadata(&result)

	if _, err := os.Stat(store.RepoRoot); err != nil {
		result.AddCheck("repo-root", StatusBlocked, SeverityError, fmt.Sprintf("repo root is not readable: %v", err))
		return finish(result), nil
	}
	result.AddCheck("repo-root", StatusAllowed, SeverityInfo, "repo root is readable")

	if info, err := os.Stat(store.BrevityRoot()); err != nil || !info.IsDir() {
		result.AddCheck("brevity-dir", StatusBlocked, SeverityError, ".brevity directory is not readable")
		return finish(result), nil
	}
	result.AddCheck("brevity-dir", StatusAllowed, SeverityInfo, ".brevity directory is readable")

	checkLock(store, now, &result)

	tasks, missingTasks, err := state.LoadTasks(store)
	if err != nil {
		result.AddCheck("tasks-json", StatusBlocked, SeverityError, err.Error())
		return finish(result), nil
	}
	if missingTasks {
		result.AddCheck("tasks-json", StatusBlocked, SeverityError, ".brevity/tasks.json is missing")
		return finish(result), nil
	}
	result.AddCheck("tasks-json", StatusAllowed, SeverityInfo, ".brevity/tasks.json is readable")

	health, missingHealth, err := state.LoadProviderHealth(store)
	if err != nil {
		result.AddCheck("provider-health", StatusBlocked, SeverityError, err.Error())
		return finish(result), nil
	}
	if missingHealth {
		result.AddCheck("provider-health", StatusWarn, SeverityWarn, ".brevity/provider-health.json is missing; provider readiness is unknown")
	} else {
		result.AddCheck("provider-health", StatusAllowed, SeverityInfo, ".brevity/provider-health.json is readable")
	}

	if !slugPattern.MatchString(strings.TrimSpace(options.Slug)) {
		result.AddCheck("slug-valid", StatusBlocked, SeverityError, "slug must contain only letters, numbers, dot, underscore, or dash and start with a letter or number")
		return finish(result), nil
	}
	result.AddCheck("slug-valid", StatusAllowed, SeverityInfo, "slug syntax is valid")

	task, exists := findTask(tasks, options.Slug)
	if options.Action == ActionTaskNew {
		if exists {
			result.AddCheck("task-absence", StatusBlocked, SeverityError, "task already exists")
		} else {
			result.AddCheck("task-absence", StatusAllowed, SeverityInfo, "task does not already exist")
		}
		return finish(result), nil
	}
	if !exists {
		result.AddCheck("task-exists", StatusBlocked, SeverityError, "task does not exist")
		return finish(result), nil
	}
	result.AddCheck("task-exists", StatusAllowed, SeverityInfo, "task exists")

	checkTaskState(task, options.Action, &result)
	checkWorktree(task, options.Action, &result)
	checkBranch(task, options.Action, &result)
	if options.Action == ActionTaskRun {
		checkRun(task, health, missingHealth, &result)
	}
	if options.Action == ActionTaskCleanup {
		checkCleanup(store.RepoRoot, tasks, task, &result)
	}
	return finish(result), nil
}

func applyActionMetadata(result *Result) {
	switch result.Action {
	case ActionTaskNew:
		result.ExpectedMutations = []string{"create task metadata", "create task branch/worktree in future implementation"}
	case ActionTaskStart:
		result.RequiresConfirmation = true
		result.ExpectedMutations = []string{"transition task to active/startable state", "create or register task worktree if PowerShell requires it"}
	case ActionTaskRun:
		result.ProviderExecution = true
		result.RequiresConfirmation = true
		result.ExpectedMutations = []string{"materialize runtime prompt if needed", "launch worker/provider execution"}
	case ActionTaskMerge:
		result.RequiresConfirmation = true
		result.ExpectedMutations = []string{"merge task branch into integration branch"}
	case ActionTaskCleanup:
		result.Destructive = true
		result.RequiresConfirmation = true
		result.ExpectedMutations = []string{"remove completed task worktree", "delete completed task branch if eligible", "update task cleanup metadata"}
	}
}

func checkLock(store state.Store, now func() time.Time, result *Result) {
	info, err := os.Stat(store.LockPath())
	if os.IsNotExist(err) {
		result.AddCheck("state-lock", StatusAllowed, SeverityInfo, "state lock is not present")
		return
	}
	if err != nil {
		result.AddCheck("state-lock", StatusBlocked, SeverityError, fmt.Sprintf("state lock could not be inspected: %v", err))
		return
	}
	age := now().UTC().Sub(info.ModTime().UTC())
	if age > 30*time.Minute {
		result.AddCheck("state-lock", StatusWarn, SeverityWarn, "state lock appears stale; inspect before mutation")
		return
	}
	result.AddCheck("state-lock", StatusBlocked, SeverityError, "state lock is currently held")
}

func checkTaskState(task state.Task, action Action, result *Result) {
	current := strings.ToLower(strings.TrimSpace(firstNonEmpty(task.NormalizedState, task.Status)))
	switch action {
	case ActionTaskStart:
		if oneOf(current, "planned", "ready", "ready-for-worker", "queued", "new", "") {
			result.AddCheck("task-state", StatusAllowed, SeverityInfo, "task state permits start")
		} else {
			result.AddCheck("task-state", StatusBlocked, SeverityError, "task state does not permit start: "+current)
		}
	case ActionTaskRun:
		if oneOf(current, "ready-for-worker", "ready", "runnable", "active", "failed") {
			result.AddCheck("task-state", StatusAllowed, SeverityInfo, "task state permits run preflight")
		} else {
			result.AddCheck("task-state", StatusBlocked, SeverityError, "task state does not permit run: "+current)
		}
	case ActionTaskMerge:
		if oneOf(current, "completed", "review", "needs-review", "done") {
			result.AddCheck("task-state", StatusAllowed, SeverityInfo, "task state permits merge review")
		} else {
			result.AddCheck("task-state", StatusBlocked, SeverityError, "task state does not permit merge: "+current)
		}
	case ActionTaskCleanup:
		if oneOf(current, "completed", "merged", "done") {
			result.AddCheck("task-state", StatusAllowed, SeverityInfo, "task state permits cleanup review")
		} else {
			result.AddCheck("task-state", StatusBlocked, SeverityError, "task state does not permit cleanup: "+current)
		}
	}
}

func checkWorktree(task state.Task, action Action, result *Result) {
	path := firstNonEmpty(task.WorktreePath, nestedWorktreePath(task.Worktree))
	if path == "" {
		if action == ActionTaskStart {
			result.AddCheck("worktree-path", StatusWarn, SeverityWarn, "worktree path is not recorded; start may materialize it later")
		} else {
			result.AddCheck("worktree-path", StatusBlocked, SeverityError, "worktree path is required")
		}
		return
	}
	clean := filepath.Clean(path)
	if clean == "." || strings.Contains(clean, "..") && !filepath.IsAbs(clean) {
		result.AddCheck("worktree-path", StatusBlocked, SeverityError, "worktree path is not valid")
		return
	}
	if _, err := os.Stat(path); err != nil {
		if action == ActionTaskStart {
			result.AddCheck("worktree-path", StatusWarn, SeverityWarn, "worktree path is recorded but missing; start may create it later")
		} else {
			result.AddCheck("worktree-path", StatusBlocked, SeverityError, "worktree path is missing")
		}
		return
	}
	result.AddCheck("worktree-path", StatusAllowed, SeverityInfo, "worktree path exists")
}

func checkBranch(task state.Task, action Action, result *Result) {
	branch := firstNonEmpty(task.Branch, nestedWorktreeBranch(task.Worktree))
	if branch == "" {
		if action == ActionTaskStart {
			result.AddCheck("branch-name", StatusWarn, SeverityWarn, "branch is not recorded; start may create it later")
		} else {
			result.AddCheck("branch-name", StatusBlocked, SeverityError, "branch name is required")
		}
		return
	}
	if strings.ContainsAny(branch, " \t~^:?*[\\") || strings.HasPrefix(branch, "-") || strings.HasSuffix(branch, "/") {
		result.AddCheck("branch-name", StatusBlocked, SeverityError, "branch name is not valid")
		return
	}
	result.AddCheck("branch-name", StatusAllowed, SeverityInfo, "branch name is valid")
}

func checkRun(task state.Task, health state.ProviderHealthState, missingHealth bool, result *Result) {
	provider := strings.ToLower(firstNonEmpty(task.Provider, task.LastProvider, task.LatestRunProvider, "codex"))
	profile := firstNonEmpty(task.Profile, task.LastProfile, task.LatestRunProfile, "default")
	if provider == "" || profile == "" {
		result.AddCheck("provider-profile", StatusBlocked, SeverityError, "provider/profile is not present or defaultable")
	} else {
		result.AddCheck("provider-profile", StatusAllowed, SeverityInfo, fmt.Sprintf("provider/profile resolves to %s/%s", provider, profile))
	}
	if !missingHealth {
		record := health[provider]
		switch record.Status {
		case state.StatusUnavailable:
			result.AddCheck("provider-ready", StatusBlocked, SeverityError, "provider is unavailable")
		case state.StatusQuotaConstrained:
			result.AddCheck("provider-ready", StatusBlocked, SeverityError, "provider is quota-constrained")
		case state.StatusCapacityDegraded, state.StatusUnknown, "":
			result.AddCheck("provider-ready", StatusWarn, SeverityWarn, "provider readiness is degraded or unknown")
		default:
			result.AddCheck("provider-ready", StatusAllowed, SeverityInfo, "provider is ready")
		}
	}
	prompt := firstNonEmpty(task.PromptPath, nestedPromptPath(task.Prompt))
	if prompt == "" {
		result.AddCheck("prompt-path", StatusWarn, SeverityWarn, "prompt path is not recorded; future run may materialize it")
	} else if _, err := os.Stat(prompt); err != nil {
		result.AddCheck("prompt-path", StatusWarn, SeverityWarn, "prompt path is missing; future run may materialize it")
	} else {
		result.AddCheck("prompt-path", StatusAllowed, SeverityInfo, "prompt path exists")
	}
	result.AddCheck("worker-execution", StatusWarn, SeverityWarn, "worker/provider execution would be required but was not performed")
}

func checkCleanup(repoRoot string, tasks state.Tasks, task state.Task, result *Result) {
	report := nativecleanup.Detect(nativecleanup.DetectOptions{RepoRoot: repoRoot, Tasks: tasks})
	for _, candidate := range report.Candidates {
		if candidate.TaskSlug != task.Key() && candidate.WorktreePath != firstNonEmpty(task.WorktreePath, nestedWorktreePath(task.Worktree)) && candidate.Branch != firstNonEmpty(task.Branch, nestedWorktreeBranch(task.Worktree)) {
			continue
		}
		if candidate.Dirty {
			result.AddCheck("cleanup-dirty", StatusBlocked, SeverityError, "dirty worktree blocks cleanup")
			return
		}
		if candidate.Severity == nativecleanup.SeverityWarn {
			result.AddCheck("cleanup-inspection", StatusWarn, SeverityWarn, candidate.Reason)
			return
		}
	}
	result.AddCheck("cleanup-inspection", StatusAllowed, SeverityInfo, "cleanup inspection found no blocking candidate for task")
}

func finish(result Result) Result {
	result.DryRunSummary = fmt.Sprintf("%s preflight for %s is %s; no mutation or provider execution occurred.", result.Action, emptyAs(result.TargetSlug, "(none)"), result.Status)
	result.recompute()
	return result
}

func findTask(tasks state.Tasks, slug string) (state.Task, bool) {
	for _, task := range tasks.Items {
		if task.Key() == strings.TrimSpace(slug) {
			return task, true
		}
	}
	return state.Task{}, false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
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

func nestedPromptPath(prompt *state.TaskPrompt) string {
	if prompt == nil {
		return ""
	}
	return prompt.Path
}
