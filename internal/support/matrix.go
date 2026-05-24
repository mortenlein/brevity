package support

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
)

type Capability struct {
	ID                    string `json:"id"`
	Capability            string `json:"capability"`
	GoSupport             string `json:"goSupport"`
	PowerShellSupport     string `json:"powerShellSupport"`
	CurrentAuthority      string `json:"currentAuthority"`
	SafetyClass           string `json:"safetyClass"`
	TestCoverage          string `json:"testCoverage"`
	TUISupport            string `json:"tuiSupport"`
	MigrationStatus       string `json:"migrationStatus"`
	NextAction            string `json:"nextAction"`
	PowerShellDisposition string `json:"powerShellDisposition"`
}

var Matrix = []Capability{
	{"dashboard", "Dashboard/watch console", "Implemented; default PowerShell source, native source optional", "Implemented legacy TUI scaffold", "Go for future operator UX", "read-only", "unit tests", "Go dashboard and Bubble Tea", "migrated frontend direction", "Keep improving native source", "keep reference"},
	{"runtime-state", "Runtime state JSON", "Native reader implemented", "Implemented legacy producer", "Go", "read-only", "unit and contract tests", "native source supported", "migrated", "Keep compatibility schema additive", "keep fallback"},
	{"runtime-supervisor", "Runtime supervisor lifecycle", "Native start, stop, status with heartbeat state", "Compatibility wrapper only", "Go", "runtime lifecycle metadata", "unit and fixture tests", "planned operator surface", "foundation migrated", "Keep supervisor observational; add queues later", "delegate only"},
	{"runtime-queue", "Runtime queue persistence and inspection", "Native add, list, inspect, remove for runtime-queue.json; Bubble Tea displays read-only queue health", "Not PowerShell-owned", "Go", "inert runtime metadata / read-only diagnostics", "unit tests", "Bubble Tea read-only visibility", "foundation migrated", "Keep execution/draining out of queue v1", "none"},
	{"runtime-scheduler", "Runtime scheduler planning and reservation", "Native scheduler plan selects first eligible runnable queue item from queue plan; reserve-next reserves that one item through queue reservation", "Not PowerShell-owned", "Go", "read-only scheduling decision / inert reservation metadata", "unit tests", "none", "contract migrated", "Keep execution out of scheduler v1", "none"},
	{"runtime-execution-records", "Runtime execution record persistence, inspection, and manual launch", "Native list, inspect, plan-from-reservation, readiness transitions, preflight, launch-dry-run, and single foreground provider launch for runtime-executions.json", "Not PowerShell-owned", "Go", "execution metadata / manual provider-executing mutation", "unit and fake-provider tests", "Bubble Tea read-only visibility", "execution contract migrated", "Keep scheduler loops and queue draining out of execution launch", "none"},
	{"provider-health", "Provider health read/write", "Native read, set, reset", "Implemented legacy compatibility", "Go", "mutating metadata", "unit tests", "actions available", "migrated", "Deprecate PowerShell-first docs", "keep wrapper/fallback"},
	{"task-metadata-reads", "Task metadata/runtime reads", "Native task status, detail, runtime-info", "Implemented legacy views", "Go", "read-only", "unit tests", "native TUI source", "migrated", "Keep Go as authority", "keep compatibility"},
	{"task-new", "Task new worktree creation", "Native implementation", "Implemented legacy compatibility", "Go", "mutating git and metadata", "fixture tests", "action available", "migrated", "Keep PowerShell as fallback only", "keep wrapper"},
	{"task-start", "Task start metadata transition", "Native implementation", "Legacy manual start helper", "Go", "mutating metadata", "unit tests", "action available", "migrated", "Clarify PowerShell legacy semantics", "deprecate behavior later"},
	{"task-run-plan", "Task run planning", "Native plan envelope", "Implemented legacy plan", "Go", "read-only provider plan", "unit tests", "action available", "migrated", "Keep plan contract stable", "keep reference"},
	{"task-run-execute", "Task run provider execution", "Native argv execution", "Implemented legacy execution", "Go", "provider-executing mutation", "fake-provider tests", "action available", "migrated", "Keep no-real-provider test rule", "keep fallback"},
	{"task-context-refresh", "Task context refresh", "Native implementation", "Implemented legacy compatibility", "Go", "mutating prompt/context metadata", "unit tests", "action available", "migrated", "Keep alias `task context refresh`", "keep wrapper"},
	{"task-merge", "Task merge", "Native plan and execute", "Implemented legacy merge", "Go", "mutating git and metadata", "fixture tests", "planned TUI enrichment", "migrated", "Add merge confirmation UI later", "keep fallback"},
	{"task-cleanup", "Selected task cleanup", "Native plan and explicit force execute", "Implemented legacy cleanup", "Go", "destructive git cleanup", "fixture tests", "planned TUI enrichment", "migrated", "Keep cleanup separate from merge", "keep fallback"},
	{"cleanup-orphans", "Orphan cleanup", "Native inspect, plan, execute", "Implemented legacy orphan helpers", "Go", "destructive cleanup", "fixture tests", "inspection in TUI", "migrated", "Prefer native candidate IDs", "deprecate duplicate helpers later"},
	{"doctor", "Doctor diagnostics", "Native read-only diagnostics", "Implemented with repair paths", "Go for read-only doctor; PowerShell for repair", "read-only / mutating repair", "unit tests", "not direct", "partially migrated", "Migrate repair only if still needed", "keep repair compatibility"},
	{"run-history-reads", "Run history reads", "Native runs and runtime summaries", "Implemented legacy reads", "Go", "read-only", "unit tests", "native source supported", "migrated", "Keep JSONL contract stable", "keep compatibility"},
	{"run-history-maintenance", "Run history maintenance", "Native runs inspect and compact", "PowerShell task runs reconcile/retention/compact remains", "Go for native compact; PowerShell for reconcile/retention", "read-only / mutating compact", "unit tests", "not direct", "partially migrated", "Migrate reconcile and retention next", "migrate next"},
	{"init-repair", "Workspace init/repair", "Native init and repair", "Implemented legacy compatibility", "Go", "mutating skeleton/config", "unit and fixture tests", "none", "migrated", "Keep PowerShell as reference/fallback", "keep reference"},
	{"task-activate-spec", "Task activate/spec", "Native activate and read-only spec", "Implemented legacy compatibility", "Go", "mutating git/metadata for activate; read-only for spec", "fixture tests", "none", "migrated", "Keep no-provider execution boundary", "keep reference"},
	{"planner-prompts", "Planner prompt generation/application", "Not native", "Implemented", "PowerShell", "mutating prompt/tasks/worktrees depending on flags", "PowerShell manual coverage", "none", "not migrated", "Split prompt generation from task creation", "migrate later"},
	{"memory-logs-session", "Memory notes, logs, session summary", "Not native or partial via run reads", "Implemented", "PowerShell", "read-only and memory mutation", "PowerShell manual coverage", "dashboard reads runtime memory", "not migrated", "Migrate small read-only views when needed", "keep helper for now"},
	{"provider-profiles", "Provider profiles/profile aliases", "Shared native metadata and resolver", "Implemented source matrix", "Go", "read-only config", "unit tests for provider resolver and Go planning", "indirect", "migrated", "Wire remaining PowerShell reference docs when PowerShell is retired", "keep reference"},
}

func WriteJSON(stdout io.Writer) error {
	output, err := json.MarshalIndent(Matrix, "", "  ")
	if err != nil {
		return err
	}
	_, err = stdout.Write(append(output, '\n'))
	return err
}

func WriteHuman(stdout io.Writer) error {
	writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "Capability\tAuthority\tSafety\tMigration\tNext action")
	for _, item := range Matrix {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", item.Capability, item.CurrentAuthority, item.SafetyClass, item.MigrationStatus, item.NextAction)
	}
	return writer.Flush()
}
