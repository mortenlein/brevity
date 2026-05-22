package runmaintenance

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
	"github.com/mortenlein/brevity/internal/state"
	"github.com/mortenlein/brevity/internal/state/locking"
)

const RunMaintenancePlanSchema = "brevity.run-maintenance-plan.v1"

type RunMaintenancePlan struct {
	Schema                   string                    `json:"schema"`
	Version                  int                       `json:"version"`
	RunsPath                 string                    `json:"runsPath"`
	TotalRuns                int                       `json:"totalRuns"`
	ValidRuns                int                       `json:"validRuns"`
	MalformedRows            []RunMaintenanceRowIssue  `json:"malformedRows"`
	DuplicateRunIDs          []RunMaintenanceDuplicate `json:"duplicateRunIds"`
	IncompleteRuns           []RunMaintenanceRunIssue  `json:"incompleteRuns"`
	StaleIncompleteRuns      []RunMaintenanceRunIssue  `json:"staleIncompleteRuns"`
	MissingLogReferences     []RunMaintenanceRunIssue  `json:"missingLogReferences"`
	CompactableRows          []RunMaintenanceRowIssue  `json:"compactableRows"`
	WouldRewriteRunsFile     bool                      `json:"wouldRewriteRunsFile"`
	WouldDeleteLogs          bool                      `json:"wouldDeleteLogs"`
	Destructive              bool                      `json:"destructive"`
	RequiresForce            bool                      `json:"requiresForce"`
	Blockers                 []contracts.ResultMessage `json:"blockers"`
	Warnings                 []contracts.ResultMessage `json:"warnings"`
	GeneratedAt              string                    `json:"generatedAt"`
	StaleThresholdMinutes    int                       `json:"staleThresholdMinutes"`
	MissingRunsFile          bool                      `json:"missingRunsFile"`
	QuarantinePath           string                    `json:"quarantinePath,omitempty"`
	RetainedRowsAfterCompact int                       `json:"retainedRowsAfterCompact"`
}

type RunMaintenanceRowIssue struct {
	LineNumber int    `json:"lineNumber"`
	RunID      string `json:"runId,omitempty"`
	Slug       string `json:"slug,omitempty"`
	Reason     string `json:"reason"`
}

type RunMaintenanceDuplicate struct {
	RunID           string `json:"runId"`
	RetainedLine    int    `json:"retainedLine"`
	DuplicateLines  []int  `json:"duplicateLines"`
	RetainedStarted string `json:"retainedStartedAt,omitempty"`
}

type RunMaintenanceRunIssue struct {
	LineNumber    int      `json:"lineNumber"`
	RunID         string   `json:"runId,omitempty"`
	Slug          string   `json:"slug,omitempty"`
	WorkerStatus  string   `json:"workerStatus,omitempty"`
	LogPath       string   `json:"logPath,omitempty"`
	RunAgeMinutes *float64 `json:"runAgeMinutes,omitempty"`
	Reason        string   `json:"reason"`
}

type RunMaintenanceOptions struct {
	Store state.Store
	Now   func() time.Time
}

type runMaintenanceRow struct {
	lineNumber int
	raw        []byte
	record     state.RunRecord
	malformed  bool
	err        error
}

