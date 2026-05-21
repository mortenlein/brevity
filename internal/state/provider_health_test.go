package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mortenlein/brevity/internal/state/locking"
)

func TestLoadProviderHealthParsesExistingShape(t *testing.T) {
	store := tempStore(t)
	writeProviderHealth(t, store, `{
  "codex": {"status": "healthy", "note": "ok", "updatedAt": "2026-05-21T10:00:00Z"},
  "gemini": {"status": "quota-constrained", "reason": "credits", "updatedAt": "2026-05-21T11:00:00Z"}
}`)

	health, missing, err := LoadProviderHealth(store)
	if err != nil {
		t.Fatalf("LoadProviderHealth returned error: %v", err)
	}
	if missing {
		t.Fatal("missing = true, want false")
	}
	if health["codex"].Status != StatusHealthy || health["codex"].Note != "ok" {
		t.Fatalf("codex health = %#v", health["codex"])
	}
	if health["gemini"].Note != "credits" {
		t.Fatalf("reason compatibility not mapped to note: %#v", health["gemini"])
	}
}

func TestLoadProviderHealthMissingFieldsDefaultUnknown(t *testing.T) {
	store := tempStore(t)
	writeProviderHealth(t, store, `{"codex": {}}`)

	health, _, err := LoadProviderHealth(store)
	if err != nil {
		t.Fatalf("LoadProviderHealth returned error: %v", err)
	}
	if health["codex"].Status != StatusUnknown {
		t.Fatalf("status = %q, want unknown", health["codex"].Status)
	}
}

func TestLoadProviderHealthRejectsInvalidStatus(t *testing.T) {
	store := tempStore(t)
	writeProviderHealth(t, store, `{"codex": {"status": "busy"}}`)

	_, _, err := LoadProviderHealth(store)
	if err == nil {
		t.Fatal("LoadProviderHealth succeeded; want invalid status error")
	}
}

func TestProviderHealthWriteReadRoundTripStableJSON(t *testing.T) {
	store := tempStore(t)
	health := ProviderHealthState{
		"gemini": {Status: StatusQuotaConstrained, Note: "credits", UpdatedAt: "2026-05-21T10:00:00Z"},
		"codex":  {Status: StatusHealthy, Note: "ok", UpdatedAt: "2026-05-21T09:00:00Z"},
	}
	if err := store.WriteJSON(ProviderHealthFile, health); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	data, err := os.ReadFile(store.Path(ProviderHealthFile))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	want := "{\n  \"codex\": {\n    \"status\": \"healthy\",\n    \"note\": \"ok\",\n    \"updatedAt\": \"2026-05-21T09:00:00Z\"\n  },\n  \"gemini\": {\n    \"status\": \"quota-constrained\",\n    \"note\": \"credits\",\n    \"updatedAt\": \"2026-05-21T10:00:00Z\"\n  }\n}\n"
	if string(data) != want {
		t.Fatalf("provider health JSON mismatch:\n%s", data)
	}

	roundTrip, _, err := LoadProviderHealth(store)
	if err != nil {
		t.Fatalf("LoadProviderHealth returned error: %v", err)
	}
	if roundTrip["gemini"].Status != StatusQuotaConstrained {
		t.Fatalf("roundtrip gemini status = %q", roundTrip["gemini"].Status)
	}
}

func TestProviderHealthServiceSetResetPreservesUnrelatedProviders(t *testing.T) {
	store := tempStore(t)
	writeProviderHealth(t, store, `{
  "codex": {"status": "unknown", "note": "", "updatedAt": ""},
  "gemini": {"status": "healthy", "note": "leave me", "updatedAt": "before"}
}`)
	service := ProviderHealthService{
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC) },
	}

	result, err := service.Set("codex", StatusQuotaConstrained, "test native go state")
	if err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Set success = false: %#v", result.Errors)
	}
	health, _, err := LoadProviderHealth(store)
	if err != nil {
		t.Fatalf("LoadProviderHealth returned error: %v", err)
	}
	if health["codex"].Status != StatusQuotaConstrained || health["codex"].Note != "test native go state" {
		t.Fatalf("codex health = %#v", health["codex"])
	}
	if health["gemini"].Note != "leave me" {
		t.Fatalf("gemini changed unexpectedly: %#v", health["gemini"])
	}

	result, err = service.Reset("codex")
	if err != nil {
		t.Fatalf("Reset returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Reset success = false: %#v", result.Errors)
	}
	health, _, err = LoadProviderHealth(store)
	if err != nil {
		t.Fatalf("LoadProviderHealth returned error: %v", err)
	}
	if health["codex"].Status != StatusUnknown {
		t.Fatalf("codex status = %q, want unknown", health["codex"].Status)
	}
}

func TestProviderHealthServiceLockedWriteReturnsCommandError(t *testing.T) {
	store := tempStore(t)
	writeProviderHealth(t, store, `{"codex": {"status": "unknown"}}`)
	if err := os.WriteFile(store.LockPath(), []byte("pid=1\ncreatedAt=2026-05-21T10:00:00Z\n"), 0o644); err != nil {
		t.Fatalf("WriteFile lock returned error: %v", err)
	}
	service := ProviderHealthService{
		Store:       store,
		LockOptions: locking.Options{Timeout: 20 * time.Millisecond, Interval: time.Millisecond},
	}

	result, err := service.Set("codex", StatusHealthy, "")
	if err == nil {
		t.Fatal("Set succeeded; want lock error")
	}
	if result.Success || len(result.Errors) == 0 || result.Errors[0].Code != "provider-health-locked" {
		t.Fatalf("lock result = %#v", result)
	}
}

func tempStore(t *testing.T) Store {
	t.Helper()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	if err := os.MkdirAll(store.BrevityRoot(), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	return store
}

func writeProviderHealth(t *testing.T, store Store, content string) {
	t.Helper()
	if !json.Valid([]byte(content)) {
		t.Fatalf("test fixture is not valid JSON: %s", content)
	}
	if err := os.WriteFile(filepath.Join(store.BrevityRoot(), ProviderHealthFile), []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}
