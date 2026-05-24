package cmux

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mortenlein/brevity/internal/contracts"
)

// CMUXMergeReportSchema is the schema identifier for the CMUX merge-report
// JSON output.
const CMUXMergeReportSchema = "brevity.cmux-merge-report.v1"

// mergeGroupOrder defines the canonical display order for the six merge groups.
// This order is deterministic and reflects actionability: items closest to
// landing (ready-for-merge) appear first; already-landed (merged) and
// uncategorised (other) appear last.
var mergeGroupOrder = []string{
	"ready-for-merge",
	"reviewing",
	"needs-run",
	"blocked",
	"merged",
	"other",
}

// mergeGroupForTask maps a task's normalised state to one of the six canonical
// merge groups used by the merge-readiness report.
func mergeGroupForTask(t contracts.TaskSummary) string {
	switch reviewTaskState(t) {
	case "ready-for-merge":
		return "ready-for-merge"
	case "reviewing":
		return "reviewing"
	case "runnable", "ready-for-worker":
		return "needs-run"
	case "blocked", "provider-gated":
		return "blocked"
	case "merged":
		return "merged"
	default:
		return "other"
	}
}

// groupTasksForMerge partitions tasks into the six merge groups.
// Within each group tasks are sorted by slug for deterministic output.
// All six groups are always present in the returned map (empty slices when no
// tasks fall into a group).
func groupTasksForMerge(tasks []contracts.TaskSummary) map[string][]contracts.TaskSummary {
	groups := make(map[string][]contracts.TaskSummary, len(mergeGroupOrder))
	for _, g := range mergeGroupOrder {
		groups[g] = make([]contracts.TaskSummary, 0)
	}
	for _, t := range tasks {
		g := mergeGroupForTask(t)
		groups[g] = append(groups[g], t)
	}
	for g := range groups {
		sort.Slice(groups[g], func(i, j int) bool {
			return groups[g][i].Slug < groups[g][j].Slug
		})
	}
	return groups
}

// CMUXMergeReport is the top-level typed output struct for --merge-report mode.
//
// Groups is always a non-nil slice containing one entry per canonical merge
// group in mergeGroupOrder.  Tasks inside each group is always a non-nil slice
// (empty when no tasks fall into that group or when limit has been applied).
type CMUXMergeReport struct {
	Schema  string           `json:"schema"`
	Source  string           `json:"source"`
	Options CMUXMergeOptions `json:"options"`
	Errors  []string         `json:"errors"`
	Groups  []CMUXMergeGroup `json:"groups"`
}

// CMUXMergeOptions records the render parameters that were active when this
// merge report was generated.
type CMUXMergeOptions struct {
	Limit  int    `json:"limit"`
	Output string `json:"output"`
}

// CMUXMergeGroup is one group entry in the merge-readiness report.
// Count is the total number of tasks in this group before any limit is applied.
// Shown is the number of tasks included in Tasks after the limit is applied.
type CMUXMergeGroup struct {
	Group string     `json:"group"`
	Count int        `json:"count"`
	Shown int        `json:"shown"`
	Tasks []CMUXTask `json:"tasks"`
}

// renderMerge dispatches to the appropriate merge-readiness renderer based on
// output mode.  Called only when opts.MergeReport is true.
func renderMerge(w io.Writer, snap Snapshot, opts RenderOptions) {
	switch opts.Output {
	case OutputMarkdown:
		renderMarkdownMerge(w, snap, opts)
	case OutputJSON:
		renderJSONMerge(w, snap, opts)
	default:
		renderTextMerge(w, snap, opts)
	}
}

// renderTextMerge writes a plain-text merge-readiness report to w.
func renderTextMerge(w io.Writer, snap Snapshot, opts RenderOptions) {
	limit := opts.effectiveLimit()

	fmt.Fprintln(w, "CMUX MERGE READINESS  [read-only]")
	fmt.Fprintln(w, "==================================")
	fmt.Fprintln(w, "source: native")
	if snap.HasRuntimeState {
		rs := snap.RuntimeState
		if rs.GeneratedAt != "" {
			fmt.Fprintf(w, "generated: %s\n", rs.GeneratedAt)
		}
		if rs.RepoRoot != "" {
			fmt.Fprintf(w, "repo: %s\n", rs.RepoRoot)
		}
	}

	if snap.RuntimeStateErr != nil {
		fmt.Fprintf(w, "\nruntime-state: error: %v\n", snap.RuntimeStateErr)
		return
	}
	if !snap.HasRuntimeState {
		fmt.Fprintln(w, "\nruntime-state: unavailable")
		return
	}

	groups := groupTasksForMerge(snap.RuntimeState.Tasks)

	for i, groupName := range mergeGroupOrder {
		if i > 0 {
			fmt.Fprintln(w, sectionSep)
		}
		group := groups[groupName]
		shown := group
		truncated := false
		if limit > 0 && len(shown) > limit {
			shown = shown[:limit]
			truncated = true
		}
		if truncated {
			fmt.Fprintf(w, "\n%s  (showing %d of %d)\n", groupName, len(shown), len(group))
		} else {
			fmt.Fprintf(w, "\n%s  (%d)\n", groupName, len(group))
		}
		if len(group) == 0 {
			fmt.Fprintln(w, "  (none)")
			continue
		}
		for j, t := range shown {
			if j > 0 {
				fmt.Fprintln(w)
			}
			fmt.Fprintf(w, "  %-32s %s\n", t.Slug, reviewTaskState(t))
			renderTaskWorktree(w, t)
			renderTaskPrompt(w, t)
			renderTaskLastRun(w, t)
		}
	}
}

