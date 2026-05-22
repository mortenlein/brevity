package state

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	bstate "github.com/mortenlein/brevity/internal/state"
	"github.com/mortenlein/brevity/internal/state/locking"
)

const (
	RuntimeFile = "runtime.json"
	StopFile    = "runtime.stop"
	LockFile    = "runtime.lock"
	Version     = 1
)

type Clock func() time.Time

type RuntimeState struct {
	PID           int    `json:"pid"`
	StartedAt     string `json:"startedAt"`
	Status        string `json:"status"`
	ActiveWorkers int    `json:"activeWorkers"`
	QueueDepth    int    `json:"queueDepth"`
	HeartbeatAt   string `json:"heartbeatAt"`
	Version       int    `json:"version"`
}

type Store struct {
	Store bstate.Store
	Now   Clock
}

type Snapshot struct {
	State          RuntimeState
	Missing        bool
	Corrupted      bool
	Error          error
	PIDAlive       bool
	HeartbeatFresh bool
	Uptime         time.Duration
	HeartbeatAge   time.Duration
	RuntimePath    string
	Interpretation string
}

func NewStore(repoRoot string) (Store, error) {
	store, err := bstate.NewStore(repoRoot)
	if err != nil {
		return Store{}, err
	}
	return Store{Store: store}, nil
}

func (store Store) RuntimePath() string {
	return store.Store.Path(RuntimeFile)
}

func (store Store) StopPath() string {
	return store.Store.Path(StopFile)
}

func (store Store) LockPath() string {
	return store.Store.Path(LockFile)
}

func (store Store) Load() (RuntimeState, bool, error) {
	var state RuntimeState
	missing, err := store.Store.ReadJSON(RuntimeFile, &state)
	if err != nil {
		return RuntimeState{}, false, err
	}
	return state, missing, nil
}

func (store Store) Save(state RuntimeState) error {
	state.Version = Version
	return store.Store.WriteJSON(RuntimeFile, state)
}

func (store Store) RequestStop() error {
	if err := os.MkdirAll(store.Store.BrevityRoot(), 0o755); err != nil {
		return fmt.Errorf("create runtime directory: %w", err)
	}
	content := []byte(store.now().UTC().Format(time.RFC3339Nano) + "\n")
	return os.WriteFile(store.StopPath(), content, 0o644)
}

func (store Store) ClearStopRequest() error {
	err := os.Remove(store.StopPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (store Store) StopRequested() bool {
	_, err := os.Stat(store.StopPath())
	return err == nil
}

func (store Store) AcquireRuntimeLock(options locking.Options) (*locking.Lock, error) {
	if options.Timeout <= 0 {
		options.Timeout = 2 * time.Second
	}
	if options.Interval <= 0 {
		options.Interval = 50 * time.Millisecond
	}
	if options.StaleAge <= 0 {
		options.StaleAge = 2 * time.Minute
	}
	return locking.Acquire(store.LockPath(), options)
}

func (store Store) Snapshot(freshWithin time.Duration) Snapshot {
	if freshWithin <= 0 {
		freshWithin = 10 * time.Second
	}
	now := store.now().UTC()
	snapshot := Snapshot{RuntimePath: store.RuntimePath()}
	runtimeState, missing, err := store.Load()
	if err != nil {
		snapshot.Error = err
		snapshot.Corrupted = !strings.Contains(err.Error(), "not exist")
		snapshot.Interpretation = "runtime state is unreadable"
		return snapshot
	}
	snapshot.State = runtimeState
	snapshot.Missing = missing
	if missing {
		snapshot.Interpretation = "runtime has not been started"
		return snapshot
	}
	snapshot.PIDAlive = ProcessAlive(runtimeState.PID)
	if startedAt, err := parseTime(runtimeState.StartedAt); err == nil {
		snapshot.Uptime = now.Sub(startedAt)
	}
	if heartbeatAt, err := parseTime(runtimeState.HeartbeatAt); err == nil {
		snapshot.HeartbeatAge = now.Sub(heartbeatAt)
		snapshot.HeartbeatFresh = snapshot.HeartbeatAge <= freshWithin
	}
	switch {
	case runtimeState.Status == "stopped":
		snapshot.Interpretation = "runtime is stopped"
	case !snapshot.PIDAlive:
		snapshot.Interpretation = "runtime pid is stale"
	case !snapshot.HeartbeatFresh:
		snapshot.Interpretation = "runtime heartbeat is stale"
	default:
		snapshot.Interpretation = "runtime is running"
	}
	return snapshot
}

func NewRunningState(pid int, now time.Time) RuntimeState {
	timestamp := now.UTC().Format(time.RFC3339)
	return RuntimeState{
		PID:           pid,
		StartedAt:     timestamp,
		Status:        "running",
		ActiveWorkers: 0,
		QueueDepth:    0,
		HeartbeatAt:   timestamp,
		Version:       Version,
	}
}

func MarkHeartbeat(current RuntimeState, now time.Time) RuntimeState {
	current.Status = "running"
	current.ActiveWorkers = 0
	current.QueueDepth = 0
	current.HeartbeatAt = now.UTC().Format(time.RFC3339)
	current.Version = Version
	return current
}

func MarkStopped(current RuntimeState, now time.Time) RuntimeState {
	current.Status = "stopped"
	current.ActiveWorkers = 0
	current.QueueDepth = 0
	current.HeartbeatAt = now.UTC().Format(time.RFC3339)
	current.Version = Version
	return current
}

func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		output, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH").Output()
		if err != nil {
			return false
		}
		return strings.Contains(string(output), fmt.Sprintf(`"%d"`, pid))
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	return true
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

func FormatDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	seconds := int64(duration.Round(time.Second).Seconds())
	if seconds < 60 {
		return strconv.FormatInt(seconds, 10) + "s"
	}
	minutes := seconds / 60
	seconds = seconds % 60
	if minutes < 60 {
		return fmt.Sprintf("%dm%02ds", minutes, seconds)
	}
	hours := minutes / 60
	minutes = minutes % 60
	return fmt.Sprintf("%dh%02dm%02ds", hours, minutes, seconds)
}

func (store Store) now() time.Time {
	if store.Now != nil {
		return store.Now()
	}
	return time.Now()
}
