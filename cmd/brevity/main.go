package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/mortenlein/brevity/internal/actions"
	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/dashboard"
	"github.com/mortenlein/brevity/internal/runtimeclient"
)

func main() {
	if err := run(os.Stdout, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "brevity-go:", err)
		os.Exit(1)
	}
}

type commandKind string

const (
	commandDashboard       commandKind = "dashboard"
	commandProviderSet     commandKind = "provider-set"
	commandProviderReset   commandKind = "provider-reset"
	commandContextRefresh  commandKind = "context-refresh"
	commandDoctor          commandKind = "doctor"
	commandTaskCleanup     commandKind = "task-cleanup"
	commandTaskNew         commandKind = "task-new"
	commandTaskRun         commandKind = "task-run"
	commandTaskRuntimeInfo commandKind = "task-runtime-info"
	commandTaskRuns        commandKind = "task-runs"
	commandRunsReconcile   commandKind = "task-runs-reconcile"
	commandRunsRetention   commandKind = "task-runs-retention"
	commandRunsCompact     commandKind = "task-runs-compact"
)

type cliOptions struct {
	help     bool
	kind     commandKind
	once     bool
	provider string
	status   string
	slug     string
	force    bool
	execute  bool
	dryRun   bool
	profile  string
	smoke    bool
}

type actionCall func() ([]byte, error)
type actionRenderer func(io.Writer, contracts.CommandResult) error
type actionCheck func(contracts.CommandResult) error

type actionSpec struct {
	call   actionCall
	render actionRenderer
	check  actionCheck
}

func parseOptions(args []string) (cliOptions, error) {
	options := cliOptions{kind: commandDashboard}
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			options.help = true
			return options, nil
		}
	}

	if len(args) > 0 && args[0] == "provider" {
		return parseProviderOptions(args)
	}
	if len(args) > 0 && args[0] == "doctor" {
		return parseDoctorOptions(args)
	}
	if len(args) > 0 && args[0] == "task" {
		return parseTaskOptions(args)
	}

	flags := flag.NewFlagSet("brevity", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.BoolVar(&options.once, "once", false, "render the dashboard once")
	jsonSource := flags.String("json-source", "powershell", "runtime JSON source")

	if err := flags.Parse(args); err != nil {
		return cliOptions{}, err
	}
	if *jsonSource != "powershell" {
		return cliOptions{}, fmt.Errorf("unsupported json source: %s", *jsonSource)
	}
	if flags.NArg() > 0 {
		return cliOptions{}, fmt.Errorf("unexpected argument: %s", flags.Arg(0))
	}

	return options, nil
}

func parseDoctorOptions(args []string) (cliOptions, error) {
	if len(args) != 1 {
		return cliOptions{}, fmt.Errorf("usage: brevity doctor")
	}

	return cliOptions{kind: commandDoctor}, nil
}

func parseProviderOptions(args []string) (cliOptions, error) {
	if len(args) < 2 {
		return cliOptions{}, fmt.Errorf("missing provider command: supported commands: set, reset")
	}

	options := cliOptions{kind: commandDashboard}
	switch args[1] {
	case "set":
		if len(args) != 4 {
			return cliOptions{}, fmt.Errorf("usage: brevity provider set <provider> <status>")
		}
		options.kind = commandProviderSet
		options.provider = args[2]
		options.status = args[3]
	case "reset":
		if len(args) != 3 {
			return cliOptions{}, fmt.Errorf("usage: brevity provider reset <provider>")
		}
		options.kind = commandProviderReset
		options.provider = args[2]
	default:
		return cliOptions{}, fmt.Errorf("unsupported provider command %q: supported commands: set, reset", args[1])
	}

	return options, nil
}

func parseTaskOptions(args []string) (cliOptions, error) {
	if len(args) < 2 {
		return cliOptions{}, fmt.Errorf("missing task command: supported commands: context refresh, runtime-info, runs, new, run, cleanup")
	}
	if args[1] == "context" {
		if len(args) != 4 || args[2] != "refresh" {
			return cliOptions{}, fmt.Errorf("usage: brevity task context refresh <slug>")
		}
		if args[3] == "" {
			return cliOptions{}, fmt.Errorf("usage: brevity task context refresh <slug>")
		}

		return cliOptions{kind: commandContextRefresh, slug: args[3]}, nil
	}
	if args[1] == "cleanup" {
		return parseTaskCleanupOptions(args)
	}
	if args[1] == "new" {
		return parseTaskNewOptions(args)
	}
	if args[1] == "run" {
		return parseTaskRunOptions(args)
	}
	if args[1] == "runtime-info" {
		return parseTaskRuntimeInfoOptions(args)
	}
	if args[1] == "runs" {
		return parseTaskRunsOptions(args)
	}

	return cliOptions{}, fmt.Errorf("unsupported task command %q: supported commands: context refresh, runtime-info, runs, new, run, cleanup", args[1])
}

