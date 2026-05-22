package runtimeclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mortenlein/brevity/internal/actions"
	nativecleanup "github.com/mortenlein/brevity/internal/cleanup"
	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/diagnostics"
	"github.com/mortenlein/brevity/internal/state"
)

type NativeClient struct {
	RepoRoot string
	Now      func() time.Time
}

func NewNativeClient(repoRoot string) NativeClient {
	return NativeClient{RepoRoot: repoRoot}
}

func (client NativeClient) RuntimeStateJSON() ([]byte, error) {
	state, err := client.RuntimeState()
	if err != nil {
		return nil, err
	}
	return json.Marshal(state)
}

func (client NativeClient) RuntimeState() (contracts.RuntimeState, error) {
	repoRoot := client.RepoRoot
	if repoRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			return contracts.RuntimeState{}, fmt.Errorf("get working directory: %w", err)
		}
		repoRoot = wd
	}

	now := time.Now
	if client.Now != nil {
		now = client.Now
	}

	runtimeState := contracts.RuntimeState{
		Schema:      contracts.RuntimeStateSchema,
		RepoRoot:    repoRoot,
		GeneratedAt: now().UTC().Format(time.RFC3339),
		Providers: contracts.Providers{
			Health: map[string]contracts.ProviderHealth{},
		},
		Tasks:                 []contracts.TaskSummary{},
		Groups:                map[string]any{},
		OrphanedTaskWorktrees: []contracts.WorktreeRecord{},
		ActiveWorktrees:       []contracts.WorktreeRecord{},
		SuggestedNextActions:  []string{"No immediate runtime action suggested."},
	}

	store, err := state.NewStore(repoRoot)
	if err != nil {
		return contracts.RuntimeState{}, err
	}
	providerState, missingProviderHealth, err := state.LoadProviderHealth(store)
	if err != nil {
		return contracts.RuntimeState{}, err
	}
	providers := map[string]contracts.ProviderHealth{}
	for provider, health := range providerState {
		providers[provider] = health.ToContract()
	}
	runtimeState.Providers.Health = providers
	runtimeState.Providers.Summary = summarizeProviders(providers)
	if missingProviderHealth {
		runtimeState.SuggestedNextActions = append(runtimeState.SuggestedNextActions, "No .brevity\\provider-health.json found.")
	}

	taskStore, missingTasks, err := state.LoadTasks(store)
	if err != nil {
		return contracts.RuntimeState{}, err
	}
	tasks := taskStore.ToContracts()
	attachPromptFiles(tasks)
	if missingTasks {
		runtimeState.SuggestedNextActions = append(runtimeState.SuggestedNextActions, "No .brevity\\tasks.json found.")
	}

	runHistory, missingRuns, err := state.LoadRuns(store, now().UTC())
	if err != nil {
		return contracts.RuntimeState{}, err
	}
	if missingRuns {
		runtimeState.SuggestedNextActions = append(runtimeState.SuggestedNextActions, ".brevity\\runs.jsonl is absent; latest run data is unavailable.")
	}
	attachLatestRuns(tasks, runHistory.LatestByTask(), runHistory.CountByTask())

	runtimeState.Tasks = tasks
	runtimeState.TaskCounts = countTasks(tasks)
	runtimeState.Groups = groupTasks(tasks)

	report := nativecleanup.Detect(nativecleanup.DetectOptions{RepoRoot: repoRoot, Tasks: taskStore, Runs: runHistory})
	for _, warning := range report.Warnings {
		runtimeState.SuggestedNextActions = append(runtimeState.SuggestedNextActions, fmt.Sprintf("Native cleanup inspection warning: %s.", warning))
	}
	worktrees, err := nativecleanup.GitInspector{}.Worktrees(repoRoot)
	if err == nil {
		runtimeState.ActiveWorktrees = cleanupWorktreesToContracts(worktrees)
		runtimeState.ActiveWorktreeCount = len(worktrees)
	}
	runtimeState.OrphanedTaskWorktrees = cleanupReportOrphanWorktrees(report)
	if report.Summary.Total > 0 {
		runtimeState.Cleanup = cleanupReportToRuntimeCleanup(report)
		runtimeState.SuggestedNextActions = append(runtimeState.SuggestedNextActions, "Review native cleanup inspection findings before cleanup; native Go executed no cleanup.")
	}

	return runtimeState, nil
}

