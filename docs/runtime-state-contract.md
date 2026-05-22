# Runtime Supervisor State Contract

This document defines the v1 file contract for the Go-native runtime supervisor
foundation introduced by `runtime-supervisor-foundation-v1`.

The contract covers these files:

- `.brevity/runtime.json`
- `.brevity/runtime.lock`
- `.brevity/runtime.stop`

These files are runtime infrastructure state. They are not task state, queue
state, provider state, or durable project memory.

## Ownership

The Go runtime owns these files. PowerShell may remain as compatibility or
reference behavior, but new supervisor-owned runtime behavior should be
implemented in Go.

The runtime supervisor may write `.brevity/runtime.json`, acquire and release
`.brevity/runtime.lock`, and read or clear `.brevity/runtime.stop`. Other
commands may read runtime state for operator status. `runtime stop` may create a
stop request file.

## `.brevity/runtime.json`

`runtime.json` is the latest supervisor heartbeat snapshot.

The supervisor writes this file when it starts, refreshes it while running, and
marks it stopped during graceful shutdown. Writes are atomic through the native
state store.

Missing `runtime.json` means the runtime is stopped or has never been started.
Readers must report this as not running, not as task failure.

### Fields

- `pid` - operating system process id for the supervisor process.
- `startedAt` - UTC RFC3339 timestamp for supervisor start.
- `heartbeatAt` - UTC RFC3339 timestamp for the latest supervisor heartbeat.
- `status` - runtime lifecycle status.
- `activeWorkers` - current number of workers owned by the supervisor. In v1 this
  is always `0`.
- `queueDepth` - current queue depth owned by the supervisor. In v1 this is
  always `0`.
- `version` - runtime state file version. Current version is `1`.

### Status Values

The v1 implementation persists:

- `running`
- `stopped`

Readers also tolerate these reserved lifecycle values so future additive changes
can be reported safely:

- `starting`
- `stopping`
- `stale`
- `unknown`

Unrecognized status values must be reported clearly as invalid runtime state.
They must not cause task metadata repair, task failure, queue execution, or
provider execution.

### Stale Runtime Behavior

A runtime is stale when `runtime.json` exists but the recorded process is not
alive, or the heartbeat is older than the reader's freshness threshold.

Stale runtime state means supervisor infrastructure is stale. It does not mean
any task failed. A stale supervisor must not corrupt, reconcile, rewrite, or infer
task execution state.

`runtime stop` may mark stale runtime state as `stopped` and clear a stop request
so the operator can recover the supervisor lifecycle. This action is limited to
runtime infrastructure files.

### Corrupted Or Invalid State Behavior

Malformed JSON must be reported as corrupted runtime state with a useful error.
Readers must not panic, attempt provider execution, or mutate task state.

Invalid but parseable state, such as an unsupported future `version`, missing
active-runtime fields, an invalid active-runtime pid, negative worker counts, or
an unrecognized status, must be reported clearly as invalid runtime state.

Unknown future versions are not silently accepted. They are reported as
unsupported so operators know the reader and producer contracts differ.

## `.brevity/runtime.lock`

`runtime.lock` is an advisory single-supervisor lock. It prevents multiple
supervisor processes from owning the heartbeat file at the same time.

The lock file contains simple diagnostic text:

```text
pid=<process-id>
createdAt=<utc-rfc3339-nano>
```

The supervisor acquires the lock before writing heartbeat state and releases it
on exit. Lock acquisition uses a timeout and may remove a stale lock after the
configured stale age. Stale lock cleanup is limited to the lock file itself.

The runtime lock is separate from `.brevity/state.lock`, which protects task and
other Brevity state mutations.

## `.brevity/runtime.stop`

`runtime.stop` is a stop-request marker.

`runtime stop` creates this file with a UTC timestamp. The running supervisor
checks for it on heartbeat ticks. When seen, the supervisor writes a final
`stopped` state and exits. The supervisor or stop command then clears the stop
request.

Creating `runtime.stop` must not kill workers, drain queues, execute providers,
or mutate task execution state. In v1 there are no supervisor-owned workers or
queues to drain.

## Safety Invariants

- Runtime state is infrastructure state, not task state.
- Supervisor failure is not task failure.
- Stale runtime state must not corrupt task state.
- Corrupted `runtime.json` must be reported safely.
- Runtime status is read-only for status/dashboard consumers.
- Dry-run paths must never start the supervisor.
- The supervisor must not execute providers.
- The supervisor must not drain queues.
- The supervisor must not mutate task execution state.
- The supervisor must not spawn workers in v1.
- The supervisor must not expose daemon APIs, HTTP, websockets, scheduling, or
  distributed execution in v1.

## Operator Commands

```powershell
go run ./cmd/brevity runtime start
go run ./cmd/brevity runtime status
go run ./cmd/brevity runtime stop
```

`runtime status` is read-only. Missing state reports stopped/not running.
Corrupted or invalid state reports a useful status and error.
