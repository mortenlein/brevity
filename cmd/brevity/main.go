package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	"github.com/mortenlein/brevity/internal/actions"
	"github.com/mortenlein/brevity/internal/bubbleteadashboard"
	nativecleanup "github.com/mortenlein/brevity/internal/cleanup"
	"github.com/mortenlein/brevity/internal/cmux"
	"github.com/mortenlein/brevity/internal/commands"
	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/dashboard"
	"github.com/mortenlein/brevity/internal/diagnostics"
	"github.com/mortenlein/brevity/internal/preflight"
	"github.com/mortenlein/brevity/internal/runmaintenance"
	runtimeexecution "github.com/mortenlein/brevity/internal/runtime/execution"
	runtimequeue "github.com/mortenlein/brevity/internal/runtime/queue"
	runtimescheduler "github.com/mortenlein/brevity/internal/runtime/scheduler"
	runtimestate "github.com/mortenlein/brevity/internal/runtime/state"
	runtimesupervisor "github.com/mortenlein/brevity/internal/runtime/supervisor"
	"github.com/mortenlein/brevity/internal/runtimeclient"
	"github.com/mortenlein/brevity/internal/state"
	"github.com/mortenlein/brevity/internal/support"
)

func main() {
	if err := run(os.Stdout, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "brevity-go:", err)
		os.Exit(1)
	}
}

type commandKind string

const (
	commandDashboard            commandKind = commandKind(commands.DashboardID)
	commandRuntimeState         commandKind = commandKind(commands.RuntimeStateID)
	commandRuntimeStart         commandKind = commandKind(commands.RuntimeStartID)
	commandRuntimeStop          commandKind = commandKind(commands.RuntimeStopID)
	commandRuntimeStatus        commandKind = commandKind(commands.RuntimeStatusID)
	commandQueueAdd             commandKind = commandKind(commands.QueueAddID)
	commandQueueList            commandKind = commandKind(commands.QueueListID)
	commandQueueInspect         commandKind = commandKind(commands.QueueInspectID)
	commandQueuePlan            commandKind = commandKind(commands.QueuePlanID)
	commandQueueRemove          commandKind = commandKind(commands.QueueRemoveID)
	commandQueueReserve         commandKind = commandKind(commands.QueueReserveID)
	commandQueueUnreserve       commandKind = commandKind(commands.QueueUnreserveID)
	commandSchedulerPlan        commandKind = commandKind(commands.SchedulerPlanID)
	commandSchedulerReserveNext commandKind = commandKind(commands.SchedulerReserveNextID)
	commandSchedulerPlanExec    commandKind = commandKind(commands.SchedulerPlanExecutionID)
	commandExecutionList        commandKind = commandKind(commands.ExecutionListID)
	commandExecutionInspect     commandKind = commandKind(commands.ExecutionInspectID)
	commandExecutionPlan        commandKind = commandKind(commands.ExecutionPlanFromReservationID)
	commandExecutionMarkReady   commandKind = commandKind(commands.ExecutionMarkReadyID)
	commandExecutionMarkPlanned commandKind = commandKind(commands.ExecutionMarkPlannedID)
	commandProviderStatus       commandKind = commandKind(commands.ProviderStatusID)
	commandProviderSet          commandKind = commandKind(commands.ProviderSetID)
	commandProviderReset        commandKind = commandKind(commands.ProviderResetID)
	commandInit                 commandKind = commandKind(commands.InitID)
	commandContextRefresh       commandKind = commandKind(commands.TaskContextRefreshID)
	commandTaskStatus           commandKind = commandKind(commands.TaskStatusID)
	commandDoctor               commandKind = commandKind(commands.DoctorID)
	commandTaskCleanup          commandKind = commandKind(commands.TaskCleanupID)
	commandTaskMerge            commandKind = commandKind(commands.TaskMergeID)
	commandTaskPreflight        commandKind = "task-preflight"
	commandTaskNew              commandKind = commandKind(commands.TaskNewID)
	commandTaskActivate         commandKind = commandKind(commands.TaskActivateID)
	commandTaskSpec             commandKind = commandKind(commands.TaskSpecID)
	commandTaskStart            commandKind = commandKind(commands.TaskStartID)
	commandTaskRun              commandKind = commandKind(commands.TaskRunID)
	commandTaskRuntimeInfo      commandKind = commandKind(commands.TaskRuntimeInfoID)
	commandTaskDetail           commandKind = commandKind(commands.TaskDetailID)
	commandTaskRuns             commandKind = commandKind(commands.TaskRunsID)
	commandRunsReconcile        commandKind = commandKind(commands.TaskRunsReconcileID)
	commandRunsRetention        commandKind = commandKind(commands.TaskRunsRetentionID)
	commandRunsCompact          commandKind = commandKind(commands.TaskRunsCompactID)
	commandRunsInspect          commandKind = "runs-inspect"
	commandRunsNativeCompact    commandKind = "runs-compact"
	commandCleanupInspect       commandKind = commandKind(commands.CleanupInspectID)
	commandCleanupPlan          commandKind = commandKind(commands.CleanupPlanID)
	commandCleanupExecute       commandKind = commandKind(commands.CleanupExecuteID)
	commandSupportMatrix        commandKind = "support-matrix"
	commandCmux                 commandKind = "cmux"
)

type cliOptions struct {
	help              bool
	kind              commandKind
	once              bool
	watch             bool
	bubble            bool
	noClear           bool
	refresh           time.Duration
	jsonSource        string
	provider          string
	status            string
	note              string
	slug              string
	force             bool
	execute           bool
	dryRun            bool
	plan              bool
	profile           string
	smoke             bool
	json              bool
	all               bool
	repair            bool
	candidateID       string
	preflightAction   preflight.Action
	cmuxLimit         int
	cmuxSection       string
	cmuxTask          string
	cmuxState         string
	cmuxOutput        string
	cmuxReview        string
	cmuxHandoff       bool
	cmuxMergeReport   bool
	cmuxBlockedReport bool
}

type actionCall func() ([]byte, error)
type actionRenderer func(io.Writer, contracts.CommandResult) error
type actionCheck func(contracts.CommandResult) error

type actionSpec struct {
	call   actionCall
	render actionRenderer
	check  actionCheck
}

func usageError(command commands.Command) error {
	return fmt.Errorf("usage: %s", command.Usage)
}

func parseOptions(args []string) (cliOptions, error) {
	options := cliOptions{kind: commandDashboard, refresh: 5 * time.Second, jsonSource: "powershell"}
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			options.help = true
			// Preserve the subcommand kind so the help dispatcher can show
			// subcommand-specific text rather than the generic dashboard help.
			if len(args) > 0 && args[0] == "cmux" {
				options.kind = commandCmux
			}
			return options, nil
		}
	}

	if len(args) > 0 && args[0] == "provider" {
		return parseProviderOptions(args)
	}
	if len(args) > 0 && args[0] == "init" {
		return parseInitOptions(args)
	}
	if len(args) > 0 && args[0] == "doctor" {
		return parseDoctorOptions(args)
	}
	if len(args) > 0 && args[0] == "runtime" {
		return parseRuntimeOptions(args)
	}
	if len(args) > 0 && args[0] == "queue" {
		return parseQueueOptions(args)
	}
	if len(args) > 0 && args[0] == "scheduler" {
		return parseSchedulerOptions(args)
	}
	if len(args) > 0 && args[0] == "execution" {
		return parseExecutionOptions(args)
	}
	if len(args) > 0 && args[0] == "task" {
		return parseTaskOptions(args)
	}
	if len(args) > 0 && args[0] == "runs" {
		return parseRunsOptions(args)
	}
	if len(args) > 0 && args[0] == "cleanup" {
		return parseCleanupOptions(args)
	}
	if len(args) > 0 && args[0] == "support" {
		return parseSupportOptions(args)
	}
	if len(args) > 0 && args[0] == "cmux" {
		return parseCmuxOptions(args)
	}

	flags := flag.NewFlagSet("brevity", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.BoolVar(&options.once, "once", false, "render the dashboard once")
	flags.BoolVar(&options.watch, "watch", false, "refresh the dashboard until interrupted")
	flags.BoolVar(&options.bubble, "bubble", false, "run the experimental Bubble Tea dashboard")
	flags.BoolVar(&options.noClear, "no-clear", false, "do not clear the screen before changed dashboard renders")
	refresh := flags.String("refresh", options.refresh.String(), "dashboard refresh interval")
	jsonSource := flags.String("json-source", options.jsonSource, "runtime JSON source")

	if err := flags.Parse(args); err != nil {
		return cliOptions{}, err
	}
	parsedRefresh, err := time.ParseDuration(*refresh)
	if err != nil {
		return cliOptions{}, fmt.Errorf("invalid --refresh value %q: %w", *refresh, err)
	}
	if parsedRefresh <= 0 {
		return cliOptions{}, fmt.Errorf("invalid --refresh value %q: duration must be greater than zero", *refresh)
	}
	options.refresh = parsedRefresh
	if *jsonSource != "powershell" && *jsonSource != "native" {
		return cliOptions{}, fmt.Errorf("unsupported json source: %s", *jsonSource)
	}
	options.jsonSource = *jsonSource
	if options.once && options.watch {
		return cliOptions{}, fmt.Errorf("--once and --watch cannot be used together")
	}
	if options.bubble && options.watch {
		return cliOptions{}, fmt.Errorf("--bubble and --watch cannot be used together")
	}
	if options.bubble && options.once {
		return cliOptions{}, fmt.Errorf("--bubble and --once cannot be used together")
	}
	if flags.NArg() > 0 {
		return cliOptions{}, fmt.Errorf("unexpected argument: %s", flags.Arg(0))
	}

	return options, nil
}

func parseQueueOptions(args []string) (cliOptions, error) {
	if len(args) < 2 {
		return cliOptions{}, fmt.Errorf("missing queue command: supported commands: add, list, inspect, plan, remove, reserve, unreserve")
	}
	switch args[1] {
	case "add":
		if len(args) != 3 || strings.TrimSpace(args[2]) == "" {
			return cliOptions{}, usageError(commands.QueueAdd)
		}
		return cliOptions{kind: commandQueueAdd, slug: args[2]}, nil
	case "list":
		if len(args) != 2 {
			return cliOptions{}, usageError(commands.QueueList)
		}
		return cliOptions{kind: commandQueueList}, nil
	case "inspect":
		options := cliOptions{kind: commandQueueInspect}
		for _, arg := range args[2:] {
			if arg != "--json" {
				return cliOptions{}, usageError(commands.QueueInspect)
			}
			options.json = true
		}
		return options, nil
	case "plan":
		options := cliOptions{kind: commandQueuePlan}
		for _, arg := range args[2:] {
			if arg != "--json" {
				return cliOptions{}, usageError(commands.QueuePlan)
			}
			options.json = true
		}
		return options, nil
	case "remove":
		if len(args) != 3 || strings.TrimSpace(args[2]) == "" {
			return cliOptions{}, usageError(commands.QueueRemove)
		}
		return cliOptions{kind: commandQueueRemove, candidateID: args[2]}, nil
	case "reserve":
		if len(args) != 3 || strings.TrimSpace(args[2]) == "" {
			return cliOptions{}, usageError(commands.QueueReserve)
		}
		return cliOptions{kind: commandQueueReserve, candidateID: args[2]}, nil
	case "unreserve":
		if len(args) != 3 || strings.TrimSpace(args[2]) == "" {
			return cliOptions{}, usageError(commands.QueueUnreserve)
		}
		return cliOptions{kind: commandQueueUnreserve, candidateID: args[2]}, nil
	default:
		return cliOptions{}, fmt.Errorf("unsupported queue command %q: supported commands: add, list, inspect, plan, remove, reserve, unreserve", args[1])
	}
}

