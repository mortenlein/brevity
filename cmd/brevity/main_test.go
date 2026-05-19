package main

import (
	"strings"
	"testing"
)

func TestParseRuntimeStateValidMinimalJSON(t *testing.T) {
	state, err := parseRuntimeState([]byte(`{"schema":"brevity.runtime-state.v1"}`))
	if err != nil {
		t.Fatalf("parseRuntimeState returned error: %v", err)
	}

	if state.Schema != runtimeStateSchema {
		t.Fatalf("Schema = %q, want %q", state.Schema, runtimeStateSchema)
	}
}

func TestParseRuntimeStateRejectsUnsupportedSchema(t *testing.T) {
	_, err := parseRuntimeState([]byte(`{"schema":"brevity.runtime-state.v2"}`))
	if err == nil {
		t.Fatal("parseRuntimeState returned nil error")
	}

	if !strings.Contains(err.Error(), "unsupported runtime state schema") {
		t.Fatalf("error = %q, want unsupported schema error", err.Error())
	}
}

func TestParseRuntimeStateRejectsEmptyJSON(t *testing.T) {
	_, err := parseRuntimeState([]byte{})
	if err == nil {
		t.Fatal("parseRuntimeState returned nil error")
	}

	if !strings.Contains(err.Error(), "invalid runtime state JSON") {
		t.Fatalf("error = %q, want invalid JSON error", err.Error())
	}
}

func TestParseRuntimeStateRejectsInvalidJSON(t *testing.T) {
	_, err := parseRuntimeState([]byte(`{"schema":`))
	if err == nil {
		t.Fatal("parseRuntimeState returned nil error")
	}

	if !strings.Contains(err.Error(), "invalid runtime state JSON") {
		t.Fatalf("error = %q, want invalid JSON error", err.Error())
	}
}
