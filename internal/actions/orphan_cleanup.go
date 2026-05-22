package actions

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	nativecleanup "github.com/mortenlein/brevity/internal/cleanup"
	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/state"
)

const (
	orphanCleanupPlanSchema      = "brevity.orphan-cleanup-plan.v1"
	orphanCleanupPlanSetSchema   = "brevity.orphan-cleanup-plan-set.v1"
	orphanCleanupExecutionSchema = "brevity.orphan-cleanup-execution.v1"
)

type OrphanCleanupService struct {
	Store state.Store
	Now   func() time.Time
	Git   GitRunner
}

func (service OrphanCleanupService) Plan(candidateID string, all bool) (contracts.CommandResult, error) {
	payload, err := service.buildPlanSet(candidateID, all)
	if err != nil {
		return orphanCleanupPlanResult(payload, false, []contracts.ResultMessage{{Code: "orphan-cleanup-plan-failed", Message: err.Error()}}, nil), err
	}
	success := true
	errors := []contracts.ResultMessage{}
	for _, plan := range payload.Plans {
		if len(plan.Blockers) > 0 {
			success = false
			errors = append(errors, plan.Blockers...)
		}
	}
	if len(payload.Plans) == 0 {
		success = false
		errors = []contracts.ResultMessage{{Code: "candidate-not-found", Message: "cleanup candidate not found"}}
	}
	return orphanCleanupPlanResult(payload, success, errors, collectOrphanWarnings(payload.Plans)), nil
}

func (service OrphanCleanupService) Execute(candidateID string, all bool, force bool) (contracts.CommandResult, error) {
	planSet, err := service.buildPlanSet(candidateID, all)
	if err != nil {
		result := orphanCleanupExecutionResult(planSet, force, nil, []contracts.ResultMessage{{Code: "orphan-cleanup-plan-failed", Message: err.Error()}}, nil)
		return result, err
	}
	if !force {
		result := orphanCleanupExecutionResult(planSet, force, nil, []contracts.ResultMessage{{Code: "force-required", Message: "orphan cleanup execution requires --force"}}, collectOrphanWarnings(planSet.Plans))
		return result, fmt.Errorf("orphan cleanup execution requires --force")
	}
	if len(planSet.Plans) == 0 {
		result := orphanCleanupExecutionResult(planSet, force, nil, []contracts.ResultMessage{{Code: "candidate-not-found", Message: "cleanup candidate not found"}}, nil)
		return result, fmt.Errorf("cleanup candidate not found")
	}

	git := service.git()
	commands := []contracts.GitCommandResult{}
	errors := []contracts.ResultMessage{}
	results := []contracts.OrphanCleanupResult{}
	for _, plan := range planSet.Plans {
		if len(plan.Blockers) > 0 {
			results = append(results, contracts.OrphanCleanupResult{CandidateID: plan.CandidateID, CandidateKind: plan.CandidateKind, Skipped: true, Message: "blocked"})
			if !all {
				errors = append(errors, plan.Blockers...)
			}
			continue
		}
		item := contracts.OrphanCleanupResult{CandidateID: plan.CandidateID, CandidateKind: plan.CandidateKind}
		if plan.WorktreeExists && plan.WorktreeRegistered {
			remove := git.Run(plan.RepoRoot, "worktree", "remove", plan.WorktreePath)
			commands = append(commands, toCommandResult(remove))
			if remove.ExitCode != 0 {
				item.Skipped = true
				item.Message = trimGitMessage(remove)
				results = append(results, item)
				if !all {
					errors = append(errors, contracts.ResultMessage{Code: "git-worktree-remove-failed", Message: item.Message})
				}
				continue
			}
			if _, statErr := os.Stat(plan.WorktreePath); os.IsNotExist(statErr) {
				item.WorktreeRemoved = true
			}
		}
		if plan.BranchExists {
			removeBranch := git.Run(plan.RepoRoot, "branch", "-d", plan.Branch)
			commands = append(commands, toCommandResult(removeBranch))
			if removeBranch.ExitCode != 0 {
				item.Skipped = true
				item.Message = trimGitMessage(removeBranch)
				results = append(results, item)
				if !all {
					errors = append(errors, contracts.ResultMessage{Code: "git-branch-delete-failed", Message: item.Message})
				}
				continue
			}
			item.BranchRemoved = git.Run(plan.RepoRoot, "show-ref", "--verify", "--quiet", "refs/heads/"+plan.Branch).ExitCode != 0
		}
		results = append(results, item)
	}

	result := orphanCleanupExecutionResult(planSet, force, commands, errors, collectOrphanWarnings(planSet.Plans))
	payload, _ := contracts.ParseOrphanCleanupExecutionPayload(result)
	payload.Results = results
	for _, item := range results {
		if item.WorktreeRemoved {
			payload.WorktreeRemoved++
		}
		if item.BranchRemoved {
			payload.BranchRemoved++
		}
		if item.Skipped {
			payload.Skipped++
		}
	}
	payload.GitCommands = commands
	raw, _ := json.Marshal(payload)
	result.Payload = raw
	result.Success = len(errors) == 0
	if !result.Success {
		result.Severity = "error"
		return result, fmt.Errorf("orphan cleanup execution blocked or failed")
	}
	return result, nil
}