func parseExecutionOptions(args []string) (cliOptions, error) {
	if len(args) < 2 {
		return cliOptions{}, fmt.Errorf("missing execution command: supported commands: list, inspect, plan-from-reservation, mark-ready, mark-planned")
	}
	switch args[1] {
	case "list":
		if len(args) != 2 {
			return cliOptions{}, usageError(commands.ExecutionList)
		}
		return cliOptions{kind: commandExecutionList}, nil
	case "inspect":
		options := cliOptions{kind: commandExecutionInspect}
		for _, arg := range args[2:] {
			if arg != "--json" {
				return cliOptions{}, usageError(commands.ExecutionInspect)
			}
			options.json = true
		}
		return options, nil
	case "plan-from-reservation":
		if len(args) != 3 || strings.TrimSpace(args[2]) == "" {
			return cliOptions{}, usageError(commands.ExecutionPlanFromReservation)
		}
		return cliOptions{kind: commandExecutionPlan, candidateID: args[2]}, nil
	case "mark-ready":
		if len(args) != 3 || strings.TrimSpace(args[2]) == "" {
			return cliOptions{}, usageError(commands.ExecutionMarkReady)
		}
		return cliOptions{kind: commandExecutionMarkReady, candidateID: args[2]}, nil
	case "mark-planned":
		if len(args) != 3 || strings.TrimSpace(args[2]) == "" {
			return cliOptions{}, usageError(commands.ExecutionMarkPlanned)
		}
		return cliOptions{kind: commandExecutionMarkPlanned, candidateID: args[2]}, nil
	default:
		return cliOptions{}, fmt.Errorf("unsupported execution command %q: supported commands: list, inspect, plan-from-reservation, mark-ready, mark-planned", args[1])
	}
}

func parseSchedulerOptions(args []string) (cliOptions, error) {
	if len(args) < 2 {
		return cliOptions{}, fmt.Errorf("missing scheduler command: supported commands: plan, reserve-next, plan-execution")
	}
	switch args[1] {
	case "plan":
		options := cliOptions{kind: commandSchedulerPlan}
		for _, arg := range args[2:] {
			if arg != "--json" {
				return cliOptions{}, usageError(commands.SchedulerPlan)
			}
			options.json = true
		}
		return options, nil
	case "reserve-next":
		if len(args) != 2 {
			return cliOptions{}, usageError(commands.SchedulerReserveNext)
		}
		return cliOptions{kind: commandSchedulerReserveNext}, nil
	case "plan-execution":
		options := cliOptions{kind: commandSchedulerPlanExec}
		for _, arg := range args[2:] {
			if arg != "--json" {
				return cliOptions{}, usageError(commands.SchedulerPlanExecution)
			}
			options.json = true
		}
		return options, nil
	default:
		return cliOptions{}, fmt.Errorf("unsupported scheduler command %q: supported commands: plan, reserve-next, plan-execution", args[1])
	}
}

func parseInitOptions(args []string) (cliOptions, error) {
	options := cliOptions{kind: commandInit}
	for _, arg := range args[1:] {
		switch arg {
		case "--repair":
			options.repair = true
		case "--json":
			options.json = true
		default:
			return cliOptions{}, usageError(commands.Init)
		}
	}
	return options, nil
}

