package locking

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAcquireWritesPIDTimestampAndReleaseRemovesLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".brevity", "state.lock")
	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)

	lock, err := Acquire(path, Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	for _, want := range []string{"pid=", "createdAt=2026-05-21T10:00:00Z"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("lock content missing %q:\n%s", want, data)
		}
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release returned error: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("lock file still exists after release: %v", err)
	}
}

func TestAcquireContentionTimesOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".brevity", "state.lock")
	first, err := Acquire(path, Options{})
	if err != nil {
		t.Fatalf("first Acquire returned error: %v", err)
	}
	defer first.Release()

	_, err = Acquire(path, Options{Timeout: 20 * time.Millisecond, Interval: time.Millisecond})
	if err == nil {
		t.Fatal("second Acquire succeeded; want timeout")
	}
	if !strings.Contains(err.Error(), "state lock timeout") {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestAcquireRemovesStaleInvalidLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".brevity", "state.lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte("not lock metadata"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("Chtimes returned error: %v", err)
	}

	lock, err := Acquire(path, Options{Timeout: time.Second, Interval: time.Millisecond, StaleAge: time.Hour})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}
	defer lock.Release()
}
