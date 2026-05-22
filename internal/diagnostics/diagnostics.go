package diagnostics

import (
	"bytes"
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
	"github.com/mortenlein/brevity/internal/runmaintenance"
	"github.com/mortenlein/brevity/internal/state"
)

type Status string

const (
	StatusOK      Status = "ok"
	StatusWarn    Status = "warn"
	StatusError   Status = "error"
	StatusSkipped Status = "skipped"
)

type Severity string

const (
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

type Check struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Status          Status   `json:"status"`
	Severity        Severity `json:"severity"`
	Message         string   `json:"message"`
	Detail          string   `json:"detail,omitempty"`
	Path            string   `json:"path,omitempty"`
	SuggestedAction string   `json:"suggestedAction,omitempty"`
	Source          string   `json:"source"`
	Timestamp       string   `json:"timestamp,omitempty"`
}

type Report struct {
	Schema               string                  `json:"schema"`
	RepoRoot             string                  `json:"repoRoot"`
	GeneratedAt          string                  `json:"generatedAt"`
	Checks               []Check                 `json:"checks"`
	Summary              Summary                 `json:"summary"`
	SuggestedNextActions []string                `json:"suggestedNextActions"`
	LegacyPayload        contracts.DoctorPayload `json:"legacyPayload"`
}

type Summary struct {
	OK      int `json:"ok"`
	Warn    int `json:"warn"`
	Error   int `json:"error"`
	Skipped int `json:"skipped"`
}

type Options struct {
	RepoRoot string
	Now      func() time.Time
	LookPath func(string) (string, error)
}

type brevityConfig struct {
	ProjectName   string `json:"projectName"`
	DevRoot       string `json:"devRoot"`
	VaultPath     string `json:"vaultPath"`
	WorktreesRoot string `json:"worktreesRoot"`
}

const Schema = "brevity.doctor.v1"

func NormalizeStatus(value string) Status {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ok", "pass", "passed", "healthy":
		return StatusOK
	case "warn", "warning":
		return StatusWarn
	case "error", "err", "failed", "fail":
		return StatusError
	case "skipped", "skip":
		return StatusSkipped
	default:
		return StatusWarn
	}
}

func NormalizeSeverity(value string) Severity {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "error", "err", "fatal":
		return SeverityError
	case "warn", "warning":
		return SeverityWarn
	default:
		return SeverityInfo
	}
}

func SeverityRank(severity Severity) int {
	switch severity {
	case SeverityError:
		return 3
	case SeverityWarn:
		return 2
	default:
		return 1
	}
}

func Run(options Options) (Report, error) {
	repoRoot := options.RepoRoot
	if repoRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			return Report{}, fmt.Errorf("get working directory: %w", err)
		}
		repoRoot = wd
	}
	now := time.Now
	if options.Now != nil {
		now = options.Now
	}
	lookPath := exec.LookPath
	if options.LookPath != nil {
		lookPath = options.LookPath
	}
	generatedAt := now().UTC().Format(time.RFC3339)
	store, err := state.NewStore(repoRoot)
	if err != nil {
		return Report{}, err
	}

	builder := reportBuilder{timestamp: generatedAt}
	if gitPath, err := lookPath("git"); err != nil {
		builder.add("git-executable", "Git executable", StatusWarn, SeverityWarn, "Git was not found on PATH.", "", "", "Install Git or add it to PATH before using worktree diagnostics.", "native-go")
	} else {
		builder.add("git-executable", "Git executable", StatusOK, SeverityInfo, "Git is available.", gitPath, gitPath, "", "native-go")
	}

	if info, err := os.Stat(repoRoot); err != nil {
		builder.add("repo-root", "Repository root", StatusError, SeverityError, "Repository root is not readable.", err.Error(), repoRoot, "Run Brevity from a readable repository root.", "native-go")
	} else if !info.IsDir() {
		builder.add("repo-root", "Repository root", StatusError, SeverityError, "Repository root is not a directory.", "", repoRoot, "Run Brevity from a repository directory.", "native-go")
	} else {
		builder.add("repo-root", "Repository root", StatusOK, SeverityInfo, "Repository root is readable.", "", repoRoot, "", "native-go")
	}

	checkDir(&builder, "brevity-directory", ".brevity directory", store.BrevityRoot(), true)
	config, configOK := checkConfig(&builder, store)
	checkProviderHealth(&builder, store)
	tasks, tasksOK := checkTasks(&builder, store)
	checkRuns(&builder, store, now().UTC())
	checkRunMaintenance(&builder, store, now().UTC())
	checkLock(&builder, store, "state-lock", "State lock", store.LockPath())
	checkLock(&builder, store, "tasks-lock", "Task metadata lock", store.Path("tasks.lock"))
	checkLock(&builder, store, "provider-health-lock", "Provider health lock", store.Path("provider-health.lock"))
	if configOK {
		checkConfiguredPath(&builder, "vault-path", "Vault path", config.VaultPath, false)
		checkConfiguredPath(&builder, "worktrees-root", "Worktrees root", config.WorktreesRoot, false)
	} else {
		builder.add("vault-path", "Vault path", StatusSkipped, SeverityInfo, "Vault path check skipped because config is unavailable.", "", "", "Make .brevity\\config.json readable to inspect vault configuration.", "native-go")
		builder.add("worktrees-root", "Worktrees root", StatusSkipped, SeverityInfo, "Worktrees root check skipped because config is unavailable.", "", "", "Make .brevity\\config.json readable to inspect worktree configuration.", "native-go")
	}
	checkTaskWorktrees(&builder, tasks, tasksOK)

	report := Report{
		Schema:      Schema,
		RepoRoot:    repoRoot,
		GeneratedAt: generatedAt,
		Checks:      builder.checks,
		Summary:     builder.summary(),
	}
	report.SuggestedNextActions = suggestedActions(report.Checks)
	report.LegacyPayload = legacyPayload(report, tasks)
	return report, nil
}