func parseCmuxOptions(args []string) (cliOptions, error) {
	flags := flag.NewFlagSet("brevity cmux", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	limit := flags.Int("limit", cmux.DefaultLimit, "maximum number of tasks to show")
	section := flags.String("section", cmux.SectionAll, "section to render: all, providers, tasks, queue, actions")
	task := flags.String("task", "", "filter task list to this exact task slug")
	state := flags.String("state", "", "filter task list to tasks with this normalised state")
	output := flags.String("output", string(cmux.OutputText), "output format: text, markdown, or json")
	review := flags.String("review", "", "generate a focused review packet for this task slug")
	handoff := flags.Bool("handoff", false, "generate an AI/operator handoff packet")
	mergeReport := flags.Bool("merge-report", false, "generate a merge readiness report grouped by state")
	blockedReport := flags.Bool("blocked-report", false, "generate a blocked task report grouped by block reason")
	if err := flags.Parse(args[1:]); err != nil {
		return cliOptions{}, fmt.Errorf("usage: brevity cmux [--limit <n>] [--section <name>] [--task <slug>] [--state <state>] [--output text|markdown|json] [--review <slug>] [--handoff] [--merge-report] [--blocked-report]")
	}
	if flags.NArg() > 0 {
		return cliOptions{}, fmt.Errorf("usage: brevity cmux [--limit <n>] [--section <name>] [--task <slug>] [--state <state>] [--output text|markdown|json] [--review <slug>] [--handoff] [--merge-report] [--blocked-report]")
	}
	switch *section {
	case cmux.SectionAll, cmux.SectionProviders, cmux.SectionTasks, cmux.SectionQueue, cmux.SectionActions:
		// valid
	default:
		return cliOptions{}, fmt.Errorf("invalid --section %q: allowed values: all, providers, tasks, queue, actions", *section)
	}
	if *limit <= 0 {
		return cliOptions{}, fmt.Errorf("invalid --limit %d: must be greater than zero", *limit)
	}
	switch cmux.OutputMode(*output) {
	case cmux.OutputText, cmux.OutputMarkdown, cmux.OutputJSON:
		// valid
	default:
		return cliOptions{}, fmt.Errorf("invalid --output %q: allowed values: text, markdown, json", *output)
	}
	return cliOptions{
		kind:              commandCmux,
		cmuxLimit:         *limit,
		cmuxSection:       *section,
		cmuxTask:          *task,
		cmuxState:         *state,
		cmuxOutput:        *output,
		cmuxReview:        *review,
		cmuxHandoff:       *handoff,
		cmuxMergeReport:   *mergeReport,
		cmuxBlockedReport: *blockedReport,
	}, nil
}

func routeCmuxCommand(stdout io.Writer, options cliOptions) error {
	snap := cmux.Read(cmux.NativeFetcher{})
	cmux.Render(stdout, snap, cmux.RenderOptions{
		Limit:         options.cmuxLimit,
		Section:       options.cmuxSection,
		TaskSlug:      options.cmuxTask,
		StateFilter:   options.cmuxState,
		Output:        cmux.OutputMode(options.cmuxOutput),
		ReviewTask:    options.cmuxReview,
		Handoff:       options.cmuxHandoff,
		MergeReport:   options.cmuxMergeReport,
		BlockedReport: options.cmuxBlockedReport,
	})
	return nil
}

func parseSupportOptions(args []string) (cliOptions, error) {
	if len(args) < 2 || args[1] != "matrix" {
		return cliOptions{}, fmt.Errorf("usage: brevity support matrix [--json]")
	}
	options := cliOptions{kind: commandSupportMatrix}
	for _, arg := range args[2:] {
		if arg != "--json" {
			return cliOptions{}, fmt.Errorf("usage: brevity support matrix [--json]")
		}
		options.json = true
	}
	return options, nil
}

func parseRunsOptions(args []string) (cliOptions, error) {
	if len(args) < 2 {
		return cliOptions{}, fmt.Errorf("missing runs command: supported commands: inspect, compact")
	}
	switch args[1] {
	case "inspect":
		options := cliOptions{kind: commandRunsInspect}
		for _, arg := range args[2:] {
			if arg != "--json" {
				return cliOptions{}, fmt.Errorf("usage: brevity runs inspect [--json]")
			}
			options.json = true
		}
		return options, nil
	case "compact":
		options := cliOptions{kind: commandRunsNativeCompact}
		for _, arg := range args[2:] {
			switch arg {
			case "--plan":
				options.plan = true
			case "--force":
				options.force = true
			case "--json":
				options.json = true
			default:
				return cliOptions{}, fmt.Errorf("usage: brevity runs compact --plan|--force [--json]")
			}
		}
		if options.plan && options.force {
			return cliOptions{}, fmt.Errorf("brevity runs compact cannot combine --plan and --force")
		}
		if !options.plan && !options.force {
			return cliOptions{}, fmt.Errorf("brevity runs compact requires --plan or --force")
		}
		return options, nil
	default:
		return cliOptions{}, fmt.Errorf("unsupported runs command %q: supported commands: inspect, compact", args[1])
	}
}

func parseCleanupOptions(args []string) (cliOptions, error) {
	if len(args) < 2 {
		return cliOptions{}, fmt.Errorf("missing cleanup command: supported commands: inspect, plan, execute")
	}
	switch args[1] {
	case "inspect":
		options := cliOptions{kind: commandCleanupInspect}
		for _, arg := range args[2:] {
			if arg != "--json" {
				return cliOptions{}, usageError(commands.CleanupInspect)
			}
			options.json = true
		}
		return options, nil
	case "plan":
		options := cliOptions{kind: commandCleanupPlan}
		for _, arg := range args[2:] {
			switch arg {
			case "--json":
				options.json = true
			case "--all":
				options.all = true
			default:
				if strings.HasPrefix(arg, "-") || options.candidateID != "" {
					return cliOptions{}, usageError(commands.CleanupPlan)
				}
				options.candidateID = arg
			}
		}
		if !options.all && strings.TrimSpace(options.candidateID) == "" {
			return cliOptions{}, usageError(commands.CleanupPlan)
		}
		if options.all && strings.TrimSpace(options.candidateID) != "" {
			return cliOptions{}, fmt.Errorf("brevity cleanup plan cannot combine --all and a candidate id")
		}
		return options, nil
	case "execute":
		options := cliOptions{kind: commandCleanupExecute}
		for _, arg := range args[2:] {
			switch arg {
			case "--json":
				options.json = true
			case "--all":
				options.all = true
			case "--force":
				options.force = true
			default:
				if strings.HasPrefix(arg, "-") || options.candidateID != "" {
					return cliOptions{}, usageError(commands.CleanupExecute)
				}
				options.candidateID = arg
			}
		}
		if !options.all && strings.TrimSpace(options.candidateID) == "" {
			return cliOptions{}, usageError(commands.CleanupExecute)
		}
		if options.all && strings.TrimSpace(options.candidateID) != "" {
			return cliOptions{}, fmt.Errorf("brevity cleanup execute cannot combine --all and a candidate id")
		}
		return options, nil
	default:
		return cliOptions{}, usageError(commands.CleanupInspect)
	}
}

func parseRuntimeOptions(args []string) (cliOptions, error) {
	if len(args) < 2 {
		return cliOptions{}, fmt.Errorf("missing runtime command: supported commands: state, start, stop, status")
	}
	switch args[1] {
	case "state":
		options := cliOptions{kind: commandRuntimeState}
		for _, arg := range args[2:] {
			if arg != "--json" {
				return cliOptions{}, usageError(commands.RuntimeState)
			}
			options.json = true
		}
		if !options.json {
			return cliOptions{}, usageError(commands.RuntimeState)
		}
		return options, nil
	case "start":
		if len(args) != 2 {
			return cliOptions{}, usageError(commands.RuntimeStart)
		}
		return cliOptions{kind: commandRuntimeStart}, nil
	case "stop":
		if len(args) != 2 {
			return cliOptions{}, usageError(commands.RuntimeStop)
		}
		return cliOptions{kind: commandRuntimeStop}, nil
	case "status":
		if len(args) != 2 {
			return cliOptions{}, usageError(commands.RuntimeStatus)
		}
		return cliOptions{kind: commandRuntimeStatus}, nil
	default:
		return cliOptions{}, fmt.Errorf("unsupported runtime command %q: supported commands: state, start, stop, status", args[1])
	}
}

func parseDoctorOptions(args []string) (cliOptions, error) {
	options := cliOptions{kind: commandDoctor}
	for _, arg := range args[1:] {
		if arg != "--json" {
			return cliOptions{}, usageError(commands.Doctor)
		}
		options.json = true
	}
	return options, nil
}

func parseProviderOptions(args []string) (cliOptions, error) {
	if len(args) < 2 {
		return cliOptions{}, fmt.Errorf("missing provider command: supported commands: status, set, reset")
	}

	options := cliOptions{kind: commandDashboard}
	switch args[1] {
	case "status":
		if len(args) != 2 {
			return cliOptions{}, usageError(commands.ProviderStatus)
		}
		options.kind = commandProviderStatus
	case "set":
		if len(args) < 4 {
			return cliOptions{}, usageError(commands.ProviderSet)
		}
		options.kind = commandProviderSet
		options.provider = args[2]
		options.status = args[3]
		for index := 4; index < len(args); index++ {
			arg := args[index]
			if arg != "--note" && arg != "-Note" {
				return cliOptions{}, fmt.Errorf("unknown argument for brevity provider set: %s", arg)
			}
			index++
			if index >= len(args) {
				return cliOptions{}, fmt.Errorf("brevity provider set %s %s --note requires a note", options.provider, options.status)
			}
			options.note = args[index]
		}
	case "reset":
		if len(args) != 3 {
			return cliOptions{}, usageError(commands.ProviderReset)
		}
		options.kind = commandProviderReset
		options.provider = args[2]
	default:
		return cliOptions{}, fmt.Errorf("unsupported provider command %q: supported commands: status, set, reset", args[1])
	}

	return options, nil
}

func parseTaskOptions(args []string) (cliOptions, error) {
	if len(args) < 2 {
		return cliOptions{}, fmt.Errorf("missing task command: supported commands: status, refresh-context, context refresh, runtime-info, detail, runs, new, activate, spec, start, run, merge, cleanup, preflight")
	}
	if args[1] == "status" {
		if len(args) != 2 {
			return cliOptions{}, usageError(commands.TaskStatus)
		}
		return cliOptions{kind: commandTaskStatus}, nil
	}
	if args[1] == "context" {
		if len(args) < 4 || args[2] != "refresh" {
			return cliOptions{}, usageError(commands.TaskContextRefresh)
		}
		if args[3] == "" {
			return cliOptions{}, usageError(commands.TaskContextRefresh)
		}
		options := cliOptions{kind: commandContextRefresh, slug: args[3]}
		for _, arg := range args[4:] {
			if arg != "--json" {
				return cliOptions{}, usageError(commands.TaskContextRefresh)
			}
			options.json = true
		}
		return options, nil
	}
	if args[1] == "refresh-context" {
		if len(args) < 3 || args[2] == "" {
			return cliOptions{}, usageError(commands.TaskContextRefresh)
		}
		options := cliOptions{kind: commandContextRefresh, slug: args[2]}
		for _, arg := range args[3:] {
			if arg != "--json" {
				return cliOptions{}, usageError(commands.TaskContextRefresh)
			}
			options.json = true
		}
		return options, nil
	}
	if args[1] == "cleanup" {
		return parseTaskCleanupOptions(args)
	}
	if args[1] == "preflight" {
		return parseTaskPreflightOptions(args)
	}
	if args[1] == "new" {
		return parseTaskNewOptions(args)
	}
	if args[1] == "activate" {
		return parseTaskSlugJSONOptions(args, commandTaskActivate, commands.TaskActivate)
	}
	if args[1] == "spec" {
		return parseTaskSlugJSONOptions(args, commandTaskSpec, commands.TaskSpec)
	}
	if args[1] == "start" {
		return parseTaskStartOptions(args)
	}
	if args[1] == "run" {
		return parseTaskRunOptions(args)
	}
	if args[1] == "merge" {
		return parseTaskMergeOptions(args)
	}
	if args[1] == "runtime-info" {
		return parseTaskRuntimeInfoOptions(args)
	}
	if args[1] == "detail" {
		return parseTaskDetailOptions(args)
	}
	if args[1] == "runs" {
		return parseTaskRunsOptions(args)
	}

	return cliOptions{}, fmt.Errorf("unsupported task command %q: supported commands: status, refresh-context, context refresh, runtime-info, detail, runs, new, start, run, merge, cleanup, preflight", args[1])
}

func parseTaskSlugJSONOptions(args []string, kind commandKind, command commands.Command) (cliOptions, error) {
	if len(args) < 3 || args[2] == "" {
		return cliOptions{}, usageError(command)
	}
	options := cliOptions{kind: kind, slug: args[2]}
	for _, arg := range args[3:] {
		if arg != "--json" {
			return cliOptions{}, usageError(command)
		}
		options.json = true
	}
	return options, nil
}

func parseTaskMergeOptions(args []string) (cliOptions, error) {
	if len(args) < 3 || args[2] == "" {
		return cliOptions{}, usageError(commands.TaskMerge)
	}
	options := cliOptions{kind: commandTaskMerge, slug: args[2]}
	for _, arg := range args[3:] {
		switch arg {
		case "--plan":
			options.plan = true
		case "--json":
			options.json = true
		default:
			return cliOptions{}, usageError(commands.TaskMerge)
		}
	}
	return options, nil
}

func parseTaskPreflightOptions(args []string) (cliOptions, error) {
	if len(args) < 4 || args[3] == "" {
		return cliOptions{}, fmt.Errorf("usage: brevity task preflight <new|start|run|merge|cleanup> <slug> [--json]")
	}
	action, ok := preflightActionFromWord(args[2])
	if !ok {
		return cliOptions{}, fmt.Errorf("unsupported preflight action %q: supported actions: new, start, run, merge, cleanup", args[2])
	}
	options := cliOptions{kind: commandTaskPreflight, preflightAction: action, slug: args[3]}
	for _, arg := range args[4:] {
		if arg != "--json" {
			return cliOptions{}, fmt.Errorf("unknown argument for brevity task preflight %s: %s", args[2], arg)
		}
		options.json = true
	}
	return options, nil
}

func preflightActionFromWord(word string) (preflight.Action, bool) {
	switch strings.ToLower(strings.TrimSpace(word)) {
	case "new":
		return preflight.ActionTaskNew, true
	case "start":
		return preflight.ActionTaskStart, true
	case "run":
		return preflight.ActionTaskRun, true
	case "merge":
		return preflight.ActionTaskMerge, true
	case "cleanup":
		return preflight.ActionTaskCleanup, true
	default:
		return "", false
	}
}

func parseTaskNewOptions(args []string) (cliOptions, error) {
	if len(args) < 3 || args[2] == "" {
		return cliOptions{}, usageError(commands.TaskNew)
	}
	options := cliOptions{kind: commandTaskNew, slug: args[2]}
	for _, arg := range args[3:] {
		if arg != "--json" {
			return cliOptions{}, usageError(commands.TaskNew)
		}
		options.json = true
	}
	return options, nil
}

func parseTaskStartOptions(args []string) (cliOptions, error) {
	if len(args) < 3 || args[2] == "" {
		return cliOptions{}, usageError(commands.TaskStart)
	}
	options := cliOptions{kind: commandTaskStart, slug: args[2]}
	for _, arg := range args[3:] {
		if arg != "--json" {
			return cliOptions{}, usageError(commands.TaskStart)
		}
		options.json = true
	}
	return options, nil
}

func parseTaskCleanupOptions(args []string) (cliOptions, error) {
	if len(args) < 3 {
		return cliOptions{}, usageError(commands.TaskCleanup)
	}

	options := cliOptions{kind: commandTaskCleanup, slug: args[2]}
	if options.slug == "" || options.slug == "--force" {
		return cliOptions{}, usageError(commands.TaskCleanup)
	}

	for _, arg := range args[3:] {
		switch arg {
		case "--force":
			options.force = true
		case "--plan":
			options.plan = true
		case "--json":
			options.json = true
		default:
			return cliOptions{}, fmt.Errorf("unknown argument for brevity task cleanup: %s", arg)
		}
	}
	if options.plan && options.force {
		return cliOptions{}, fmt.Errorf("brevity task cleanup cannot combine --plan and --force")
	}
	if !options.force && !options.plan {
		return cliOptions{}, fmt.Errorf("brevity task cleanup requires --force")
	}

	return options, nil
}

func parseTaskRunOptions(args []string) (cliOptions, error) {
	if len(args) < 3 {
		return cliOptions{}, usageError(commands.TaskRun)
	}

	options := cliOptions{kind: commandTaskRun, slug: args[2]}
	if options.slug == "" || options.slug == "--execute" {
		return cliOptions{}, usageError(commands.TaskRun)
	}

	for index := 3; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--plan":
			options.dryRun = true
		case "--json":
			options.json = true
		case "--execute":
			options.execute = true
		case "--smoke":
			options.smoke = true
		case "--profile":
			index++
			if index >= len(args) || args[index] == "" || args[index][0] == '-' {
				return cliOptions{}, fmt.Errorf("brevity task run --profile requires a profile name")
			}
			options.profile = args[index]
		default:
			return cliOptions{}, fmt.Errorf("unknown argument for brevity task run: %s", arg)
		}
	}
	if options.execute && options.dryRun {
		return cliOptions{}, fmt.Errorf("brevity task run cannot combine --plan and --execute")
	}
	if !options.execute && !options.dryRun {
		return cliOptions{}, fmt.Errorf("brevity task run requires --plan or --execute")
	}

	return options, nil
}

func parseTaskRuntimeInfoOptions(args []string) (cliOptions, error) {
	if len(args) < 3 || args[2] == "" {
		return cliOptions{}, usageError(commands.TaskRuntimeInfo)
	}
	options := cliOptions{kind: commandTaskRuntimeInfo, slug: args[2]}
	for _, arg := range args[3:] {
		if arg != "--json" {
			return cliOptions{}, usageError(commands.TaskRuntimeInfo)
		}
		options.json = true
	}
	return options, nil
}

func parseTaskDetailOptions(args []string) (cliOptions, error) {
	if len(args) < 3 || args[2] == "" {
		return cliOptions{}, usageError(commands.TaskDetail)
	}
	options := cliOptions{kind: commandTaskDetail, slug: args[2]}
	for _, arg := range args[3:] {
		if arg != "--json" {
			return cliOptions{}, usageError(commands.TaskDetail)
		}
		options.json = true
	}
	return options, nil
}

func parseTaskRunsOptions(args []string) (cliOptions, error) {
	if len(args) >= 3 && (args[2] == "reconcile" || args[2] == "retention" || args[2] == "compact") {
		return parseTaskRunsMaintenanceOptions(args)
	}
	if len(args) < 3 || args[2] == "" {
		return cliOptions{}, usageError(commands.TaskRuns)
	}
	options := cliOptions{kind: commandTaskRuns, slug: args[2]}
	for _, arg := range args[3:] {
		if arg != "--json" {
			return cliOptions{}, usageError(commands.TaskRuns)
		}
		options.json = true
	}
	return options, nil
}

func parseTaskRunsMaintenanceOptions(args []string) (cliOptions, error) {
	if len(args) < 4 {
		return cliOptions{}, fmt.Errorf("brevity task runs %s requires --dry-run", args[2])
	}

	options := cliOptions{dryRun: false}
	switch args[2] {
	case "reconcile":
		options.kind = commandRunsReconcile
	case "retention":
		options.kind = commandRunsRetention
	case "compact":
		options.kind = commandRunsCompact
	default:
		return cliOptions{}, fmt.Errorf("unsupported task runs command %q", args[2])
	}

	for _, arg := range args[3:] {
		if arg == "--dry-run" {
			options.dryRun = true
			continue
		}
		return cliOptions{}, fmt.Errorf("unknown argument for brevity task runs %s: %s", args[2], arg)
	}
	if !options.dryRun {
		return cliOptions{}, fmt.Errorf("brevity task runs %s requires --dry-run", args[2])
	}

	return options, nil
}

