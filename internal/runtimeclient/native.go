package runtimeclient

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
		Tasks:  []contracts.TaskSummary{},
		Groups: map[string]any{},
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
