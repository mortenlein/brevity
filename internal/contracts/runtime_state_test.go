package contracts

import (
	"strings"
	"testing"
)

func TestParseRuntimeStateValidMinimalJSON(t *testing.T) {
	state, err := ParseRuntimeState([]byte(`{"schema":"brevity.runtime-state.v1"}`))
	if err != nil {
		t.Fatalf("ParseRuntimeState returned error: %v", err)
	}

	if state.Schema != RuntimeStateSchema {
		t.Fatalf("Schema = %q, want %q", state.Schema, RuntimeStateSchema)
	}
}

func TestParseRuntimeStateRejectsUnsupportedSchema(t *testing.T) {
	_, err := ParseRuntimeState([]byte(`{"schema":"brevity.runtime-state.v2"}`))
	if err == nil {
		t.Fatal("ParseRuntimeState returned nil error")
	}

	if !strings.Contains(err.Error(), "unsupported runtime state schema") {
		t.Fatalf("error = %q, want unsupported schema error", err.Error())
	}
}

func TestParseRuntimeStateRejectsEmptyJSON(t *testing.T) {
	_, err := ParseRuntimeState([]byte{})
	if err == nil {
		t.Fatal("ParseRuntimeState returned nil error")
	}

	if !strings.Contains(err.Error(), "invalid runtime state JSON") {
		t.Fatalf("error = %q, want invalid JSON error", err.Error())
	}
}

func TestParseRuntimeStateRejectsInvalidJSON(t *testing.T) {
	_, err := ParseRuntimeState([]byte(`{"schema":`))
	if err == nil {
		t.Fatal("ParseRuntimeState returned nil error")
	}

	if !strings.Contains(err.Error(), "invalid runtime state JSON") {
		t.Fatalf("error = %q, want invalid JSON error", err.Error())
	}
}