func run(stdout io.Writer, args []string) error {
	options, err := parseOptions(args)
	if err != nil {
		return err
	}
	if options.help {
		if options.kind == commandCmux {
			writeCmuxUsage(stdout)
		} else {
			writeUsage(stdout)
		}
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client := runtimeclient.Client(runtimeclient.NewPowerShellClient())
	if options.kind == commandDashboard && options.jsonSource == "native" {
		client = runtimeclient.NewNativeClient("")
	}

	return runWithContextOptions(ctx, stdout, client, options)
}

func runWithClient(stdout io.Writer, client runtimeclient.Client) error {
	return runWithOptions(stdout, client, cliOptions{kind: commandDashboard, refresh: 5 * time.Second})
}

func runWithOptions(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	return runWithContextOptions(context.Background(), stdout, client, options)
}

func runWithContextOptions(ctx context.Context, stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	switch options.kind {
	case commandRuntimeState:
		return routeRuntimeStateCommand(stdout)
	case commandRuntimeStart, commandRuntimeStop, commandRuntimeStatus:
		return routeRuntimeSupervisorCommand(ctx, stdout, options)
	case commandQueueAdd, commandQueueList, commandQueueInspect, commandQueuePlan, commandQueueRemove, commandQueueReserve, commandQueueUnreserve:
		return routeQueueCommand(stdout, options)
	case commandSchedulerPlan, commandSchedulerReserveNext, commandSchedulerPlanExec:
		return routeSchedulerCommand(stdout, options)
	case commandExecutionList, commandExecutionInspect, commandExecutionPlan, commandExecutionMarkReady, commandExecutionMarkPlanned:
		return routeExecutionCommand(stdout, options)
	case commandProviderStatus, commandProviderSet, commandProviderReset:
		return routeProviderCommand(stdout, options)
	case commandInit:
		return runInit(stdout, options)
	case commandTaskStatus:
		return routeTaskStatusCommand(stdout)
	case commandContextRefresh:
		return routeTaskContextCommand(stdout, client, options)
	case commandDoctor:
		return routeDoctorCommand(stdout, options)
	case commandTaskCleanup, commandTaskNew, commandTaskActivate, commandTaskSpec, commandTaskStart, commandTaskRun, commandTaskMerge, commandTaskRuntimeInfo, commandTaskDetail:
		return routeTaskCommand(stdout, client, options)
	case commandTaskPreflight:
		return routeTaskPreflightCommand(stdout, options)
	case commandTaskRuns, commandRunsReconcile, commandRunsRetention, commandRunsCompact:
		return routeTaskRunsCommand(stdout, client, options)
	case commandRunsInspect, commandRunsNativeCompact:
		return routeRunsCommand(stdout, options)
	case commandCleanupInspect, commandCleanupPlan, commandCleanupExecute:
		return routeCleanupCommand(stdout, options)
	case commandSupportMatrix:
		return routeSupportMatrixCommand(stdout, options)
	case commandCmux:
		return routeCmuxCommand(stdout, options)
	default:
		if options.bubble {
			if options.refresh <= 0 {
				options.refresh = 5 * time.Second
			}
			return bubbleteadashboard.RunWithSource(ctx, os.Stdin, stdout, client, options.refresh, options.jsonSource)
		}
		if options.watch {
			if options.refresh <= 0 {
				options.refresh = 5 * time.Second
			}
			return watchDashboard(ctx, stdout, client, watchOptions{
				refresh: options.refresh,
				clear:   !options.noClear,
				input:   os.Stdin,
			})
		}
		return routeDashboardCommand(stdout, client)
	}
}

func routeExecutionCommand(stdout io.Writer, options cliOptions) error {
	store, err := runtimeexecution.NewStore("")
	if err != nil {
		return err
	}
	switch options.kind {
	case commandExecutionList:
		executions, missing, err := store.Load()
		fmt.Fprintln(stdout, "Runtime executions")
		fmt.Fprintf(stdout, "Path: %s\n", store.Path())
		fmt.Fprintln(stdout)
		if err != nil {
			fmt.Fprintf(stdout, "File error: %v\n", err)
			return nil
		}
		if missing || len(executions.Records) == 0 {
			fmt.Fprintln(stdout, "No planned runtime executions.")
			return nil
		}
		fmt.Fprintf(stdout, "Executions: %d\n", len(executions.Records))
		counts := executionStatusCounts(executions.Records)
		if len(counts) > 0 {
			statuses := make([]string, 0, len(counts))
			for status := range counts {
				statuses = append(statuses, status)
			}
			sort.Strings(statuses)
			for _, status := range statuses {
				fmt.Fprintf(stdout, "%s: %d\n", status, counts[status])
			}
		}
		fmt.Fprintln(stdout)
		for _, record := range executions.Records {
			status := strings.ToLower(strings.TrimSpace(record.Status))
			if status != runtimeexecution.StatusPlanned && status != runtimeexecution.StatusReady {
				continue
			}
			fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\n", record.ID, record.QueueItemID, record.Task, record.ReservationID, record.Status, record.CreatedAt)
		}
		return nil
	case commandExecutionInspect:
		inspection := store.Inspect()
		if options.json {
			output, err := json.Marshal(inspection)
			if err != nil {
				return err
			}
			_, err = stdout.Write(append(output, '\n'))
			return err
		}
		renderExecutionInspection(stdout, inspection)
		return nil
	case commandExecutionPlan:
		record, err := store.PlanFromReservation(options.candidateID)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, "Planned runtime execution")
		fmt.Fprintf(stdout, "Path: %s\n", store.Path())
		fmt.Fprintln(stdout)
		fmt.Fprintf(stdout, "id: %s\n", record.ID)
		fmt.Fprintf(stdout, "queueItemId: %s\n", record.QueueItemID)
		fmt.Fprintf(stdout, "task: %s\n", record.Task)
		fmt.Fprintf(stdout, "reservationId: %s\n", record.ReservationID)
		fmt.Fprintf(stdout, "status: %s\n", record.Status)
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Execution plan only: no provider, worker, supervisor, task state, run history, or queue drain was started.")
		return nil
	case commandExecutionMarkReady:
		result, err := store.MarkReady(options.candidateID)
		if err != nil {
			return err
		}
		renderExecutionTransition(stdout, "Marked runtime execution ready", store.Path(), result)
		return nil
	case commandExecutionMarkPlanned:
		result, err := store.MarkPlanned(options.candidateID)
		if err != nil {
			return err
		}
		renderExecutionTransition(stdout, "Marked runtime execution planned", store.Path(), result)
		return nil
	default:
		return fmt.Errorf("unsupported execution command: %s", options.kind)
	}
}

func executionStatusCounts(records []runtimeexecution.Record) map[string]int {
	counts := map[string]int{}
	for _, record := range records {
		status := strings.ToLower(strings.TrimSpace(record.Status))
		if status == "" {
			status = "(missing)"
		}
		counts[status]++
	}
	return counts
}

func renderExecutionTransition(stdout io.Writer, title string, path string, result runtimeexecution.TransitionResult) {
	fmt.Fprintln(stdout, title)
	fmt.Fprintf(stdout, "Path: %s\n", path)
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "id: %s\n", result.ID)
	fmt.Fprintf(stdout, "task: %s\n", result.Task)
	fmt.Fprintf(stdout, "oldStatus: %s\n", result.OldStatus)
	fmt.Fprintf(stdout, "newStatus: %s\n", result.NewStatus)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Execution metadata only: no provider, worker, supervisor, task state, run history, or queue drain was started.")
}

func renderExecutionInspection(stdout io.Writer, inspection runtimeexecution.Inspection) {
	fmt.Fprintln(stdout, "Runtime execution inspection")
	fmt.Fprintf(stdout, "Path: %s\n", inspection.Path)
	fmt.Fprintf(stdout, "File health: %s\n", inspection.State)
	if strings.TrimSpace(inspection.Error) != "" {
		fmt.Fprintf(stdout, "Error: %s\n", inspection.Error)
	}
	fmt.Fprintf(stdout, "Version: %d (supported %d)\n", inspection.Version, inspection.SupportedVersion)
	fmt.Fprintf(stdout, "Executions: %d\n", inspection.TotalExecutions)
	fmt.Fprintln(stdout)
	if len(inspection.CountsByStatus) == 0 {
		fmt.Fprintln(stdout, "Status counts: none")
	} else {
		fmt.Fprintln(stdout, "Status counts:")
		statuses := make([]string, 0, len(inspection.CountsByStatus))
		for status := range inspection.CountsByStatus {
			statuses = append(statuses, status)
		}
		sort.Strings(statuses)
		for _, status := range statuses {
			fmt.Fprintf(stdout, "- %s: %d\n", status, inspection.CountsByStatus[status])
		}
	}
	if len(inspection.DuplicateIDs) > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintf(stdout, "Duplicate execution ids: %s\n", strings.Join(inspection.DuplicateIDs, ", "))
	}
	if len(inspection.InvalidRecords) > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Invalid record warnings:")
		for _, warning := range inspection.InvalidRecords {
			fmt.Fprintf(stdout, "- %s\n", warning)
		}
	}
}

func routeSchedulerCommand(stdout io.Writer, options cliOptions) error {
	store, err := runtimequeue.NewStore("")
	if err != nil {
		return err
	}
	plan := runtimescheduler.Planner{Queue: store}.Plan()
	if options.kind == commandSchedulerReserveNext {
		return reserveNextSchedulerItem(stdout, store, plan)
	}
	if options.kind == commandSchedulerPlanExec {
		return planSchedulerExecution(stdout, plan, options.json)
	}
	if options.json {
		output, err := json.Marshal(plan)
		if err != nil {
			return err
		}
		_, err = stdout.Write(append(output, '\n'))
		return err
	}
	renderSchedulerPlan(stdout, plan)
	return nil
}

type schedulerPlannedExecution struct {
	QueueItemID   string `json:"queueItemId"`
	Task          string `json:"task"`
	ReservationID string `json:"reservationId"`
	ExecutionID   string `json:"executionId"`
	Status        string `json:"status"`
}

func planSchedulerExecution(stdout io.Writer, plan runtimescheduler.Plan, jsonOutput bool) error {
	if plan.FirstReserved == nil {
		if plan.Selected == nil {
			return fmt.Errorf("no selectable scheduler item: %s", fallbackDash(plan.NoSelectionReason))
		}
		return fmt.Errorf("selected queue item is not reserved: %s", plan.Selected.ID)
	}
	store, err := runtimeexecution.NewStore("")
	if err != nil {
		return err
	}
	record, err := store.PlanFromReservation(plan.FirstReserved.ID)
	if err != nil {
		return err
	}
	result := schedulerPlannedExecution{
		QueueItemID:   record.QueueItemID,
		Task:          record.Task,
		ReservationID: record.ReservationID,
		ExecutionID:   record.ID,
		Status:        record.Status,
	}
	if jsonOutput {
		output, err := json.Marshal(result)
		if err != nil {
			return err
		}
		_, err = stdout.Write(append(output, '\n'))
		return err
	}
	fmt.Fprintln(stdout, "Planned scheduler execution")
	fmt.Fprintf(stdout, "Path: %s\n", store.Path())
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "id: %s\n", result.QueueItemID)
	fmt.Fprintf(stdout, "task: %s\n", result.Task)
	fmt.Fprintf(stdout, "reservationId: %s\n", result.ReservationID)
	fmt.Fprintf(stdout, "executionId: %s\n", result.ExecutionID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Execution plan only: no provider, worker, supervisor, task state, run history, or queue drain was started.")
	return nil
}

func reserveNextSchedulerItem(stdout io.Writer, store runtimequeue.Store, plan runtimescheduler.Plan) error {
	if plan.Selected == nil {
		return fmt.Errorf("no selectable scheduler item: %s", fallbackDash(plan.NoSelectionReason))
	}
	item, err := store.Reserve(plan.Selected.ID)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Reserved scheduler queue item")
	fmt.Fprintf(stdout, "Path: %s\n", store.QueuePath())
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "id: %s\n", item.ID)
	fmt.Fprintf(stdout, "task: %s\n", item.Task)
	if item.Reservation != nil {
		fmt.Fprintf(stdout, "reservationId: %s\n", item.Reservation.ReservationID)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Reservation only: no provider, worker, supervisor, task state, run history, or queue drain was started.")
	return nil
}

