// Package cmux is the read-only CMUX operator layer over Brevity runtime contracts.
//
// CMUX consumes two existing Brevity CLI contracts:
//   - brevity.runtime-state.v1  (brevity runtime state --json)
//   - brevity.runtime-scheduler-plan.v1 (brevity scheduler plan --json)
//
// It never reads .brevity files directly, never mutates runtime state,
// and never owns a scheduler, task store, provider registry, or worker lifecycle.
package cmux

import (
	"github.com/mortenlein/brevity/internal/contracts"
	runtimescheduler "github.com/mortenlein/brevity/internal/runtime/scheduler"
)

// Snapshot is the parsed CMUX runtime view assembled from both contracts.
// HasRuntimeState and HasSchedulerPlan signal successful parses.
// Error fields capture failures without preventing partial rendering.
type Snapshot struct {
	RuntimeState    contracts.RuntimeState
	HasRuntimeState bool
	RuntimeStateErr error

	SchedulerPlan    runtimescheduler.Plan
	HasSchedulerPlan bool
	SchedulerPlanErr error
}
