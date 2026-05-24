package cmux

import (
	"encoding/json"
	"fmt"
	"io"
)

func RenderMergeReadinessText(stdout io.Writer, report MergeReadinessReport) error {
	fmt.Fprintln(stdout, "MERGE READINESS REPORT")
	fmt.Fprintln(stdout)
	renderTextGroup(stdout, GroupReadyForReview, report.ReadyForReview)
	renderTextGroup(stdout, GroupLikelyMergeable, report.LikelyMergeable)
	renderTextGroup(stdout, GroupNeedsInspection, report.NeedsInspection)
	renderTextGroup(stdout, GroupNotReady, report.NotReady)
	fmt.Fprintln(stdout, "Safety note:")
	fmt.Fprintln(stdout, report.SafetyNote)
	return nil
}

func RenderMergeReadinessMarkdown(stdout io.Writer, report MergeReadinessReport) error {
	fmt.Fprintln(stdout, "# Merge Readiness Report")
	fmt.Fprintln(stdout)
	renderMarkdownGroup(stdout, GroupReadyForReview, report.ReadyForReview)
	renderMarkdownGroup(stdout, GroupLikelyMergeable, report.LikelyMergeable)
	renderMarkdownGroup(stdout, GroupNeedsInspection, report.NeedsInspection)
	renderMarkdownGroup(stdout, GroupNotReady, report.NotReady)
	fmt.Fprintln(stdout, "## Safety Note")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, report.SafetyNote)
	return nil
}

func RenderMergeReadinessJSON(stdout io.Writer, report MergeReadinessReport) error {
	output, err := json.Marshal(report)
	if err != nil {
		return err
	}
	_, err = stdout.Write(append(output, '\n'))
	return err
}

func renderTextGroup(stdout io.Writer, name string, items []MergeReadinessItem) {
	fmt.Fprintf(stdout, "%s (%d)\n", name, len(items))
	for _, item := range items {
		fmt.Fprintf(stdout, "- %s\n", item.Task)
		fmt.Fprintf(stdout, "  reason: %s\n", item.Reason)
		fmt.Fprintf(stdout, "  inspect: %s\n", item.InspectNext)
	}
	fmt.Fprintln(stdout)
}

func renderMarkdownGroup(stdout io.Writer, name string, items []MergeReadinessItem) {
	fmt.Fprintf(stdout, "## %s (%d)\n", name, len(items))
	fmt.Fprintln(stdout)
	for _, item := range items {
		fmt.Fprintf(stdout, "- %s\n", item.Task)
		fmt.Fprintf(stdout, "  reason: %s\n", item.Reason)
		fmt.Fprintf(stdout, "  inspect: %s\n", item.InspectNext)
	}
	if len(items) > 0 {
		fmt.Fprintln(stdout)
	}
}