func routeQueueCommand(stdout io.Writer, options cliOptions) error {
	store, err := runtimequeue.NewStore("")
	if err != nil {
		return err
	}
	switch options.kind {
	case commandQueueAdd:
		item, err := store.Add(options.slug)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, "Queued runtime item")
		fmt.Fprintf(stdout, "Path: %s\n", store.QueuePath())
		fmt.Fprintln(stdout)
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\n", item.ID, item.Task, item.Provider, item.Profile, item.Status)
		return nil
	case commandQueueList:
		queue, missing, err := store.Load()
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, "Runtime queue")
		fmt.Fprintf(stdout, "Path: %s\n", store.QueuePath())
		fmt.Fprintln(stdout)
		if missing || len(queue.Items) == 0 {
			fmt.Fprintln(stdout, "No queued runtime items.")
			return nil
		}
		fmt.Fprintf(stdout, "Items: %d\n", len(queue.Items))
		fmt.Fprintln(stdout)
		for _, item := range queue.Items {
			fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\n", item.ID, item.Task, item.Provider, item.Profile, item.Status, item.CreatedAt)
		}
		return nil
	case commandQueueInspect:
		inspection := store.Inspect()
		if options.json {
			output, err := json.Marshal(inspection)
			if err != nil {
				return err
			}
			_, err = stdout.Write(append(output, '\n'))
			return err
		}
		renderQueueInspection(stdout, inspection)
		return nil
	case commandQueuePlan:
		plan := store.Plan()
		if options.json {
			output, err := json.Marshal(plan)
			if err != nil {
				return err
			}
			_, err = stdout.Write(append(output, '\n'))
			return err
		}
		renderQueuePlan(stdout, plan)
		return nil
	case commandQueueRemove:
		item, err := store.Remove(options.candidateID)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, "Removed runtime queue item")
		fmt.Fprintf(stdout, "Path: %s\n", store.QueuePath())
		fmt.Fprintln(stdout)
		fmt.Fprintf(stdout, "%s\t%s\t%s\n", item.ID, item.Task, item.Status)
		return nil
	case commandQueueReserve:
		item, err := store.Reserve(options.candidateID)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, "Reserved runtime queue item")
		fmt.Fprintf(stdout, "Path: %s\n", store.QueuePath())
		fmt.Fprintln(stdout)
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", item.ID, item.Task, item.Status, reservationLabel(item.Reservation))
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Reservation only: no provider, worker, supervisor, task state, or queue drain was started.")
		return nil
	case commandQueueUnreserve:
		item, err := store.Unreserve(options.candidateID)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, "Unreserved runtime queue item")
		fmt.Fprintf(stdout, "Path: %s\n", store.QueuePath())
		fmt.Fprintln(stdout)
		fmt.Fprintf(stdout, "%s\t%s\t%s\n", item.ID, item.Task, item.Status)
		return nil
	default:
		return fmt.Errorf("unsupported queue command: %s", options.kind)
	}
}

