package diagnostics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
)

func TestNormalizeStatusAndSeverity(t *testing.T) {
	if NormalizeStatus("warning") != StatusWarn {
		t.Fatalf("warning status did not normalize to warn")
	}
	if NormalizeStatus("failed") != StatusError {
		t.Fatalf("failed status did not normalize to error")
	}
	if NormalizeSeverity("fatal") != SeverityError {
		t.Fatalf("fatal severity did not normalize to error")
	}
	if SeverityRank(SeverityError) <= SeverityRank(SeverityWarn) {
		t.Fatalf("severity rank did not order error above warn")
	}
}

func TestCommandResultJSONStableShape(t *testing.T) {
	report := Report{
		Schema:      Schema,
		RepoRoot:    `C:\repo`,
		GeneratedAt: "2026-05-21T10:00:00Z",
		Checks: []Check{{
			ID:        "repo-root",
			Title:     "Repository root",
			Status:    StatusOK,
			Severity:  SeverityInfo,
			Message:   "Repository root is readable.",
			Source:    "native-go",
			Timestamp: "2026-05-21T10:00:00Z",
		}},
		Summary: Summary{OK: 1},
	}
	result := CommandResult(report)
	output, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	for _, want := range []string{
		`"schema":"brevity.command-result.v1"`,
		`"command":"doctor"`,
		`"success":true`,
		`"payload":{"schema":"brevity.doctor.v1"`,
		`"checks":[{"id":"repo-root"`,
	} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunHealthyFixture(t *testing.T) {
	repoRoot := diagnosticRepo(t)
	writeDiagnosticFile(t, repoRoot, ".brevity/config.json", "\uFEFF"+`{"vaultPath":"`+jsonPath(filepath.Join(repoRoot, "vaults", "AI-Vault"))+`","worktreesRoot":"`+jsonPath(filepath.Join(repoRoot, "worktrees", "active"))+`"}`)
	writeDiagnosticFile(t, repoRoot, ".brevity/tasks.json", `[{"slug":"my-task","status":"ready-for-worker"}]`)
	writeDiagnosticFile(t, repoRoot, ".brevity/provider-health.json", `{"codex":{"status":"healthy"}}`)
	writeDiagnosticFile(t, repoRoot, ".brevity/runs.jsonl", `{"slug":"my-task","runId":"run-1","finishedAt":"2026-05-21T09:00:00Z","exitCode":0}`+"\n")
	mkdirDiagnostic(t, filepath.Join(repoRoot, "vaults", "AI-Vault"))
	mkdirDiagnostic(t, filepath.Join(repoRoot, "worktrees", "active"))

	report := runDiagnostic(t, repoRoot)
	if report.Summary.Error != 0 {
		t.Fatalf("errors = %d, want 0: %#v", report.Summary.Error, report.Checks)
	}
	if !hasCheck(report, "tasks-readable", StatusOK) || !hasCheck(report, "runs-readable", StatusOK) {
		t.Fatalf("report missing ok runtime file checks: %#v", report.Checks)
	}
}

func TestRunMissingBrevityAndMalformedFiles(t *testing.T) {
	repoRoot := t.TempDir()
	report := runDiagnostic(t, repoRoot)
	if !hasCheck(report, "brevity-directory", StatusError) {
		t.Fatalf("missing .brevity was not an error: %#v", report.Checks)
	}

	repoRoot = diagnosticRepo(t)
	writeDiagnosticFile(t, repoRoot, ".brevity/tasks.json", `{not-json}`)
	writeDiagnosticFile(t, repoRoot, ".brevity/provider-health.json", `{"codex":{"status":"not-real"}}`)
	writeDiagnosticFile(t, repoRoot, ".brevity/runs.jsonl", `{not-json}`+"\n")
	report = runDiagnostic(t, repoRoot)
	for _, id := range []string{"tasks-readable", "provider-health-readable", "runs-readable"} {
		if !hasCheck(report, id, StatusError) {
			t.Fatalf("report missing error check %s: %#v", id, report.Checks)
		}
	}
}

func TestRunMissingGitWithSeam(t *testing.T) {
	repoRoot := diagnosticRepo(t)
	writeDiagnosticFile(t, repoRoot, ".brevity/tasks.json", `[]`)
	writeDiagnosticFile(t, repoRoot, ".brevity/provider-health.json", `{}`)
	report, err := Run(Options{
		RepoRoot: repoRoot,
		Now:      fixedNow,
		LookPath: func(string) (string, error) {
			return "", os.ErrNotExist
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !hasCheck(report, "git-executable", StatusWarn) {
		t.Fatalf("missing git did not produce warning: %#v", report.Checks)
	}
}

func TestRenderDiagnosticReportHumanOutput(t *testing.T) {
	report := Report{
		Schema:      Schema,
		RepoRoot:    `C:\repo`,
		GeneratedAt: "2026-05-21T10:00:00Z",
		Checks:      []Check{{ID: "repo-root", Status: StatusOK, Message: "Repository root is readable.", Source: "native-go"}},
		Summary:     Summary{OK: 1},
	}
	result := CommandResult(report)
	if result.Success != true || len(result.Errors) != 0 {
		t.Fatalf("result = %#v, want success without errors", result)
	}
	if _, err := contracts.ParseCommandResult(mustJSON(t, result)); err != nil {
		t.Fatalf("ParseCommandResult returned error: %v", err)
	}
}

func runDiagnostic(t *testing.T, repoRoot string) Report {
	t.Helper()
	report, err := Run(Options{RepoRoot: repoRoot, Now: fixedNow})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	return report
}

func fixedNow() time.Time {
	return time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
}

func diagnosticRepo(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	mkdirDiagnostic(t, filepath.Join(repoRoot, ".brevity"))
	return repoRoot
}

func writeDiagnosticFile(t *testing.T, repoRoot string, relativePath string, content string) {
	t.Helper()
	path := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	mkdirDiagnostic(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s returned error: %v", path, err)
	}
}

func mkdirDiagnostic(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll %s returned error: %v", path, err)
	}
}

func hasCheck(report Report, id string, status Status) bool {
	for _, check := range report.Checks {
		if check.ID == id && check.Status == status {
			return true
		}
	}
	return false
}

func jsonPath(path string) string {
	return strings.ReplaceAll(path, `\`, `\\`)
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	output, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	return output
}
