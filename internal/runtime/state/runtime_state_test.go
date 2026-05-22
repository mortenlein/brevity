package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRuntimeStateSaveLoadUsesBrevityRuntimeJSON(t *testing.T) {
	repoRoot := t.TempDir()
	store, err := NewStore(repoRoot)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	state := NewRunningState(12345, now)
	if err := store.Save(state); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".brevity", "runtime.json")); err != nil {
		t.Fatalf("runtime.json was not written: %v", err)
	}
	loaded, missing, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if missing {
		t.Fatal("Load reported missing runtime.json")
	}
	if loaded.PID != 12345 || loaded.Status != "running" || loaded.Version != Version {
		t.Fatalf("loaded state = %+v", loaded)
	}
}

func TestRuntimeSnapshotToleratesMissingRuntimeFile(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	snapshot := store.Snapshot(time.Second)
	if !snapshot.Missing {
		t.Fatalf("Missing = false, want true: %+v", snapshot)
	}
	if snapshot.Interpretation != "runtime has not been started" {
		t.Fatalf("Interpretation = %q", snapshot.Interpretation)
	}
}

func TestRuntimeSnapshotToleratesCorruptedRuntimeFile(t *testing.T) {
	repoRoot := t.TempDir()
	store, err := NewStore(repoRoot)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, ".brevity"), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(store.RuntimePath(), []byte("{not-json"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	snapshot := store.Snapshot(time.Second)
	if !snapshot.Corrupted {
		t.Fatalf("Corrupted = false, want true: %+v", snapshot)
	}
	if snapshot.Error == nil || !strings.Contains(snapshot.Error.Error(), "parse runtime.json") {
		t.Fatalf("Error = %v", snapshot.Error)
	}
}

func TestRuntimeSnapshotReportsUnsupportedFutureVersion(t *testing.T) {
	repoRoot := t.TempDir()
	store, err := NewStore(repoRoot)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, ".brevity"), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	content := `{"pid":12345,"startedAt":"2026-05-22T12:00:00Z","heartbeatAt":"2026-05-22T12:00:00Z","status":"running","activeWorkers":0,"queueDepth":0,"version":99}`
	if err := os.WriteFile(store.RuntimePath(), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	snapshot := store.Snapshot(time.Second)
	if snapshot.Error == nil || !strings.Contains(snapshot.Error.Error(), "unsupported runtime.json version 99") {
		t.Fatalf("Error = %v", snapshot.Error)
	}
	if snapshot.Interpretation != "runtime state is invalid" {
		t.Fatalf("Interpretation = %q", snapshot.Interpretation)
	}
}

func TestRuntimeSnapshotReportsInvalidActiveRuntimeFields(t *testing.T) {
	repoRoot := t.TempDir()
	store, err := NewStore(repoRoot)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, ".brevity"), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	content := `{"status":"running","activeWorkers":0,"queueDepth":0,"version":1}`
	if err := os.WriteFile(store.RuntimePath(), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	snapshot := store.Snapshot(time.Second)
	if snapshot.Error == nil || !strings.Contains(snapshot.Error.Error(), "pid is missing or invalid") {
		t.Fatalf("Error = %v", snapshot.Error)
	}
	if snapshot.Interpretation != "runtime state is invalid" {
		t.Fatalf("Interpretation = %q", snapshot.Interpretation)
	}
}

func TestRuntimeSnapshotDetectsStalePID(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	state := NewRunningState(999999, time.Now().UTC().Add(-time.Minute))
	if err := store.Save(state); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	snapshot := store.Snapshot(10 * time.Second)
	if snapshot.PIDAlive {
		t.Fatal("PIDAlive = true, want false")
	}
	if snapshot.Interpretation != "runtime pid is stale" {
		t.Fatalf("Interpretation = %q", snapshot.Interpretation)
	}
}