func renderSchedulerPlan(stdout io.Writer, plan runtimescheduler.Plan) {
	fmt.Fprintln(stdout, "SCHEDULER PLAN")
	fmt.Fprintf(stdout, "Queue: %s\n", plan.QueuePath)
	fmt.Fprintf(stdout, "Queue state: %s\n", plan.QueueState)
	if strings.TrimSpace(plan.Error) != "" {
		fmt.Fprintf(stdout, "Error: %s\n", plan.Error)
	}
	fmt.Fprintln(stdout)
	if plan.Selected == nil {
		fmt.Fprintln(stdout, "Selected: none")
		fmt.Fprintf(stdout, "Reason: %s\n", fallbackDash(plan.NoSelectionReason))
	} else {
		fmt.Fprintln(stdout, "Selected:")
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\n", plan.Selected.ID, plan.Selected.Task, plan.Selected.Provider, plan.Selected.Profile, plan.Selected.Status)
		fmt.Fprintf(stdout, "Reason: %s\n", plan.Selected.Reason)
	}
	fmt.Fprintf(stdout, "Reserved items: %d\n", plan.ReservedItemCount)
	if plan.FirstReserved != nil {
		fmt.Fprintf(stdout, "First reserved: %s\t%s\t%s\n", plan.FirstReserved.ID, plan.FirstReserved.Task, plan.FirstReserved.Reason)
	}
	fmt.Fprintf(stdout, "Reservation: %s\n", plan.ReservationEligibility)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Safety checks:")
	for _, check := range plan.SafetyChecks {
		status := "blocked"
		if check.Passed {
			status = "ok"
		}
		fmt.Fprintf(stdout, "- %s: %s (%s)\n", check.Name, status, check.Reason)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Scheduler planning is read-only: no provider, worker, supervisor, task state, run history, queue drain, or reservation was started.")
}

func reservationLabel(reservation *runtimequeue.Reservation) string {
	if reservation == nil {
		return "unreserved"
	}
	return fmt.Sprintf("%s %s", reservation.Owner, reservation.ReservationID)
}

func renderQueuePlan(stdout io.Writer, plan runtimequeue.Plan) {
	fmt.Fprintln(stdout, "QUEUE PLAN")
	fmt.Fprintf(stdout, "Path: %s\n", plan.Path)
	fmt.Fprintf(stdout, "State: %s\n", plan.State)
	if strings.TrimSpace(plan.Error) != "" {
		fmt.Fprintf(stdout, "Error: %s\n", plan.Error)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Runnable:")
	if len(plan.Runnable) == 0 {
		fmt.Fprintln(stdout, "none")
	} else {
		for index, item := range plan.Runnable {
			label := item.Task
			if strings.TrimSpace(label) == "" {
				label = fallbackDash(item.ID)
			}
			fmt.Fprintf(stdout, "%d. %s\n", index+1, label)
			fmt.Fprintf(stdout, "   reason: %s\n", item.Reason)
		}
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Skipped:")
	if len(plan.Skipped) == 0 {
		fmt.Fprintln(stdout, "none")
	} else {
		for _, item := range plan.Skipped {
			label := item.Task
			if strings.TrimSpace(label) == "" {
				label = fallbackDash(item.ID)
			}
			fmt.Fprintf(stdout, "- %s\n", label)
			fmt.Fprintf(stdout, "  reason: %s\n", item.Reason)
		}
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Summary:")
	fmt.Fprintf(stdout, "- runnable: %d\n", plan.Summary.Runnable)
	fmt.Fprintf(stdout, "- skipped: %d\n", plan.Summary.Skipped)
	fmt.Fprintf(stdout, "- reserved: %d\n", plan.Summary.Reserved)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Read-only: plan does not execute providers, spawn workers, start the supervisor, drain the queue, or reserve ownership.")
}

func renderQueueInspection(stdout io.Writer, inspection runtimequeue.Inspection) {
	fmt.Fprintln(stdout, "Runtime queue inspection")
	fmt.Fprintf(stdout, "Path: %s\n", inspection.Path)
	fmt.Fprintf(stdout, "State: %s\n", inspection.State)
	fmt.Fprintf(stdout, "Version: %d (supported: %d)\n", inspection.Version, inspection.SupportedVersion)
	fmt.Fprintf(stdout, "Items: %d\n", inspection.TotalItems)
	fmt.Fprintf(stdout, "Reserved: %d\n", inspection.ReservedItems)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Status counts:")
	if len(inspection.CountsByStatus) == 0 {
		fmt.Fprintln(stdout, "  none")
	} else {
		statuses := make([]string, 0, len(inspection.CountsByStatus))
		for status := range inspection.CountsByStatus {
			statuses = append(statuses, status)
		}
		sort.Strings(statuses)
		for _, status := range statuses {
			fmt.Fprintf(stdout, "  %s: %d\n", status, inspection.CountsByStatus[status])
		}
	}
	fmt.Fprintf(stdout, "Oldest queued item age: %s\n", fallbackDash(inspection.OldestQueuedItemAge))
	fmt.Fprintf(stdout, "Newest queued item age: %s\n", fallbackDash(inspection.NewestQueuedItemAge))
	fmt.Fprintf(stdout, "Duplicate ids: %s\n", joinOrNone(inspection.DuplicateIDs))
	fmt.Fprintf(stdout, "Invalid items: %s\n", joinOrNone(inspection.InvalidItems))
	fmt.Fprintf(stdout, "Invalid reservations: %s\n", joinOrNone(inspection.InvalidReservations))
	if inspection.UnsupportedFutureVersion {
		fmt.Fprintln(stdout, "Warning: unsupported future queue version.")
	}
	if strings.TrimSpace(inspection.Error) != "" {
		fmt.Fprintf(stdout, "Error: %s\n", inspection.Error)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Read-only: inspection does not schedule, execute, start the supervisor, or rewrite the queue file.")
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, "; ")
}

func routeSupportMatrixCommand(stdout io.Writer, options cliOptions) error {
	if options.json {
		return support.WriteJSON(stdout)
	}
	return support.WriteHuman(stdout)
}

func routeTaskPreflightCommand(stdout io.Writer, options cliOptions) error {
	result, err := preflight.Run(preflight.Options{Action: options.preflightAction, Slug: options.slug})
	if err != nil {
		return err
	}
	if options.json {
		output, err := json.Marshal(result)
		if err != nil {
			return err
		}
		_, err = stdout.Write(append(output, '\n'))
		return err
	}
	return preflight.RenderHuman(stdout, result)
}

func routeCleanupCommand(stdout io.Writer, options cliOptions) error {
	switch options.kind {
	case commandCleanupInspect:
		return routeCleanupInspectCommand(stdout, options)
	case commandCleanupPlan:
		return runOrphanCleanupPlan(stdout, options)
	case commandCleanupExecute:
		return runOrphanCleanupExecute(stdout, options)
	default:
		return fmt.Errorf("unsupported cleanup command: %s", options.kind)
	}
}

func routeRunsCommand(stdout io.Writer, options cliOptions) error {
	store, err := state.NewStore("")
	if err != nil {
		return err
	}
	switch options.kind {
	case commandRunsInspect:
		plan, _, err := runmaintenance.BuildRunMaintenancePlan(runmaintenance.RunMaintenanceOptions{Store: store})
		if err != nil {
			return err
		}
		if options.json {
			output, err := json.Marshal(plan)
			if err != nil {
				return err
			}
			_, err = stdout.Write(append(output, '\n'))
			return err
		}
		renderRunMaintenancePlan(stdout, "Run history inspection", plan)
		return nil
	case commandRunsNativeCompact:
		if options.plan {
			plan, _, err := runmaintenance.BuildRunMaintenancePlan(runmaintenance.RunMaintenanceOptions{Store: store})
			if err != nil {
				return err
			}
			if options.json {
				result := runMaintenanceCommandResult("runs compact", true, "info", plan)
				output, err := json.Marshal(result)
				if err != nil {
					return err
				}
				_, err = stdout.Write(append(output, '\n'))
				return err
			}
			renderRunMaintenancePlan(stdout, "Run history compaction plan", plan)
			return nil
		}
		result, runErr := runmaintenance.ExecuteRunCompaction(runmaintenance.RunMaintenanceOptions{Store: store}, options.force)
		if options.json {
			output, err := json.Marshal(result)
			if err != nil {
				return err
			}
			if _, err := stdout.Write(append(output, '\n')); err != nil {
				return err
			}
		} else {
			var plan runmaintenance.RunMaintenancePlan
			_ = json.Unmarshal(result.Payload, &plan)
			renderRunMaintenancePlan(stdout, "Run history compaction", plan)
			fmt.Fprintf(stdout, "success: %t\n", result.Success)
		}
		if runErr != nil {
			return runErr
		}
		if !result.Success {
			return fmt.Errorf("%s reported success=false", result.Command)
		}
		return nil
	default:
		return fmt.Errorf("unsupported runs command: %s", options.kind)
	}
}

func renderRunMaintenancePlan(stdout io.Writer, title string, plan runmaintenance.RunMaintenancePlan) {
	fmt.Fprintln(stdout, title)
	fmt.Fprintf(stdout, "runsPath: %s\n", plan.RunsPath)
	fmt.Fprintf(stdout, "totalRuns: %d\n", plan.TotalRuns)
	fmt.Fprintf(stdout, "validRuns: %d\n", plan.ValidRuns)
	fmt.Fprintf(stdout, "malformedRows: %d\n", len(plan.MalformedRows))
	fmt.Fprintf(stdout, "duplicateRunIds: %d\n", len(plan.DuplicateRunIDs))
	fmt.Fprintf(stdout, "incompleteRuns: %d\n", len(plan.IncompleteRuns))
	fmt.Fprintf(stdout, "staleIncompleteRuns: %d\n", len(plan.StaleIncompleteRuns))
	fmt.Fprintf(stdout, "missingLogReferences: %d\n", len(plan.MissingLogReferences))
	fmt.Fprintf(stdout, "compactableRows: %d\n", len(plan.CompactableRows))
	fmt.Fprintf(stdout, "wouldRewriteRunsFile: %t\n", plan.WouldRewriteRunsFile)
	fmt.Fprintf(stdout, "wouldDeleteLogs: %t\n", plan.WouldDeleteLogs)
	fmt.Fprintf(stdout, "requiresForce: %t\n", plan.RequiresForce)
	if plan.QuarantinePath != "" {
		fmt.Fprintf(stdout, "quarantinePath: %s\n", plan.QuarantinePath)
	}
}

func runMaintenanceCommandResult(command string, success bool, severity string, plan runmaintenance.RunMaintenancePlan) contracts.CommandResult {
	payload, _ := json.Marshal(plan)
	return contracts.CommandResult{
		Schema:               contracts.CommandResultSchema,
		Command:              command,
		Success:              success,
		Severity:             severity,
		Warnings:             plan.Warnings,
		Errors:               []contracts.ResultMessage{},
		SuggestedNextActions: []string{},
		Payload:              payload,
	}
}

func routeCleanupInspectCommand(stdout io.Writer, options cliOptions) error {
	output, err := runtimeclient.NewNativeClient("").CleanupInspectJSON()
	if err != nil {
		return err
	}
	if options.json {
		_, err := stdout.Write(append(output, '\n'))
		return err
	}
	var report nativecleanup.Report
	if err := json.Unmarshal(output, &report); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Cleanup inspection")
	fmt.Fprintf(stdout, "Candidates: %d total, %d removable, %d destructive\n", report.Summary.Total, report.Summary.Removable, report.Summary.Destructive)
	fmt.Fprintln(stdout, "No cleanup executed.")
	if len(report.Warnings) > 0 {
		fmt.Fprintln(stdout)
		for _, warning := range report.Warnings {
			fmt.Fprintf(stdout, "warning: %s\n", warning)
		}
	}
	if len(report.Candidates) == 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "No cleanup candidates found.")
		return nil
	}
	fmt.Fprintln(stdout)
	for _, candidate := range report.Candidates {
		fmt.Fprintf(stdout, "- %s [%s] %s\n", candidate.ID, candidate.Severity, candidate.Kind)
		if candidate.TaskSlug != "" {
			fmt.Fprintf(stdout, "  task: %s\n", candidate.TaskSlug)
		}
		if candidate.Branch != "" {
			fmt.Fprintf(stdout, "  branch: %s\n", candidate.Branch)
		}
		if candidate.WorktreePath != "" {
			fmt.Fprintf(stdout, "  worktree: %s\n", candidate.WorktreePath)
		}
		fmt.Fprintf(stdout, "  dirty: %t removable: %t destructive: %t\n", candidate.Dirty, candidate.Removable, candidate.Destructive)
		fmt.Fprintf(stdout, "  reason: %s\n", candidate.Reason)
		fmt.Fprintf(stdout, "  suggestedAction: %s\n", candidate.SuggestedAction)
	}
	return nil
}

func runOrphanCleanupPlan(stdout io.Writer, options cliOptions) error {
	store, err := state.NewStore("")
	if err != nil {
		return err
	}
	result, runErr := actions.OrphanCleanupService{Store: store}.Plan(options.candidateID, options.all)
	if options.json {
		output, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if _, err := stdout.Write(append(output, '\n')); err != nil {
			return err
		}
	} else if renderErr := actions.RenderOrphanCleanupPlanResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if runErr != nil {
		return runErr
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}
	return nil
}

func runOrphanCleanupExecute(stdout io.Writer, options cliOptions) error {
	store, err := state.NewStore("")
	if err != nil {
		return err
	}
	result, runErr := actions.OrphanCleanupService{Store: store}.Execute(options.candidateID, options.all, options.force)
	if options.json {
		output, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if _, err := stdout.Write(append(output, '\n')); err != nil {
			return err
		}
	} else if renderErr := actions.RenderOrphanCleanupExecutionResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if runErr != nil {
		return runErr
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}
	return nil
}

func routeDashboardCommand(stdout io.Writer, client runtimeclient.Client) error {
	output, err := client.RuntimeStateJSON()
	if err != nil {
		return err
	}
	state, err := contracts.ParseRuntimeState(output)
	if err != nil {
		return err
	}
	dashboard.Render(stdout, state)
	return err
}

func routeRuntimeStateCommand(stdout io.Writer) error {
	output, err := runtimeclient.NewNativeClient("").RuntimeStateJSON()
	if err != nil {
		return err
	}
	_, err = stdout.Write(append(output, '\n'))
	return err
}

func routeRuntimeSupervisorCommand(ctx context.Context, stdout io.Writer, options cliOptions) error {
	if options.kind == commandRuntimeStart && runtimesupervisor.IsChildProcess() {
		store, err := runtimestate.NewStore("")
		if err != nil {
			return err
		}
		return (runtimesupervisor.Supervisor{Store: store}).Run(ctx)
	}
	switch options.kind {
	case commandRuntimeStart:
		state, started, err := runtimesupervisor.Start("")
		if err != nil {
			return err
		}
		if started {
			fmt.Fprintln(stdout, "Runtime supervisor started.")
		} else {
			fmt.Fprintln(stdout, "Runtime supervisor already running.")
		}
		fmt.Fprintf(stdout, "pid: %d\n", state.PID)
		fmt.Fprintf(stdout, "status: %s\n", fallbackDash(state.Status))
		fmt.Fprintf(stdout, "runtimeState: %s\n", mustRuntimeStorePath())
		return nil
	case commandRuntimeStop:
		snapshot, err := runtimesupervisor.Stop("")
		renderRuntimeSupervisorStatus(stdout, snapshot)
		return err
	case commandRuntimeStatus:
		snapshot, err := runtimesupervisor.Status("")
		if err != nil {
			return err
		}
		renderRuntimeSupervisorStatus(stdout, snapshot)
		return nil
	default:
		return fmt.Errorf("unsupported runtime supervisor command: %s", options.kind)
	}
}

func renderRuntimeSupervisorStatus(stdout io.Writer, snapshot runtimestate.Snapshot) {
	fmt.Fprintln(stdout, "Runtime supervisor")
	fmt.Fprintf(stdout, "state: %s\n", snapshot.Interpretation)
	fmt.Fprintf(stdout, "path: %s\n", snapshot.RuntimePath)
	if snapshot.Missing {
		fmt.Fprintln(stdout, "status: missing")
		return
	}
	if snapshot.Corrupted {
		fmt.Fprintln(stdout, "status: corrupted")
		if snapshot.Error != nil {
			fmt.Fprintf(stdout, "error: %v\n", snapshot.Error)
		}
		return
	}
	if snapshot.Error != nil {
		fmt.Fprintln(stdout, "status: invalid")
		fmt.Fprintf(stdout, "error: %v\n", snapshot.Error)
		return
	}
	state := snapshot.State
	fmt.Fprintf(stdout, "status: %s\n", fallbackDash(state.Status))
	fmt.Fprintf(stdout, "pid: %d\n", state.PID)
	fmt.Fprintf(stdout, "pidAlive: %t\n", snapshot.PIDAlive)
	fmt.Fprintf(stdout, "uptime: %s\n", runtimestate.FormatDuration(snapshot.Uptime))
	fmt.Fprintf(stdout, "heartbeatAt: %s\n", fallbackDash(state.HeartbeatAt))
	fmt.Fprintf(stdout, "heartbeatFresh: %t\n", snapshot.HeartbeatFresh)
	fmt.Fprintf(stdout, "heartbeatAge: %s\n", runtimestate.FormatDuration(snapshot.HeartbeatAge))
	fmt.Fprintf(stdout, "activeWorkers: %d\n", state.ActiveWorkers)
	fmt.Fprintf(stdout, "queueDepth: %d\n", state.QueueDepth)
}

func mustRuntimeStorePath() string {
	store, err := runtimestate.NewStore("")
	if err != nil {
		return ".brevity\\runtime.json"
	}
	return store.RuntimePath()
}

type watchOptions struct {
	refresh time.Duration
	clear   bool
	input   io.Reader
}

func watchDashboard(ctx context.Context, stdout io.Writer, client runtimeclient.Client, options watchOptions) error {
	model := dashboard.InteractiveModel{}
	lastItemCount := 0
	inputs := readInputLines(ctx, options.input)
	lastSuccess := ""
	lastRenderKey := ""
	var currentState contracts.RuntimeState
	hasCurrentState := false
	if render, err := renderDashboardRefresh(stdout, client, time.Now, options.clear, lastSuccess, true, model); err == nil {
		lastRenderKey = render.key
		lastSuccess = time.Now().Format(time.RFC3339)
		lastItemCount = render.itemCount
		currentState = render.state
		hasCurrentState = true
	}

	ticker := time.NewTicker(options.refresh)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(stdout, "\nStopped.")
			return nil
		case input, ok := <-inputs:
			if !ok {
				inputs = nil
				continue
			}
			changed, refreshNow, quit := applyDashboardInput(&model, input, lastItemCount)
			if quit {
				fmt.Fprintln(stdout, "\nStopped.")
				return nil
			}
			if !changed && !refreshNow {
				continue
			}
			var render dashboardSnapshot
			var err error
			if refreshNow || !hasCurrentState {
				render, err = renderDashboardSnapshot(client, model)
				if err != nil {
					if options.clear {
						clearScreen(stdout)
					}
					renderDashboardError(stdout, time.Now, lastSuccess, err)
					continue
				}
				currentState = render.state
				hasCurrentState = true
			} else {
				render = renderDashboardState(currentState, model)
			}
			if options.clear {
				clearScreen(stdout)
			}
			fmt.Fprint(stdout, render.body)
			if refreshNow {
				lastSuccess = time.Now().Format(time.RFC3339)
			}
			fmt.Fprintf(stdout, "\nLast successful refresh: %s\n", fallbackRefresh(lastSuccess))
			lastRenderKey = render.key
			lastItemCount = render.itemCount
		case <-ticker.C:
			render, err := renderDashboardSnapshot(client, model)
			if err != nil {
				if options.clear {
					clearScreen(stdout)
				}
				renderDashboardError(stdout, time.Now, lastSuccess, err)
				continue
			}
			if render.key == lastRenderKey {
				lastSuccess = time.Now().Format(time.RFC3339)
				continue
			}
			if options.clear {
				clearScreen(stdout)
			}
			fmt.Fprint(stdout, render.body)
			fmt.Fprintf(stdout, "\nLast successful refresh: %s\n", time.Now().Format(time.RFC3339))
			lastRenderKey = render.key
			lastSuccess = time.Now().Format(time.RFC3339)
			lastItemCount = render.itemCount
			currentState = render.state
			hasCurrentState = true
		}
	}
}

