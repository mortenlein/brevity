package execution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runtimequeue "github.com/mortenlein/brevity/internal/runtime/queue"
	bstate "github.com/mortenlein/brevity/internal/state"
)

func TestMissingExecutionsFileReadsAsEmpty(t *testing.T) {
	store := testStore(t)
	executions, missing, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !missing {
		t.Fatalf("missing = false, want true")
	}
	if executions.Version != Version || len(executions.Records) != 0 {
		t.Fatalf("executions = %#v, want empty v1 executions", executions)
	}
}

func TestInspectMissingExecutionsFile(t *testing.T) {
	store := testStore(t)
	inspection := store.Inspect()
	if inspection.State != "missing" {
		t.Fatalf("state = %q, want missing", inspection.State)
	}
	if inspection.Version != Version || inspection.TotalExecutions != 0 {
		t.Fatalf("inspection = %#v, want empty missing inspection", inspection)
	}
}

func TestCorruptedExecutionsFileReportsUsefulError(t *testing.T) {
	store := testStore(t)
	writeRawExecutions(t, store, `{"version":1,"executions":[`)
	_, _, err := store.Load()
	if err == nil {
		t.Fatal("Load succeeded, want parse error")
	}
	if !strings.Contains(err.Error(), "parse runtime-executions.json") {
		t.Fatalf("error = %q, want parse runtime-executions.json", err.Error())
	}
	inspection := store.Inspect()
	if inspection.State != "corrupted" {
		t.Fatalf("state = %q, want corrupted", inspection.State)
	}
	if !strings.Contains(inspection.Error, "parse runtime-executions.json") {
		t.Fatalf("inspection error = %q", inspection.Error)
	}
}

func TestPlanFromReservationCreatesPlannedExecution(t *testing.T) {
	store := testStore(t)
	item := reservedQueueItemFixture("queue-abc123", "some-task", "res-abc123")
	writeQueue(t, store, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{item}})

	record, err := store.PlanFromReservation(item.ID)
	if err != nil {
		t.Fatalf("PlanFromReservation returned error: %v", err)
	}
	if record.ID != "exec-test-abc123" || record.QueueItemID != item.ID || record.Task != item.Task || record.ReservationID != "res-abc123" {
		t.Fatalf("record = %#v", record)
	}
	if record.Status != StatusPlanned {
		t.Fatalf("status = %q, want planned", record.Status)
	}
	if record.CreatedAt != "2026-05-22T12:00:00Z" || record.UpdatedAt != "2026-05-22T12:00:00Z" {
		t.Fatalf("timestamps = %q %q", record.CreatedAt, record.UpdatedAt)
	}
	loaded, missing, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if missing || len(loaded.Records) != 1 {
		t.Fatalf("loaded missing=%v records=%d", missing, len(loaded.Records))
	}
}

func TestPlanFromReservationRejectsUnreservedQueueItem(t *testing.T) {
	store := testStore(t)
	item := queueItemFixture("queue-abc123", "some-task")
	writeQueue(t, store, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{item}})
	if _, err := store.PlanFromReservation(item.ID); err == nil {
		t.Fatal("PlanFromReservation succeeded, want unreserved error")
	}
	if _, err := os.Stat(store.Path()); !os.IsNotExist(err) {
		t.Fatalf("executions file exists after rejected plan: %v", err)
	}
}

func TestPlanFromReservationRejectsDuplicateQueueItemReservation(t *testing.T) {
	store := testStore(t)
	item := reservedQueueItemFixture("queue-abc123", "some-task", "res-abc123")
	writeQueue(t, store, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{item}})
	if _, err := store.PlanFromReservation(item.ID); err != nil {
		t.Fatalf("first PlanFromReservation returned error: %v", err)
	}
	if _, err := store.PlanFromReservation(item.ID); err == nil {
		t.Fatal("second PlanFromReservation succeeded, want duplicate error")
	}
	loaded, _, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(loaded.Records) != 1 {
		t.Fatalf("records = %d, want 1", len(loaded.Records))
	}
}

func TestExecutionListLoadIsReadOnly(t *testing.T) {
	store := testStore(t)
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", "planned")}})
	before := readFile(t, store.Path())
	if _, _, err := store.Load(); err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	after := readFile(t, store.Path())
	if before != after {
		t.Fatalf("Load mutated executions file\nbefore: %s\nafter: %s", before, after)
	}
}

func TestExecutionInspectIsReadOnly(t *testing.T) {
	store := testStore(t)
	writeRawExecutions(t, store, `{"version":1,"executions":[]}`)
	before := readFile(t, store.Path())
	_ = store.Inspect()
	after := readFile(t, store.Path())
	if before != after {
		t.Fatalf("Inspect mutated executions file\nbefore: %s\nafter: %s", before, after)
	}
}

func testStore(t *testing.T) Store {
	t.Helper()
	root := t.TempDir()
	base, err := bstate.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	queueStore, err := runtimequeue.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	return Store{
		Store: base,
		Queue: queueStore,
		Now:   func() time.Time { return now },
		GenerateID: func(time.Time) (string, error) {
			return "exec-test-abc123", nil
		},
	}
}

func writeQueue(t *testing.T, store Store, queue runtimequeue.Queue) {
	t.Helper()
	if err := store.Store.WriteJSON(runtimequeue.FileName, queue); err != nil {
		t.Fatal(err)
	}
}

func writeExecutions(t *testing.T, store Store, executions Executions) {
	t.Helper()
	if err := store.Store.WriteJSON(FileName, executions); err != nil {
		t.Fatal(err)
	}
}

func writeRawExecutions(t *testing.T, store Store, contents string) {
	t.Helper()
	if err := os.MkdirAll(store.Store.BrevityRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path(), []byte(contents), 0o644); err != nil {
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

func reservedQueueItemFixture(id, task, reservationID string) runtimequeue.Item {
	item := queueItemFixture(id, task)
	item.Reservation = &runtimequeue.Reservation{
		Owner:         "runtime-supervisor",
		ReservedAt:    "2026-05-22T11:00:00Z",
		ReservationID: reservationID,
	}
	return item
}

func queueItemFixture(id, task string) runtimequeue.Item {
	return runtimequeue.Item{
		ID:        id,
		Task:      task,
		Provider:  "gemini",
		Profile:   "default",
		Status:    runtimequeue.StatusQueued,
		CreatedAt: "2026-05-22T10:00:00Z",
		UpdatedAt: "2026-05-22T10:00:00Z",
	}
}

func recordFixture(id, status string) Record {
	return Record{
		ID:            id,
		QueueItemID:   "queue-abc123",
		Task:          "some-task",
		ReservationID: "res-abc123",
		Status:        status,
		CreatedAt:     "2026-05-22T12:00:00Z",
		UpdatedAt:     "2026-05-22T12:00:00Z",
	}
}