func checkRunMaintenance(builder *reportBuilder, store state.Store, now time.Time) {
	plan, _, err := runmaintenance.BuildRunMaintenancePlan(runmaintenance.RunMaintenanceOptions{
		Store: store,
		Now:   func() time.Time { return now },
	})
	path := store.Path(state.RunsFile)
	if err != nil {
		builder.add("runs-maintenance", "Run history maintenance", StatusWarn, SeverityWarn, "Run maintenance plan could not be built.", err.Error(), path, "Inspect .brevity\\runs.jsonl before compacting run history.", "native-go")
		return
	}
	if plan.MissingRunsFile {
		builder.add("runs-maintenance", "Run history maintenance", StatusSkipped, SeverityInfo, "Run maintenance skipped because run history is missing.", "", path, "", "native-go")
		return
	}
	if len(plan.MalformedRows)+len(plan.DuplicateRunIDs)+len(plan.StaleIncompleteRuns) > 0 {
		detail := fmt.Sprintf("malformed=%d duplicateRunIds=%d staleIncomplete=%d missingLogs=%d", len(plan.MalformedRows), len(plan.DuplicateRunIDs), len(plan.StaleIncompleteRuns), len(plan.MissingLogReferences))
		builder.add("runs-maintenance", "Run history maintenance", StatusWarn, SeverityWarn, "Run history maintenance found issues.", detail, path, "Run brevity runs inspect before relying on latest run summaries.", "native-go")
		return
	}
	builder.add("runs-maintenance", "Run history maintenance", StatusOK, SeverityInfo, "Run history maintenance found no compaction warnings.", "", path, "", "native-go")
}

type reportBuilder struct {
	checks    []Check
	timestamp string
}

func (builder *reportBuilder) add(id, title string, status Status, severity Severity, message, detail, path, action, source string) {
	builder.checks = append(builder.checks, Check{
		ID:              id,
		Title:           title,
		Status:          status,
		Severity:        severity,
		Message:         message,
		Detail:          detail,
		Path:            path,
		SuggestedAction: action,
		Source:          source,
		Timestamp:       builder.timestamp,
	})
}

func (builder reportBuilder) summary() Summary {
	var summary Summary
	for _, check := range builder.checks {
		switch check.Status {
		case StatusOK:
			summary.OK++
		case StatusWarn:
			summary.Warn++
		case StatusError:
			summary.Error++
		case StatusSkipped:
			summary.Skipped++
		}
	}
	return summary
}

func checkDir(builder *reportBuilder, id, title, path string, critical bool) {
	info, err := os.Stat(path)
	if err != nil {
		status, severity := optionalStatus(critical)
		builder.add(id, title, status, severity, "Directory is not readable.", err.Error(), path, "Run brevity init or onboard when mutation is appropriate.", "native-go")
		return
	}
	if !info.IsDir() {
		builder.add(id, title, StatusError, SeverityError, "Path exists but is not a directory.", "", path, "Move or replace this path before using Brevity runtime state.", "native-go")
		return
	}
	builder.add(id, title, StatusOK, SeverityInfo, "Directory is readable.", "", path, "", "native-go")
}