func BuildRunMaintenancePlan(options RunMaintenanceOptions) (RunMaintenancePlan, []runMaintenanceRow, error) {
	store := options.Store
	if store.RepoRoot == "" {
		var err error
		store, err = state.NewStore("")
		if err != nil {
			return RunMaintenancePlan{}, nil, err
		}
	}
	now := time.Now
	if options.Now != nil {
		now = options.Now
	}
	generatedAt := now().UTC().Format(time.RFC3339)
	runsPath := store.Path(state.RunsFile)
	plan := RunMaintenancePlan{
		Schema:                RunMaintenancePlanSchema,
		Version:               1,
		RunsPath:              runsPath,
		GeneratedAt:           generatedAt,
		StaleThresholdMinutes: state.WorkerRunStaleThresholdMinutes,
		Warnings:              []contracts.ResultMessage{},
		Blockers:              []contracts.ResultMessage{},
		MalformedRows:         []RunMaintenanceRowIssue{},
		DuplicateRunIDs:       []RunMaintenanceDuplicate{},
		IncompleteRuns:        []RunMaintenanceRunIssue{},
		StaleIncompleteRuns:   []RunMaintenanceRunIssue{},
		MissingLogReferences:  []RunMaintenanceRunIssue{},
		CompactableRows:       []RunMaintenanceRowIssue{},
	}

	rows, missing, err := scanRunMaintenanceRows(store, now().UTC())
	if err != nil {
		return RunMaintenancePlan{}, nil, err
	}
	if missing {
		plan.MissingRunsFile = true
		plan.Warnings = append(plan.Warnings, contracts.ResultMessage{Code: "runs-missing", Message: ".brevity/runs.jsonl is missing."})
		return plan, rows, nil
	}

	byID := map[string][]runMaintenanceRow{}
	retainedByID := map[string]runMaintenanceRow{}
	for _, row := range rows {
		plan.TotalRuns++
		if row.malformed {
			plan.MalformedRows = append(plan.MalformedRows, RunMaintenanceRowIssue{LineNumber: row.lineNumber, Reason: row.err.Error()})
			plan.CompactableRows = append(plan.CompactableRows, RunMaintenanceRowIssue{LineNumber: row.lineNumber, Reason: "malformed"})
			continue
		}
		plan.ValidRuns++
		record := row.record
		if record.Incomplete {
			issue := runIssue(row, "incomplete")
			plan.IncompleteRuns = append(plan.IncompleteRuns, issue)
			if record.Stale {
				issue.Reason = "stale-incomplete"
				plan.StaleIncompleteRuns = append(plan.StaleIncompleteRuns, issue)
			}
		}
		if missingLog(store, record.LogPath) {
			plan.MissingLogReferences = append(plan.MissingLogReferences, runIssue(row, "missing-log"))
		}
		runID := strings.TrimSpace(record.RunID)
		if runID != "" {
			byID[runID] = append(byID[runID], row)
			if retained, ok := retainedByID[runID]; !ok || rowIsNewer(row, retained) {
				retainedByID[runID] = row
			}
		}
	}

	for runID, grouped := range byID {
		if len(grouped) < 2 {
			continue
		}
		retained := retainedByID[runID]
		duplicateLines := []int{}
		for _, row := range grouped {
			if row.lineNumber != retained.lineNumber {
				duplicateLines = append(duplicateLines, row.lineNumber)
				plan.CompactableRows = append(plan.CompactableRows, RunMaintenanceRowIssue{LineNumber: row.lineNumber, RunID: runID, Slug: row.record.Slug, Reason: "duplicate-run-id"})
			}
		}
		sort.Ints(duplicateLines)
		plan.DuplicateRunIDs = append(plan.DuplicateRunIDs, RunMaintenanceDuplicate{RunID: runID, RetainedLine: retained.lineNumber, DuplicateLines: duplicateLines, RetainedStarted: retained.record.StartedAt})
	}
	sort.Slice(plan.DuplicateRunIDs, func(i, j int) bool { return plan.DuplicateRunIDs[i].RunID < plan.DuplicateRunIDs[j].RunID })
	sort.Slice(plan.CompactableRows, func(i, j int) bool { return plan.CompactableRows[i].LineNumber < plan.CompactableRows[j].LineNumber })

	plan.WouldRewriteRunsFile = len(plan.CompactableRows) > 0
	plan.Destructive = plan.WouldRewriteRunsFile
	plan.RequiresForce = plan.WouldRewriteRunsFile
	plan.RetainedRowsAfterCompact = plan.ValidRuns - duplicateCompactableCount(plan.CompactableRows)
	if len(plan.MalformedRows) > 0 {
		plan.QuarantinePath = store.Path("runs-malformed.jsonl")
		plan.Warnings = append(plan.Warnings, contracts.ResultMessage{Code: "malformed-runs", Message: "Malformed rows will be quarantined during forced compaction.", Count: len(plan.MalformedRows)})
	}
	if len(plan.DuplicateRunIDs) > 0 {
		plan.Warnings = append(plan.Warnings, contracts.ResultMessage{Code: "duplicate-run-ids", Message: "Duplicate run IDs were found.", Count: len(plan.DuplicateRunIDs)})
	}
	if len(plan.StaleIncompleteRuns) > 0 {
		plan.Warnings = append(plan.Warnings, contracts.ResultMessage{Code: "stale-incomplete-runs", Message: "Stale incomplete runs were found.", Count: len(plan.StaleIncompleteRuns)})
	}
	if len(plan.MissingLogReferences) > 0 {
		plan.Warnings = append(plan.Warnings, contracts.ResultMessage{Code: "missing-log-references", Message: "Some run records reference missing logs.", Count: len(plan.MissingLogReferences)})
	}

	return plan, rows, nil
}

