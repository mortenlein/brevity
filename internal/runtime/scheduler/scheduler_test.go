package scheduler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runtimequeue "github.com/mortenlein/brevity/internal/runtime/queue"
	bstate "github.com/mortenlein/brevity/internal/state"
)

func TestPlanEmptyQueueSelectsNothing(t *testing.T) {
	store := testQueueStore(t)

	plan := Planner{Queue: store}.Plan()

	if plan.Selected != nil {
		t.Fatalf("selected = %#v, want nil", plan.Selected)
	}
	if plan.NoSelectionReason != "no eligible runnable queue item" {
		t.Fatalf("no selection reason = %q", plan.NoSelectionReason)
	}
	if plan.ReservationEligible {
		t.Fatalf("reservation eligible = true, want false")
	}
}

func TestPlanSelectsFirstRunnableItem(t *testing.T) {
	store := testQueueStore(t)
	writeQueue(t, store, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		testItem("first", "alpha", runtimequeue.StatusQueued, "2026-05-22T10:00:00Z"),
		testItem("second", "beta", runtimequeue.StatusQueued, "2026-05-22T11:00:00Z"),
	}})

	plan := Planner{Queue: store}.Plan()

	if plan.Selected == nil || plan.Selected.ID != "first" || plan.Selected.Task != "alpha" {
		t.Fatalf("selected = %#v, want first alpha", plan.Selected)
	}
	if !plan.ReservationEligible {
		t.Fatalf("reservation eligible = false, want true")
	}
	if plan.Selected.Reason != "first eligible runnable queue item in queue order" {
		t.Fatalf("reason = %q", plan.Selected.Reason)
	}
}

func TestPlanReservedFirstItemSelectsNextRunnableItem(t *testing.T) {
	store := testQueueStore(t)
	reserved := testItem("first", "alpha", runtimequeue.StatusQueued, "2026-05-22T10:00:00Z")
	reserved.Reservation = &runtimequeue.Reservation{
		Owner:         "runtime-supervisor",
		ReservedAt:    "2026-05-22T12:00:00Z",
		ReservationID: "res-test-abc123",
	}
	writeQueue(t, store, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		reserved,
		testItem("second", "beta", runtimequeue.StatusQueued, "2026-05-22T11:00:00Z"),
	}})

	plan := Planner{Queue: store}.Plan()

	if plan.Selected == nil || plan.Selected.ID != "second" {
		t.Fatalf("selected = %#v, want second", plan.Selected)
	}
	if len(plan.Skipped) != 1 || !strings.Contains(plan.Skipped[0].Reason, "reserved") {
		t.Fatalf("skipped = %#v, want reserved first item", plan.Skipped)
	}
}

func TestPlanInvalidFirstItemIsSkipped(t *testing.T) {
	store := testQueueStore(t)
	writeQueue(t, store, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		testItem("bad", "../bad", runtimequeue.StatusQueued, "2026-05-22T10:00:00Z"),
		testItem("good", "beta", runtimequeue.StatusQueued, "2026-05-22T11:00:00Z"),
	}})

	plan := Planner{Queue: store}.Plan()

	if plan.Selected == nil || plan.Selected.ID != "good" {
		t.Fatalf("selected = %#v, want good", plan.Selected)
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].Reason != "invalid task slug" {
		t.Fatalf("skipped = %#v", plan.Skipped)
	}
}

func TestPlanUnsupportedStatusIsSkipped(t *testing.T) {
	store := testQueueStore(t)
	writeQueue(t, store, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{
		testItem("running", "alpha", "running", "2026-05-22T10:00:00Z"),
		testItem("good", "beta", runtimequeue.StatusQueued, "2026-05-22T11:00:00Z"),
	}})

	plan := Planner{Queue: store}.Plan()

	if plan.Selected == nil || plan.Selected.ID != "good" {
		t.Fatalf("selected = %#v, want good", plan.Selected)
	}
	if len(plan.Skipped) != 1 || !strings.Contains(plan.Skipped[0].Reason, "unsupported status") {
		t.Fatalf("skipped = %#v", plan.Skipped)
	}
}

func TestPlanCorruptedQueueSelectsNothingWithClearReason(t *testing.T) {
	store := testQueueStore(t)
	writeRawQueue(t, store, `{"version":1,"items":[`)

	plan := Planner{Queue: store}.Plan()

	if plan.Selected != nil {
		t.Fatalf("selected = %#v, want nil", plan.Selected)
	}
	if plan.QueueState != "corrupted" {
		t.Fatalf("queue state = %q, want corrupted", plan.QueueState)
	}
	if !strings.Contains(plan.NoSelectionReason, "parse runtime-queue.json") {
		t.Fatalf("reason = %q", plan.NoSelectionReason)
	}
}

func TestPlanFutureQueueVersionSelectsNothingWithClearReason(t *testing.T) {
	store := testQueueStore(t)
	writeQueue(t, store, runtimequeue.Queue{Version: runtimequeue.Version + 1, Items: []runtimequeue.Item{
		testItem("future", "alpha", runtimequeue.StatusQueued, "2026-05-22T10:00:00Z"),
	}})

	plan := Planner{Queue: store}.Plan()

	if plan.Selected != nil {
		t.Fatalf("selected = %#v, want nil", plan.Selected)
	}
	if plan.QueueState != "invalid" {
		t.Fatalf("queue state = %q, want invalid", plan.QueueState)
	}
	if !strings.Contains(plan.NoSelectionReason, "unsupported future") {
		t.Fatalf("reason = %q", plan.NoSelectionReason)
	}
}

func TestPlanIsReadOnly(t *testing.T) {
	store := testQueueStore(t)
	plan := Planner{Queue: store}.Plan()
	if !plan.ReadOnly {
		t.Fatalf("readOnly = false, want true")
	}
	for _, check := range plan.SafetyChecks {
		if check.Name == "read-only planning" && !check.Passed {
			t.Fatalf("read-only check = %#v, want passed", check)
		}
	}
}

func TestPlanDoesNotMutateQueueFile(t *testing.T) {
	store := testQueueStore(t)
	writeRawQueue(t, store, `{"version":1,"items":[]}`)
	before := readFile(t, store.QueuePath())

	_ = Planner{Queue: store}.Plan()

	after := readFile(t, store.QueuePath())
	if before != after {
		t.Fatalf("scheduler plan mutated queue file\nbefore: %s\nafter: %s", before, after)
	}
}

func testQueueStore(t *testing.T) runtimequeue.Store {
	t.Helper()
	base, err := bstate.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return runtimequeue.Store{
		Store: base,
		Now: func() time.Time {
			return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
		},
	}
}

func writeQueue(t *testing.T, store runtimequeue.Store, queue runtimequeue.Queue) {
	t.Helper()
	if err := store.Store.WriteJSON(runtimequeue.FileName, queue); err != nil {
		t.Fatal(err)
	}
}

func writeRawQueue(t *testing.T, store runtimequeue.Store, contents string) {
	t.Helper()
	if err := os.MkdirAll(store.Store.BrevityRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.QueuePath(), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
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

func testItem(id, task, status, createdAt string) runtimequeue.Item {
	return runtimequeue.Item{
		ID:        id,
		Task:      task,
		Provider:  "gemini",
		Profile:   "default",
		Status:    status,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
}
