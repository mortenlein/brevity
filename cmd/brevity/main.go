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
	commandDashboard     commandKind = "dashboard"
	commandProviderSet   commandKind = "provider-set"
	commandProviderReset commandKind = "provider-reset"
)

type cliOptions struct {
	help     bool
	kind     commandKind
	once     bool
	provider string
	status   string
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
	fmt.Fprintln(stdout, "  brevity provider set <provider> <status>")
	fmt.Fprintln(stdout, "  brevity provider reset <provider>")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "The dashboard remains read-only. Provider mutations are dispatched")
	fmt.Fprintln(stdout, `through .\brevity.ps1 provider ... --json command-result contracts.`)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Flags:")
	fmt.Fprintln(stdout, "  --once                    Render the dashboard once.")
	fmt.Fprintln(stdout, "  -h, --help                Show this help text.")
}
