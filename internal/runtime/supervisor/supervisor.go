package supervisor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	rstate "github.com/mortenlein/brevity/internal/runtime/state"
	"github.com/mortenlein/brevity/internal/state/locking"
)

const childEnv = "BREVITY_RUNTIME_SUPERVISOR_CHILD"

type Supervisor struct {
	Store             rstate.Store
	HeartbeatInterval time.Duration
	LockOptions       locking.Options
	Now               func() time.Time
}

func IsChildProcess() bool {
	return os.Getenv(childEnv) == "1"
}

func Start(repoRoot string) (rstate.RuntimeState, bool, error) {
	store, err := rstate.NewStore(repoRoot)
	if err != nil {
		return rstate.RuntimeState{}, false, err
	}
	snapshot := store.Snapshot(10 * time.Second)
	if !snapshot.Missing && snapshot.State.Status == "running" && snapshot.PIDAlive && snapshot.HeartbeatFresh {
		return snapshot.State, false, nil
	}
	if err := store.ClearStopRequest(); err != nil {
		return rstate.RuntimeState{}, false, err
	}
	command, err := startCommand(store.Store.RepoRoot)
	if err != nil {
		return rstate.RuntimeState{}, false, err
	}
	command.Dir = store.Store.RepoRoot
	command.Env = append(os.Environ(), childEnv+"=1")
	if err := command.Start(); err != nil {
		return rstate.RuntimeState{}, false, fmt.Errorf("start runtime supervisor: %w", err)
	}
	_ = command.Process.Release()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		snapshot = store.Snapshot(10 * time.Second)
		if !snapshot.Missing && snapshot.State.PID != 0 && snapshot.State.Status == "running" {
			return snapshot.State, true, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return rstate.RuntimeState{PID: command.Process.Pid, Status: "starting", Version: rstate.Version}, true, nil
}

func startCommand(repoRoot string) (*exec.Cmd, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable: %w", err)
	}
	if sourceTreeAvailable(repoRoot) && strings.HasPrefix(strings.ToLower(executable), strings.ToLower(os.TempDir())) {
		return exec.Command("go", "run", "./cmd/brevity", "runtime", "start"), nil
	}
	return exec.Command(executable, "runtime", "start"), nil
}

func sourceTreeAvailable(repoRoot string) bool {
	info, err := os.Stat(filepath.Join(repoRoot, "cmd", "brevity"))
	return err == nil && info.IsDir()
}

func Stop(repoRoot string) (rstate.Snapshot, error) {
	store, err := rstate.NewStore(repoRoot)
	if err != nil {
		return rstate.Snapshot{}, err
	}
	snapshot := store.Snapshot(10 * time.Second)
	if snapshot.Missing || snapshot.State.Status == "stopped" || !snapshot.PIDAlive {
		state := snapshot.State
		if state.PID == 0 {
			state.PID = snapshot.State.PID
		}
		state = rstate.MarkStopped(state, time.Now().UTC())
		_ = store.Save(state)
		_ = store.ClearStopRequest()
		return store.Snapshot(10 * time.Second), nil
	}
	if err := store.RequestStop(); err != nil {
		return snapshot, err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		next := store.Snapshot(10 * time.Second)
		if next.State.Status == "stopped" {
			_ = store.ClearStopRequest()
			return next, nil
		}
		if !next.PIDAlive {
			stopped := rstate.MarkStopped(next.State, time.Now().UTC())
			_ = store.Save(stopped)
			_ = store.ClearStopRequest()
			return store.Snapshot(10 * time.Second), nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return store.Snapshot(10 * time.Second), errors.New("runtime supervisor did not stop before timeout")
}

func Status(repoRoot string) (rstate.Snapshot, error) {
	store, err := rstate.NewStore(repoRoot)
	if err != nil {
		return rstate.Snapshot{}, err
	}
	return store.Snapshot(10 * time.Second), nil
}

func (supervisor Supervisor) Run(ctx context.Context) error {
	store := supervisor.Store
	if store.Store.RepoRoot == "" {
		var err error
		store, err = rstate.NewStore("")
		if err != nil {
			return err
		}
	}
	lock, err := store.AcquireRuntimeLock(supervisor.LockOptions)
	if err != nil {
		return err
	}
	defer lock.Release()
	defer store.ClearStopRequest()

	current := rstate.NewRunningState(os.Getpid(), supervisor.now().UTC())
	if err := saveRuntimeState(store, current); err != nil {
		return err
	}
	interval := supervisor.HeartbeatInterval
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			current = rstate.MarkStopped(current, supervisor.now().UTC())
			return saveRuntimeState(store, current)
		case <-ticker.C:
			if store.StopRequested() {
				current = rstate.MarkStopped(current, supervisor.now().UTC())
				return saveRuntimeState(store, current)
			}
			current = rstate.MarkHeartbeat(current, supervisor.now().UTC())
			if err := saveRuntimeState(store, current); err != nil {
				return err
			}
		}
	}
}

func saveRuntimeState(store rstate.Store, state rstate.RuntimeState) error {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if err := store.Save(state); err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
			continue
		}
		return nil
	}
	return lastErr
}

func (supervisor Supervisor) now() time.Time {
	if supervisor.Now != nil {
		return supervisor.Now()
	}
	return time.Now()
}