func checkConfig(builder *reportBuilder, store state.Store) (brevityConfig, bool) {
	path := store.Path("config.json")
	var config brevityConfig
	data, err := os.ReadFile(path)
	if err != nil {
		builder.add("config-readable", "Config readable", StatusWarn, SeverityWarn, "Config file is missing or unreadable.", err.Error(), path, "Run brevity init or onboard when mutation is appropriate.", "native-go")
		return config, false
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if err := json.Unmarshal(data, &config); err != nil {
		builder.add("config-readable", "Config readable", StatusError, SeverityError, "Config file is malformed.", err.Error(), path, "Fix .brevity\\config.json before relying on runtime inspection.", "native-go")
		return config, false
	}
	builder.add("config-readable", "Config readable", StatusOK, SeverityInfo, "Config file is readable.", "", path, "", "native-go")
	return config, true
}

func checkProviderHealth(builder *reportBuilder, store state.Store) {
	health, missing, err := state.LoadProviderHealth(store)
	path := store.Path(state.ProviderHealthFile)
	if err != nil {
		builder.add("provider-health-readable", "Provider health readable", StatusError, SeverityError, "Provider health file is malformed or unreadable.", err.Error(), path, "Fix .brevity\\provider-health.json or reset provider state when mutation is appropriate.", "native-go")
		return
	}
	if missing {
		builder.add("provider-health-readable", "Provider health readable", StatusWarn, SeverityWarn, "Provider health file is missing.", "", path, "Run provider status or initialize provider health when mutation is appropriate.", "native-go")
		return
	}
	summary := summarizeProviders(health)
	status, severity, message := StatusOK, SeverityInfo, "Provider health file is readable."
	if summary.Degraded+summary.Unavailable > 0 {
		status, severity, message = StatusWarn, SeverityWarn, "One or more providers are degraded or unavailable."
	}
	builder.add("provider-health-readable", "Provider health readable", status, severity, message, fmt.Sprintf("providers=%d degraded=%d unavailable=%d", summary.Total, summary.Degraded, summary.Unavailable), path, "Run brevity provider status before launching more work.", "native-go")
}

func checkTasks(builder *reportBuilder, store state.Store) (state.Tasks, bool) {
	tasks, missing, err := state.LoadTasks(store)
	path := store.Path(state.TasksFile)
	if err != nil {
		builder.add("tasks-readable", "Task metadata readable", StatusError, SeverityError, "Task metadata file is malformed or unreadable.", err.Error(), path, "Fix .brevity\\tasks.json before relying on task inspection.", "native-go")
		return state.Tasks{}, false
	}
	if missing {
		builder.add("tasks-readable", "Task metadata readable", StatusWarn, SeverityWarn, "Task metadata file is missing.", "", path, "No tasks are tracked until task metadata exists.", "native-go")
		return tasks, true
	}
	builder.add("tasks-readable", "Task metadata readable", StatusOK, SeverityInfo, fmt.Sprintf("Task metadata is readable: %d tracked.", len(tasks.Items)), "", path, "", "native-go")
	return tasks, true
}

func checkRuns(builder *reportBuilder, store state.Store, now time.Time) {
	history, missing, err := state.LoadRuns(store, now)
	path := store.Path(state.RunsFile)
	if err != nil {
		builder.add("runs-readable", "Run history readable", StatusError, SeverityError, "Run history file is malformed or unreadable.", err.Error(), path, "Fix malformed .brevity\\runs.jsonl rows before relying on latest run data.", "native-go")
		return
	}
	if missing {
		builder.add("runs-readable", "Run history readable", StatusWarn, SeverityWarn, "Run history file is missing.", "", path, "Latest run data is unavailable until workers record runs.", "native-go")
		return
	}
	builder.add("runs-readable", "Run history readable", StatusOK, SeverityInfo, fmt.Sprintf("Run history is readable: %d records.", len(history.Items)), "", path, "", "native-go")
}

func checkLock(builder *reportBuilder, store state.Store, id, title, path string) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			builder.add(id, title, StatusOK, SeverityInfo, "No lock file is present.", "", path, "", "native-go")
			return
		}
		builder.add(id, title, StatusWarn, SeverityWarn, "Lock status could not be read.", err.Error(), path, "Inspect the lock path manually.", "native-go")
		return
	}
	age := time.Since(info.ModTime()).Round(time.Second)
	builder.add(id, title, StatusWarn, SeverityWarn, "Lock file is present.", "age="+age.String(), path, "Confirm no Brevity process is active before any manual cleanup.", "native-go")
}

func checkConfiguredPath(builder *reportBuilder, id, title, path string, critical bool) {
	if strings.TrimSpace(path) == "" {
		status, severity := optionalStatus(critical)
		builder.add(id, title, status, severity, "Path is not configured.", "", "", "Set this path through init/onboard when mutation is appropriate.", "native-go")
		return
	}
	checkDir(builder, id, title, path, critical)
}

