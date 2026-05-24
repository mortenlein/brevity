package execution

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	runtimequeue "github.com/mortenlein/brevity/internal/runtime/queue"
	bstate "github.com/mortenlein/brevity/internal/state"
	"github.com/mortenlein/brevity/internal/state/locking"
)

const (
	FileName        = "runtime-executions.json"
	LockFile        = "runtime-executions.lock"
	Version         = 1
	StatusPlanned   = "planned"
	StatusReady     = "ready"
	StatusLaunching = "launching"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

type Clock func() time.Time

type IDGenerator func(time.Time) (string, error)

type Executions struct {
	Version int      `json:"version"`
	Records []Record `json:"executions"`
}

type Record struct {
	ID            string `json:"id"`
	QueueItemID   string `json:"queueItemId"`
	Task          string `json:"task"`
	ReservationID string `json:"reservationId"`
	Status        string `json:"status"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

type TransitionResult struct {
	ID        string
	Task      string
	OldStatus string
	NewStatus string
}

type Store struct {
	Store       bstate.Store
	Queue       runtimequeue.Store
	Now         Clock
	GenerateID  IDGenerator
	LockOptions locking.Options
}

type Inspection struct {
	Path                     string         `json:"path"`
	State                    string         `json:"state"`
	Version                  int            `json:"version"`
	SupportedVersion         int            `json:"supportedVersion"`
	TotalExecutions          int            `json:"totalExecutions"`
	CountsByStatus           map[string]int `json:"countsByStatus"`
	NewestExecutionTask      string         `json:"newestExecutionTask,omitempty"`
	NewestExecutionStatus    string         `json:"newestExecutionStatus,omitempty"`
	NewestPlannedTask        string         `json:"newestPlannedTask,omitempty"`
	DuplicateIDs             []string       `json:"duplicateIds,omitempty"`
	InvalidRecords           []string       `json:"invalidRecords,omitempty"`
	UnsupportedFutureVersion bool           `json:"unsupportedFutureVersion"`
	Error                    string         `json:"error,omitempty"`
}

func NewStore(repoRoot string) (Store, error) {
	store, err := bstate.NewStore(repoRoot)
	if err != nil {
		return Store{}, err
	}
	queueStore, err := runtimequeue.NewStore(repoRoot)
	if err != nil {
		return Store{}, err
	}
	return Store{Store: store, Queue: queueStore}, nil
}

func (store Store) Path() string {
	return store.Store.Path(FileName)
}

func (store Store) LockPath() string {
	return store.Store.Path(LockFile)
}

func (store Store) Load() (Executions, bool, error) {
	var executions Executions
	missing, err := store.Store.ReadJSON(FileName, &executions)
	if err != nil {
		return Executions{}, false, err
	}
	if missing {
		return Executions{Version: Version, Records: []Record{}}, true, nil
	}
	if err := Validate(executions); err != nil {
		return Executions{}, false, err
	}
	if executions.Records == nil {
		executions.Records = []Record{}
	}
	return executions, false, nil
}

func (store Store) Inspect() Inspection {
	result := Inspection{
		Path:             store.Path(),
		State:            "missing",
		Version:          Version,
		SupportedVersion: Version,
		CountsByStatus:   map[string]int{},
	}
	data, err := os.ReadFile(filepath.Clean(store.Path()))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return result
		}
		result.State = "invalid"
		result.Error = fmt.Sprintf("read %s: %v", FileName, err)
		return result
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		result.State = "corrupted"
		result.Error = fmt.Sprintf("read %s: file is empty", FileName)
		return result
	}
	var executions Executions
	if err := json.Unmarshal(data, &executions); err != nil {
		result.State = "corrupted"
		result.Error = fmt.Sprintf("parse %s: %v", FileName, err)
		return result
	}
	result.State = "valid"
	result.Version = executions.Version
	result.TotalExecutions = len(executions.Records)
	result.UnsupportedFutureVersion = executions.Version > Version
	if executions.Version != Version {
		result.State = "invalid"
		if executions.Version > Version {
			result.Error = fmt.Sprintf("unsupported future runtime-executions.json version %d; supported version is %d", executions.Version, Version)
		} else {
			result.Error = fmt.Sprintf("unsupported runtime-executions.json version %d; supported version is %d", executions.Version, Version)
		}
	}

	seen := map[string]struct{}{}
	duplicates := map[string]struct{}{}
	var newestExecutionAt time.Time
	var newestPlannedAt time.Time
	for index, record := range executions.Records {
		status := strings.ToLower(strings.TrimSpace(record.Status))
		if status == "" {
			status = "(missing)"
		}
		result.CountsByStatus[status]++
		if createdAt, err := parseTime(record.CreatedAt); err == nil {
			if newestExecutionAt.IsZero() || createdAt.After(newestExecutionAt) {
				newestExecutionAt = createdAt
				result.NewestExecutionTask = record.Task
				result.NewestExecutionStatus = status
			}
		}
		if status == StatusPlanned {
			if createdAt, err := parseTime(record.CreatedAt); err == nil && (newestPlannedAt.IsZero() || createdAt.After(newestPlannedAt)) {
				newestPlannedAt = createdAt
				result.NewestPlannedTask = record.Task
			}
		}
		if strings.TrimSpace(record.ID) == "" {
			result.InvalidRecords = append(result.InvalidRecords, fmt.Sprintf("execution[%d] id is required", index))
		} else if _, exists := seen[record.ID]; exists {
			duplicates[record.ID] = struct{}{}
		} else {
			seen[record.ID] = struct{}{}
		}
		if err := validateRecord(record); err != nil {
			result.InvalidRecords = append(result.InvalidRecords, fmt.Sprintf("execution[%d] is invalid: %v", index, err))
		}
	}
	for id := range duplicates {
		result.DuplicateIDs = append(result.DuplicateIDs, id)
	}
	sort.Strings(result.DuplicateIDs)
	if len(result.DuplicateIDs) > 0 || len(result.InvalidRecords) > 0 {
		result.State = "invalid"
	}
	return result
}

func (store Store) PlanFromReservation(queueItemID string) (Record, error) {
	queueItemID = strings.TrimSpace(queueItemID)
	if queueItemID == "" {
		return Record{}, errors.New("queue item id is required")
	}
	lock, err := store.acquireLock()
	if err != nil {
		return Record{}, err
	}
	defer func() { _ = lock.Release() }()

	queue, _, err := store.Queue.Load()
	if err != nil {
		return Record{}, err
	}
	item, err := reservedQueueItem(queue, queueItemID)
	if err != nil {
		return Record{}, err
	}
	reservationID := strings.TrimSpace(item.Reservation.ReservationID)

	executions, missing, err := store.Load()
	if err != nil {
		return Record{}, err
	}
	if missing {
		executions = Executions{Version: Version, Records: []Record{}}
	}
	for _, record := range executions.Records {
		if record.QueueItemID == item.ID && record.ReservationID == reservationID {
			return Record{}, fmt.Errorf("execution already planned for queue item %s reservation %s", item.ID, reservationID)
		}
	}

	now := store.now().UTC()
	id, err := store.newID(now, executions)
	if err != nil {
		return Record{}, err
	}
	timestamp := now.Format(time.RFC3339)
	record := Record{
		ID:            id,
		QueueItemID:   item.ID,
		Task:          item.Task,
		ReservationID: reservationID,
		Status:        StatusPlanned,
		CreatedAt:     timestamp,
		UpdatedAt:     timestamp,
	}
	executions.Version = Version
	executions.Records = append(executions.Records, record)
	if err := store.Store.WriteJSON(FileName, executions); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (store Store) MarkReady(executionID string) (TransitionResult, error) {
	return store.transitionStatus(executionID, StatusPlanned, StatusReady)
}

func (store Store) MarkPlanned(executionID string) (TransitionResult, error) {
	return store.transitionStatus(executionID, StatusReady, StatusPlanned)
}

func (store Store) MarkLaunching(executionID string) (TransitionResult, error) {
	return store.transitionStatus(executionID, StatusReady, StatusLaunching)
}

func (store Store) MarkCompleted(executionID string) (TransitionResult, error) {
	return store.transitionStatus(executionID, StatusLaunching, StatusCompleted)
}

func (store Store) MarkFailed(executionID string) (TransitionResult, error) {
	return store.transitionStatus(executionID, StatusLaunching, StatusFailed)
}

func (store Store) transitionStatus(executionID string, fromStatus string, toStatus string) (TransitionResult, error) {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return TransitionResult{}, errors.New("execution id is required")
	}
	lock, err := store.acquireLock()
	if err != nil {
		return TransitionResult{}, err
	}
	defer func() { _ = lock.Release() }()

	executions, missing, err := store.Load()
	if err != nil {
		return TransitionResult{}, err
	}
	if missing {
		return TransitionResult{}, fmt.Errorf("execution not found: %s", executionID)
	}
	for index, record := range executions.Records {
		if record.ID != executionID {
			continue
		}
		oldStatus := strings.ToLower(strings.TrimSpace(record.Status))
		if oldStatus != fromStatus {
			return TransitionResult{}, fmt.Errorf("execution %s status is %s, want %s", executionID, fallbackStatus(oldStatus), fromStatus)
		}
		executions.Records[index].Status = toStatus
		executions.Records[index].UpdatedAt = store.now().UTC().Format(time.RFC3339)
		if err := store.Store.WriteJSON(FileName, executions); err != nil {
			return TransitionResult{}, err
		}
		return TransitionResult{
			ID:        record.ID,
			Task:      record.Task,
			OldStatus: oldStatus,
			NewStatus: toStatus,
		}, nil
	}
	return TransitionResult{}, fmt.Errorf("execution not found: %s", executionID)
}

func Validate(executions Executions) error {
	if executions.Version != Version {
		return fmt.Errorf("unsupported runtime-executions.json version %d; supported version is %d", executions.Version, Version)
	}
	seen := map[string]struct{}{}
	for index, record := range executions.Records {
		if strings.TrimSpace(record.ID) == "" {
			return fmt.Errorf("runtime-executions.json execution[%d] id is required", index)
		}
		if _, exists := seen[record.ID]; exists {
			return fmt.Errorf("runtime-executions.json duplicate execution id: %s", record.ID)
		}
		seen[record.ID] = struct{}{}
		if err := validateRecord(record); err != nil {
			return fmt.Errorf("runtime-executions.json execution[%d] is invalid: %w", index, err)
		}
	}
	return nil
}

func (store Store) newID(now time.Time, executions Executions) (string, error) {
	generateID := store.GenerateID
	if generateID == nil {
		generateID = GenerateID
	}
	existing := map[string]struct{}{}
	for _, record := range executions.Records {
		existing[record.ID] = struct{}{}
	}
	for attempt := 0; attempt < 8; attempt++ {
		id, err := generateID(now)
		if err != nil {
			return "", err
		}
		if _, exists := existing[id]; !exists {
			return id, nil
		}
	}
	return "", errors.New("could not generate unique execution id")
}

func GenerateID(now time.Time) (string, error) {
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate execution id: %w", err)
	}
	return "exec-" + now.UTC().Format("20060102T150405") + "-" + hex.EncodeToString(random[:]), nil
}

func reservedQueueItem(queue runtimequeue.Queue, id string) (runtimequeue.Item, error) {
	for _, item := range queue.Items {
		if item.ID != id {
			continue
		}
		if item.Reservation == nil {
			return runtimequeue.Item{}, fmt.Errorf("queue item is not reserved: %s", id)
		}
		if strings.TrimSpace(item.Reservation.ReservationID) == "" {
			return runtimequeue.Item{}, fmt.Errorf("queue item reservation id is required: %s", id)
		}
		return item, nil
	}
	return runtimequeue.Item{}, fmt.Errorf("queue item not found: %s", id)
}

func validateRecord(record Record) error {
	if strings.TrimSpace(record.QueueItemID) == "" {
		return errors.New("queueItemId is required")
	}
	if _, err := runtimequeue.NormalizeTaskSlug(record.Task); err != nil {
		return fmt.Errorf("task is invalid: %w", err)
	}
	if strings.TrimSpace(record.ReservationID) == "" {
		return errors.New("reservationId is required")
	}
	status := strings.ToLower(strings.TrimSpace(record.Status))
	if status != StatusPlanned && status != StatusReady && status != StatusLaunching && status != StatusCompleted && status != StatusFailed && status != StatusCancelled {
		return fmt.Errorf("status %q is not recognized", record.Status)
	}
	if _, err := parseTime(record.CreatedAt); err != nil {
		return fmt.Errorf("createdAt is invalid: %w", err)
	}
	if _, err := parseTime(record.UpdatedAt); err != nil {
		return fmt.Errorf("updatedAt is invalid: %w", err)
	}
	return nil
}

func fallbackStatus(status string) string {
	if strings.TrimSpace(status) == "" {
		return "(missing)"
	}
	return status
}

func (store Store) acquireLock() (*locking.Lock, error) {
	options := store.LockOptions
	if options.Timeout <= 0 {
		options.Timeout = 2 * time.Second
	}
	if options.Interval <= 0 {
		options.Interval = 50 * time.Millisecond
	}
	if options.StaleAge <= 0 {
		options.StaleAge = 2 * time.Minute
	}
	if options.Now == nil && store.Now != nil {
		options.Now = func() time.Time { return store.Now().UTC() }
	}
	return locking.Acquire(store.LockPath(), options)
}

func (store Store) now() time.Time {
	if store.Now != nil {
		return store.Now()
	}
	return time.Now()
}

func parseTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, errors.New("missing time")
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}
	return time.Parse(time.RFC3339, value)
}