func parseTaskNewOptions(args []string) (cliOptions, error) {
	if len(args) != 3 || args[2] == "" {
		return cliOptions{}, fmt.Errorf("usage: brevity task new <slug>")
	}

	return cliOptions{kind: commandTaskNew, slug: args[2]}, nil
}

func parseTaskCleanupOptions(args []string) (cliOptions, error) {
	if len(args) < 3 {
		return cliOptions{}, fmt.Errorf("usage: brevity task cleanup <slug> --force")
	}

	options := cliOptions{kind: commandTaskCleanup, slug: args[2]}
	if options.slug == "" || options.slug == "--force" {
		return cliOptions{}, fmt.Errorf("usage: brevity task cleanup <slug> --force")
	}

	for _, arg := range args[3:] {
		if arg == "--force" {
			options.force = true
			continue
		}
		return cliOptions{}, fmt.Errorf("unknown argument for brevity task cleanup: %s", arg)
	}
	if !options.force {
		return cliOptions{}, fmt.Errorf("brevity task cleanup requires --force")
	}

	return options, nil
}

func parseTaskRunOptions(args []string) (cliOptions, error) {
	if len(args) < 3 {
		return cliOptions{}, fmt.Errorf("usage: brevity task run <slug> --execute [--profile <profile>] [--smoke]")
	}

	options := cliOptions{kind: commandTaskRun, slug: args[2]}
	if options.slug == "" || options.slug == "--execute" {
		return cliOptions{}, fmt.Errorf("usage: brevity task run <slug> --execute [--profile <profile>] [--smoke]")
	}

	for index := 3; index < len(args); index++ {
		arg := args[index]
		switch arg {
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
	if !options.execute {
		return cliOptions{}, fmt.Errorf("brevity task run requires --execute")
	}

	return options, nil
}

func parseTaskRuntimeInfoOptions(args []string) (cliOptions, error) {
	if len(args) != 3 || args[2] == "" {
		return cliOptions{}, fmt.Errorf("usage: brevity task runtime-info <slug>")
	}

	return cliOptions{kind: commandTaskRuntimeInfo, slug: args[2]}, nil
}

func parseTaskRunsOptions(args []string) (cliOptions, error) {
	if len(args) >= 3 && (args[2] == "reconcile" || args[2] == "retention" || args[2] == "compact") {
		return parseTaskRunsMaintenanceOptions(args)
	}
	if len(args) != 3 || args[2] == "" {
		return cliOptions{}, fmt.Errorf("usage: brevity task runs <slug>")
	}

	return cliOptions{kind: commandTaskRuns, slug: args[2]}, nil
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
		writeUsage(stdout)
		return nil
	}

	return runWithOptions(stdout, runtimeclient.NewPowerShellClient(), options)
}

func runWithClient(stdout io.Writer, client runtimeclient.Client) error {
	return runWithOptions(stdout, client, cliOptions{kind: commandDashboard})
}

func runWithOptions(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	switch options.kind {
	case commandProviderSet, commandProviderReset:
		return routeProviderCommand(stdout, client, options)
	case commandContextRefresh:
		return routeTaskContextCommand(stdout, client, options)
	case commandDoctor:
		return routeDoctorCommand(stdout, client)
	case commandTaskCleanup, commandTaskNew, commandTaskRun, commandTaskRuntimeInfo:
		return routeTaskCommand(stdout, client, options)
	case commandTaskRuns, commandRunsReconcile, commandRunsRetention, commandRunsCompact:
		return routeTaskRunsCommand(stdout, client, options)
	default:
		return routeDashboardCommand(stdout, client)
	}
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
	return nil
}

func routeProviderCommand(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	return runProviderAction(stdout, client, options)
}

func routeTaskCommand(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	switch options.kind {
	case commandTaskCleanup:
		return runTaskCleanup(stdout, client, options)
	case commandTaskNew:
		return runTaskNew(stdout, client, options)
	case commandTaskRun:
		return runTaskRun(stdout, client, options)
	case commandTaskRuntimeInfo:
		return runTaskRuntimeInfo(stdout, client, options)
	default:
		return fmt.Errorf("unsupported task command: %s", options.kind)
	}
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

func routeDoctorCommand(stdout io.Writer, client runtimeclient.Client) error {
	return runDoctor(stdout, client)
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
		return "task runs reconcile"
	case commandRunsRetention:
		return "task runs retention"
	case commandRunsCompact:
		return "task runs compact"
	default:
		return string(kind)
	}
}

func runDoctor(stdout io.Writer, client runtimeclient.Client) error {
	return runPowerShellAction(stdout, actionSpec{
		call:   client.DoctorJSON,
		render: actions.RenderDoctorResult,
	})
}

func runTaskNew(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	return runPowerShellAction(stdout, actionSpec{
		call: func() ([]byte, error) {
			return client.TaskNewJSON(options.slug)
		},
		render: actions.RenderTaskNewResult,
	})
}

func runTaskRun(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	if !options.execute {
		return fmt.Errorf("brevity task run requires --execute")
	}

	return runPowerShellAction(stdout, actionSpec{
		call: func() ([]byte, error) {
			return client.TaskRunJSON(options.slug, options.profile, options.smoke)
		},
		render: actions.RenderTaskRunResult,
		check: func(result contracts.CommandResult) error {
			if isWorkerFailure(result) {
				return fmt.Errorf("%s worker failed", result.Command)
			}
			return nil
		},
	})
}

func runTaskRuntimeInfo(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	return runPowerShellAction(stdout, actionSpec{
		call: func() ([]byte, error) {
			return client.TaskRuntimeInfoJSON(options.slug)
		},
		render: actions.RenderTaskRuntimeInfoResult,
	})
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
	return runPowerShellAction(stdout, actionSpec{
		call: func() ([]byte, error) {
			return client.TaskRunsJSON(options.slug)
		},
		render: actions.RenderTaskRunsResult,
	})
}

func runTaskCleanup(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	if !options.force {
		return fmt.Errorf("brevity task cleanup requires --force")
	}

	return runPowerShellAction(stdout, actionSpec{
		call: func() ([]byte, error) {
			return client.TaskCleanupJSON(options.slug)
		},
		render: actions.RenderTaskCleanupResult,
	})
}

func runTaskContextRefresh(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	return runPowerShellAction(stdout, actionSpec{
		call: func() ([]byte, error) {
			return client.TaskContextRefreshJSON(options.slug)
		},
		render: actions.RenderTaskContextRefreshResult,
	})
}

func runProviderAction(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	var spec actionSpec
	switch options.kind {
	case commandProviderSet:
		spec = actionSpec{
			call: func() ([]byte, error) {
				return client.ProviderSetJSON(options.provider, options.status)
			},
			render: actions.RenderProviderResult,
		}
	case commandProviderReset:
		spec = actionSpec{
			call: func() ([]byte, error) {
				return client.ProviderResetJSON(options.provider)
			},
			render: actions.RenderProviderResult,
		}
	default:
		return fmt.Errorf("unsupported provider action: %s", options.kind)
	}

	return runPowerShellAction(stdout, spec)
}

func writeUsage(stdout io.Writer) {
	fmt.Fprintln(stdout, "Brevity Go Dashboard")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Usage:")
	fmt.Fprintln(stdout, "  brevity [--once]")
	fmt.Fprintln(stdout, "  brevity doctor")
	fmt.Fprintln(stdout, "  brevity provider set <provider> <status>")
	fmt.Fprintln(stdout, "  brevity provider reset <provider>")
	fmt.Fprintln(stdout, "  brevity task context refresh <slug>")
	fmt.Fprintln(stdout, "  brevity task new <slug>")
	fmt.Fprintln(stdout, "  brevity task run <slug> --execute [--profile <profile>] [--smoke]")
	fmt.Fprintln(stdout, "  brevity task runtime-info <slug>")
	fmt.Fprintln(stdout, "  brevity task runs <slug>")
	fmt.Fprintln(stdout, "  brevity task runs reconcile --dry-run")
	fmt.Fprintln(stdout, "  brevity task runs retention --dry-run")
	fmt.Fprintln(stdout, "  brevity task runs compact --dry-run")
	fmt.Fprintln(stdout, "  brevity task cleanup <slug> --force")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "The dashboard remains read-only. Mutating actions are dispatched")
	fmt.Fprintln(stdout, `through .\brevity.ps1 ... --json command-result contracts.`)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Flags:")
	fmt.Fprintln(stdout, "  --once                    Render the dashboard once.")
	fmt.Fprintln(stdout, "  -h, --help                Show this help text.")
}