func (service OrphanCleanupService) buildPlanSet(candidateID string, all bool) (contracts.OrphanCleanupPlanSetPayload, error) {
	now := service.now().UTC().Format(time.RFC3339Nano)
	payload := contracts.OrphanCleanupPlanSetPayload{Schema: orphanCleanupPlanSetSchema, Version: 1, CandidateID: strings.TrimSpace(candidateID), All: all, Plans: []contracts.OrphanCleanupPlanPayload{}, GeneratedAt: now}
	report, err := service.inspect()
	if err != nil {
		return payload, err
	}
	for _, candidate := range report.Candidates {
		if !all && candidate.ID != payload.CandidateID {
			continue
		}
		payload.Plans = append(payload.Plans, service.planCandidate(candidate, now))
	}
	return payload, nil
}

func (service OrphanCleanupService) planCandidate(candidate nativecleanup.Candidate, generatedAt string) contracts.OrphanCleanupPlanPayload {
	git := service.git()
	plan := contracts.OrphanCleanupPlanPayload{
		Schema:                   orphanCleanupPlanSchema,
		Version:                  1,
		CandidateID:              candidate.ID,
		CandidateKind:            string(candidate.Kind),
		WorktreePath:             candidate.WorktreePath,
		Branch:                   candidate.Branch,
		RepoRoot:                 service.Store.RepoRoot,
		Dirty:                    candidate.Dirty,
		Destructive:              candidate.Destructive,
		RequiresForce:            candidate.Destructive,
		ExpectedMetadataMutation: "none",
		Blockers:                 []contracts.ResultMessage{},
		Warnings:                 []contracts.ResultMessage{},
		GeneratedAt:              generatedAt,
	}
	switch candidate.Kind {
	case nativecleanup.KindOrphanWorktree:
		plan.WorktreeExists = candidate.WorktreePath != "" && pathExists(candidate.WorktreePath)
		plan.WorktreeRegistered = plan.WorktreePath != "" && worktreeRegistered(git, plan.RepoRoot, plan.WorktreePath)
		if candidate.Dirty {
			plan.Blockers = append(plan.Blockers, contracts.ResultMessage{Code: "dirty-worktree", Message: "orphan worktree has uncommitted changes"})
		}
		if !plan.WorktreeExists {
			plan.Blockers = append(plan.Blockers, contracts.ResultMessage{Code: "missing-worktree", Message: "orphan worktree path is missing"})
		}
		if !plan.WorktreeRegistered {
			plan.Blockers = append(plan.Blockers, contracts.ResultMessage{Code: "unregistered-worktree", Message: "orphan worktree is not registered with Git"})
		}
	case nativecleanup.KindOrphanBranch:
	default:
		plan.Blockers = append(plan.Blockers, contracts.ResultMessage{Code: "unsupported-candidate-kind", Message: "orphan cleanup only executes orphan worktree and orphan branch candidates"})
	}
	if plan.Branch != "" {
		plan.BranchExists = git.Run(plan.RepoRoot, "show-ref", "--verify", "--quiet", "refs/heads/"+plan.Branch).ExitCode == 0
		if plan.BranchExists {
			merged := git.Run(plan.RepoRoot, "branch", "--merged", "HEAD", "--list", plan.Branch)
			if merged.ExitCode == 0 {
				plan.BranchMergedKnown = true
				plan.BranchMerged = strings.TrimSpace(merged.Stdout) != ""
			}
			if plan.BranchMergedKnown && !plan.BranchMerged {
				plan.Blockers = append(plan.Blockers, contracts.ResultMessage{Code: "unmerged-branch", Message: "orphan branch is not merged into current HEAD"})
			} else if !plan.BranchMergedKnown {
				plan.Blockers = append(plan.Blockers, contracts.ResultMessage{Code: "branch-merge-state-unknown", Message: "could not determine whether orphan branch is merged"})
			}
		}
	}
	if plan.WorktreeExists && plan.WorktreeRegistered {
		plan.ExpectedGitCommands = append(plan.ExpectedGitCommands, []string{"git", "worktree", "remove", plan.WorktreePath})
	}
	if plan.BranchExists {
		plan.ExpectedGitCommands = append(plan.ExpectedGitCommands, []string{"git", "branch", "-d", plan.Branch})
	}
	plan.Removable = len(plan.Blockers) == 0
	return plan
}

