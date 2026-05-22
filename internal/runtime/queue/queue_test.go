package queue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bstate "github.com/mortenlein/brevity/internal/state"
)

func TestMissingQueueFileReadsAsEmpty(t *testing.T) {
	store := testStore(t)
	queue, missing, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !missing {
		t.Fatalf("Load missing = false, want true")
	}
	if queue.Version != Version || len(queue.Items) != 0 {
		t.Fatalf("queue = %#v, want empty v1 queue", queue)
	}
}

func TestAddCreatesRuntimeQueueFile(t *testing.T) {
	store := testStore(t)
	item, err := store.Add("runtime-queue-smoke")
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if item.ID != "20260522-abc123" {
		t.Fatalf("id = %q", item.ID)
	}
	if item.Status != StatusQueued {
		t.Fatalf("status = %q", item.Status)
	}
	if _, err := os.Stat(store.QueuePath()); err != nil {
		t.Fatalf("queue file was not created: %v", err)
	}
	loaded, missing, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if missing || len(loaded.Items) != 1 {
		t.Fatalf("loaded missing=%v items=%d, want one item", missing, len(loaded.Items))
	}
}

func TestAddRejectsUnsafeTaskSlug(t *testing.T) {
	store := testStore(t)
	for _, slug := range []string{"", "   ", "../task", "bad\\task", "bad/task", "-starts-bad", "bad:task"} {
		if _, err := store.Add(slug); err == nil {
			t.Fatalf("Add(%q) succeeded, want error", slug)
		}
	}
	if _, err := os.Stat(store.QueuePath()); !os.IsNotExist(err) {
		t.Fatalf("queue file exists after rejected add: %v", err)
	}
}

func TestListLoadDoesNotMutateQueueFile(t *testing.T) {
	store := testStore(t)
	if _, err := store.Add("stable"); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	before := readFile(t, store.QueuePath())
	if _, _, err := store.Load(); err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	after := readFile(t, store.QueuePath())
	if before != after {
		t.Fatalf("Load mutated queue file\nbefore: %s\nafter: %s", before, after)
	}
}

func TestCorruptedQueueFileReportsUsefulError(t *testing.T) {
	store := testStore(t)
	if err := os.MkdirAll(store.Store.BrevityRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.QueuePath(), []byte(`{"version":1,"items":[`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := store.Load()
	if err == nil {
		t.Fatalf("Load succeeded, want parse error")
	}
	if !strings.Contains(err.Error(), "parse runtime-queue.json") {
		t.Fatalf("error = %q, want parse runtime-queue.json", err.Error())
	}
}

func TestRemoveByID(t *testing.T) {
	store := testStore(t)
	first, err := store.Add("first")
	if err != nil {
		t.Fatalf("Add first: %v", err)
	}
	if _, err := store.Add("second"); err != nil {
		t.Fatalf("Add second: %v", err)
	}
	removed, err := store.Remove(first.ID)
	if err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if removed.Task != "first" {
		t.Fatalf("removed task = %q", removed.Task)
	}
	loaded, _, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(loaded.Items) != 1 || loaded.Items[0].Task != "second" {
		t.Fatalf("remaining queue = %#v", loaded.Items)
	}
	if _, err := store.Remove("missing"); err == nil {
		t.Fatalf("Remove missing id succeeded, want error")
	}
}

func TestDuplicateIDsAreAvoided(t *testing.T) {
	store := testStore(t)
	calls := 0
	store.GenerateID = func(now time.Time) (string, error) {
		calls++
		if calls <= 2 {
			return "20260522-abc123", nil
		}
		return "20260522-def456", nil
	}
	if _, err := store.Add("first"); err != nil {
		t.Fatalf("Add first: %v", err)
	}
	second, err := store.Add("second")
	if err != nil {
		t.Fatalf("Add second: %v", err)
	}
	if second.ID != "20260522-def456" {
		t.Fatalf("second id = %q, want regenerated id", second.ID)
	}
}

func testStore(t *testing.T) Store {
	t.Helper()
	root := t.TempDir()
	base, err := bstate.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	next := 0
	return Store{
		Store: base,
		Now:   func() time.Time { return now },
		GenerateID: func(time.Time) (string, error) {
			next++
			if next == 1 {
				return "20260522-abc123", nil
			}
			return "20260522-def456", nil
		},
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
