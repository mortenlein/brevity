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

func TestMarkReadyTransitionsPlannedExecution(t *testing.T) {
	store := testStore(t)
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusPlanned)}})

	result, err := store.MarkReady("exec-1")
	if err != nil {
		t.Fatalf("MarkReady returned error: %v", err)
	}
	if result.ID != "exec-1" || result.Task != "some-task" || result.OldStatus != StatusPlanned || result.NewStatus != StatusReady {
		t.Fatalf("result = %#v", result)
	}
	loaded, _, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.Records[0].Status != StatusReady {
		t.Fatalf("status = %q, want ready", loaded.Records[0].Status)
	}
}

func TestMarkReadyRejectsMissingExecutionID(t *testing.T) {
	store := testStore(t)
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusPlanned)}})
	before := readFile(t, store.Path())
	if _, err := store.MarkReady(" "); err == nil {
		t.Fatal("MarkReady succeeded, want missing id error")
	}
	after := readFile(t, store.Path())
	if after != before {
		t.Fatalf("executions mutated\nbefore: %s\nafter: %s", before, after)
	}
}

func TestMarkReadyRejectsCancelledExecution(t *testing.T) {
	store := testStore(t)
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusCancelled)}})
	before := readFile(t, store.Path())
	_, err := store.MarkReady("exec-1")
	if err == nil {
		t.Fatal("MarkReady succeeded, want cancelled status error")
	}
	if !strings.Contains(err.Error(), "status is cancelled, want planned") {
		t.Fatalf("error = %q", err.Error())
	}
	if after := readFile(t, store.Path()); after != before {
		t.Fatalf("executions mutated\nbefore: %s\nafter: %s", before, after)
	}
}

func TestMarkReadyRejectsAlreadyReadyExecution(t *testing.T) {
	store := testStore(t)
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusReady)}})
	before := readFile(t, store.Path())
	_, err := store.MarkReady("exec-1")
	if err == nil {
		t.Fatal("MarkReady succeeded, want ready status error")
	}
	if !strings.Contains(err.Error(), "status is ready, want planned") {
		t.Fatalf("error = %q", err.Error())
	}
	if after := readFile(t, store.Path()); after != before {
		t.Fatalf("executions mutated\nbefore: %s\nafter: %s", before, after)
	}
}

func TestMarkReadyUpdatesUpdatedAt(t *testing.T) {
	store := testStore(t)
	store.Now = func() time.Time { return time.Date(2026, 5, 22, 13, 30, 0, 0, time.UTC) }
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusPlanned)}})
	if _, err := store.MarkReady("exec-1"); err != nil {
		t.Fatalf("MarkReady returned error: %v", err)
	}
	loaded, _, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.Records[0].CreatedAt != "2026-05-22T12:00:00Z" {
		t.Fatalf("createdAt = %q", loaded.Records[0].CreatedAt)
	}
	if loaded.Records[0].UpdatedAt != "2026-05-22T13:30:00Z" {
		t.Fatalf("updatedAt = %q, want transition time", loaded.Records[0].UpdatedAt)
	}
}

func TestMarkReadyDoesNotMutateQueueState(t *testing.T) {
	store := testStore(t)
	item := reservedQueueItemFixture("queue-abc123", "some-task", "res-abc123")
	writeQueue(t, store, runtimequeue.Queue{Version: runtimequeue.Version, Items: []runtimequeue.Item{item}})
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusPlanned)}})
	before := readFile(t, store.Queue.QueuePath())
	if _, err := store.MarkReady("exec-1"); err != nil {
		t.Fatalf("MarkReady returned error: %v", err)
	}
	if after := readFile(t, store.Queue.QueuePath()); after != before {
		t.Fatalf("queue mutated\nbefore: %s\nafter: %s", before, after)
	}
}

func TestInspectCountsReadyStatus(t *testing.T) {
	store := testStore(t)
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{
		recordFixture("exec-1", StatusPlanned),
		recordFixture("exec-2", StatusReady),
	}})
	inspection := store.Inspect()
	if inspection.CountsByStatus[StatusPlanned] != 1 || inspection.CountsByStatus[StatusReady] != 1 {
		t.Fatalf("counts = %#v", inspection.CountsByStatus)
	}
}

func TestInspectReportsNewestExecutionTaskAndStatus(t *testing.T) {
	store := testStore(t)
	oldRecord := recordFixture("exec-1", StatusPlanned)
	oldRecord.Task = "old-task"
	oldRecord.CreatedAt = "2026-05-22T12:00:00Z"
	newRecord := recordFixture("exec-2", StatusFailed)
	newRecord.Task = "new-task"
	newRecord.CreatedAt = "2026-05-22T13:00:00Z"
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{oldRecord, newRecord}})

	inspection := store.Inspect()
	if inspection.NewestExecutionTask != "new-task" || inspection.NewestExecutionStatus != StatusFailed {
		t.Fatalf("newest execution = %q/%q, want new-task/failed", inspection.NewestExecutionTask, inspection.NewestExecutionStatus)
	}
}

func TestMarkPlannedRollsReadyBackToPlanned(t *testing.T) {
	store := testStore(t)
	writeExecutions(t, store, Executions{Version: Version, Records: []Record{recordFixture("exec-1", StatusReady)}})
	result, err := store.MarkPlanned("exec-1")
	if err != nil {
		t.Fatalf("MarkPlanned returned error: %v", err)
	}
	if result.OldStatus != StatusReady || result.NewStatus != StatusPlanned {
		t.Fatalf("result = %#v", result)
	}
	loaded, _, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.Records[0].Status != StatusPlanned {
		t.Fatalf("status = %q, want planned", loaded.Records[0].Status)
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
