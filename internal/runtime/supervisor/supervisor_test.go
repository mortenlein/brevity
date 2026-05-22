package supervisor

import (
	"context"
	"strings"
	"testing"
	"time"

	rstate "github.com/mortenlein/brevity/internal/runtime/state"
	"github.com/mortenlein/brevity/internal/state/locking"
)

func TestSupervisorRunInitializesRuntimeState(t *testing.T) {
	store, err := rstate.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = (Supervisor{Store: store, HeartbeatInterval: time.Millisecond}).Run(ctx)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	loaded, missing, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if missing {
		t.Fatal("runtime.json missing")
	}
	if loaded.Status != "stopped" || loaded.ActiveWorkers != 0 || loaded.QueueDepth != 0 {
		t.Fatalf("loaded state = %+v", loaded)
	}
}

func TestSupervisorRunGracefulShutdownViaStopRequest(t *testing.T) {
	store, err := rstate.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- (Supervisor{Store: store, HeartbeatInterval: 10 * time.Millisecond}).Run(context.Background())
	}()
	waitForStatus(t, store, "running")
	if err := store.RequestStop(); err != nil {
		t.Fatalf("RequestStop returned error: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not stop after stop request")
	}
	loaded, _, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.Status != "stopped" {
		t.Fatalf("status = %q, want stopped", loaded.Status)
	}
}

func TestSupervisorRunLockAcquisitionBehavior(t *testing.T) {
	store, err := rstate.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	lock, err := store.AcquireRuntimeLock(locking.Options{})
	if err != nil {
		t.Fatalf("AcquireRuntimeLock returned error: %v", err)
	}
	defer lock.Release()
	err = (Supervisor{
		Store: store,
		LockOptions: locking.Options{
			Timeout:  20 * time.Millisecond,
			Interval: time.Millisecond,
		},
	}).Run(context.Background())
	if err == nil {
		t.Fatal("Run succeeded while runtime lock was held")
	}
	if !strings.Contains(err.Error(), "state lock timeout") {
		t.Fatalf("Run error = %v", err)
	}
}

func TestStopMarksStaleRuntimeStopped(t *testing.T) {
	repoRoot := t.TempDir()
	store, err := rstate.NewStore(repoRoot)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	state := rstate.NewRunningState(-1, time.Now().UTC().Add(-time.Minute))
	if err := store.Save(state); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	snapshot, err := Stop(repoRoot)
	if err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if snapshot.State.Status != "stopped" {
		t.Fatalf("status = %q, want stopped", snapshot.State.Status)
	}
}

func waitForStatus(t *testing.T, store rstate.Store, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state, missing, err := store.Load()
		if err == nil && !missing && state.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("runtime state never reached status %q", want)
}
