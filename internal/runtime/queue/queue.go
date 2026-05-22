package queue

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	bstate "github.com/mortenlein/brevity/internal/state"
	"github.com/mortenlein/brevity/internal/state/locking"
)

const (
	FileName        = "runtime-queue.json"
	LockFile        = "runtime-queue.lock"
	Version         = 1
	StatusQueued    = "queued"
	StatusCancelled = "cancelled"
)

var safeTaskSlug = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type Clock func() time.Time

type IDGenerator func(time.Time) (string, error)

type Queue struct {
	Version int    `json:"version"`
	Items   []Item `json:"items"`
}

type Item struct {
	ID        string `json:"id"`
	Task      string `json:"task"`
	Provider  string `json:"provider"`
	Profile   string `json:"profile"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type Store struct {
	Store       bstate.Store
	Now         Clock
	GenerateID  IDGenerator
	LockOptions locking.Options
}

type Inspection struct {
	Path                     string         `json:"path"`
	State                    string         `json:"state"`
	Version                  int            `json:"version"`
	SupportedVersion         int            `json:"supportedVersion"`
	TotalItems               int            `json:"totalItems"`
	CountsByStatus           map[string]int `json:"countsByStatus"`
	OldestQueuedItemAge      string         `json:"oldestQueuedItemAge,omitempty"`
	NewestQueuedItemAge      string         `json:"newestQueuedItemAge,omitempty"`
	DuplicateIDs             []string       `json:"duplicateIds,omitempty"`
	InvalidItems             []string       `json:"invalidItems,omitempty"`
	UnsupportedFutureVersion bool           `json:"unsupportedFutureVersion"`
	Error                    string         `json:"error,omitempty"`
}

func NewStore(repoRoot string) (Store, error) {
	store, err := bstate.NewStore(repoRoot)
	if err != nil {
		return Store{}, err
	}
	return Store{Store: store}, nil
}

func (store Store) QueuePath() string {
	return store.Store.Path(FileName)
}

func (store Store) LockPath() string {
	return store.Store.Path(LockFile)
}

func (store Store) Load() (Queue, bool, error) {
	var queue Queue
	missing, err := store.Store.ReadJSON(FileName, &queue)
	if err != nil {
		return Queue{}, false, err
	}
	if missing {
		return Queue{Version: Version, Items: []Item{}}, true, nil
	}
	if err := Validate(queue); err != nil {
		return Queue{}, false, err
	}
	if queue.Items == nil {
		queue.Items = []Item{}
	}
	return queue, false, nil
}

func (store Store) Inspect() Inspection {
	result := Inspection{
		Path:             store.QueuePath(),
		State:            "missing",
		SupportedVersion: Version,
		CountsByStatus:   map[string]int{},
	}
	data, err := os.ReadFile(filepath.Clean(store.QueuePath()))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			result.Version = Version
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
	var queue Queue
	if err := json.Unmarshal(data, &queue); err != nil {
		result.State = "corrupted"
		result.Error = fmt.Sprintf("parse %s: %v", FileName, err)
		return result
	}
	result.State = "valid"
	result.Version = queue.Version
	result.TotalItems = len(queue.Items)
	result.UnsupportedFutureVersion = queue.Version > Version
	if queue.Version != Version {
		result.State = "invalid"
		if queue.Version > Version {
			result.Error = fmt.Sprintf("unsupported future runtime-queue.json version %d; supported version is %d", queue.Version, Version)
		} else {
			result.Error = fmt.Sprintf("unsupported runtime-queue.json version %d; supported version is %d", queue.Version, Version)
		}
	}

	seen := map[string]struct{}{}
	duplicates := map[string]struct{}{}
	var oldest, newest time.Time
	for index, item := range queue.Items {
		status := strings.ToLower(strings.TrimSpace(item.Status))
		if status == "" {
			status = "(missing)"
		}
		result.CountsByStatus[status]++
		if strings.TrimSpace(item.ID) == "" {
			result.InvalidItems = append(result.InvalidItems, fmt.Sprintf("item[%d] id is required", index))
		} else if _, exists := seen[item.ID]; exists {
			duplicates[item.ID] = struct{}{}
		} else {
			seen[item.ID] = struct{}{}
		}
		if _, err := NormalizeTaskSlug(item.Task); err != nil {
			result.InvalidItems = append(result.InvalidItems, fmt.Sprintf("item[%d] task is invalid: %v", index, err))
		}
		if status != StatusQueued && status != StatusCancelled {
			result.InvalidItems = append(result.InvalidItems, fmt.Sprintf("item[%d] status %q is not recognized", index, item.Status))
		}
		createdAt, err := parseTime(item.CreatedAt)
		if err != nil {
			result.InvalidItems = append(result.InvalidItems, fmt.Sprintf("item[%d] createdAt is invalid: %v", index, err))
		} else if status == StatusQueued {
			createdAt = createdAt.UTC()
			if oldest.IsZero() || createdAt.Before(oldest) {
				oldest = createdAt
			}
			if newest.IsZero() || createdAt.After(newest) {
				newest = createdAt
			}
		}
		if _, err := parseTime(item.UpdatedAt); err != nil {
			result.InvalidItems = append(result.InvalidItems, fmt.Sprintf("item[%d] updatedAt is invalid: %v", index, err))
		}
	}
	for id := range duplicates {
		result.DuplicateIDs = append(result.DuplicateIDs, id)
	}
	sort.Strings(result.DuplicateIDs)
	if len(result.DuplicateIDs) > 0 || len(result.InvalidItems) > 0 {
		result.State = "invalid"
	}
	now := store.now().UTC()
	if !oldest.IsZero() {
		result.OldestQueuedItemAge = formatAge(now.Sub(oldest))
	}
	if !newest.IsZero() {
		result.NewestQueuedItemAge = formatAge(now.Sub(newest))
	}
	return result
}

func (store Store) Add(task string) (Item, error) {
	task, err := NormalizeTaskSlug(task)
	if err != nil {
		return Item{}, err
	}
	lock, err := store.acquireLock()
	if err != nil {
		return Item{}, err
	}
	defer func() { _ = lock.Release() }()

	queue, missing, err := store.Load()
	if err != nil {
		return Item{}, err
	}
	if missing {
		queue = Queue{Version: Version, Items: []Item{}}
	}
	now := store.now().UTC()
	item, err := store.newItem(task, queue, now)
	if err != nil {
		return Item{}, err
	}
	queue.Version = Version
	queue.Items = append(queue.Items, item)
	if err := store.Store.WriteJSON(FileName, queue); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (store Store) Remove(id string) (Item, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Item{}, errors.New("queue item id is required")
	}
	lock, err := store.acquireLock()
	if err != nil {
		return Item{}, err
	}
	defer func() { _ = lock.Release() }()

	queue, _, err := store.Load()
	if err != nil {
		return Item{}, err
	}
	next := make([]Item, 0, len(queue.Items))
	var removed Item
	found := false
	for _, item := range queue.Items {
		if item.ID == id {
			removed = item
			found = true
			continue
		}
		next = append(next, item)
	}
	if !found {
		return Item{}, fmt.Errorf("queue item not found: %s", id)
	}
	queue.Items = next
	queue.Version = Version
	if err := store.Store.WriteJSON(FileName, queue); err != nil {
		return Item{}, err
	}
	return removed, nil
}

func NormalizeTaskSlug(task string) (string, error) {
	task = strings.TrimSpace(task)
	if task == "" {
		return "", errors.New("task slug is required")
	}
	if strings.Contains(task, "..") || strings.ContainsAny(task, `/\:`) || !safeTaskSlug.MatchString(task) {
		return "", fmt.Errorf("unsafe task slug: %s", task)
	}
	return task, nil
}

func Validate(queue Queue) error {
	if queue.Version != Version {
		return fmt.Errorf("unsupported runtime-queue.json version %d; supported version is %d", queue.Version, Version)
	}
	seen := map[string]struct{}{}
	for index, item := range queue.Items {
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("runtime-queue.json item[%d] id is required", index)
		}
		if _, exists := seen[item.ID]; exists {
			return fmt.Errorf("runtime-queue.json duplicate queue item id: %s", item.ID)
		}
		seen[item.ID] = struct{}{}
		if _, err := NormalizeTaskSlug(item.Task); err != nil {
			return fmt.Errorf("runtime-queue.json item[%d] task is invalid: %w", index, err)
		}
		status := strings.ToLower(strings.TrimSpace(item.Status))
		if status != StatusQueued && status != StatusCancelled {
			return fmt.Errorf("runtime-queue.json item[%d] status %q is not recognized", index, item.Status)
		}
		if _, err := parseTime(item.CreatedAt); err != nil {
			return fmt.Errorf("runtime-queue.json item[%d] createdAt is invalid: %w", index, err)
		}
		if _, err := parseTime(item.UpdatedAt); err != nil {
			return fmt.Errorf("runtime-queue.json item[%d] updatedAt is invalid: %w", index, err)
		}
	}
	return nil
}

func (store Store) newItem(task string, queue Queue, now time.Time) (Item, error) {
	generateID := store.GenerateID
	if generateID == nil {
		generateID = GenerateID
	}
	existing := map[string]struct{}{}
	for _, item := range queue.Items {
		existing[item.ID] = struct{}{}
	}
	var id string
	var err error
	for attempt := 0; attempt < 8; attempt++ {
		id, err = generateID(now)
		if err != nil {
			return Item{}, err
		}
		if _, exists := existing[id]; !exists {
			break
		}
		id = ""
	}
	if id == "" {
		return Item{}, errors.New("could not generate unique queue item id")
	}
	timestamp := now.Format(time.RFC3339)
	return Item{
		ID:        id,
		Task:      task,
		Provider:  defaultProvider(store.Store),
		Profile:   "default",
		Status:    StatusQueued,
		CreatedAt: timestamp,
		UpdatedAt: timestamp,
	}, nil
}

func GenerateID(now time.Time) (string, error) {
	var random [3]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate queue id: %w", err)
	}
	return now.UTC().Format("20060102") + "-" + hex.EncodeToString(random[:]), nil
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

func defaultProvider(store bstate.Store) string {
	config, missing, err := bstate.LoadConfig(store)
	if err == nil && !missing && strings.TrimSpace(config.DefaultProvider) != "" {
		return strings.TrimSpace(config.DefaultProvider)
	}
	return "gemini"
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

func formatAge(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	return duration.Truncate(time.Second).String()
}