func ExecuteRunCompaction(options RunMaintenanceOptions, force bool) (contracts.CommandResult, error) {
	plan, rows, err := BuildRunMaintenancePlan(options)
	if err != nil {
		return commandResult("runs compact", false, "error", nil, contracts.ResultMessage{Code: "runs-plan-failed", Message: err.Error()}), err
	}
	if plan.RequiresForce && !force {
		msg := contracts.ResultMessage{Code: "force-required", Message: "Run compaction requires --force."}
		return commandResult("runs compact", false, "error", plan, msg), fmt.Errorf("runs compact requires --force")
	}
	if plan.MissingRunsFile || !plan.WouldRewriteRunsFile {
		return commandResult("runs compact", true, "info", plan, contracts.ResultMessage{}), nil
	}

	store := options.Store
	if store.RepoRoot == "" {
		store, err = state.NewStore("")
		if err != nil {
			return commandResult("runs compact", false, "error", plan, contracts.ResultMessage{Code: "store-error", Message: err.Error()}), err
		}
	}
	lock, err := locking.Acquire(store.LockPath(), locking.Options{Timeout: 5 * time.Second})
	if err != nil {
		msg := contracts.ResultMessage{Code: "state-lock-timeout", Message: err.Error()}
		return commandResult("runs compact", false, "error", plan, msg), err
	}
	defer lock.Release()

	retained, malformed := compactRows(rows)
	if len(malformed) > 0 {
		if err := appendQuarantine(store.Path("runs-malformed.jsonl"), malformed); err != nil {
			msg := contracts.ResultMessage{Code: "quarantine-write-failed", Message: err.Error()}
			return commandResult("runs compact", false, "error", plan, msg), err
		}
	}
	if err := writeRunsAtomically(store, retained); err != nil {
		msg := contracts.ResultMessage{Code: "runs-write-failed", Message: err.Error()}
		return commandResult("runs compact", false, "error", plan, msg), err
	}
	return commandResult("runs compact", true, "info", plan, contracts.ResultMessage{}), nil
}

func scanRunMaintenanceRows(store state.Store, now time.Time) ([]runMaintenanceRow, bool, error) {
	file, err := os.Open(store.Path(state.RunsFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []runMaintenanceRow{}, true, nil
		}
		return nil, false, fmt.Errorf("read runs.jsonl: %w", err)
	}
	defer file.Close()

	rows := []runMaintenanceRow{}
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		row := runMaintenanceRow{lineNumber: lineNumber, raw: []byte(line)}
		if err := json.Unmarshal([]byte(line), &row.record); err != nil {
			row.malformed = true
			row.err = fmt.Errorf("parse runs.jsonl line %d: %w", lineNumber, err)
		} else {
			row.record.Raw = append([]byte(nil), []byte(line)...)
			row.record.Source = "index"
			normalizeRunRecord(&row.record, now)
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, false, fmt.Errorf("scan runs.jsonl: %w", err)
	}
	return rows, false, nil
}

