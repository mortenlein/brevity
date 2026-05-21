package cleanup

import (
	"errors"
	"testing"

	"github.com/mortenlein/brevity/internal/state"
)

type fakeInspector struct {
	worktrees []Worktree
	branches  []string
	exists    map[string]bool
	dirty     map[string]bool
	dirtyErr  map[string]error
}

func (inspector fakeInspector) Worktrees(string) ([]Worktree, error) { return inspector.worktrees, nil }
func (inspector fakeInspector) Branches(string) ([]string, error)    { return inspector.branches, nil }
func (inspector fakeInspector) PathExists(path string) bool          { return inspector.exists[path] }
func (inspector fakeInspector) Dirty(path string) (bool, error) {
	return inspector.dirty[path], inspector.dirtyErr[path]
}

func TestDetectHealthyFixtureNoFalsePositives(t *testing.T) {
	tasks := state.Tasks{Items: []state.Task{{Slug: "known", Branch: "task/known", WorktreePath: "C:/repo/wt/known"}}}
	report := Detect(DetectOptions{
		Tasks: tasks,
		Inspect: fakeInspector{
			worktrees: []Worktree{{Path: "C:/repo/wt/known", Branch: "task/known"}},
			branches:  []string{"main", "task/known"},
			exists:    map[string]bool{"C:/repo/wt/known": true},
			dirty:     map[string]bool{"C:/repo/wt/known": false},
			dirtyErr:  map[string]error{},
		},
	})
	if report.Summary.Total != 0 {
		t.Fatalf("report = %#v, want no candidates", report)
	}
}

func TestDetectMissingWorktreeOrphanBranchAndDirtyWorktree(t *testing.T) {
	tasks := state.Tasks{Items: []state.Task{
		{Slug: "missing", Branch: "task/missing", WorktreePath: "C:/repo/wt/missing"},
		{Slug: "dirty", Branch: "task/dirty", WorktreePath: "C:/repo/wt/dirty"},
	}}
	report := Detect(DetectOptions{
		Tasks: tasks,
		Inspect: fakeInspector{
			worktrees: []Worktree{{Path: "C:/repo/wt/dirty", Branch: "task/dirty"}},
			branches:  []string{"task/dirty", "task/lost"},
			exists:    map[string]bool{"C:/repo/wt/dirty": true},
			dirty:     map[string]bool{"C:/repo/wt/dirty": true},
			dirtyErr:  map[string]error{},
		},
	})
	if report.Summary.ByKind[string(KindMissingWorktree)] != 1 || report.Summary.ByKind[string(KindDirtyWorktree)] != 1 || report.Summary.ByKind[string(KindOrphanBranch)] != 1 {
		t.Fatalf("summary = %#v, want missing, dirty, orphan branch", report.Summary)
	}
}

func TestDetectCleanAndDirtyOrphanWorktrees(t *testing.T) {
	report := Detect(DetectOptions{
		Inspect: fakeInspector{
			worktrees: []Worktree{
				{Path: "C:/repo/wt/clean", Branch: "task/clean"},
				{Path: "C:/repo/wt/dirty", Branch: "task/dirty"},
				{Path: "C:/repo/wt/unknown", Branch: "task/unknown"},
			},
			branches: []string{"task/clean", "task/dirty", "task/unknown"},
			exists:   map[string]bool{},
			dirty:    map[string]bool{"C:/repo/wt/dirty": true},
			dirtyErr: map[string]error{"C:/repo/wt/unknown": errors.New("no status")},
		},
	})
	if report.Summary.ByKind[string(KindOrphanWorktree)] != 3 {
		t.Fatalf("summary = %#v, want three orphan worktrees", report.Summary)
	}
	if report.Summary.Removable != 1 || report.Summary.Destructive != 3 {
		t.Fatalf("summary = %#v, want one removable and three destructive", report.Summary)
	}
}
