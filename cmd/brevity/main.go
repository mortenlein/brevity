package main

import (
	"flag"
	"fmt"
	"io"
	"os"

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

type cliOptions struct {
	once       bool
	jsonSource string
}

func parseOptions(args []string) (cliOptions, error) {
	options := cliOptions{jsonSource: "powershell"}
	flags := flag.NewFlagSet("brevity", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.BoolVar(&options.once, "once", false, "render the dashboard once")
	flags.StringVar(&options.jsonSource, "json-source", options.jsonSource, "runtime state JSON source")

	if err := flags.Parse(args); err != nil {
		return cliOptions{}, err
	}
	if flags.NArg() > 0 {
		return cliOptions{}, fmt.Errorf("unexpected argument: %s", flags.Arg(0))
	}
	if options.jsonSource != "powershell" {
		return cliOptions{}, fmt.Errorf("unsupported --json-source %q: supported values: powershell", options.jsonSource)
	}

	return options, nil
}

func run(stdout io.Writer, args []string) error {
	options, err := parseOptions(args)
	if err != nil {
		return err
	}

	return runWithOptions(stdout, runtimeclient.NewPowerShellClient(), options)
}

func runWithClient(stdout io.Writer, client runtimeclient.Client) error {
	return runWithOptions(stdout, client, cliOptions{jsonSource: "powershell"})
}

func runWithOptions(stdout io.Writer, client runtimeclient.Client, options cliOptions) error {
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