func attachPromptFiles(tasks []contracts.TaskSummary) {
	for index := range tasks {
		tasks[index].PromptExists = fileExists(tasks[index].PromptPath)
		tasks[index].PromptStatus = promptStatus(tasks[index].PromptPath, tasks[index].PromptRefreshedAt)
	}
}

func (client NativeClient) CleanupInspectJSON() ([]byte, error) {
	store, now, err := client.storeAndNow()
	if err != nil {
		return nil, err
	}
	tasks, _, err := state.LoadTasks(store)
	if err != nil {
		return nil, err
	}
	runs, _, err := state.LoadRuns(store, now)
	if err != nil {
		return nil, err
	}
	report := nativecleanup.Detect(nativecleanup.DetectOptions{RepoRoot: store.RepoRoot, Tasks: tasks, Runs: runs})
	return json.Marshal(report)
}

func cleanupWorktreesToContracts(worktrees []nativecleanup.Worktree) []contracts.WorktreeRecord {
	records := make([]contracts.WorktreeRecord, 0, len(worktrees))
	for _, worktree := range worktrees {
		records = append(records, contracts.WorktreeRecord{
			Path:     worktree.Path,
			Branch:   worktree.Branch,
			Head:     worktree.Head,
			Bare:     worktree.Bare,
			Detached: worktree.Detached,
		})
	}
	return records
}

func cleanupReportOrphanWorktrees(report nativecleanup.Report) []contracts.WorktreeRecord {
	records := []contracts.WorktreeRecord{}
	for _, candidate := range report.Candidates {
		if candidate.Kind != nativecleanup.KindOrphanWorktree {
			continue
		}
		records = append(records, contracts.WorktreeRecord{Path: candidate.WorktreePath, Branch: candidate.Branch})
	}
	return records
}

func cleanupReportToRuntimeCleanup(report nativecleanup.Report) *contracts.Cleanup {
	worktrees := []contracts.CleanupCandidate{}
	branches := []contracts.CleanupCandidate{}
	for _, candidate := range report.Candidates {
		runtimeCandidate := cleanupCandidateToContract(candidate)
		switch candidate.Kind {
		case nativecleanup.KindOrphanBranch:
			branches = append(branches, runtimeCandidate)
		default:
			worktrees = append(worktrees, runtimeCandidate)
		}
	}
	return &contracts.Cleanup{
		Summary:               cleanupSummary(worktrees, branches),
		OrphanedTaskWorktrees: worktrees,
		OrphanedTaskBranches:  branches,
	}
}

func cleanupCandidateToContract(candidate nativecleanup.Candidate) contracts.CleanupCandidate {
	removable := candidate.Removable
	destructive := candidate.Destructive
	return contracts.CleanupCandidate{
		ID:                    candidate.ID,
		Severity:              string(candidate.Severity),
		Category:              string(candidate.Kind),
		Path:                  candidate.WorktreePath,
		Branch:                candidate.Branch,
		Dirty:                 candidate.Dirty,
		DirtyReasons:          []string{candidate.Reason},
		SuggestedCommands:     []string{candidate.SuggestedAction, "No cleanup was executed."},
		RemovableByExecute:    &removable,
		DestructiveIfUnmerged: &destructive,
	}
}

