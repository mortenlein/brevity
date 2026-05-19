package runtimeclient

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
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

	state := contracts.RuntimeState{
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
		SuggestedNextActions: []string{
			"Native Go runtime reader is experimental and partial; PowerShell remains source of truth.",
		},
	}

	providers, missingProviderHealth, err := readProviderHealth(filepath.Join(repoRoot, ".brevity", "provider-health.json"))
	if err != nil {
		return contracts.RuntimeState{}, err
	}
	state.Providers.Health = providers
	state.Providers.Summary = summarizeProviders(providers)
	if missingProviderHealth {
		state.SuggestedNextActions = append(state.SuggestedNextActions, "No .brevity\\provider-health.json found.")
	}

	tasks, missingTasks, err := readTasks(filepath.Join(repoRoot, ".brevity", "tasks.json"))
	if err != nil {
		return contracts.RuntimeState{}, err
	}
	if missingTasks {
		state.SuggestedNextActions = append(state.SuggestedNextActions, "No .brevity\\tasks.json found.")
	}

	latestRuns, missingRuns, err := readLatestRuns(filepath.Join(repoRoot, ".brevity", "runs.jsonl"))
	if err != nil {
		return contracts.RuntimeState{}, err
	}
	if missingRuns {
		state.SuggestedNextActions = append(state.SuggestedNextActions, ".brevity\\runs.jsonl is absent; latest run data is unavailable.")
	}
	attachLatestRuns(tasks, latestRuns)

	state.Tasks = tasks
	state.TaskCounts = countTasks(tasks)
	state.Groups = groupTasks(tasks)

	worktrees, err := readGitWorktrees(repoRoot)
	if err != nil {
		state.SuggestedNextActions = append(state.SuggestedNextActions, fmt.Sprintf("Native worktree scan skipped: %v.", err))
	} else {
		state.ActiveWorktrees = worktrees
		state.ActiveWorktreeCount = len(worktrees)
		state.OrphanedTaskWorktrees = orphanedTaskWorktrees(tasks, worktrees)
		branches, err := readGitBranches(repoRoot)
		if err != nil {
			state.SuggestedNextActions = append(state.SuggestedNextActions, fmt.Sprintf("Native branch scan skipped: %v.", err))
		}
		orphanedBranches := orphanedTaskBranches(tasks, worktrees, branches)
		worktreeCandidates := cleanupCandidatesForOrphanedTaskWorktrees(state.OrphanedTaskWorktrees)
		branchCandidates := cleanupCandidatesForOrphanedTaskBranches(orphanedBranches)
		if len(worktreeCandidates)+len(branchCandidates) > 0 {
			state.Cleanup = &contracts.Cleanup{
				Summary:               cleanupSummary(worktreeCandidates, branchCandidates),
				OrphanedTaskWorktrees: worktreeCandidates,
				OrphanedTaskBranches:  branchCandidates,
			}
			state.SuggestedNextActions = append(state.SuggestedNextActions, "Review native orphaned task cleanup findings with PowerShell before cleanup; native Go remains read-only.")
		}
	}

	return state, nil
}

func (client NativeClient) DoctorJSON() ([]byte, error) { return nil, nativeUnsupported("doctor") }
func (client NativeClient) ProviderSetJSON(provider string, status string) ([]byte, error) {
	return nil, nativeUnsupported("provider set")
}
func (client NativeClient) ProviderResetJSON(provider string) ([]byte, error) {
	return nil, nativeUnsupported("provider reset")
}
func (client NativeClient) TaskContextRefreshJSON(slug string) ([]byte, error) {
	return nil, nativeUnsupported("task context refresh")
}
func (client NativeClient) TaskCleanupJSON(slug string) ([]byte, error) {
	return nil, nativeUnsupported("task cleanup")
}
func (client NativeClient) TaskNewJSON(slug string) ([]byte, error) {
	return nil, nativeUnsupported("task new")
}
func (client NativeClient) TaskRunJSON(slug string, profile string, smoke bool) ([]byte, error) {
	return nil, nativeUnsupported("task run")
}
func (client NativeClient) TaskRuntimeInfoJSON(slug string) ([]byte, error) {
	return nil, nativeUnsupported("task runtime-info")
}
func (client NativeClient) TaskRunsJSON(slug string) ([]byte, error) {
	return nil, nativeUnsupported("task runs")
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

func readProviderHealth(path string) (map[string]contracts.ProviderHealth, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]contracts.ProviderHealth{}, true, nil
		}
		return nil, false, fmt.Errorf("read provider health: %w", err)
	}
	var health map[string]contracts.ProviderHealth
	if err := json.Unmarshal(data, &health); err != nil {
		return nil, false, fmt.Errorf("parse provider health: %w", err)
	}
	if health == nil {
		health = map[string]contracts.ProviderHealth{}
	}
	return health, false, nil
}

func readTasks(path string) ([]contracts.TaskSummary, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []contracts.TaskSummary{}, true, nil
		}
		return nil, false, fmt.Errorf("read tasks: %w", err)
	}
	var tasks []contracts.TaskSummary
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, false, fmt.Errorf("parse tasks: %w", err)
	}
	if tasks == nil {
		tasks = []contracts.TaskSummary{}
	}
	return tasks, false, nil
}

func readLatestRuns(path string) (map[string]json.RawMessage, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]json.RawMessage{}, true, nil
		}
		return nil, false, fmt.Errorf("read runs index: %w", err)
	}
	defer file.Close()

	latest := map[string]json.RawMessage{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var values map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &values); err != nil {
			return nil, false, fmt.Errorf("parse runs index line: %w", err)
		}
		slug := rawString(values, "slug")
		if slug == "" {
			continue
		}
		latest[slug] = append(json.RawMessage(nil), []byte(line)...)
	}
	if err := scanner.Err(); err != nil {
		return nil, false, fmt.Errorf("scan runs index: %w", err)
	}
	return latest, false, nil
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

func attachLatestRuns(tasks []contracts.TaskSummary, latestRuns map[string]json.RawMessage) {
	for index := range tasks {
		run := latestRuns[tasks[index].Slug]
		if len(run) == 0 {
			continue
		}
		tasks[index].LatestRun = run
		var values map[string]json.RawMessage
		if err := json.Unmarshal(run, &values); err != nil {
			continue
		}
		tasks[index].LatestRunID = rawString(values, "runId")
		tasks[index].LatestRunLogPath = rawString(values, "logPath")
		tasks[index].LatestRunProvider = rawString(values, "provider")
		tasks[index].LatestRunProfile = rawString(values, "profile")
		tasks[index].LatestRunWorkerStatus = rawString(values, "workerStatus")
		if exit, ok := values["exitCode"]; ok {
			var value any
			if err := json.Unmarshal(exit, &value); err == nil {
				tasks[index].LatestRunExitCode = value
			}
		}
	}
}

func rawString(values map[string]json.RawMessage, key string) string {
	raw, ok := values[key]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}
	return ""
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