func checkTaskWorktrees(builder *reportBuilder, tasks state.Tasks, ok bool) {
	if !ok {
		builder.add("task-worktrees", "Task worktrees", StatusSkipped, SeverityInfo, "Task worktree check skipped because task metadata is unavailable.", "", "", "Fix task metadata first.", "native-go")
		return
	}
	missing := []string{}
	for _, task := range tasks.Items {
		path := task.WorktreePath
		if path == "" && task.Worktree != nil {
			path = task.Worktree.Path
		}
		if strings.TrimSpace(path) == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			missing = append(missing, task.Key()+": "+path)
		}
	}
	if len(missing) == 0 {
		builder.add("task-worktrees", "Task worktrees", StatusOK, SeverityInfo, "Tracked task worktree paths are readable or absent from metadata.", "", "", "", "native-go")
		return
	}
	sort.Strings(missing)
	builder.add("task-worktrees", "Task worktrees", StatusWarn, SeverityWarn, fmt.Sprintf("%d tracked task worktree path(s) are missing.", len(missing)), strings.Join(missing, "; "), "", "Use task detail/status to inspect stale task metadata.", "native-go")
}

func optionalStatus(critical bool) (Status, Severity) {
	if critical {
		return StatusError, SeverityError
	}
	return StatusWarn, SeverityWarn
}

func summarizeProviders(health state.ProviderHealthState) contracts.ProviderSummary {
	summary := contracts.ProviderSummary{Total: len(health)}
	for _, provider := range health {
		switch strings.ToLower(strings.TrimSpace(string(provider.Status))) {
		case "capacity-degraded", "quota-constrained":
			summary.Degraded++
		case "unavailable":
			summary.Unavailable++
		}
	}
	return summary
}

func suggestedActions(checks []Check) []string {
	actions := []string{}
	seen := map[string]bool{}
	for _, check := range checks {
		if check.Status != StatusError && check.Status != StatusWarn {
			continue
		}
		action := strings.TrimSpace(check.SuggestedAction)
		if action == "" || seen[action] {
			continue
		}
		seen[action] = true
		actions = append(actions, action)
	}
	if len(actions) == 0 {
		return []string{"No immediate doctor action suggested."}
	}
	return actions
}

func legacyPayload(report Report, _ state.Tasks) contracts.DoctorPayload {
	return contracts.DoctorPayload{
		WarningCount: report.Summary.Warn,
		ErrorCount:   report.Summary.Error,
		Providers: contracts.DoctorProviders{
			Summary: providerSummaryFromChecks(report.Checks),
		},
		WorktreeCounts: contracts.DoctorWorktreeCounts{
			OrphanedTaskWorktrees: 0,
		},
		BranchCounts: contracts.DoctorBranchCounts{
			Orphaned: 0,
		},
		Lock: contracts.DoctorLock{
			Exists: lockPresent(report.Checks),
			Path:   filepath.Join(report.RepoRoot, state.DirectoryName, "tasks.lock"),
		},
		SuggestedNextActions: report.SuggestedNextActions,
	}
}

func providerSummaryFromChecks(checks []Check) contracts.ProviderSummary {
	for _, check := range checks {
		if check.ID != "provider-health-readable" || !strings.Contains(check.Detail, "providers=") {
			continue
		}
		var summary contracts.ProviderSummary
		fmt.Sscanf(check.Detail, "providers=%d degraded=%d unavailable=%d", &summary.Total, &summary.Degraded, &summary.Unavailable)
		return summary
	}
	return contracts.ProviderSummary{}
}

func lockPresent(checks []Check) bool {
	for _, check := range checks {
		if strings.HasSuffix(check.ID, "-lock") && check.Status == StatusWarn {
			return true
		}
	}
	return false
}

func CommandResult(report Report) contracts.CommandResult {
	warnings := []contracts.ResultMessage{}
	errorsOut := []contracts.ResultMessage{}
	for _, check := range report.Checks {
		message := contracts.ResultMessage{
			Code:    check.ID,
			Message: check.Message,
			Details: map[string]any{
				"status":   string(check.Status),
				"severity": string(check.Severity),
				"path":     check.Path,
				"detail":   check.Detail,
			},
		}
		switch check.Status {
		case StatusWarn:
			warnings = append(warnings, message)
		case StatusError:
			errorsOut = append(errorsOut, message)
		}
	}
	payload, _ := json.Marshal(report)
	return contracts.CommandResult{
		Schema:               contracts.CommandResultSchema,
		Command:              "doctor",
		Success:              report.Summary.Error == 0,
		Severity:             commandSeverity(report.Summary),
		Warnings:             warnings,
		Errors:               errorsOut,
		SuggestedNextActions: report.SuggestedNextActions,
		Payload:              payload,
	}
}

func commandSeverity(summary Summary) string {
	if summary.Error > 0 {
		return "error"
	}
	if summary.Warn > 0 {
		return "warning"
	}
	return "info"
}
