package cleanup

import (
	"sort"
	"strings"
)

const ReportSchema = "brevity.cleanup-inspection.v1"

type Kind string
type Severity string

const (
	KindTrackedTask     Kind = "tracked-task"
	KindMissingWorktree Kind = "missing-worktree"
	KindOrphanWorktree  Kind = "orphan-worktree"
	KindOrphanBranch    Kind = "orphan-branch"
	KindDirtyWorktree   Kind = "dirty-worktree"
	KindStaleRun        Kind = "stale-run"
	KindUnknown         Kind = "unknown"

	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

type Report struct {
	Schema     string      `json:"schema"`
	Summary    Summary     `json:"summary"`
	Candidates []Candidate `json:"candidates"`
	Warnings   []string    `json:"warnings,omitempty"`
}

type Summary struct {
	Total       int            `json:"total"`
	Info        int            `json:"info"`
	Warn        int            `json:"warn"`
	Error       int            `json:"error"`
	Removable   int            `json:"removable"`
	Destructive int            `json:"destructive"`
	ByKind      map[string]int `json:"byKind"`
}

type Candidate struct {
	ID              string   `json:"id"`
	Kind            Kind     `json:"kind"`
	Severity        Severity `json:"severity"`
	TaskSlug        string   `json:"taskSlug,omitempty"`
	Branch          string   `json:"branch,omitempty"`
	WorktreePath    string   `json:"worktreePath,omitempty"`
	Dirty           bool     `json:"dirty"`
	Removable       bool     `json:"removable"`
	Destructive     bool     `json:"destructive"`
	Reason          string   `json:"reason"`
	SuggestedAction string   `json:"suggestedAction"`
	Source          string   `json:"source"`
}

func NormalizeCandidate(candidate Candidate) Candidate {
	candidate.ID = strings.TrimSpace(candidate.ID)
	candidate.Kind = normalizeKind(candidate.Kind)
	candidate.Severity = normalizeSeverity(candidate.Severity)
	candidate.TaskSlug = strings.TrimSpace(candidate.TaskSlug)
	candidate.Branch = strings.TrimSpace(candidate.Branch)
	candidate.WorktreePath = strings.TrimSpace(candidate.WorktreePath)
	candidate.Reason = strings.TrimSpace(candidate.Reason)
	candidate.SuggestedAction = strings.TrimSpace(candidate.SuggestedAction)
	candidate.Source = strings.TrimSpace(candidate.Source)
	if candidate.Source == "" {
		candidate.Source = "native"
	}
	if candidate.ID == "" {
		candidate.ID = string(candidate.Kind) + ":" + safeID(firstNonEmpty(candidate.TaskSlug, candidate.Branch, candidate.WorktreePath, "unknown"))
	}
	if candidate.Dirty {
		candidate.Removable = false
	}
	if candidate.Destructive {
		candidate.Severity = maxSeverity(candidate.Severity, SeverityWarn)
	}
	return candidate
}

func NewReport(candidates []Candidate, warnings []string) Report {
	normalized := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		normalized = append(normalized, NormalizeCandidate(candidate))
	}
	SortCandidates(normalized)
	report := Report{
		Schema:     ReportSchema,
		Candidates: normalized,
		Warnings:   cleanStrings(warnings),
	}
	report.Summary = Summarize(normalized)
	return report
}

func Summarize(candidates []Candidate) Summary {
	summary := Summary{ByKind: map[string]int{}}
	for _, candidate := range candidates {
		candidate = NormalizeCandidate(candidate)
		summary.Total++
		summary.ByKind[string(candidate.Kind)]++
		switch candidate.Severity {
		case SeverityError:
			summary.Error++
		case SeverityWarn:
			summary.Warn++
		default:
			summary.Info++
		}
		if candidate.Removable {
			summary.Removable++
		}
		if candidate.Destructive {
			summary.Destructive++
		}
	}
	return summary
}

func SortCandidates(candidates []Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if severityRank(left.Severity) != severityRank(right.Severity) {
			return severityRank(left.Severity) > severityRank(right.Severity)
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return left.ID < right.ID
	})
}

func normalizeKind(kind Kind) Kind {
	switch Kind(strings.ToLower(strings.TrimSpace(string(kind)))) {
	case KindTrackedTask, KindMissingWorktree, KindOrphanWorktree, KindOrphanBranch, KindDirtyWorktree, KindStaleRun:
		return Kind(strings.ToLower(strings.TrimSpace(string(kind))))
	default:
		return KindUnknown
	}
}

func normalizeSeverity(severity Severity) Severity {
	switch Severity(strings.ToLower(strings.TrimSpace(string(severity)))) {
	case SeverityError:
		return SeverityError
	case SeverityWarn, "warning":
		return SeverityWarn
	default:
		return SeverityInfo
	}
}

func maxSeverity(left Severity, right Severity) Severity {
	if severityRank(left) >= severityRank(right) {
		return left
	}
	return right
}

func severityRank(severity Severity) int {
	switch normalizeSeverity(severity) {
	case SeverityError:
		return 3
	case SeverityWarn:
		return 2
	default:
		return 1
	}
}

func cleanStrings(values []string) []string {
	cleaned := []string{}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			cleaned = append(cleaned, strings.TrimSpace(value))
		}
	}
	sort.Strings(cleaned)
	return cleaned
}

func safeID(value string) string {
	replacer := strings.NewReplacer("\\", "-", "/", "-", ":", "-", " ", "-")
	return strings.Trim(replacer.Replace(strings.TrimSpace(value)), "-")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