func (client NativeClient) DoctorJSON() ([]byte, error) {
	report, err := diagnostics.Run(diagnostics.Options{RepoRoot: client.RepoRoot, Now: client.Now})
	if err != nil {
		return nil, err
	}
	return json.Marshal(diagnostics.CommandResult(report))
}
func (client NativeClient) ProviderSetJSON(provider string, status string) ([]byte, error) {
	return nil, nativeUnsupported("provider set")
}
func (client NativeClient) ProviderResetJSON(provider string) ([]byte, error) {
	return nil, nativeUnsupported("provider reset")
}
func (client NativeClient) TaskContextRefreshJSON(slug string) ([]byte, error) {
	store, err := state.NewStore(client.RepoRoot)
	if err != nil {
		return nil, err
	}
	service := actions.TaskContextRefreshService{Store: store, Now: client.Now}
	result, _ := service.Refresh(slug)
	return json.Marshal(result)
}
func (client NativeClient) TaskCleanupJSON(slug string) ([]byte, error) {
	return nil, nativeUnsupported("task cleanup")
}
func (client NativeClient) TaskNewJSON(slug string) ([]byte, error) {
	store, err := state.NewStore(client.RepoRoot)
	if err != nil {
		return nil, err
	}
	result, _ := actions.TaskNewService{Store: store, Now: client.Now}.Create(slug)
	return json.Marshal(result)
}
func (client NativeClient) TaskRunJSON(slug string, profile string, smoke bool) ([]byte, error) {
	store, err := state.NewStore(client.RepoRoot)
	if err != nil {
		return nil, err
	}
	result, err := actions.TaskRunExecuteService{Store: store, Now: client.Now}.Execute(context.Background(), slug, profile)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
func (client NativeClient) TaskRunPlanJSON(slug string, profile string) ([]byte, error) {
	store, err := state.NewStore(client.RepoRoot)
	if err != nil {
		return nil, err
	}
	result, err := actions.TaskRunPlanService{Store: store, Now: client.Now}.Plan(slug, profile)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
func (client NativeClient) TaskRuntimeInfoJSON(slug string) ([]byte, error) {
	result, err := client.TaskRuntimeInfo(slug)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
func (client NativeClient) TaskRunsJSON(slug string) ([]byte, error) {
	result, err := client.TaskRuns(slug)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
func (client NativeClient) TaskRunsReconcileJSON() ([]byte, error) {
	return nil, nativeUnsupported("task runs reconcile")
}
func (client NativeClient) TaskRunsRetentionJSON() ([]byte, error) {
	return nil, nativeUnsupported("task runs retention")
}
func (client NativeClient) TaskRunsCompactJSON() ([]byte, error) {
	return nil, nativeUnsupported("task runs compact")
}

func nativeUnsupported(command string) error {
	return fmt.Errorf("native json source is read-only and does not support %s; use powershell", command)
}

func (client NativeClient) TaskRuns(slug string) (contracts.CommandResult, error) {
	store, now, err := client.storeAndNow()
	if err != nil {
		return contracts.CommandResult{}, err
	}
	tasks, _, err := state.LoadTasks(store)
	if err != nil {
		return contracts.CommandResult{}, err
	}
	if _, ok := findTask(tasks, slug); !ok {
		return commandResult("task runs", false, "error", contracts.TaskRunsPayload{Slug: slug, Runs: []contracts.TaskRunPayload{}}, taskNotFound(slug)), nil
	}
	history, _, err := state.LoadRuns(store, now)
	if err != nil {
		return contracts.CommandResult{}, err
	}
	runs := runsForTask(history, slug)
	payload := contracts.TaskRunsPayload{Slug: slug, Count: len(runs), Runs: runs}
	return commandResult("task runs", true, "info", payload, nil), nil
}

func (client NativeClient) TaskRuntimeInfo(slug string) (contracts.CommandResult, error) {
	store, now, err := client.storeAndNow()
	if err != nil {
		return contracts.CommandResult{}, err
	}
	tasks, _, err := state.LoadTasks(store)
	if err != nil {
		return contracts.CommandResult{}, err
	}
	task, ok := findTask(tasks, slug)
	if !ok {
		payload := contracts.TaskRuntimeInfoPayload{Slug: slug, TaskExists: false}
		return commandResult("task runtime-info", false, "error", payload, taskNotFound(slug)), nil
	}
	history, _, err := state.LoadRuns(store, now)
	if err != nil {
		return contracts.CommandResult{}, err
	}
	runs := runsForTask(history, slug)
	summary := task.ToContract()
	payload := contracts.TaskRuntimeInfoPayload{
		Slug:              summary.Slug,
		Status:            summary.Status,
		NormalizedState:   summary.NormalizedState,
		TaskExists:        true,
		Branch:            summary.Branch,
		PromptPath:        summary.PromptPath,
		PromptExists:      fileExists(summary.PromptPath),
		PromptStatus:      promptStatus(summary.PromptPath, task.PromptRefreshedAt),
		PromptRefreshedAt: task.PromptRefreshedAt,
		Provider:          firstNonEmpty(summary.Provider, summary.LastProvider),
		Profile:           firstNonEmpty(summary.Profile, summary.LastProfile),
		RunCount:          len(runs),
		Worktree: contracts.TaskRuntimeWorktreePayload{
			Exists: summary.WorktreeExists != nil && *summary.WorktreeExists,
			Path:   summary.WorktreePath,
		},
		Execution: contracts.TaskRuntimeExecutionPayload{
			Status:            firstNonEmpty(summary.WorkerStatus, summary.LatestRunWorkerStatus),
			LastRunID:         firstNonEmpty(summary.LastRunID, summary.LatestRunID),
			LastRunStartedAt:  summary.LatestRunStartedAt,
			LastRunFinishedAt: summary.LatestRunFinishedAt,
			LastExitCode:      firstNonNil(summary.LastExitCode, summary.LatestRunExitCode),
			LastFailureType:   summary.LatestRunFailureType,
			LastLogPath:       firstNonEmpty(summary.LastLogPath, summary.LatestRunLogPath),
			LastProvider:      firstNonEmpty(summary.LastProvider, summary.LatestRunProvider),
			LastProfile:       firstNonEmpty(summary.LastProfile, summary.LatestRunProfile),
		},
	}
	if summary.Context != nil {
		payload.Context.MaterializedFileCount = summary.Context.MaterializedFileCount
		payload.Context.MissingFiles = append([]string{}, summary.Context.MissingFiles...)
	}
	if len(runs) > 0 {
		latest := runs[0]
		payload.LatestRun = &latest
		payload.RunCount = len(runs)
		payload.Execution.Status = firstNonEmpty(latest.WorkerStatus, payload.Execution.Status)
		payload.Execution.LastRunID = firstNonEmpty(latest.RunID, payload.Execution.LastRunID)
		payload.Execution.LastRunStartedAt = firstNonEmpty(latest.StartedAt, payload.Execution.LastRunStartedAt)
		payload.Execution.LastRunFinishedAt = firstNonEmpty(latest.FinishedAt, payload.Execution.LastRunFinishedAt)
		payload.Execution.LastExitCode = firstNonNil(latest.ExitCode, payload.Execution.LastExitCode)
		payload.Execution.LastFailureType = firstNonEmpty(latest.FailureType, payload.Execution.LastFailureType)
		payload.Execution.LastLogPath = firstNonEmpty(latest.LogPath, payload.Execution.LastLogPath)
		payload.Execution.LastProvider = firstNonEmpty(latest.Provider, payload.Execution.LastProvider)
		payload.Execution.LastProfile = firstNonEmpty(latest.Profile, payload.Execution.LastProfile)
		payload.Provider = firstNonEmpty(payload.Provider, latest.Provider)
		payload.Profile = firstNonEmpty(payload.Profile, latest.Profile)
		payload.Stale = latest.Stale
		payload.Incomplete = latest.Incomplete
		payload.LogPath = latest.LogPath
	}
	payload.Interpretation = runtimeInterpretation(payload)
	return commandResult("task runtime-info", true, "info", payload, nil), nil
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func promptStatus(path string, refreshedAt string) string {
	if strings.TrimSpace(path) == "" {
		return "missing-path"
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "missing"
	}
	if strings.TrimSpace(refreshedAt) == "" {
		return "unknown"
	}
	refreshed, err := time.Parse(time.RFC3339Nano, refreshedAt)
	if err != nil {
		return "unknown"
	}
	if info.ModTime().After(refreshed.Add(1 * time.Second)) {
		return "stale"
	}
	return "fresh"
}

func (client NativeClient) storeAndNow() (state.Store, time.Time, error) {
	repoRoot := client.RepoRoot
	if repoRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			return state.Store{}, time.Time{}, fmt.Errorf("get working directory: %w", err)
		}
		repoRoot = wd
	}
	now := time.Now
	if client.Now != nil {
		now = client.Now
	}
	store, err := state.NewStore(repoRoot)
	return store, now().UTC(), err
}

func findTask(tasks state.Tasks, slug string) (state.Task, bool) {
	for _, task := range tasks.Items {
		if task.Key() == slug {
			return task, true
		}
	}
	return state.Task{}, false
}

func runsForTask(history state.RunHistory, slug string) []contracts.TaskRunPayload {
	runs := []contracts.TaskRunPayload{}
	for _, run := range history.Items {
		if strings.TrimSpace(run.Slug) != slug {
			continue
		}
		runs = append(runs, contracts.TaskRunPayload{
			RunID:         run.RunID,
			WorkerStatus:  run.WorkerStatus,
			ExitCode:      run.ExitCode,
			Provider:      run.Provider,
			Profile:       run.Profile,
			StartedAt:     run.StartedAt,
			FinishedAt:    run.FinishedAt,
			FailureType:   run.FailureType,
			LogPath:       run.LogPath,
			Incomplete:    run.Incomplete,
			Stale:         run.Stale,
			RunAgeMinutes: run.RunAgeMinutes,
			Source:        run.Source,
		})
	}
	return runs
}

func commandResult(command string, success bool, severity string, payload any, resultErr *contracts.ResultMessage) contracts.CommandResult {
	payloadJSON, _ := json.Marshal(payload)
	result := contracts.CommandResult{
		Schema:               contracts.CommandResultSchema,
		Command:              command,
		Success:              success,
		Severity:             severity,
		Warnings:             []contracts.ResultMessage{},
		Errors:               []contracts.ResultMessage{},
		SuggestedNextActions: []string{},
		Payload:              payloadJSON,
	}
	if resultErr != nil {
		result.Errors = append(result.Errors, *resultErr)
	}
	return result
}

func taskNotFound(slug string) *contracts.ResultMessage {
	return &contracts.ResultMessage{
		Code:    "task-not-found",
		Message: "Task not found: " + slug,
		Details: map[string]any{
			"slug": slug,
		},
	}
}

func runtimeInterpretation(payload contracts.TaskRuntimeInfoPayload) string {
	if !payload.TaskExists {
		return "Task metadata was not found."
	}
	if payload.RunCount == 0 {
		return "No task runs recorded yet."
	}
	if payload.Stale {
		return "Latest run is incomplete and stale; inspect the log before retrying."
	}
	if payload.Incomplete {
		return "Latest run is incomplete; it may still be running or may need inspection."
	}
	switch strings.ToLower(strings.TrimSpace(payload.Execution.Status)) {
	case "succeeded":
		return "Latest run completed successfully."
	case "failed":
		return "Latest run failed; inspect the log path."
	default:
		return ""
	}
}

func readGitWorktrees(repoRoot string) ([]contracts.WorktreeRecord, error) {
	command := exec.Command("git", "worktree", "list", "--porcelain")
	command.Dir = repoRoot
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list --porcelain failed: %w", err)
	}
	return parseGitWorktreePorcelain(string(output)), nil
}

func readGitBranches(repoRoot string) ([]string, error) {
	command := exec.Command("git", "branch", "--format", "%(refname:short)")
	command.Dir = repoRoot
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("git branch --format %q failed: %w", "%(refname:short)", err)
	}
	return parseGitBranchOutput(string(output)), nil
}

func parseGitBranchOutput(output string) []string {
	seen := map[string]bool{}
	var branches []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		branch := strings.TrimSpace(scanner.Text())
		branch = strings.TrimPrefix(branch, "*")
		branch = strings.TrimSpace(branch)
		if branch == "" || seen[branch] {
			continue
		}
		seen[branch] = true
		branches = append(branches, branch)
	}
	sort.Strings(branches)
	return branches
}

