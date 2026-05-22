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

func TestInspectMissingQueueFile(t *testing.T) {
	store := testStore(t)
	inspection := store.Inspect()
	if inspection.State != "missing" {
		t.Fatalf("state = %q, want missing", inspection.State)
	}
	if inspection.TotalItems != 0 || len(inspection.CountsByStatus) != 0 {
		t.Fatalf("inspection = %#v, want empty missing inspection", inspection)
	}
}

func TestInspectEmptyQueueFile(t *testing.T) {
	store := testStore(t)
	writeQueue(t, store, Queue{Version: Version, Items: []Item{}})
	inspection := store.Inspect()
	if inspection.State != "valid" {
		t.Fatalf("state = %q, want valid", inspection.State)
	}
	if inspection.Version != Version || inspection.TotalItems != 0 {
		t.Fatalf("inspection = %#v, want empty v1 queue", inspection)
	}
}

func TestInspectValidQueueWithCounts(t *testing.T) {
	store := testStore(t)
	writeQueue(t, store, Queue{Version: Version, Items: []Item{
		testItem("old", "alpha", StatusQueued, "2026-05-22T10:00:00Z"),
		testItem("new", "beta", StatusQueued, "2026-05-22T11:30:00Z"),
		testItem("cancelled", "gamma", StatusCancelled, "2026-05-22T11:45:00Z"),
	}})
	inspection := store.Inspect()
	if inspection.State != "valid" {
		t.Fatalf("state = %q, want valid", inspection.State)
	}
	if inspection.TotalItems != 3 {
		t.Fatalf("total = %d, want 3", inspection.TotalItems)
	}
	if inspection.CountsByStatus[StatusQueued] != 2 || inspection.CountsByStatus[StatusCancelled] != 1 {
		t.Fatalf("counts = %#v", inspection.CountsByStatus)
	}
	if inspection.OldestQueuedItemAge != "2h0m0s" || inspection.NewestQueuedItemAge != "30m0s" {
		t.Fatalf("ages oldest=%q newest=%q", inspection.OldestQueuedItemAge, inspection.NewestQueuedItemAge)
	}
}

func TestInspectDuplicateIDs(t *testing.T) {
	store := testStore(t)
	writeQueue(t, store, Queue{Version: Version, Items: []Item{
		testItem("same", "alpha", StatusQueued, "2026-05-22T10:00:00Z"),
		testItem("same", "beta", StatusQueued, "2026-05-22T11:00:00Z"),
	}})
	inspection := store.Inspect()
	if inspection.State != "invalid" {
		t.Fatalf("state = %q, want invalid", inspection.State)
	}
	if len(inspection.DuplicateIDs) != 1 || inspection.DuplicateIDs[0] != "same" {
		t.Fatalf("duplicate ids = %#v", inspection.DuplicateIDs)
	}
}

func TestInspectCorruptedQueueFile(t *testing.T) {
	store := testStore(t)
	writeRawQueue(t, store, `{"version":1,"items":[`)
	inspection := store.Inspect()
	if inspection.State != "corrupted" {
		t.Fatalf("state = %q, want corrupted", inspection.State)
	}
	if !strings.Contains(inspection.Error, "parse runtime-queue.json") {
		t.Fatalf("error = %q, want parse runtime-queue.json", inspection.Error)
	}
}

func TestInspectUnsupportedFutureVersion(t *testing.T) {
	store := testStore(t)
	writeQueue(t, store, Queue{Version: Version + 1, Items: []Item{}})
	inspection := store.Inspect()
	if inspection.State != "invalid" {
		t.Fatalf("state = %q, want invalid", inspection.State)
	}
	if !inspection.UnsupportedFutureVersion {
		t.Fatalf("unsupported future version = false, want true")
	}
}

func TestInspectInvalidItem(t *testing.T) {
	store := testStore(t)
	writeQueue(t, store, Queue{Version: Version, Items: []Item{
		testItem("", "../bad", "running", "not-a-time"),
	}})
	inspection := store.Inspect()
	if inspection.State != "invalid" {
		t.Fatalf("state = %q, want invalid", inspection.State)
	}
	if len(inspection.InvalidItems) < 4 {
		t.Fatalf("invalid items = %#v, want multiple item diagnostics", inspection.InvalidItems)
	}
}

