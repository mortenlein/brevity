package cmux

import (
	"encoding/json"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
	runtimequeue "github.com/mortenlein/brevity/internal/runtime/queue"
	runtimescheduler "github.com/mortenlein/brevity/internal/runtime/scheduler"
	"github.com/mortenlein/brevity/internal/runtimeclient"
	"github.com/mortenlein/brevity/internal/state"
)

// Fetcher is the contract boundary between CMUX and the Brevity runtime.
// Implementations must return JSON matching the documented Brevity CLI contracts.
// CMUX never calls this boundary for mutations, only for read-only state.
type Fetcher interface {
	// RuntimeStateJSON returns brevity.runtime-state.v1 JSON,
	// equivalent to: brevity runtime state --json
	RuntimeStateJSON() ([]byte, error)

	// SchedulerPlanJSON returns brevity.runtime-scheduler-plan.v1 JSON,
	// equivalent to: brevity scheduler plan --json
	SchedulerPlanJSON() ([]byte, error)
}

// NativeFetcher implements Fetcher using the native Go runtime client.
// It is the production path: it maps exactly to what the Brevity CLI
// subcommands would produce.
type NativeFetcher struct {
	RepoRoot string
}

// RuntimeStateJSON returns the native runtime state contract.
func (f NativeFetcher) RuntimeStateJSON() ([]byte, error) {
	return runtimeclient.NewNativeClient(f.RepoRoot).RuntimeStateJSON()
}

// SchedulerPlanJSON returns the native scheduler plan contract.
func (f NativeFetcher) SchedulerPlanJSON() ([]byte, error) {
	base, err := state.NewStore(f.RepoRoot)
	if err != nil {
		return nil, err
	}
	queueStore := runtimequeue.Store{
		Store: base,
		Now:   time.Now,
	}
	plan := runtimescheduler.Planner{Queue: queueStore}.Plan()
	return json.Marshal(plan)
}

// Read fetches and parses both runtime contracts into a Snapshot.
// Fetch or parse errors are captured inside the Snapshot rather than
// propagated: the renderer can display a partial or degraded view.
func Read(fetcher Fetcher) Snapshot {
	snap := Snapshot{}

	stateBytes, err := fetcher.RuntimeStateJSON()
	if err != nil {
		snap.RuntimeStateErr = err
	} else {
		rs, parseErr := contracts.ParseRuntimeState(stateBytes)
		if parseErr != nil {
			snap.RuntimeStateErr = parseErr
		} else {
			snap.RuntimeState = rs
			snap.HasRuntimeState = true
		}
	}

	planBytes, err := fetcher.SchedulerPlanJSON()
	if err != nil {
		snap.SchedulerPlanErr = err
	} else {
		var plan runtimescheduler.Plan
		if parseErr := json.Unmarshal(planBytes, &plan); parseErr != nil {
			snap.SchedulerPlanErr = parseErr
		} else {
			snap.SchedulerPlan = plan
			snap.HasSchedulerPlan = true
		}
	}

	return snap
}