func parseGitWorktreePorcelain(output string) []contracts.WorktreeRecord {
	var records []contracts.WorktreeRecord
	var current contracts.WorktreeRecord
	hasCurrent := false

	flush := func() {
		if hasCurrent && strings.TrimSpace(current.Path) != "" {
			records = append(records, current)
		}
		current = contracts.WorktreeRecord{}
		hasCurrent = false
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			flush()
			continue
		}
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			key = line
			value = ""
		}
		switch key {
		case "worktree":
			flush()
			current.Path = value
			hasCurrent = true
		case "HEAD":
			current.Head = value
		case "branch":
			current.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "bare":
			current.Bare = true
		case "detached":
			current.Detached = true
		}
	}
	flush()
	return records
}

func orphanedTaskWorktrees(tasks []contracts.TaskSummary, worktrees []contracts.WorktreeRecord) []contracts.WorktreeRecord {
	metadataBranches := map[string]bool{}
	metadataPaths := map[string]bool{}
	for _, task := range tasks {
		addMetadataValue(metadataBranches, task.Branch)
		if task.Worktree != nil {
			addMetadataValue(metadataBranches, task.Worktree.Branch)
			addMetadataValue(metadataPaths, comparablePath(task.Worktree.Path))
		}
		addMetadataValue(metadataPaths, comparablePath(task.WorktreePath))
	}

	var orphaned []contracts.WorktreeRecord
	for _, worktree := range worktrees {
		if !strings.HasPrefix(worktree.Branch, "task/") {
			continue
		}
		if metadataBranches[worktree.Branch] || metadataPaths[comparablePath(worktree.Path)] {
			continue
		}
		orphaned = append(orphaned, worktree)
	}
	sort.Slice(orphaned, func(i, j int) bool {
		if orphaned[i].Path == orphaned[j].Path {
			return orphaned[i].Branch < orphaned[j].Branch
		}
		return orphaned[i].Path < orphaned[j].Path
	})
	return orphaned
}

