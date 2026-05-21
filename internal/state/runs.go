package state

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mortenlein/brevity/internal/state/locking"
)

const RunsFile = "runs.jsonl"

const WorkerRunStaleThresholdMinutes = 30

type RunHistory struct {
	Items []RunRecord
}

type RunRecord struct {
	RunID                string          `json:"runId,omitempty"`
	Slug                 string          `json:"slug,omitempty"`
	Provider             string          `json:"provider,omitempty"`
	Profile              string          `json:"profile,omitempty"`
	StartedAt            string          `json:"startedAt,omitempty"`
	FinishedAt           string          `json:"finishedAt,omitempty"`
	ExitCode             any             `json:"exitCode,omitempty"`
	WorkerStatus         string          `json:"workerStatus,omitempty"`
	FailureType          string          `json:"failureType,omitempty"`
	LogPath              string          `json:"logPath,omitempty"`
	StdoutPath           string          `json:"stdoutPath,omitempty"`
	StderrPath           string          `json:"stderrPath,omitempty"`
	Summary              string          `json:"summary,omitempty"`
	Message              string          `json:"message,omitempty"`
	RecordedWorkerStatus string          `json:"recordedWorkerStatus,omitempty"`
	Incomplete           bool            `json:"incomplete,omitempty"`
	Stale                bool            `json:"stale,omitempty"`
	RunAgeMinutes        *float64        `json:"runAgeMinutes,omitempty"`
	Source               string          `json:"source,omitempty"`
	Raw                  json.RawMessage `json:"-"`
	lineNumber           int
}

type AppendRunOptions struct {
	LockOptions locking.Options
}

func AppendRun(store Store, record RunRecord, options AppendRunOptions) error {
	lockOptions := options.LockOptions
	if lockOptions.Timeout == 0 {
		lockOptions.Timeout = 5 * time.Second
	}
	lock, err := locking.Acquire(store.LockPath(), lockOptions)
	if err != nil {
		return fmt.Errorf("runs metadata locked: %w", err)
	}
	defer lock.Release()

	if _, _, err := LoadRuns(store, time.Now().UTC()); err != nil {
		return err
	}
	if err := os.MkdirAll(store.BrevityRoot(), 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal run record: %w", err)
	}
	file, err := os.OpenFile(store.Path(RunsFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open runs.jsonl: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append runs.jsonl: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("flush runs.jsonl: %w", err)
	}
	return nil
}

func LoadRuns(store Store, now time.Time) (RunHistory, bool, error) {
	path := store.Path(RunsFile)
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RunHistory{Items: []RunRecord{}}, true, nil
		}
		return RunHistory{}, false, fmt.Errorf("read runs.jsonl: %w", err)
	}
	defer file.Close()

	var history RunHistory
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record RunRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return RunHistory{}, false, fmt.Errorf("parse runs.jsonl line %d: %w", lineNumber, err)
		}
		record.Raw = append(json.RawMessage(nil), []byte(line)...)
		record.lineNumber = lineNumber
		record.Source = firstNonEmpty(record.Source, "index")
		record.normalizeStatus(now)
		history.Items = append(history.Items, record)
	}
	if err := scanner.Err(); err != nil {
		return RunHistory{}, false, fmt.Errorf("scan runs.jsonl: %w", err)
	}
	history.SortLatestFirst()
	return history, false, nil
}

func (history *RunHistory) SortLatestFirst() {
	sort.SliceStable(history.Items, func(i, j int) bool {
		left := history.Items[i]
		right := history.Items[j]
		if left.sortTimestamp() == right.sortTimestamp() {
			if left.lineNumber == right.lineNumber {
				return left.RunID > right.RunID
			}
			return left.lineNumber > right.lineNumber
		}
		return left.sortTimestamp() > right.sortTimestamp()
	})
}

func (history RunHistory) LatestByTask() map[string]RunRecord {
	latest := map[string]RunRecord{}
	for _, run := range history.Items {
		slug := strings.TrimSpace(run.Slug)
		if slug == "" {
			continue
		}
		if _, exists := latest[slug]; !exists {
			latest[slug] = run
		}
	}
	return latest
}

func (history RunHistory) CountByTask() map[string]int {
	counts := map[string]int{}
	for _, run := range history.Items {
		slug := strings.TrimSpace(run.Slug)
		if slug != "" {
			counts[slug]++
		}
	}
	return counts
}

func (run RunRecord) sortTimestamp() string {
	return firstNonEmpty(run.FinishedAt, run.StartedAt)
}

func (run *RunRecord) normalizeStatus(now time.Time) {
	recorded := strings.TrimSpace(run.WorkerStatus)
	run.RecordedWorkerStatus = recorded
	status := recorded
	if strings.TrimSpace(run.FinishedAt) == "" {
		run.Incomplete = true
		switch status {
		case "":
			status = "running-unknown"
		case "running", "running-unknown", "incomplete", "stale":
		default:
			status = "incomplete"
		}
		if startedAt, ok := parseRunTime(run.StartedAt); ok {
			age := now.UTC().Sub(startedAt).Minutes()
			rounded := float64(int(age*100+0.5)) / 100
			run.RunAgeMinutes = &rounded
			if rounded >= WorkerRunStaleThresholdMinutes {
				run.Stale = true
				status = "stale"
			}
		} else if status == "running" {
			status = "running-unknown"
		}
	} else if status == "" {
		switch fmt.Sprint(run.ExitCode) {
		case "0":
			status = "succeeded"
		case "<nil>":
			status = "completed"
		default:
			status = "failed"
		}
	}
	run.WorkerStatus = status
}

func parseRunTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	timestamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false
	}
	return timestamp.UTC(), true
}
