package main

import (
	"bytes"
	"strings"
	"testing"
)

type fakeRuntimeClient struct {
	output []byte
	err    error
}

func (client fakeRuntimeClient) RuntimeStateJSON() ([]byte, error) {
	return client.output, client.err
}

func TestRunWithClientRendersRuntimeState(t *testing.T) {
	client := fakeRuntimeClient{
		output: []byte(`{"schema":"brevity.runtime-state.v1","repoRoot":"C:\\dev\\repos\\active\\brevity","taskCounts":{"tracked":7}}`),
	}

	var stdout bytes.Buffer
	if err := runWithClient(&stdout, client); err != nil {
		t.Fatalf("runWithClient returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Brevity Runtime Dashboard") {
		t.Fatalf("output missing dashboard title:\n%s", output)
	}
	if !strings.Contains(output, "tracked: 7") {
		t.Fatalf("output missing task count:\n%s", output)
	}
}