func orphanedTaskBranches(tasks []contracts.TaskSummary, worktrees []contracts.WorktreeRecord, branches []string) []contracts.WorktreeRecord {
	metadataBranches := map[string]bool{}
	for _, task := range tasks {
		addMetadataValue(metadataBranches, task.Branch)
		if task.Worktree != nil {
			addMetadataValue(metadataBranches, task.Worktree.Branch)
		}
	}

	checkedOutBranches := map[string]bool{}
	for _, worktree := range worktrees {
		addMetadataValue(checkedOutBranches, worktree.Branch)
	}

	var orphaned []contracts.WorktreeRecord
	for _, branch := range branches {
		branch = strings.TrimSpace(branch)
		if !strings.HasPrefix(branch, "task/") {
			continue
		}
		if checkedOutBranches[branch] || metadataBranches[branch] {
			continue
		}
		orphaned = append(orphaned, contracts.WorktreeRecord{Branch: branch})
	}
	sort.Slice(orphaned, func(i, j int) bool {
		return orphaned[i].Branch < orphaned[j].Branch
	})
	return orphaned
}

func addMetadataValue(values map[string]bool, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		values[value] = true
	}
}

func comparablePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return strings.ToLower(filepath.Clean(path))
}

func cleanupCandidatesForOrphanedTaskWorktrees(worktrees []contracts.WorktreeRecord) []contracts.CleanupCandidate {
	candidates := make([]contracts.CleanupCandidate, 0, len(worktrees))
	removable := false
	for _, worktree := range worktrees {
		candidates = append(candidates, contracts.CleanupCandidate{
			ID:                 "orphan-worktree:" + cleanupSafeID(worktree.Branch, worktree.Path),
			Severity:           "warning",
			Category:           "requires-inspection",
			Path:               worktree.Path,
			Branch:             worktree.Branch,
			Dirty:              false,
			DirtyReasons:       []string{"dirty status was not inspected by native runtime reader"},
			SuggestedCommands:  []string{fmt.Sprintf("git -C %s status --short", quoteCommandArg(worktree.Path)), fmt.Sprintf("git -C %s diff --stat", quoteCommandArg(worktree.Path))},
			RemovableByExecute: &removable,
		})
	}
	return candidates
}