func (service OrphanCleanupService) inspect() (nativecleanup.Report, error) {
	tasks, _, err := state.LoadTasks(service.Store)
	if err != nil {
		return nativecleanup.Report{}, err
	}
	runs, _, err := state.LoadRuns(service.Store, service.now())
	if err != nil {
		return nativecleanup.Report{}, err
	}
	return nativecleanup.Detect(nativecleanup.DetectOptions{RepoRoot: service.Store.RepoRoot, Tasks: tasks, Runs: runs}), nil
}

func RenderOrphanCleanupPlanResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseOrphanCleanupPlanSetPayload(result)
	if err != nil {
		return err
	}
	renderStatusLine(stdout, "Orphan cleanup plan", result.Success)
	for _, plan := range payload.Plans {
		fmt.Fprintf(stdout, "candidate: %s [%s]\n", plan.CandidateID, plan.CandidateKind)
		fmt.Fprintf(stdout, "worktreePath: %s\nbranch: %s\n", plan.WorktreePath, plan.Branch)
		fmt.Fprintf(stdout, "dirty: %t branchMerged: %t removable: %t destructive: %t requiresForce: %t\n", plan.Dirty, plan.BranchMerged, plan.Removable, plan.Destructive, plan.RequiresForce)
		for _, cmd := range plan.ExpectedGitCommands {
			fmt.Fprintf(stdout, "gitCommand: %s\n", strings.Join(cmd, " "))
		}
	}
	renderMessages(stdout, result)
	return nil
}

func RenderOrphanCleanupExecutionResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseOrphanCleanupExecutionPayload(result)
	if err != nil {
		return err
	}
	renderStatusLine(stdout, "Orphan cleanup execute", result.Success)
	fmt.Fprintf(stdout, "worktreeRemoved: %d\nbranchRemoved: %d\nskipped: %d\n", payload.WorktreeRemoved, payload.BranchRemoved, payload.Skipped)
	for _, item := range payload.Results {
		fmt.Fprintf(stdout, "candidate: %s worktreeRemoved=%t branchRemoved=%t skipped=%t\n", item.CandidateID, item.WorktreeRemoved, item.BranchRemoved, item.Skipped)
	}
	renderMessages(stdout, result)
	return nil
}

func orphanCleanupPlanResult(payload contracts.OrphanCleanupPlanSetPayload, success bool, errors []contracts.ResultMessage, warnings []contracts.ResultMessage) contracts.CommandResult {
	raw, _ := json.Marshal(payload)
	severity := "info"
	if !success {
		severity = "error"
	} else if len(warnings) > 0 {
		severity = "warning"
	}
	return contracts.CommandResult{Schema: contracts.CommandResultSchema, Command: "cleanup plan", Success: success, Severity: severity, Warnings: emptyMessages(warnings), Errors: emptyMessages(errors), SuggestedNextActions: orphanCleanupSuggested(success), Payload: raw}
}

func orphanCleanupExecutionResult(planSet contracts.OrphanCleanupPlanSetPayload, force bool, commands []contracts.GitCommandResult, errors []contracts.ResultMessage, warnings []contracts.ResultMessage) contracts.CommandResult {
	payload := contracts.OrphanCleanupExecutionPayload{Schema: orphanCleanupExecutionSchema, Version: 1, CandidateID: planSet.CandidateID, All: planSet.All, Force: force, Plans: planSet.Plans, Results: []contracts.OrphanCleanupResult{}, GitCommands: commands, GeneratedAt: planSet.GeneratedAt}
	raw, _ := json.Marshal(payload)
	success := len(errors) == 0
	severity := "info"
	if !success {
		severity = "error"
	} else if len(warnings) > 0 {
		severity = "warning"
	}
	return contracts.CommandResult{Schema: contracts.CommandResultSchema, Command: "cleanup execute", Success: success, Severity: severity, Warnings: emptyMessages(warnings), Errors: emptyMessages(errors), SuggestedNextActions: orphanCleanupSuggested(success), Payload: raw}
}

func collectOrphanWarnings(plans []contracts.OrphanCleanupPlanPayload) []contracts.ResultMessage {
	warnings := []contracts.ResultMessage{}
	for _, plan := range plans {
		warnings = append(warnings, plan.Warnings...)
	}
	return warnings
}

func (service OrphanCleanupService) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func (service OrphanCleanupService) git() GitRunner {
	if service.Git != nil {
		return service.Git
	}
	return ExecGitRunner{}
}

func pathExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func orphanCleanupSuggested(success bool) []string {
	if success {
		return []string{"refresh-runtime-state"}
	}
	return []string{"inspect cleanup plan", "resolve cleanup blockers"}
}
