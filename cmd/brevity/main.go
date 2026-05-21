package main

import (
	"bufio"
	"context"
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
	"github.com/mortenlein/brevity/internal/commands"
	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/dashboard"
	"github.com/mortenlein/brevity/internal/runtimeclient"
	"github.com/mortenlein/brevity/internal/state"
)

func main() {
	if err := run(os.Stdout, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "brevity-go:", err)
		os.Exit(1)
	}
}

type commandKind string

const (
	commandDashboard       commandKind = commandKind(commands.DashboardID)
	commandProviderStatus  commandKind = commandKind(commands.ProviderStatusID)
	commandProviderSet     commandKind = commandKind(commands.ProviderSetID)
	commandProviderReset   commandKind = commandKind(commands.ProviderResetID)
	commandContextRefresh  commandKind = commandKind(commands.TaskContextRefreshID)
	commandDoctor          commandKind = commandKind(commands.DoctorID)
	commandTaskCleanup     commandKind = commandKind(commands.TaskCleanupID)
	commandTaskNew         commandKind = commandKind(commands.TaskNewID)
	commandTaskRun         commandKind = commandKind(commands.TaskRunID)
	commandTaskRuntimeInfo commandKind = commandKind(commands.TaskRuntimeInfoID)
	commandTaskRuns        commandKind = commandKind(commands.TaskRunsID)
	commandRunsReconcile   commandKind = commandKind(commands.TaskRunsReconcileID)
	commandRunsRetention   commandKind = commandKind(commands.TaskRunsRetentionID)
	commandRunsCompact     commandKind = commandKind(commands.TaskRunsCompactID)
)

type cliOptions struct {
	help       bool
	kind       commandKind
	once       bool
	watch      bool
	bubble     bool
	noClear    bool
	refresh    time.Duration
	jsonSource string
	provider   string
	status     string
	note       string
	slug       string
	force      bool
	execute    bool
	dryRun     bool
	profile    string
	smoke      bool
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

func parseDoctorOptions(args []string) (cliOptions, error) {
	if len(args) != 1 {
		return cliOptions{}, usageError(commands.Doctor)
	}

	return cliOptions{kind: commandDoctor}, nil
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
		return cliOptions{}, fmt.Errorf("missing task command: supported commands: context refresh, runtime-info, runs, new, run, cleanup")
	}
	if args[1] == "context" {
		if len(args) != 4 || args[2] != "refresh" {
			return cliOptions{}, usageError(commands.TaskContextRefresh)
		}
		if args[3] == "" {
			return cliOptions{}, usageError(commands.TaskContextRefresh)
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
		return cliOptions{}, usageError(commands.TaskNew)
	}

	return cliOptions{kind: commandTaskNew, slug: args[2]}, nil
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
		return cliOptions{}, usageError(commands.TaskRun)
	}

	options := cliOptions{kind: commandTaskRun, slug: args[2]}
	if options.slug == "" || options.slug == "--execute" {
		return cliOptions{}, usageError(commands.TaskRun)
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
		return cliOptions{}, usageError(commands.TaskRuntimeInfo)
	}

	return cliOptions{kind: commandTaskRuntimeInfo, slug: args[2]}, nil
}

func parseTaskRunsOptions(args []string) (cliOptions, error) {
	if len(args) >= 3 && (args[2] == "reconcile" || args[2] == "retention" || args[2] == "compact") {
		return parseTaskRunsMaintenanceOptions(args)
	}
	if len(args) != 3 || args[2] == "" {
		return cliOptions{}, usageError(commands.TaskRuns)
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
	case commandProviderStatus, commandProviderSet, commandProviderReset:
		return routeProviderCommand(stdout, options)
	case commandContextRefresh:
		return routeTaskContextCommand(stdout, client, options)
	case commandDoctor:
		return routeDoctorCommand(stdout, client)
	case commandTaskCleanup, commandTaskNew, commandTaskRun, commandTaskRuntimeInfo:
		return routeTaskCommand(stdout, client, options)
	case commandTaskRuns, commandRunsReconcile, commandRunsRetention, commandRunsCompact:
		return routeTaskRunsCommand(stdout, client, options)
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
		return commands.TaskRunsReconcile.Name()
	case commandRunsRetention:
		return commands.TaskRunsRetention.Name()
	case commandRunsCompact:
		return commands.TaskRunsCompact.Name()
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