func normalizeRunRecord(record *state.RunRecord, now time.Time) {
	history := state.RunHistory{Items: []state.RunRecord{*record}}
	_ = history
	if strings.TrimSpace(record.FinishedAt) == "" {
		record.Incomplete = true
		if t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(record.StartedAt)); err == nil {
			age := now.Sub(t.UTC()).Minutes()
			rounded := float64(int(age*100+0.5)) / 100
			record.RunAgeMinutes = &rounded
			if rounded >= state.WorkerRunStaleThresholdMinutes {
				record.Stale = true
				record.WorkerStatus = "stale"
				return
			}
		}
		if strings.TrimSpace(record.WorkerStatus) == "" {
			record.WorkerStatus = "running-unknown"
		}
		return
	}
	if strings.TrimSpace(record.WorkerStatus) == "" {
		if fmt.Sprint(record.ExitCode) == "0" {
			record.WorkerStatus = "succeeded"
		} else {
			record.WorkerStatus = "failed"
		}
	}
}

func missingLog(store state.Store, logPath string) bool {
	logPath = strings.TrimSpace(logPath)
	if logPath == "" {
		return false
	}
	if !filepath.IsAbs(logPath) {
		logPath = filepath.Join(store.RepoRoot, logPath)
	}
	_, err := os.Stat(logPath)
	return err != nil
}

func runIssue(row runMaintenanceRow, reason string) RunMaintenanceRunIssue {
	return RunMaintenanceRunIssue{LineNumber: row.lineNumber, RunID: row.record.RunID, Slug: row.record.Slug, WorkerStatus: row.record.WorkerStatus, LogPath: row.record.LogPath, RunAgeMinutes: row.record.RunAgeMinutes, Reason: reason}
}

func rowIsNewer(left, right runMaintenanceRow) bool {
	leftTime := firstRunSortTime(left.record)
	rightTime := firstRunSortTime(right.record)
	if leftTime == rightTime {
		return left.lineNumber > right.lineNumber
	}
	return leftTime > rightTime
}

func firstRunSortTime(record state.RunRecord) string {
	if strings.TrimSpace(record.FinishedAt) != "" {
		return record.FinishedAt
	}
	return record.StartedAt
}

func duplicateCompactableCount(rows []RunMaintenanceRowIssue) int {
	count := 0
	for _, row := range rows {
		if row.Reason == "duplicate-run-id" {
			count++
		}
	}
	return count
}

func compactRows(rows []runMaintenanceRow) ([][]byte, [][]byte) {
	retainedByID := map[string]runMaintenanceRow{}
	for _, row := range rows {
		if row.malformed {
			continue
		}
		runID := strings.TrimSpace(row.record.RunID)
		if runID == "" {
			continue
		}
		if retained, ok := retainedByID[runID]; !ok || rowIsNewer(row, retained) {
			retainedByID[runID] = row
		}
	}
	retained := [][]byte{}
	malformed := [][]byte{}
	for _, row := range rows {
		if row.malformed {
			malformed = append(malformed, row.raw)
			continue
		}
		runID := strings.TrimSpace(row.record.RunID)
		if runID != "" && retainedByID[runID].lineNumber != row.lineNumber {
			continue
		}
		retained = append(retained, row.raw)
	}
	return retained, malformed
}

func appendQuarantine(path string, rows [][]byte) error {
	if len(rows) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	for _, row := range rows {
		if _, err := file.Write(append(row, '\n')); err != nil {
			return err
		}
	}
	return file.Sync()
}

func writeRunsAtomically(store state.Store, rows [][]byte) error {
	if err := os.MkdirAll(store.BrevityRoot(), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(store.BrevityRoot(), state.RunsFile+".*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	for _, row := range rows {
		if _, err := temp.Write(append(row, '\n')); err != nil {
			_ = temp.Close()
			return err
		}
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, store.Path(state.RunsFile))
}

func commandResult(command string, success bool, severity string, payload any, resultErr contracts.ResultMessage) contracts.CommandResult {
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
	if resultErr.Code != "" || resultErr.Message != "" {
		result.Errors = append(result.Errors, resultErr)
	}
	if plan, ok := payload.(RunMaintenancePlan); ok {
		result.Warnings = append(result.Warnings, plan.Warnings...)
	}
	return result
}