// renderMarkdownMerge writes a GitHub-Flavoured Markdown merge-readiness report
// to w.
func renderMarkdownMerge(w io.Writer, snap Snapshot, opts RenderOptions) {
	limit := opts.effectiveLimit()

	fmt.Fprintln(w, "# CMUX Merge Readiness [read-only]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "**Source:** native")
	if snap.HasRuntimeState {
		rs := snap.RuntimeState
		if rs.GeneratedAt != "" {
			fmt.Fprintf(w, "**Generated:** %s\n", rs.GeneratedAt)
		}
		if rs.RepoRoot != "" {
			fmt.Fprintf(w, "**Repo:** %s\n", rs.RepoRoot)
		}
	}

	if snap.RuntimeStateErr != nil {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "> **runtime-state error:** %v\n", snap.RuntimeStateErr)
		return
	}
	if !snap.HasRuntimeState {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "> **runtime-state:** unavailable")
		return
	}

	groups := groupTasksForMerge(snap.RuntimeState.Tasks)

	for _, groupName := range mergeGroupOrder {
		group := groups[groupName]
		shown := group
		truncated := false
		if limit > 0 && len(shown) > limit {
			shown = shown[:limit]
			truncated = true
		}
		fmt.Fprintln(w)
		if truncated {
			fmt.Fprintf(w, "## %s (showing %d of %d)\n", groupName, len(shown), len(group))
		} else {
			fmt.Fprintf(w, "## %s (%d)\n", groupName, len(group))
		}
		fmt.Fprintln(w)
		if len(group) == 0 {
			fmt.Fprintln(w, "_None._")
			continue
		}
		for _, t := range shown {
			fmt.Fprintf(w, "### %s\n", t.Slug)
			fmt.Fprintln(w)
			fmt.Fprintf(w, "**State:** %s\n", reviewTaskState(t))
			fmt.Fprintln(w)
			renderMarkdownTaskDetail(w, t)
			fmt.Fprintln(w)
		}
	}
}

// renderJSONMerge writes a CMUXMergeReport as indented JSON to w.
func renderJSONMerge(w io.Writer, snap Snapshot, opts RenderOptions) {
	limit := opts.effectiveLimit()
	outputStr := string(opts.Output)
	if strings.TrimSpace(outputStr) == "" {
		outputStr = string(OutputText)
	}

	report := CMUXMergeReport{
		Schema: CMUXMergeReportSchema,
		Source: "native",
		Options: CMUXMergeOptions{
			Limit:  limit,
			Output: outputStr,
		},
		Errors: make([]string, 0),
		Groups: make([]CMUXMergeGroup, 0),
	}

	if snap.RuntimeStateErr != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("runtime-state: %v", snap.RuntimeStateErr))
	}
	if snap.SchedulerPlanErr != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("scheduler-plan: %v", snap.SchedulerPlanErr))
	}

	if snap.HasRuntimeState {
		groups := groupTasksForMerge(snap.RuntimeState.Tasks)
		for _, groupName := range mergeGroupOrder {
			group := groups[groupName]
			shown := group
			if limit > 0 && len(shown) > limit {
				shown = shown[:limit]
			}
			tasks := make([]CMUXTask, 0, len(shown))
			for _, t := range shown {
				tasks = append(tasks, buildJSONTask(t))
			}
			report.Groups = append(report.Groups, CMUXMergeGroup{
				Group: groupName,
				Count: len(group),
				Shown: len(shown),
				Tasks: tasks,
			})
		}
	}

	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(w, "{\"schema\":%q,\"error\":\"json marshal failed: %v\"}\n", CMUXMergeReportSchema, err)
		return
	}
	_, _ = w.Write(append(out, '\n'))
}
