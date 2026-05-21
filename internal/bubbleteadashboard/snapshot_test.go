package bubbleteadashboard

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDashboardRenderSnapshots(t *testing.T) {
	tests := []struct {
		name  string
		model Model
		want  string
	}{
		{
			name:  "normal populated runtime state",
			model: snapshotModel(func(model *Model) {}),
			want: `
Brevity | native | read-only | alerts !3 | p:1 t:1 c:1

Runtime Summary
  repo      C:\repo
  generated (unknown)
  status    1 providers | 1 degraded | 0 unavailable | 0 runnable | 1 cleanu... !3
  tasks     1 tracked | 0 runnable | 1 blocked | 0 stale | 0 gated | 0 review !1
  cleanup   1 candidates | 1 inspect !1

Runtime Signals
> prov  codex    degraded !
  task  task-one  blocked !
  clean inspect  orphan-branch:task-old !
  next  inspect  state.

Selected Detail
  select a row, then press d for details

j/k r p d q quit ? help | native | read-only
`,
		},
		{
			name: "empty runtime state",
			model: snapshotModel(func(model *Model) {
				model.state = emptyBubbleState()
				model.lastRefresh = "2026-05-20T10:00:00Z"
			}),
			want: `
Brevity | native | read-only | ok | generated 2026-05-20T10:00:00Z

Runtime Summary
  repo      C:\repo
  generated 2026-05-20T10:00:00Z
  status    0 providers ok | 0 runnable | 0 cleanup candidates ok
  tasks     0 tracked | 0 runnable | 0 blocked | 0 stale | 0 gated | 0 review ok
  cleanup   0 candidates | 0 inspect ok

Runtime Signals
  No runtime signals
  PowerShell backend is authoritative. This dashboard is read-only.
  Refresh to re-read state.

Selected Detail
  select a row, then press d for details

j/k r p d q quit ? help | native | read-only
`,
		},
		{
			name: "loading no state",
			model: snapshotModel(func(model *Model) {
				model.state = bubbleState()
				model.hasState = false
				model.lastRefresh = ""
			}),
			want: `
Brevity | native | read-only | loading

Runtime Summary
  status     Loading runtime state
  source     native / read-only
  authority  PowerShell runtime state

Runtime Signals
  No runtime signals
  PowerShell backend is authoritative. This dashboard is read-only.
  Refresh to re-read state.

j/k r p d q quit ? help | native | read-only
`,
		},
		{
			name: "polling error no state",
			model: snapshotModel(func(model *Model) {
				model.state = bubbleState()
				model.hasState = false
				model.lastRefresh = ""
				model.lastError = errors.New("runtime unavailable")
			}),
			want: `
Brevity | native | read-only | error

Runtime Summary
  status     Loading runtime state
  source     native / read-only
  authority  PowerShell runtime state

Runtime Signals
  No runtime signals
  PowerShell backend is authoritative. This dashboard is read-only.
  Refresh to re-read state.

Warnings
  ! polling error  runtime unavailable

j/k r p d q quit ? help | native | read-only
`,
		},
		{
			name: "action palette open",
			model: snapshotModel(func(model *Model) {
				model.paletteOpen = true
				model.paletteSelected = 4
			}),
			want: `
Brevity | native | read-only | alerts !3 | p:1 t:1 c:1

Runtime Summary
  repo      C:\repo
  generated (unknown)
  status    1 providers | 1 degraded | 0 unavailable | 0 runnable | 1 cleanu... !3
  tasks     1 tracked | 0 runnable | 1 blocked | 0 stale | 0 gated | 0 review !1
  cleanup   1 candidates | 1 inspect !1

Runtime Signals
> prov  codex    degraded !
  task  task-one  blocked !
  clean inspect  orphan-branch:task-old !
  next  inspect  state.

Selected Detail
  select a row, then press d for details

Actions
  Start task        future PowerShell action; confirmation required; not enable...
  Run worker        future PowerShell action; confirmation required; not enable...
  Merge task        future PowerShell action; confirmation required; not enable...
  Cleanup task      future PowerShell action; confirmation required; not enable...
> Refresh state     enter refreshes state

j/k r p d q quit ? help | native | read-only
`,
		},
		{
			name: "help overlay open",
			model: snapshotModel(func(model *Model) {
				model.helpOpen = true
			}),
			want: `
Brevity | native | read-only | alerts !3 | p:1 t:1 c:1

Runtime Summary
  repo      C:\repo
  generated (unknown)
  status    1 providers | 1 degraded | 0 unavailable | 0 runnable | 1 cleanu... !3
  tasks     1 tracked | 0 runnable | 1 blocked | 0 stale | 0 gated | 0 review !1
  cleanup   1 candidates | 1 inspect !1

Runtime Signals
> prov  codex    degraded !
  task  task-one  blocked !
  clean inspect  orphan-branch:task-old !
  next  inspect  state.

Selected Detail
  select a row, then press d for details

Help
  navigate with up/down or j/k; d toggles selected details
  r refreshes runtime state through the command bridge
  ... help truncated

j/k r p d q quit ? help | native | read-only
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizedSnapshotView(tt.model)
			if got != strings.TrimPrefix(tt.want, "\n") {
				t.Fatalf("snapshot mismatch (-want +got):\n%s", got)
			}
		})
	}
}

func snapshotModel(configure func(*Model)) Model {
	model := NewModelWithSource(&fakeClient{}, time.Second, "native")
	model.state = bubbleState()
	model.hasState = true
	model.width = 82
	model.height = 24
	model.lastRefresh = "2026-05-19T10:00:00Z"
	configure(&model)
	return model
}

func normalizedSnapshotView(model Model) string {
	output := plainView(model.View())
	output = strings.ReplaceAll(output, "\r\n", "\n")
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) > 0 {
		footerIndex := len(lines) - 1
		for footerIndex >= 0 && strings.TrimSpace(lines[footerIndex]) == "" {
			footerIndex--
		}
		if footerIndex >= 0 && strings.Contains(lines[footerIndex], "q quit") {
			blankStart := footerIndex - 1
			for blankStart >= 0 && strings.TrimSpace(lines[blankStart]) == "" {
				blankStart--
			}
			if footerIndex-blankStart > 2 {
				normalized := append([]string{}, lines[:blankStart+1]...)
				normalized = append(normalized, "")
				normalized = append(normalized, lines[footerIndex:]...)
				lines = normalized
			}
		}
	}
	return strings.Join(lines, "\n") + "\n"
}
