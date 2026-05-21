package cleanup

import "testing"

func TestParseWorktreePorcelain(t *testing.T) {
	records := ParseWorktreePorcelain("worktree C:/repo\nHEAD abc\nbranch refs/heads/main\n\nworktree C:/repo/wt\nHEAD def\nbranch refs/heads/task/lost\n\n")
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	if records[1].Path != "C:/repo/wt" || records[1].Branch != "task/lost" || records[1].Head != "def" {
		t.Fatalf("record = %#v, want parsed task worktree", records[1])
	}
}

func TestParseBranchOutput(t *testing.T) {
	branches := ParseBranchOutput("main\n* task/beta\n task/alpha \nmain\n")
	if len(branches) != 3 || branches[0] != "main" || branches[1] != "task/alpha" || branches[2] != "task/beta" {
		t.Fatalf("branches = %#v", branches)
	}
}
