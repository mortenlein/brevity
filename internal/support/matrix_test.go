package support

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMatrixContainsMajorMigratedCapabilities(t *testing.T) {
	want := []string{"provider-health", "task-new", "task-run-execute", "task-merge", "cleanup-orphans"}
	for _, id := range want {
		if !hasCapability(id) {
			t.Fatalf("Matrix missing capability %q", id)
		}
	}
}

func TestWriteJSONStableShape(t *testing.T) {
	var output bytes.Buffer
	if err := WriteJSON(&output); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	var parsed []Capability
	if err := json.Unmarshal(output.Bytes(), &parsed); err != nil {
		t.Fatalf("support matrix JSON did not parse: %v", err)
	}
	if len(parsed) != len(Matrix) {
		t.Fatalf("parsed %d capabilities, want %d", len(parsed), len(Matrix))
	}
}

func TestDocsSupportMatrixJSONMatchesIDs(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "brevity-support-matrix.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read docs support matrix: %v", err)
	}
	var parsed []Capability
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("docs support matrix JSON did not parse: %v", err)
	}
	if len(parsed) != len(Matrix) {
		t.Fatalf("docs matrix has %d capabilities, want %d", len(parsed), len(Matrix))
	}
	for index, capability := range Matrix {
		if parsed[index].ID != capability.ID {
			t.Fatalf("docs matrix id[%d] = %q, want %q", index, parsed[index].ID, capability.ID)
		}
	}
}

func hasCapability(id string) bool {
	for _, capability := range Matrix {
		if capability.ID == id {
			return true
		}
	}
	return false
}
