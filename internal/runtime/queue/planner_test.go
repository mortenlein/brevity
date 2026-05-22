package queue

import (
	"strings"
	"testing"
)

func TestPlanMissingQueueIsEmpty(t *testing.T) {
	store := testStore(t)
	plan := store.Plan()
	if plan.State != "missing" {
		t.Fatalf("state = %q, want missing", plan.State)
	}
	if plan.Summary.Runnable != 0 || plan.Summary.Skipped != 0 {
		t.Fatalf("summary = %#v, want empty", plan.Summary)
	}
	if !plan.ReadOnly {
		t.Fatalf("ReadOnly = false, want true")
	}
}

func TestPlanValidRunnableItemsInQueueOrder(t *testing.T) {
	store := testStore(t)
	writeQueue(t, store, Queue{Version: Version, Items: []Item{
		testItem("b", "second-in-file", StatusQueued, "2026-05-22T10:00:00Z"),
		testItem("a", "first-in-priorityless-order", StatusQueued, "2026-05-22T11:00:00Z"),
	}})
	plan := store.Plan()
	if plan.State != "valid" {
		t.Fatalf("state = %q, want valid", plan.State)
	}
	if plan.Summary.Runnable != 2 || plan.Summary.Skipped != 0 {
		t.Fatalf("summary = %#v", plan.Summary)
	}
	if plan.Runnable[0].Task != "second-in-file" || plan.Runnable[1].Task != "first-in-priorityless-order" {
		t.Fatalf("runnable order = %#v", plan.Runnable)
	}
}

func TestPlanSkippedInvalidItem(t *testing.T) {
	store := testStore(t)
	writeQueue(t, store, Queue{Version: Version, Items: []Item{
		testItem("bad", "../bad", StatusQueued, "2026-05-22T10:00:00Z"),
	}})
	plan := store.Plan()
	if plan.Summary.Runnable != 0 || plan.Summary.Skipped != 1 {
		t.Fatalf("summary = %#v", plan.Summary)
	}
	if plan.Skipped[0].Reason != "invalid task slug" {
		t.Fatalf("reason = %q", plan.Skipped[0].Reason)
	}
}

func TestPlanSkipsDuplicateIDs(t *testing.T) {
	store := testStore(t)
	writeQueue(t, store, Queue{Version: Version, Items: []Item{
		testItem("same", "alpha", StatusQueued, "2026-05-22T10:00:00Z"),
		testItem("same", "beta", StatusQueued, "2026-05-22T11:00:00Z"),
		testItem("other", "gamma", StatusQueued, "2026-05-22T11:30:00Z"),
	}})
	plan := store.Plan()
	if plan.Summary.Runnable != 1 || plan.Runnable[0].Task != "gamma" {
		t.Fatalf("runnable = %#v", plan.Runnable)
	}
	if plan.Summary.Skipped != 2 {
		t.Fatalf("skipped = %#v", plan.Skipped)
	}
	for _, item := range plan.Skipped {
		if item.Reason != "duplicate queue id" {
			t.Fatalf("skipped reason = %q", item.Reason)
		}
	}
}

func TestPlanUnsupportedStatus(t *testing.T) {
	store := testStore(t)
	writeQueue(t, store, Queue{Version: Version, Items: []Item{
		testItem("cancelled", "done-for-now", StatusCancelled, "2026-05-22T10:00:00Z"),
	}})
	plan := store.Plan()
	if plan.Summary.Runnable != 0 || plan.Summary.Skipped != 1 {
		t.Fatalf("summary = %#v", plan.Summary)
	}
	if !strings.Contains(plan.Skipped[0].Reason, "unsupported status") {
		t.Fatalf("reason = %q", plan.Skipped[0].Reason)
	}
}

func TestPlanExcludesReservedItems(t *testing.T) {
	store := testStore(t)
	reserved := testItem("reserved", "reserved-task", StatusQueued, "2026-05-22T10:00:00Z")
	reserved.Reservation = &Reservation{
		Owner:         "runtime-supervisor",
		ReservedAt:    "2026-05-22T11:00:00Z",
		ReservationID: "res-test-abc123",
	}
	writeQueue(t, store, Queue{Version: Version, Items: []Item{
		reserved,
		testItem("open", "open-task", StatusQueued, "2026-05-22T10:30:00Z"),
	}})
	plan := store.Plan()
	if plan.Summary.Runnable != 1 || plan.Summary.Skipped != 1 || plan.Summary.Reserved != 1 {
		t.Fatalf("summary = %#v", plan.Summary)
	}
	if plan.Runnable[0].Task != "open-task" {
		t.Fatalf("runnable = %#v", plan.Runnable)
	}
	if plan.Skipped[0].Reason != "reserved by runtime-supervisor" {
		t.Fatalf("skip reason = %q", plan.Skipped[0].Reason)
	}
}

func TestPlanDetectsInvalidReservation(t *testing.T) {
	store := testStore(t)
	reserved := testItem("reserved", "reserved-task", StatusQueued, "2026-05-22T10:00:00Z")
	reserved.Reservation = &Reservation{Owner: "", ReservedAt: "2026-05-22T11:00:00Z", ReservationID: "res-test-abc123"}
	writeQueue(t, store, Queue{Version: Version, Items: []Item{reserved}})
	plan := store.Plan()
	if plan.Summary.Runnable != 0 || plan.Summary.Skipped != 1 || plan.Summary.Reserved != 0 {
		t.Fatalf("summary = %#v", plan.Summary)
	}
	if !strings.Contains(plan.Skipped[0].Reason, "invalid reservation") {
		t.Fatalf("skip reason = %q", plan.Skipped[0].Reason)
	}
}

func TestPlanCorruptedQueue(t *testing.T) {
	store := testStore(t)
	writeRawQueue(t, store, `{"version":1,"items":[`)
	plan := store.Plan()
	if plan.State != "corrupted" {
		t.Fatalf("state = %q, want corrupted", plan.State)
	}
	if !strings.Contains(plan.Error, "parse runtime-queue.json") {
		t.Fatalf("error = %q", plan.Error)
	}
	if plan.Summary.Runnable != 0 || plan.Summary.Skipped != 0 {
		t.Fatalf("summary = %#v", plan.Summary)
	}
}

func TestPlanFutureVersion(t *testing.T) {
	store := testStore(t)
	writeQueue(t, store, Queue{Version: Version + 1, Items: []Item{
		testItem("future", "future-task", StatusQueued, "2026-05-22T10:00:00Z"),
	}})
	plan := store.Plan()
	if plan.State != "invalid" {
		t.Fatalf("state = %q, want invalid", plan.State)
	}
	if !strings.Contains(plan.Error, "unsupported future") {
		t.Fatalf("error = %q", plan.Error)
	}
	if plan.Summary.Runnable != 0 {
		t.Fatalf("runnable = %d, want 0", plan.Summary.Runnable)
	}
}

func TestPlanDoesNotMutateQueueFile(t *testing.T) {
	store := testStore(t)
	writeRawQueue(t, store, `{"version":1,"items":[]}`)
	before := readFile(t, store.QueuePath())
	_ = store.Plan()
	after := readFile(t, store.QueuePath())
	if before != after {
		t.Fatalf("Plan mutated queue file\nbefore: %s\nafter: %s", before, after)
	}
}