func cleanupCandidatesForOrphanedTaskBranches(branches []contracts.WorktreeRecord) []contracts.CleanupCandidate {
	candidates := make([]contracts.CleanupCandidate, 0, len(branches))
	destructiveIfUnmerged := true
	for _, branch := range branches {
		candidates = append(candidates, contracts.CleanupCandidate{
			ID:                    "orphan-branch:" + cleanupSafeID(branch.Branch, ""),
			Severity:              "warning",
			Category:              "destructive-if-removed",
			Branch:                branch.Branch,
			SuggestedCommands:     []string{fmt.Sprintf("git branch -D %s", quoteCommandArg(branch.Branch))},
			DestructiveIfUnmerged: &destructiveIfUnmerged,
		})
	}
	return candidates
}

func cleanupSummary(worktreeCandidates []contracts.CleanupCandidate, branchCandidates []contracts.CleanupCandidate) *contracts.CleanupSummary {
	bySeverity := map[string]int{}
	byCategory := map[string]int{}
	candidates := append([]contracts.CleanupCandidate{}, worktreeCandidates...)
	candidates = append(candidates, branchCandidates...)
	for _, candidate := range candidates {
		bySeverity[candidate.Severity]++
		byCategory[candidate.Category]++
	}
	removableByExecuteCount := 0
	for _, candidate := range candidates {
		if candidate.RemovableByExecute != nil && *candidate.RemovableByExecute {
			removableByExecuteCount++
		}
	}
	return &contracts.CleanupSummary{
		TotalCandidates:           len(worktreeCandidates) + len(branchCandidates),
		RequiresInspectionCount:   byCategory["requires-inspection"],
		RemovableByExecuteCount:   removableByExecuteCount,
		OrphanedTaskWorktreeCount: len(worktreeCandidates),
		OrphanedTaskBranchCount:   len(branchCandidates),
		BySeverity:                bySeverity,
		ByCategory:                byCategory,
	}
}

