package cleanup

import (
	"fmt"
	"strings"

	"github.com/mortenlein/brevity/internal/state"
)

type DetectOptions struct {
	RepoRoot string
	Tasks    state.Tasks
	Runs     state.RunHistory
	Inspect  Inspector
}

func Detect(options DetectOptions) Report {
	inspector := options.Inspect
	if inspector == nil {
		inspector = GitInspector{}
	}
	warnings := []string{}
	worktrees, err := inspector.Worktrees(options.RepoRoot)
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	branches, err := inspector.Branches(options.RepoRoot)
	if err != nil {
		warnings = append(warnings, err.Error())
	}

	candidates := []Candidate{}
	trackedBranches := map[string]bool{}
	trackedPaths := map[string]bool{}
	for _, task := range options.Tasks.Items {
		slug := task.Key()
		branch := firstNonEmpty(task.Branch, nestedWorktreeBranch(task.Worktree))
		path := firstNonEmpty(task.WorktreePath, nestedWorktreePath(task.Worktree))
		if branch != "" {
			trackedBranches[branch] = true
		}
		if path != "" {
			trackedPaths[ComparablePath(path)] = true
			if !inspector.PathExists(path) {
				candidates = append(candidates, Candidate{
					ID:              "missing-worktree:" + safeID(slug),
					Kind:            KindMissingWorktree,
					Severity:        SeverityWarn,
					TaskSlug:        slug,
					Branch:          branch,
					WorktreePath:    path,
					Reason:          "task metadata references a worktree path that does not exist",
					SuggestedAction: "inspect task metadata before cleanup or recreation",
					Source:          "native-task-store",
				})
			}
		}
	}

	checkedOutBranches := map[string]bool{}
	for _, worktree := range worktrees {
		if worktree.Branch != "" {
			checkedOutBranches[worktree.Branch] = true
		}
		if !strings.HasPrefix(worktree.Branch, "task/") {
			continue
		}
		if trackedBranches[worktree.Branch] || trackedPaths[ComparablePath(worktree.Path)] {
			dirty, dirtyErr := inspector.Dirty(worktree.Path)
			if dirtyErr != nil {
				warnings = append(warnings, dirtyErr.Error())
			}
			if dirty {
				candidates = append(candidates, Candidate{
					ID:              "dirty-worktree:" + safeID(worktree.Branch),
					Kind:            KindDirtyWorktree,
					Severity:        SeverityWarn,
					Branch:          worktree.Branch,
					WorktreePath:    worktree.Path,
					Dirty:           true,
					Reason:          "tracked task worktree has uncommitted changes",
					SuggestedAction: "inspect git status before merge or cleanup",
					Source:          "git-status",
				})
			}
			continue
		}
		dirty, dirtyErr := inspector.Dirty(worktree.Path)
		if dirtyErr != nil {
			warnings = append(warnings, dirtyErr.Error())
		}
		candidates = append(candidates, Candidate{
			ID:              "orphan-worktree:" + safeID(firstNonEmpty(worktree.Branch, worktree.Path)),
			Kind:            KindOrphanWorktree,
			Severity:        SeverityWarn,
			Branch:          worktree.Branch,
			WorktreePath:    worktree.Path,
			Dirty:           dirty,
			Removable:       !dirty && dirtyErr == nil,
			Destructive:     true,
			Reason:          orphanWorktreeReason(dirty, dirtyErr),
			SuggestedAction: orphanWorktreeAction(dirty, dirtyErr),
			Source:          "git-worktree-list",
		})
	}

	for _, branch := range branches {
		if !strings.HasPrefix(branch, "task/") || trackedBranches[branch] || checkedOutBranches[branch] {
			continue
		}
		candidates = append(candidates, Candidate{
			ID:              "orphan-branch:" + safeID(branch),
			Kind:            KindOrphanBranch,
			Severity:        SeverityWarn,
			Branch:          branch,
			Destructive:     true,
			Reason:          "local task branch is not tracked by task metadata and is not checked out by a registered worktree",
			SuggestedAction: "inspect branch history and merge status before manual deletion",
			Source:          "git-branch",
		})
	}

	for _, run := range options.Runs.Items {
		if run.Stale {
			candidates = append(candidates, Candidate{
				ID:              "stale-run:" + safeID(firstNonEmpty(run.RunID, run.Slug)),
				Kind:            KindStaleRun,
				Severity:        SeverityInfo,
				TaskSlug:        run.Slug,
				Reason:          fmt.Sprintf("run %s is incomplete and stale", firstNonEmpty(run.RunID, "(unknown)")),
				SuggestedAction: "inspect run log before retrying or compacting run history",
				Source:          "run-history",
			})
		}
	}

	return NewReport(candidates, warnings)
}

func orphanWorktreeReason(dirty bool, err error) string {
	if err != nil {
		return "registered task worktree is not tracked by task metadata and dirty status could not be inspected"
	}
	if dirty {
		return "registered task worktree is not tracked by task metadata and has uncommitted changes"
	}
	return "registered task worktree is not tracked by task metadata and is clean"
}

func orphanWorktreeAction(dirty bool, err error) string {
	if err != nil || dirty {
		return "inspect git status before any cleanup"
	}
	return "eligible for cleanup after operator review; no cleanup was executed"
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