func renderDashboardRefresh(stdout io.Writer, client runtimeclient.Client, now func() time.Time, clear bool, lastSuccess string, showErrors bool, model dashboard.InteractiveModel) (dashboardSnapshot, error) {
	if clear {
		clearScreen(stdout)
	}

	render, err := renderDashboardSnapshot(client, model)
	if err != nil {
		if showErrors {
			renderDashboardError(stdout, now, lastSuccess, err)
		}
		return dashboardSnapshot{}, err
	}

	fmt.Fprint(stdout, render.body)
	if showErrors {
		fmt.Fprintf(stdout, "\nLast successful refresh: %s\n", now().Format(time.RFC3339))
	}
	return render, nil
}

type dashboardSnapshot struct {
	body      string
	key       string
	itemCount int
	state     contracts.RuntimeState
}

func renderDashboardSnapshot(client runtimeclient.Client, model dashboard.InteractiveModel) (dashboardSnapshot, error) {
	output, err := client.RuntimeStateJSON()
	if err != nil {
		return dashboardSnapshot{}, err
	}

	state, err := contracts.ParseRuntimeState(output)
	if err != nil {
		return dashboardSnapshot{}, err
	}

	return renderDashboardState(state, model), nil
}

func renderDashboardState(state contracts.RuntimeState, model dashboard.InteractiveModel) dashboardSnapshot {
	keyState := state
	keyState.GeneratedAt = ""
	items := dashboard.SelectableItems(state)
	model.Clamp(len(items))

	return dashboardSnapshot{
		body:      dashboard.RenderInteractiveString(state, model),
		key:       dashboard.RenderInteractiveString(keyState, model),
		itemCount: len(items),
		state:     state,
	}
}

func renderDashboardError(stdout io.Writer, now func() time.Time, lastSuccess string, err error) {
	fmt.Fprintln(stdout, "Brevity Runtime Dashboard")
	fmt.Fprintln(stdout, "=========================")
	fmt.Fprintf(stdout, "Last successful refresh: %s\n", fallbackRefresh(lastSuccess))
	fmt.Fprintf(stdout, "Refresh attempted: %s\n", now().Format(time.RFC3339))
	fmt.Fprintf(stdout, "Polling error: %v\n", err)
}

func clearScreen(stdout io.Writer) {
	fmt.Fprint(stdout, "\x1b[H\x1b[2J")
}

func fallbackRefresh(value string) string {
	if value == "" {
		return "(none)"
	}
	return value
}

func readInputLines(ctx context.Context, input io.Reader) <-chan string {
	if input == nil {
		return nil
	}
	lines := make(chan string)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(input)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			case lines <- scanner.Text():
			}
		}
	}()
	return lines
}

func applyDashboardInput(model *dashboard.InteractiveModel, input string, itemCount int) (changed bool, refreshNow bool, quit bool) {
	switch normalizeDashboardInput(input) {
	case "q":
		return false, false, true
	case "r":
		return false, true, false
	case "j", "down":
		before := model.SelectedIndex
		model.MoveDown(itemCount)
		return before != model.SelectedIndex, false, false
	case "k", "up":
		before := model.SelectedIndex
		model.MoveUp(itemCount)
		return before != model.SelectedIndex, false, false
	case "d", "enter":
		model.ToggleDetails()
		return true, false, false
	case "?":
		model.ToggleHelp()
		return true, false, false
	default:
		return false, false, false
	}
}

func normalizeDashboardInput(input string) string {
	input = strings.TrimSpace(input)
	switch input {
	case "":
		return "enter"
	case "\x1b[B":
		return "down"
	case "\x1b[A":
		return "up"
	default:
		return strings.ToLower(input)
	}
}

func routeProviderCommand(stdout io.Writer, options cliOptions) error {
	store, err := state.NewStore("")
	if err != nil {
		return err
	}
	service := state.ProviderHealthService{Store: store}
	switch options.kind {
	case commandProviderStatus:
		return renderNativeProviderStatus(stdout, service)
	case commandProviderSet:
		status, err := state.NormalizeProviderStatus(options.status)
		if err != nil {
			return err
		}
		result, runErr := service.Set(options.provider, status, options.note)
		if renderErr := actions.RenderProviderResult(stdout, result); renderErr != nil {
			return renderErr
		}
		if runErr != nil {
			return runErr
		}
		if !result.Success {
			return fmt.Errorf("%s reported success=false", result.Command)
		}
		return nil
	case commandProviderReset:
		result, runErr := service.Reset(options.provider)
		if renderErr := actions.RenderProviderResult(stdout, result); renderErr != nil {
			return renderErr
		}
		if runErr != nil {
			return runErr
		}
		if !result.Success {
			return fmt.Errorf("%s reported success=false", result.Command)
		}
		return nil
	default:
		return fmt.Errorf("unsupported provider action: %s", options.kind)
	}
}

func renderNativeProviderStatus(stdout io.Writer, service state.ProviderHealthService) error {
	health, missing, err := service.List()
	if err != nil {
		return err
	}
	if missing {
		health = state.DefaultProviderHealthState()
	}
	summary := summarizeNativeProviders(health)
	fmt.Fprintln(stdout, "Provider health")
	fmt.Fprintf(stdout, "Path: %s\n", service.Store.Path(state.ProviderHealthFile))
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "Providers: %d total, %d degraded, %d unavailable\n", summary.Total, summary.Degraded, summary.Unavailable)
	fmt.Fprintln(stdout)
	names := make([]string, 0, len(health))
	for provider := range health {
		names = append(names, provider)
	}
	sort.Strings(names)
	for _, provider := range names {
		record := health[provider]
		updatedAt := record.UpdatedAt
		if strings.TrimSpace(updatedAt) == "" {
			updatedAt = "-"
		}
		note := record.Note
		if strings.TrimSpace(note) == "" {
			note = "-"
		}
		status := string(record.Status)
		if strings.TrimSpace(status) == "" {
			status = "unknown"
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", provider, status, updatedAt, note)
	}
	return nil
}

func routeTaskStatusCommand(stdout io.Writer) error {
	store, err := state.NewStore("")
	if err != nil {
		return err
	}
	tasks, missing, err := state.LoadTasks(store)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Task status")
	fmt.Fprintf(stdout, "Path: %s\n", store.Path(state.TasksFile))
	fmt.Fprintln(stdout)
	if missing || len(tasks.Items) == 0 {
		fmt.Fprintln(stdout, "No tasks tracked.")
		return nil
	}
	fmt.Fprintf(stdout, "Tasks: %d tracked\n", len(tasks.Items))
	fmt.Fprintln(stdout)
	for _, task := range tasks.Items {
		summary := task.ToContract()
		status := summary.NormalizedState
		if strings.TrimSpace(status) == "" {
			status = summary.Status
		}
		if strings.TrimSpace(status) == "" {
			status = "unknown"
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", summary.Slug, status, fallbackDash(summary.Branch), fallbackDash(summary.WorktreePath))
	}
	return nil
}

func fallbackDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func summarizeNativeProviders(health state.ProviderHealthState) contracts.ProviderSummary {
	summary := contracts.ProviderSummary{Total: len(health)}
	for _, provider := range health {
		switch strings.ToLower(strings.TrimSpace(string(provider.Status))) {
		case "capacity-degraded":
			summary.Degraded++
		case "unavailable":
			summary.Unavailable++
		}
	}
	return summary
}

func routeTaskCommand(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	switch options.kind {
	case commandTaskCleanup:
		return runTaskCleanup(stdout, client, options)
	case commandTaskNew:
		return runTaskNew(stdout, client, options)
	case commandTaskActivate:
		return runTaskActivate(stdout, options)
	case commandTaskSpec:
		return runTaskSpec(stdout, options)
	case commandTaskStart:
		return runTaskStart(stdout, options)
	case commandTaskRun:
		return runTaskRun(stdout, client, options)
	case commandTaskMerge:
		return runTaskMerge(stdout, options)
	case commandTaskRuntimeInfo, commandTaskDetail:
		return runTaskRuntimeInfo(stdout, client, options)
	default:
		return fmt.Errorf("unsupported task command: %s", options.kind)
	}
}

func runInit(stdout io.Writer, options cliOptions) error {
	store, err := state.NewStore("")
	if err != nil {
		return err
	}
	result, runErr := actions.InitService{Store: store, Repair: options.repair}.Run()
	if options.json {
		output, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if _, err := stdout.Write(append(output, '\n')); err != nil {
			return err
		}
	} else if renderErr := actions.RenderInitResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if runErr != nil {
		return runErr
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}
	return nil
}

func runTaskMerge(stdout io.Writer, options cliOptions) error {
	store, err := state.NewStore("")
	if err != nil {
		return err
	}
	service := actions.TaskMergeService{Store: store}
	var result contracts.CommandResult
	var runErr error
	if options.plan {
		result, runErr = service.Plan(options.slug)
	} else {
		result, runErr = service.Merge(options.slug)
	}
	if options.json {
		output, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if _, err := stdout.Write(append(output, '\n')); err != nil {
			return err
		}
	} else if options.plan {
		if renderErr := actions.RenderTaskMergePlanResult(stdout, result); renderErr != nil {
			return renderErr
		}
	} else if renderErr := actions.RenderTaskMergeResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if runErr != nil {
		return runErr
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}
	return nil
}

func routeTaskContextCommand(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	return runTaskContextRefresh(stdout, client, options)
}

func routeTaskRunsCommand(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	switch options.kind {
	case commandTaskRuns:
		return runTaskRuns(stdout, client, options)
	case commandRunsReconcile, commandRunsRetention, commandRunsCompact:
		return runTaskRunsMaintenance(stdout, client, options)
	default:
		return fmt.Errorf("unsupported task runs command: %s", options.kind)
	}
}

func routeDoctorCommand(stdout io.Writer, options cliOptions) error {
	return runDoctor(stdout, options)
}

func runTaskRunsMaintenance(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	if !options.dryRun {
		return fmt.Errorf("brevity %s requires --dry-run", maintenanceCommandName(options.kind))
	}

	var spec actionSpec
	switch options.kind {
	case commandRunsReconcile:
		spec = actionSpec{
			call:   client.TaskRunsReconcileJSON,
			render: actions.RenderTaskRunsReconcileResult,
		}
	case commandRunsRetention:
		spec = actionSpec{
			call:   client.TaskRunsRetentionJSON,
			render: actions.RenderTaskRunsRetentionResult,
		}
	case commandRunsCompact:
		spec = actionSpec{
			call:   client.TaskRunsCompactJSON,
			render: actions.RenderTaskRunsCompactResult,
		}
	default:
		return fmt.Errorf("unsupported task runs maintenance command: %s", options.kind)
	}

	return runPowerShellAction(stdout, spec)
}

func runPowerShellAction(stdout io.Writer, spec actionSpec) error {
	output, err := spec.call()
	result, parseErr := contracts.ParseCommandResult(output)
	if parseErr != nil {
		if err != nil {
			return fmt.Errorf("%v; additionally failed to parse command result: %w", err, parseErr)
		}
		return parseErr
	}

	if renderErr := spec.render(stdout, result); renderErr != nil {
		return renderErr
	}
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}
	if spec.check != nil {
		return spec.check(result)
	}

	return nil
}

func maintenanceCommandName(kind commandKind) string {
	switch kind {
	case commandRunsReconcile:
		return commands.TaskRunsReconcile.Name()
	case commandRunsRetention:
		return commands.TaskRunsRetention.Name()
	case commandRunsCompact:
		return commands.TaskRunsCompact.Name()
	default:
		return string(kind)
	}
}

