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

func TestParseOptionsDefaultsToPowerShellSource(t *testing.T) {
	options, err := parseOptions(nil)
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if options.jsonSource != "powershell" {
		t.Fatalf("jsonSource = %q, want powershell", options.jsonSource)
	}
	if options.once {
		t.Fatal("once = true, want false")
	}
}

func TestParseOptionsAcceptsOnce(t *testing.T) {
	options, err := parseOptions([]string{"--once"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if !options.once {
		t.Fatal("once = false, want true")
	}
}

func TestParseOptionsAcceptsPowerShellSource(t *testing.T) {
	options, err := parseOptions([]string{"--json-source", "powershell"})
	if err != nil {
		t.Fatalf("parseOptions returned error: %v", err)
	}

	if options.jsonSource != "powershell" {
		t.Fatalf("jsonSource = %q, want powershell", options.jsonSource)
	}
}

func TestParseOptionsAcceptsHelp(t *testing.T) {
	for _, arg := range []string{"--help", "-h"} {
		t.Run(arg, func(t *testing.T) {
			options, err := parseOptions([]string{arg})
			if err != nil {
				t.Fatalf("parseOptions returned error: %v", err)
			}
			if !options.help {
				t.Fatal("help = false, want true")
			}
		})
	}
}

func TestRunWritesHelp(t *testing.T) {
	var stdout bytes.Buffer
	if err := run(&stdout, []string{"--help"}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"read-only dashboard spike",
		`.\brevity.ps1 runtime state --json`,
		"PowerShell remains the",
		"--once",
		"--json-source powershell",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
}

func TestParseOptionsRejectsUnknownFlag(t *testing.T) {
	_, err := parseOptions([]string{"--unknown"})
	if err == nil {
		t.Fatal("parseOptions returned nil error")
	}
	if !strings.Contains(err.Error(), "flag provided but not defined: -unknown") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseOptionsRejectsInvalidJSONSource(t *testing.T) {
	_, err := parseOptions([]string{"--json-source", "native"})
	if err == nil {
		t.Fatal("parseOptions returned nil error")
	}
	if !strings.Contains(err.Error(), `unsupported --json-source "native"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
