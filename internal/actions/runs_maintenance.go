package actions

import (
	"fmt"
	"io"

	"github.com/mortenlein/brevity/internal/contracts"
)

func RenderTaskRunsReconcileResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseTaskRunsReconcilePayload(result)
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, "Task runs reconcile")
	fmt.Fprintf(stdout, "candidateCount: %d\n", payload.CandidateCount)
	fmt.Fprintf(stdout, "staleThresholdMinutes: %d\n", payload.StaleThresholdMinutes)
	if len(payload.Candidates) == 0 {
		fmt.Fprintln(stdout, "staleOrIncompleteRunIds: none")
	} else {
		fmt.Fprintln(stdout, "staleOrIncompleteRunIds:")
		for _, candidate := range payload.Candidates {
			fmt.Fprintf(stdout, "- %s", emptyAsUnknown(candidate.RunID))
			if candidate.Slug != "" {
				fmt.Fprintf(stdout, " slug=%s", candidate.Slug)
			}
			fmt.Fprintf(stdout, " stale=%t incomplete=%t", candidate.Stale, candidate.Incomplete)
			if candidate.WorkerStatus != "" {
				fmt.Fprintf(stdout, " status=%s", candidate.WorkerStatus)
			}
			fmt.Fprintln(stdout)
		}
	}

	renderMessages(stdout, result)
	return nil
}

func RenderTaskRunsRetentionResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseTaskRunsRetentionPayload(result)
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, "Task runs retention")
	fmt.Fprintf(stdout, "totalRecords: %d\n", payload.TotalRecords)
	fmt.Fprintf(stdout, "validRecords: %d\n", payload.ValidRecords)
	fmt.Fprintf(stdout, "invalidRecords: %d\n", payload.InvalidRecords)
	fmt.Fprintf(stdout, "staleRecords: %d\n", payload.StaleRecords)
	fmt.Fprintf(stdout, "incompleteRecords: %d\n", payload.IncompleteRecords)
	fmt.Fprintf(stdout, "staleThresholdMinutes: %d\n", payload.StaleThresholdMinutes)
	if len(payload.TopTasks) == 0 {
		fmt.Fprintln(stdout, "topTasks: none")
	} else {
		fmt.Fprintln(stdout, "topTasks:")
		for _, task := range payload.TopTasks {
			fmt.Fprintf(stdout, "- %s: %d\n", emptyAsUnknown(task.Slug), task.Records)
		}
	}

	renderMessages(stdout, result)
	return nil
}

func RenderTaskRunsCompactResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseTaskRunsCompactPayload(result)
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, "Task runs compact")
	fmt.Fprintf(stdout, "retainedRecords: %d\n", payload.RetainedRecordCount)
	fmt.Fprintf(stdout, "archiveSummaryCandidates: %d\n", payload.CandidateArchiveSummaryCount)
	fmt.Fprintf(stdout, "discardCandidates: %d\n", payload.CandidateDiscardCount)
	fmt.Fprintf(stdout, "preservedFailedRecords: %d\n", payload.PreservedFailedCount)
	fmt.Fprintf(stdout, "preservedStaleIncompleteRecords: %d\n", payload.PreservedStaleIncompleteCount)

	renderMessages(stdout, result)
	return nil
}