func runDoctor(stdout io.Writer, options cliOptions) error {
	report, err := diagnostics.Run(diagnostics.Options{})
	if err != nil {
		return err
	}
	result := diagnostics.CommandResult(report)
	if options.json {
		output, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if _, err := stdout.Write(append(output, '\n')); err != nil {
			return err
		}
	} else if err := actions.RenderDoctorResult(stdout, result); err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("doctor reported error diagnostics")
	}
	return nil
}

func runTaskNew(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	store, err := state.NewStore("")
	if err != nil {
		return err
	}
	result, runErr := actions.TaskNewService{Store: store}.Create(options.slug)
	if options.json {
		output, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if _, err := stdout.Write(append(output, '\n')); err != nil {
			return err
		}
	} else if renderErr := actions.RenderTaskNewResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if runErr != nil {
		return runErr
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}
	return nil
}

func runTaskActivate(stdout io.Writer, options cliOptions) error {
	store, err := state.NewStore("")
	if err != nil {
		return err
	}
	result, runErr := actions.TaskActivateService{Store: store}.Activate(options.slug)
	if options.json {
		output, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if _, err := stdout.Write(append(output, '\n')); err != nil {
			return err
		}
	} else if renderErr := actions.RenderTaskActivateResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if runErr != nil {
		return runErr
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}
	return nil
}

func runTaskSpec(stdout io.Writer, options cliOptions) error {
	store, err := state.NewStore("")
	if err != nil {
		return err
	}
	result, runErr := actions.TaskSpecService{Store: store}.Show(options.slug)
	if options.json {
		output, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if _, err := stdout.Write(append(output, '\n')); err != nil {
			return err
		}
	} else if renderErr := actions.RenderTaskSpecResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if runErr != nil {
		return runErr
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}
	return nil
}

func runTaskStart(stdout io.Writer, options cliOptions) error {
	store, err := state.NewStore("")
	if err != nil {
		return err
	}
	service := actions.TaskStartService{Store: store}
	result, runErr := service.Start(options.slug)
	if options.json {
		output, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if _, err := stdout.Write(append(output, '\n')); err != nil {
			return err
		}
	} else if renderErr := actions.RenderTaskStartResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if runErr != nil {
		return runErr
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}
	return nil
}

func runTaskRun(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	if options.dryRun {
		return runTaskRunPlan(stdout, options)
	}
	if !options.execute {
		return fmt.Errorf("brevity task run requires --plan or --execute")
	}

	output, err := runtimeclient.NewNativeClient("").TaskRunJSON(options.slug, options.profile, options.smoke)
	result, parseErr := contracts.ParseCommandResult(output)
	if parseErr != nil {
		if err != nil {
			return fmt.Errorf("%v; additionally failed to parse command result: %w", err, parseErr)
		}
		return parseErr
	}
	if options.json {
		if _, writeErr := stdout.Write(append(output, '\n')); writeErr != nil {
			return writeErr
		}
	} else if renderErr := actions.RenderTaskRunResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}
	if isWorkerFailure(result) {
		return fmt.Errorf("%s worker failed", result.Command)
	}
	return nil
}

func runTaskRunPlan(stdout io.Writer, options cliOptions) error {
	output, err := runtimeclient.NewNativeClient("").TaskRunPlanJSON(options.slug, options.profile)
	if err != nil {
		return err
	}
	if options.json {
		_, writeErr := stdout.Write(append(output, '\n'))
		return writeErr
	}
	result, parseErr := contracts.ParseCommandResult(output)
	if parseErr != nil {
		return parseErr
	}
	if renderErr := actions.RenderTaskRunPlanResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if !result.Success {
		return fmt.Errorf("%s plan reported success=false", result.Command)
	}
	return nil
}

func runTaskRuntimeInfo(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	return runNativeInspection(stdout, options, func(native runtimeclient.NativeClient) ([]byte, error) {
		return native.TaskRuntimeInfoJSON(options.slug)
	}, actions.RenderTaskRuntimeInfoResult)
}

func isWorkerFailure(result contracts.CommandResult) bool {
	payload, err := contracts.ParseTaskRunExecutionPayload(result)
	if err != nil {
		return false
	}
	if payload.WorkerStatus == "failed" {
		return true
	}
	if payload.ExitCode == nil {
		return false
	}
	exitCode := fmt.Sprint(payload.ExitCode)
	return exitCode != "" && exitCode != "0"
}

func runTaskRuns(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	return runNativeInspection(stdout, options, func(native runtimeclient.NativeClient) ([]byte, error) {
		return native.TaskRunsJSON(options.slug)
	}, actions.RenderTaskRunsResult)
}

func runNativeInspection(stdout io.Writer, options cliOptions, call func(runtimeclient.NativeClient) ([]byte, error), render actionRenderer) error {
	output, err := call(runtimeclient.NewNativeClient(""))
	if err != nil {
		return err
	}
	if options.json {
		_, writeErr := stdout.Write(append(output, '\n'))
		return writeErr
	}
	result, parseErr := contracts.ParseCommandResult(output)
	if parseErr != nil {
		return parseErr
	}
	if renderErr := render(stdout, result); renderErr != nil {
		return renderErr
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}
	return nil
}

func runTaskCleanup(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	store, err := state.NewStore("")
	if err != nil {
		return err
	}
	service := actions.TaskCleanupService{Store: store}
	var result contracts.CommandResult
	var runErr error
	if options.plan {
		result, runErr = service.Plan(options.slug)
	} else {
		result, runErr = service.Cleanup(options.slug, options.force)
	}
	if options.json {
		output, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if _, err := stdout.Write(append(output, '\n')); err != nil {
			return err
		}
	} else if options.plan {
		if renderErr := actions.RenderTaskCleanupPlanResult(stdout, result); renderErr != nil {
			return renderErr
		}
	} else if renderErr := actions.RenderTaskCleanupResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if runErr != nil {
		return runErr
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}
	return nil
}

func runTaskContextRefresh(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	store, err := state.NewStore("")
	if err != nil {
		return err
	}
	service := actions.TaskContextRefreshService{Store: store}
	result, runErr := service.Refresh(options.slug)
	if options.json {
		output, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if _, err := stdout.Write(append(output, '\n')); err != nil {
			return err
		}
	} else if renderErr := actions.RenderTaskContextRefreshResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if runErr != nil {
		return runErr
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}
	return nil
}

func writeUsage(stdout io.Writer) {
	fmt.Fprintln(stdout, "Brevity Go Dashboard")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Usage:")
	for _, command := range commands.UsageCommands {
		fmt.Fprintf(stdout, "  %s\n", command.Usage)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "The dashboard remains read-only. Mutating actions are dispatched")
	fmt.Fprintln(stdout, "through native Go where implemented; PowerShell remains legacy fallback.")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Flags:")
	fmt.Fprintln(stdout, "  --once                    Render the dashboard once.")
	fmt.Fprintln(stdout, "  --watch                   Refresh the dashboard until interrupted.")
	fmt.Fprintln(stdout, "  --bubble                  Run the experimental Bubble Tea dashboard.")
	fmt.Fprintln(stdout, "  --refresh <duration>      Set the dashboard refresh interval.")
	fmt.Fprintln(stdout, "  --json-source <source>    Runtime JSON source: powershell or native.")
	fmt.Fprintln(stdout, "  --no-clear                Do not clear before changed dashboard renders.")
	fmt.Fprintln(stdout, "  -h, --help                Show this help text.")
}

// writeCmuxUsage writes the brevity cmux subcommand help to stdout.
func writeCmuxUsage(stdout io.Writer) {
	fmt.Fprintln(stdout, "brevity cmux  [read-only]")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Render a one-shot CMUX operator report from the native Brevity runtime.")
	fmt.Fprintln(stdout, "Exits after a single render.  No watch mode, no terminal clearing,")
	fmt.Fprintln(stdout, "no keyboard input.  Safe in remote sessions, CI pipelines, and AI contexts.")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Usage:")
	fmt.Fprintln(stdout, "  brevity cmux [flags]")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Flags:")
	fmt.Fprintln(stdout, "  --limit <n>               Maximum tasks in the task list (default: 10).")
	fmt.Fprintln(stdout, "  --section <name>          Section to render (default: all).")
	fmt.Fprintln(stdout, "                            Allowed: all, providers, tasks, queue, actions.")
	fmt.Fprintln(stdout, "  --task <slug>             Show only the task with this exact slug.")
	fmt.Fprintln(stdout, "  --state <state>           Show only tasks with this normalized state")
	fmt.Fprintln(stdout, "                            (case-insensitive).")
	fmt.Fprintln(stdout, "  --output <mode>           Output format (default: text).")
	fmt.Fprintln(stdout, "                            Allowed: text, markdown, json.")
	fmt.Fprintln(stdout, "  --review <slug>           Generate a focused review packet for this task.")
	fmt.Fprintln(stdout, "                            Overrides --section and --task; --output applies.")
	fmt.Fprintln(stdout, "  --handoff                 Generate an AI/operator handoff packet.")
	fmt.Fprintln(stdout, "                            Overrides --section, --task, --state.")
	fmt.Fprintln(stdout, "                            --limit and --output still apply.")
	fmt.Fprintln(stdout, "  --merge-report            Generate a merge readiness report.")
	fmt.Fprintln(stdout, "                            Groups tasks: ready-for-merge, reviewing,")
	fmt.Fprintln(stdout, "                            needs-run, blocked, merged, other.")
	fmt.Fprintln(stdout, "                            Overrides --section, --task, --state.")
	fmt.Fprintln(stdout, "                            --limit and --output still apply.")
	fmt.Fprintln(stdout, "  --blocked-report          Generate a blocked task report.")
	fmt.Fprintln(stdout, "                            Groups: provider-gated, blocked,")
	fmt.Fprintln(stdout, "                            reserved-or-queue-gated, unknown.")
	fmt.Fprintln(stdout, "                            Overrides --section, --task, --state.")
	fmt.Fprintln(stdout, "                            --limit and --output still apply.")
	fmt.Fprintln(stdout, "  -h, --help                Show this help.")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Output modes:")
	fmt.Fprintln(stdout, "  text       Terminal operator report.  Plain text, no ANSI, pipe-safe.")
	fmt.Fprintln(stdout, "  markdown   AI/human-readable report.  GitHub-Flavoured Markdown.")
	fmt.Fprintln(stdout, "  json       Machine-readable report.   Schema: brevity.cmux-report.v1.")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Examples:")
	fmt.Fprintln(stdout, "  brevity cmux")
	fmt.Fprintln(stdout, "  brevity cmux --section tasks")
	fmt.Fprintln(stdout, "  brevity cmux --section tasks --state reviewing")
	fmt.Fprintln(stdout, "  brevity cmux --task my-task")
	fmt.Fprintln(stdout, "  brevity cmux --output markdown")
	fmt.Fprintln(stdout, "  brevity cmux --output json --section queue")
	fmt.Fprintln(stdout, "  brevity cmux --limit 20")
	fmt.Fprintln(stdout, "  brevity cmux --review my-task")
	fmt.Fprintln(stdout, "  brevity cmux --review my-task --output markdown")
	fmt.Fprintln(stdout, "  brevity cmux --review my-task --output json")
	fmt.Fprintln(stdout, "  brevity cmux --handoff")
	fmt.Fprintln(stdout, "  brevity cmux --handoff --output markdown")
	fmt.Fprintln(stdout, "  brevity cmux --handoff --output json")
	fmt.Fprintln(stdout, "  brevity cmux --handoff --limit 20")
	fmt.Fprintln(stdout, "  brevity cmux --merge-report")
	fmt.Fprintln(stdout, "  brevity cmux --merge-report --output markdown")
	fmt.Fprintln(stdout, "  brevity cmux --merge-report --output json")
	fmt.Fprintln(stdout, "  brevity cmux --blocked-report")
	fmt.Fprintln(stdout, "  brevity cmux --blocked-report --output markdown")
	fmt.Fprintln(stdout, "  brevity cmux --blocked-report --output json")
}
