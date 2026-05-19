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
	commandTaskRuntimeInfo commandKind = "task-runtime-info"
	commandTaskRuns        commandKind = "task-runs"
)

type cliOptions struct {
	help     bool
	kind     commandKind
	once     bool
	provider string
	status   string
	slug     string
	force    bool
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

	if err := flags.Parse(args); err != nil {
		return cliOptions{}, err
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
		return cliOptions{}, fmt.Errorf("missing task command: supported commands: context refresh, runtime-info, runs, new, cleanup")
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
	if args[1] == "runtime-info" {
		return parseTaskRuntimeInfoOptions(args)
	}
	if args[1] == "runs" {
		return parseTaskRunsOptions(args)
	}

	return cliOptions{}, fmt.Errorf("unsupported task command %q: supported commands: context refresh, runtime-info, runs, new, cleanup", args[1])
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

func parseTaskRuntimeInfoOptions(args []string) (cliOptions, error) {
	if len(args) != 3 || args[2] == "" {
		return cliOptions{}, fmt.Errorf("usage: brevity task runtime-info <slug>")
	}

	return cliOptions{kind: commandTaskRuntimeInfo, slug: args[2]}, nil
}

func parseTaskRunsOptions(args []string) (cliOptions, error) {
	if len(args) != 3 || args[2] == "" {
		return cliOptions{}, fmt.Errorf("usage: brevity task runs <slug>")
	}

	return cliOptions{kind: commandTaskRuns, slug: args[2]}, nil
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
	if options.kind == commandProviderSet || options.kind == commandProviderReset {
		return runProviderAction(stdout, client, options)
	}
	if options.kind == commandContextRefresh {
		return runTaskContextRefresh(stdout, client, options)
	}
	if options.kind == commandDoctor {
		return runDoctor(stdout, client)
	}
	if options.kind == commandTaskCleanup {
		return runTaskCleanup(stdout, client, options)
	}
	if options.kind == commandTaskNew {
		return runTaskNew(stdout, client, options)
	}
	if options.kind == commandTaskRuntimeInfo {
		return runTaskRuntimeInfo(stdout, client, options)
	}
	if options.kind == commandTaskRuns {
		return runTaskRuns(stdout, client, options)
	}

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

func runDoctor(stdout io.Writer, client runtimeclient.Client) error {
	output, err := client.DoctorJSON()
	result, parseErr := contracts.ParseCommandResult(output)
	if parseErr != nil {
		if err != nil {
			return fmt.Errorf("%v; additionally failed to parse command result: %w", err, parseErr)
		}
		return parseErr
	}

	if renderErr := actions.RenderDoctorResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}

	return nil
}

func runTaskNew(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	output, err := client.TaskNewJSON(options.slug)
	result, parseErr := contracts.ParseCommandResult(output)
	if parseErr != nil {
		if err != nil {
			return fmt.Errorf("%v; additionally failed to parse command result: %w", err, parseErr)
		}
		return parseErr
	}

	if renderErr := actions.RenderTaskNewResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}

	return nil
}

func runTaskRuntimeInfo(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	output, err := client.TaskRuntimeInfoJSON(options.slug)
	result, parseErr := contracts.ParseCommandResult(output)
	if parseErr != nil {
		if err != nil {
			return fmt.Errorf("%v; additionally failed to parse command result: %w", err, parseErr)
		}
		return parseErr
	}

	if renderErr := actions.RenderTaskRuntimeInfoResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}

	return nil
}

func runTaskRuns(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	output, err := client.TaskRunsJSON(options.slug)
	result, parseErr := contracts.ParseCommandResult(output)
	if parseErr != nil {
		if err != nil {
			return fmt.Errorf("%v; additionally failed to parse command result: %w", err, parseErr)
		}
		return parseErr
	}

	if renderErr := actions.RenderTaskRunsResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}

	return nil
}

func runTaskCleanup(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	if !options.force {
		return fmt.Errorf("brevity task cleanup requires --force")
	}

	output, err := client.TaskCleanupJSON(options.slug)
	result, parseErr := contracts.ParseCommandResult(output)
	if parseErr != nil {
		if err != nil {
			return fmt.Errorf("%v; additionally failed to parse command result: %w", err, parseErr)
		}
		return parseErr
	}

	if renderErr := actions.RenderTaskCleanupResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}

	return nil
}

func runTaskContextRefresh(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	output, err := client.TaskContextRefreshJSON(options.slug)
	result, parseErr := contracts.ParseCommandResult(output)
	if parseErr != nil {
		if err != nil {
			return fmt.Errorf("%v; additionally failed to parse command result: %w", err, parseErr)
		}
		return parseErr
	}

	if renderErr := actions.RenderTaskContextRefreshResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("%s reported success=false", result.Command)
	}

	return nil
}

func runProviderAction(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
	var (
		output []byte
		err    error
	)

	switch options.kind {
	case commandProviderSet:
		output, err = client.ProviderSetJSON(options.provider, options.status)
	case commandProviderReset:
		output, err = client.ProviderResetJSON(options.provider)
	default:
		return fmt.Errorf("unsupported provider action: %s", options.kind)
	}

	result, parseErr := contracts.ParseCommandResult(output)
	if parseErr != nil {
		if err != nil {
			return fmt.Errorf("%v; additionally failed to parse command result: %w", err, parseErr)
		}
		return parseErr
	}

	if renderErr := actions.RenderProviderResult(stdout, result); renderErr != nil {
		return renderErr
	}
	if err != nil {
		return err
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
	fmt.Fprintln(stdout, "  brevity [--once]")
	fmt.Fprintln(stdout, "  brevity doctor")
	fmt.Fprintln(stdout, "  brevity provider set <provider> <status>")
	fmt.Fprintln(stdout, "  brevity provider reset <provider>")
	fmt.Fprintln(stdout, "  brevity task context refresh <slug>")
	fmt.Fprintln(stdout, "  brevity task new <slug>")
	fmt.Fprintln(stdout, "  brevity task runtime-info <slug>")
	fmt.Fprintln(stdout, "  brevity task runs <slug>")
	fmt.Fprintln(stdout, "  brevity task cleanup <slug> --force")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "The dashboard remains read-only. Mutating actions are dispatched")
	fmt.Fprintln(stdout, `through .\brevity.ps1 ... --json command-result contracts.`)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Flags:")
	fmt.Fprintln(stdout, "  --once                    Render the dashboard once.")
	fmt.Fprintln(stdout, "  -h, --help                Show this help text.")
}