func TestInspectDoesNotMutateQueueFile(t *testing.T) {
	store := testStore(t)
	writeRawQueue(t, store, `{"version":1,"items":[]}`)
	before := readFile(t, store.QueuePath())
	_ = store.Inspect()
	after := readFile(t, store.QueuePath())
	if before != after {
		t.Fatalf("Inspect mutated queue file\nbefore: %s\nafter: %s", before, after)
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

func TestReserveQueueItem(t *testing.T) {
	store := testStore(t)
	item, err := store.Add("reservable")
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	reserved, err := store.Reserve(item.ID)
	if err != nil {
		t.Fatalf("Reserve returned error: %v", err)
	}
	if reserved.Reservation == nil {
		t.Fatal("reservation = nil, want metadata")
	}
	if reserved.Reservation.Owner != "runtime-supervisor" || reserved.Reservation.ReservationID != "res-test-abc123" {
		t.Fatalf("reservation = %#v", reserved.Reservation)
	}
	if reserved.Reservation.ReservedAt != "2026-05-22T12:00:00Z" || reserved.UpdatedAt != "2026-05-22T12:00:00Z" {
		t.Fatalf("timestamps reservedAt=%q updatedAt=%q", reserved.Reservation.ReservedAt, reserved.UpdatedAt)
	}
	loaded, _, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.Items[0].Reservation == nil || loaded.Items[0].Reservation.ReservationID != "res-test-abc123" {
		t.Fatalf("loaded reservation = %#v", loaded.Items[0].Reservation)
	}
}

func TestUnreserveQueueItem(t *testing.T) {
	store := testStore(t)
	item, err := store.Add("reservable")
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if _, err := store.Reserve(item.ID); err != nil {
		t.Fatalf("Reserve returned error: %v", err)
	}
	unreserved, err := store.Unreserve(item.ID)
	if err != nil {
		t.Fatalf("Unreserve returned error: %v", err)
	}
	if unreserved.Reservation != nil {
		t.Fatalf("reservation = %#v, want nil", unreserved.Reservation)
	}
	again, err := store.Unreserve(item.ID)
	if err != nil {
		t.Fatalf("second Unreserve returned error: %v", err)
	}
	if again.Reservation != nil {
		t.Fatalf("second reservation = %#v, want nil", again.Reservation)
	}
}

func TestReserveRejectsDuplicateReservation(t *testing.T) {
	store := testStore(t)
	item, err := store.Add("reservable")
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if _, err := store.Reserve(item.ID); err != nil {
		t.Fatalf("Reserve returned error: %v", err)
	}
	if _, err := store.Reserve(item.ID); err == nil {
		t.Fatal("second Reserve succeeded, want error")
	}
}

func TestReserveRejectsMissingItem(t *testing.T) {
	store := testStore(t)
	if _, err := store.Add("reservable"); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if _, err := store.Reserve("missing"); err == nil {
		t.Fatal("Reserve missing id succeeded, want error")
	}
}

func TestReservationPersistenceCompatibility(t *testing.T) {
	store := testStore(t)
	writeRawQueue(t, store, `{"version":1,"items":[{"id":"plain","task":"plain-task","provider":"gemini","profile":"default","status":"queued","createdAt":"2026-05-22T10:00:00Z","updatedAt":"2026-05-22T10:00:00Z"}]}`)
	queue, missing, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if missing || queue.Items[0].Reservation != nil {
		t.Fatalf("queue missing=%v reservation=%#v, want compatible unreserved item", missing, queue.Items[0].Reservation)
	}
}

func TestInspectCountsReservationsAndDetectsInvalidReservation(t *testing.T) {
	store := testStore(t)
	item := testItem("reserved", "reserved-task", StatusQueued, "2026-05-22T10:00:00Z")
	item.Reservation = &Reservation{Owner: "runtime-supervisor", ReservedAt: "bad-time", ReservationID: "res-bad"}
	writeQueue(t, store, Queue{Version: Version, Items: []Item{item}})
	inspection := store.Inspect()
	if inspection.ReservedItems != 1 {
		t.Fatalf("ReservedItems = %d, want 1", inspection.ReservedItems)
	}
	if inspection.State != "invalid" || len(inspection.InvalidReservations) != 1 {
		t.Fatalf("inspection = %#v, want invalid reservation", inspection)
	}
}

func TestReservationOperationsDoNotMutateTaskState(t *testing.T) {
	store := testStore(t)
	if err := os.MkdirAll(store.Store.BrevityRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	tasksPath := store.Store.Path("tasks.json")
	before := `{"items":[]}` + "\n"
	if err := os.WriteFile(tasksPath, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	item, err := store.Add("reservable")
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if _, err := store.Reserve(item.ID); err != nil {
		t.Fatalf("Reserve returned error: %v", err)
	}
	if _, err := store.Unreserve(item.ID); err != nil {
		t.Fatalf("Unreserve returned error: %v", err)
	}
	after := readFile(t, tasksPath)
	if after != before {
		t.Fatalf("tasks.json mutated\nbefore: %s\nafter: %s", before, after)
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
		GenerateReservationID: func(time.Time) (string, error) {
			return "res-test-abc123", nil
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

func writeQueue(t *testing.T, store Store, queue Queue) {
	t.Helper()
	if err := store.Store.WriteJSON(FileName, queue); err != nil {
		t.Fatal(err)
	}
}

func writeRawQueue(t *testing.T, store Store, contents string) {
	t.Helper()
	if err := os.MkdirAll(store.Store.BrevityRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.QueuePath(), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testItem(id, task, status, createdAt string) Item {
	return Item{
		ID:        id,
		Task:      task,
		Provider:  "gemini",
		Profile:   "default",
		Status:    status,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
}
