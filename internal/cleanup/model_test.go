package cleanup

import (
	"encoding/json"
	"testing"
)

func TestNormalizeCandidateDefaultsAndClassification(t *testing.T) {
	candidate := NormalizeCandidate(Candidate{
		Kind:        "orphan-worktree",
		Severity:    "warning",
		Branch:      " task/lost ",
		Dirty:       true,
		Removable:   true,
		Destructive: true,
	})
	if candidate.ID != "orphan-worktree:task-lost" {
		t.Fatalf("ID = %q, want generated stable ID", candidate.ID)
	}
	if candidate.Severity != SeverityWarn || candidate.Kind != KindOrphanWorktree {
		t.Fatalf("classification = %s/%s", candidate.Severity, candidate.Kind)
	}
	if candidate.Removable {
		t.Fatal("dirty candidate remained removable")
	}
	if candidate.Source != "native" {
		t.Fatalf("Source = %q, want native", candidate.Source)
	}
}

func TestReportJSONStabilityAndSeverityOrdering(t *testing.T) {
	report := NewReport([]Candidate{
		{ID: "info", Kind: KindTrackedTask, Severity: SeverityInfo},
		{ID: "error", Kind: KindMissingWorktree, Severity: SeverityError},
		{ID: "warn", Kind: KindOrphanBranch, Severity: SeverityWarn, Destructive: true},
	}, nil)
	if report.Candidates[0].ID != "error" || report.Candidates[1].ID != "warn" || report.Candidates[2].ID != "info" {
		t.Fatalf("candidates = %#v, want severity order", report.Candidates)
	}
	output, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	want := `{"schema":"brevity.cleanup-inspection.v1","summary":{"total":3,"info":1,"warn":1,"error":1,"removable":0,"destructive":1,"byKind":{"missing-worktree":1,"orphan-branch":1,"tracked-task":1}}`
	if len(output) < len(want) || string(output[:len(want)]) != want {
		t.Fatalf("json = %s, want stable prefix %s", output, want)
	}
}
