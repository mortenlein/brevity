package bubbleteadashboard

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mortenlein/brevity/internal/pscontract"
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
				model.paletteSelected = 1
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
  Refresh state     enter refreshes state
> Provider status   executable read-only
  Task status       executable read-only
  Start task        future PowerShell action; select a task row to enable
  Run worker        plan preview only; select a runnable task row
  Merge task        future PowerShell action; confirmation required; not enable...
  Cleanup task      future PowerShell action; confirmation required; not enable...

up/down or j/k choose | enter run/preview | esc close | native | read-only
`,
		},
		{
			name: "action palette open with start enabled",
			model: snapshotModel(func(model *Model) {
				model.selection.SelectedIndex = 1
				model.paletteOpen = true
				model.paletteSelected = 3
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
  prov  codex    degraded !
> task  task-one  blocked !
  clean inspect  orphan-branch:task-old !
  next  inspect  state.

Selected Detail
  select a row, then press d for details

Actions
  Refresh state     enter refreshes state
  Provider status   executable read-only
  Task status       executable read-only
> Start task        confirmation required for task-one
  Run worker        plan preview only; select a runnable task row
  Merge task        future PowerShell action; confirmation required; not enable...
  Cleanup task      future PowerShell action; confirmation required; not enable...

up/down or j/k choose | enter run/preview | esc close | native | read-only
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
  r refreshes runtime state; l toggles live polling
  ... help truncated

esc or ? closes help | read-only boundary | ? help | native | read-only
`,
		},
		{
			name: "disabled action preview open",
			model: snapshotModel(func(model *Model) {
				action := actionDescriptors()[3]
				model.actionPreview = &action
				model.width = 100
				model.height = 32
			}),
			want: `
Brevity | native | read-only | alerts !3 | p:1 t:1 c:1

Runtime Summary
  repo      C:\repo
  generated (unknown)
  status    1 providers | 1 degraded | 0 unavailable | 0 runnable | 1 cleanup candidate !3
  tasks     1 tracked | 0 runnable | 1 blocked | 0 stale | 0 gated | 0 review !1
  cleanup   1 candidates | 1 inspect !1

Runtime Signals
> prov  codex    degraded !
  task  task-one  blocked !
  clean inspect  orphan-branch:task-old !
  next  inspect  state.

Selected Detail
  select a row, then press d for details

Command Preview
  action        Start task
  status        disabled / blocked
  command       powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\\brevity.ps1 task start...
  blocked       not enabled yet; select a task row to enable Start task
  confirm       confirmation required before any future execution
  authority     PowerShell is authoritative; Go will not write .brevity
  close         esc, q, or p returns to the dashboard

up/down or j/k move | r refresh | p actions | d details | q quit | ? help | native | read-only
`,
		},
		{
			name: "start confirmation panel open",
			model: snapshotModel(func(model *Model) {
				model.selection.SelectedIndex = 1
				action := model.actionDescriptors()[3]
				confirmation, _ := model.confirmationForAction(action)
				model.confirmation = &confirmation
				model.confirmAction = &action
				model.confirmArgs = []string{"task-one"}
				model.width = 100
				model.height = 32
			}),
			want: `
Brevity | native | read-only | alerts !3 | p:1 t:1 c:1

Runtime Summary
  repo      C:\repo
  generated (unknown)
  status    1 providers | 1 degraded | 0 unavailable | 0 runnable | 1 cleanup candidate !3
  tasks     1 tracked | 0 runnable | 1 blocked | 0 stale | 0 gated | 0 review !1
  cleanup   1 candidates | 1 inspect !1

Runtime Signals
  prov  codex    degraded !
> task  task-one  blocked !
  clean inspect  orphan-branch:task-old !
  next  inspect  state.

Selected Detail
  select a row, then press d for details

Confirm Action
  action        Start task
  status        not executable unless enabled by command descriptor
  command       powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\\brevity.ps1 task start...
  prompt        Start task task-one changes task state through PowerShell. Go will not write .bre...
  authority     PowerShell is authoritative; Go does not mutate task state
  warning       this changes task state
  preflight     blocked / error
  native gate   task-start preflight for task-one is blocked; no mutation or provider execution o...
  blocked       .brevity directory is not readable
  confirm       enter confirms
  cancel        esc, q, or n cancels

up/down or j/k move | r refresh | p actions | d details | q quit | ? help | native | read-only
`,
		},
		{
			name: "command result panel open",
			model: snapshotModel(func(model *Model) {
				result := pscontract.ExecutionResult{
					ActionID:            pscontract.ActionRunWorker,
					CommandDisplayLabel: "Run worker",
					ExitCode:            2,
					Stderr:              "worker failed",
					RefreshAfter:        true,
				}
				model.commandRun = &commandRunState{
					id:     1,
					action: ActionDescriptor{Label: "Run worker"},
					status: commandFailed,
					result: &result,
				}
				model.width = 100
				model.height = 32
			}),
			want: `
Brevity | native | read-only | alerts !3 | p:1 t:1 c:1

Runtime Summary
  repo      C:\repo
  generated (unknown)
  status    1 providers | 1 degraded | 0 unavailable | 0 runnable | 1 cleanup candidate !3
  tasks     1 tracked | 0 runnable | 1 blocked | 0 stale | 0 gated | 0 review !1
  cleanup   1 candidates | 1 inspect !1

Runtime Signals
> prov  codex    degraded !
  task  task-one  blocked !
  clean inspect  orphan-branch:task-old !
  next  inspect  state.

Selected Detail
  select a row, then press d for details

Command Result
  action        Run worker
  status        failed
  exit code     2
  message       Run worker failed with exit code 2: worker failed
  stderr        worker failed
  close         esc or q closes result

up/down or j/k scroll | home/end | esc close | q close | native | read-only | 1s refresh
`,
		},
		{
			name: "start success result panel open",
			model: snapshotModel(func(model *Model) {
				result := pscontract.ExecutionResult{
					ActionID:            pscontract.ActionStartTask,
					CommandDisplayLabel: "Start task",
					ExitCode:            0,
					Stdout:              "started task-one",
					RefreshAfter:        true,
				}
				model.commandRun = &commandRunState{
					id:     1,
					action: ActionDescriptor{Label: "Start task"},
					status: commandSucceeded,
					result: &result,
				}
				model.width = 100
				model.height = 32
			}),
			want: `
Brevity | native | read-only | alerts !3 | p:1 t:1 c:1

Runtime Summary
  repo      C:\repo
  generated (unknown)
  status    1 providers | 1 degraded | 0 unavailable | 0 runnable | 1 cleanup candidate !3
  tasks     1 tracked | 0 runnable | 1 blocked | 0 stale | 0 gated | 0 review !1
  cleanup   1 candidates | 1 inspect !1

Runtime Signals
> prov  codex    degraded !
  task  task-one  blocked !
  clean inspect  orphan-branch:task-old !
  next  inspect  state.

Selected Detail
  select a row, then press d for details

Command Result
  action        Start task
  status        succeeded
  exit code     0
  message       Start task succeeded
  stdout        started task-one
  follow-up     automatic runtime refresh requested
  close         esc or q closes result

up/down or j/k scroll | home/end | esc close | q close | native | read-only | 1s refresh
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
		if footerIndex >= 0 && (strings.Contains(lines[footerIndex], "q quit") || strings.Contains(lines[footerIndex], "q close")) {
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