func cleanupSafeID(branch string, path string) string {
	value := strings.TrimSpace(branch)
	if value == "" {
		value = comparablePath(path)
	}
	replacer := strings.NewReplacer("\\", "-", "/", "-", ":", "-", " ", "-")
	return strings.Trim(replacer.Replace(value), "-")
}

func quoteCommandArg(value string) string {
	if strings.TrimSpace(value) == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \t\"") {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return value
}

func attachLatestRuns(tasks []contracts.TaskSummary, latestRuns map[string]state.RunRecord, runCounts map[string]int) {
	for index := range tasks {
		run := latestRuns[tasks[index].Slug]
		if runCounts[tasks[index].Slug] > 0 {
			tasks[index].RunCount = runCounts[tasks[index].Slug]
		}
		if len(run.Raw) == 0 {
			continue
		}
		tasks[index].LatestRun = run.Raw
		tasks[index].LatestRunID = run.RunID
		tasks[index].LatestRunLogPath = run.LogPath
		tasks[index].LatestRunProvider = run.Provider
		tasks[index].LatestRunProfile = run.Profile
		tasks[index].LatestRunWorkerStatus = run.WorkerStatus
		tasks[index].LatestRunExitCode = run.ExitCode
		tasks[index].LatestRunStartedAt = run.StartedAt
		tasks[index].LatestRunFinishedAt = run.FinishedAt
		tasks[index].LatestRunFailureType = run.FailureType
		tasks[index].LatestRunIncomplete = run.Incomplete
		tasks[index].LatestRunStale = run.Stale
		tasks[index].LatestRunAgeMinutes = run.RunAgeMinutes
		tasks[index].LatestRunSource = run.Source
		tasks[index].LastRunID = firstNonEmpty(tasks[index].LastRunID, run.RunID)
		tasks[index].LastLogPath = firstNonEmpty(tasks[index].LastLogPath, run.LogPath)
		tasks[index].LastProvider = firstNonEmpty(tasks[index].LastProvider, run.Provider)
		tasks[index].LastProfile = firstNonEmpty(tasks[index].LastProfile, run.Profile)
		tasks[index].LastExitCode = firstNonNil(tasks[index].LastExitCode, run.ExitCode)
		tasks[index].WorkerStatus = firstNonEmpty(tasks[index].WorkerStatus, run.WorkerStatus)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func summarizeProviders(health map[string]contracts.ProviderHealth) contracts.ProviderSummary {
	summary := contracts.ProviderSummary{Total: len(health)}
	for _, provider := range health {
		switch strings.ToLower(strings.TrimSpace(provider.Status)) {
		case "degraded", "capacity-degraded":
			summary.Degraded++
		case "unavailable", "down", "offline":
			summary.Unavailable++
		}
	}
	return summary
}

func countTasks(tasks []contracts.TaskSummary) contracts.TaskCounts {
	counts := contracts.TaskCounts{Tracked: len(tasks)}
	for _, task := range tasks {
		state := normalizedTaskState(task)
		switch state {
		case "ready-for-worker", "ready", "runnable":
			counts.Runnable++
		case "blocked":
			counts.Blocked++
		case "stale":
			counts.Stale++
		case "provider-gated":
			counts.ProviderGated++
		case "review", "needs-review":
			counts.Review++
		}
	}
	return counts
}

func groupTasks(tasks []contracts.TaskSummary) map[string]any {
	groups := map[string][]string{}
	for _, task := range tasks {
		state := normalizedTaskState(task)
		if state == "" {
			state = "unknown"
		}
		groups[state] = append(groups[state], task.Slug)
	}
	result := map[string]any{}
	for key, slugs := range groups {
		sort.Strings(slugs)
		result[key] = slugs
	}
	return result
}

func normalizedTaskState(task contracts.TaskSummary) string {
	if strings.TrimSpace(task.NormalizedState) != "" {
		return strings.ToLower(strings.TrimSpace(task.NormalizedState))
	}
	return strings.ToLower(strings.TrimSpace(task.Status))
}
