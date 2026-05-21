package actions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/state"
	"github.com/mortenlein/brevity/internal/state/locking"
)

func TestTaskStartServiceSuccessfulStart(t *testing.T) {
	store := taskStartStore(t, `[{"slug":"alpha","status":"planned","custom":"keep"}]`)
	service := TaskStartService{
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC) },
	}
	result, err := service.Start("alpha")
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if !result.Success || result.Command != "task start" {
		t.Fatalf("result = %#v", result)
	}
	payload, err := contracts.ParseTaskStartPayload(result)
	if err != nil {
		t.Fatalf("ParseTaskStartPayload returned error: %v", err)
	}
	if payload.Slug != "alpha" || payload.OldState != "planned" || payload.NewState != "ready-for-worker" || !payload.RefreshExpected || !payload.NoExecution {
		t.Fatalf("payload = %#v", payload)
	}
	data, err := os.ReadFile(store.Path(state.TasksFile))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	for _, want := range []string{`"status": "ready-for-worker"`, `"normalizedState": "ready-for-worker"`, `"custom": "keep"`, `"startedAt": "2026-05-21T10:00:00Z"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("tasks.json missing %q:\n%s", want, data)
		}
	}
}

func TestTaskStartServiceBlockedByPreflight(t *testing.T) {
	store := taskStartStore(t, `[{"slug":"alpha","status":"blocked"}]`)
	result, err := TaskStartService{Store: store}.Start("alpha")
	if err == nil {
		t.Fatal("Start succeeded; want blocked error")
	}
	if result.Success || len(result.Errors) == 0 || result.Errors[0].Code != "preflight-blocked" {
		t.Fatalf("result = %#v", result)
	}
}

func TestTaskStartServiceMissingLockAndMalformed(t *testing.T) {
	for name, setup := range map[string]func(t *testing.T) state.Store{
		"missing": func(t *testing.T) state.Store {
			return taskStartStore(t, `[{"slug":"alpha","status":"planned"}]`)
		},
		"locked": func(t *testing.T) state.Store {
			store := taskStartStore(t, `[{"slug":"alpha","status":"planned"}]`)
			if err := os.WriteFile(store.LockPath(), []byte("pid=1\ncreatedAt=2026-05-21T10:00:00Z\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			return store
		},
		"malformed": func(t *testing.T) state.Store {
			return taskStartStore(t, `[{"slug":`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			store := setup(t)
			service := TaskStartService{Store: store, LockOptions: locking.Options{Timeout: 20 * time.Millisecond, Interval: time.Millisecond}}
			slug := "alpha"
			if name == "missing" {
				slug = "missing"
			}
			result, err := service.Start(slug)
			if err == nil {
				t.Fatal("Start succeeded; want error")
			}
			if result.Success || len(result.Errors) == 0 {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestTaskStartResultJSONShape(t *testing.T) {
	store := taskStartStore(t, `[{"slug":"alpha","status":"ready-for-worker"}]`)
	result, err := TaskStartService{Store: store}.Start("alpha")
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	for _, want := range []string{`"schema":"brevity.command-result.v1"`, `"command":"task start"`, `"action":"task-start"`, `"slug":"alpha"`, `"refreshExpected":true`, `"noExecution":true`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("result JSON missing %q:\n%s", want, data)
		}
	}
}

func taskStartStore(t *testing.T, tasksJSON string) state.Store {
	t.Helper()
	repoRoot := t.TempDir()
	store, err := state.NewStore(repoRoot)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	if err := os.MkdirAll(store.BrevityRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path(state.TasksFile), []byte(tasksJSON+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path(state.ProviderHealthFile), []byte(`{"codex":{"status":"healthy"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(repoRoot, "worktrees", "active", "alpha"), 0o755)
	return store
}
