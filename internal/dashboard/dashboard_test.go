package dashboard

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mortenlein/brevity/internal/contracts"
)

func TestRenderDashboardMinimalState(t *testing.T) {
	state := contracts.RuntimeState{
		Schema:      contracts.RuntimeStateSchema,
		RepoRoot:    `C:\dev\repos\active\brevity`,
		GeneratedAt: "2026-05-19T10:00:00Z",
		TaskCounts: contracts.TaskCounts{
			Tracked:       2,
			Runnable:      1,
			Blocked:       0,
			Stale:         3,
			ProviderGated: 4,
			Review:        5,
		},
	}

	output := renderDashboardString(t, state)

	assertContains(t, output, "Brevity Runtime Dashboard")
	assertContains(t, output, `Repo: C:\dev\repos\active\brevity`)
	assertContains(t, output, "tracked: 2")
	assertContains(t, output, "runnable: 1")
	assertContains(t, output, "blocked: 0")
	assertContains(t, output, "stale: 3")
	assertContains(t, output, "provider gated: 4")
	assertContains(t, output, "review: 5")
}

func TestRenderDashboardCleanupSummary(t *testing.T) {
	state := contracts.RuntimeState{
		Schema: contracts.RuntimeStateSchema,
		Cleanup: &contracts.Cleanup{
			Summary: &contracts.CleanupSummary{
				TotalCandidates:           6,
				RequiresInspectionCount:   2,
				RemovableByExecuteCount:   4,
				OrphanedTaskWorktreeCount: 1,
				OrphanedTaskBranchCount:   3,
				BySeverity:                map[string]int{"high": 1, "low": 5},
				ByCategory:                map[string]int{"branch": 3, "worktree": 1},
			},
		},
	}

	output := renderDashboardString(t, state)

	assertContains(t, output, "Cleanup")
	assertContains(t, output, "total candidates: 6")
	assertContains(t, output, "requires inspection: 2")
	assertContains(t, output, "removable by execute: 4")
	assertContains(t, output, "orphaned worktrees: 1")
	assertContains(t, output, "orphaned branches: 3")
	assertContains(t, output, "by severity: high=1, low=5")
	assertContains(t, output, "by category: branch=3, worktree=1")
}

func TestRenderDashboardSuggestedActions(t *testing.T) {
	state := contracts.RuntimeState{
		Schema:               contracts.RuntimeStateSchema,
		SuggestedNextActions: []string{"Run brevity task status", "Review blocked tasks"},
	}

	output := renderDashboardString(t, state)

	assertContains(t, output, "Suggested Next Actions")
	assertContains(t, output, "- Run brevity task status")
	assertContains(t, output, "- Review blocked tasks")
}

func TestRenderDashboardProviderSummary(t *testing.T) {
	state := contracts.RuntimeState{
		Schema: contracts.RuntimeStateSchema,
		Providers: contracts.Providers{
			Summary: contracts.ProviderSummary{
				Total:       3,
				Degraded:    1,
				Unavailable: 2,
			},
			Health: map[string]contracts.ProviderHealth{
				"local": {
					Status:    "ok",
					UpdatedAt: "2026-05-19T10:01:00Z",
					Note:      "ready",
				},
			},
		},
	}

	output := renderDashboardString(t, state)

	assertContains(t, output, "Providers")
	assertContains(t, output, "total: 3, degraded: 1, unavailable: 2")
	assertContains(t, output, "local: ok (2026-05-19T10:01:00Z) - ready")
}

func renderDashboardString(t *testing.T, state contracts.RuntimeState) string {
	t.Helper()

	var output bytes.Buffer
	Render(&output, state)
	return output.String()
}

func assertContains(t *testing.T, output string, want string) {
	t.Helper()

	if !strings.Contains(output, want) {
		t.Fatalf("rendered dashboard missing %q\noutput:\n%s", want, output)
	}
}
