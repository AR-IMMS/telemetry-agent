package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/agentlifecycle"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	librehardwaremonitor "github.com/ar-imms/telemetry-agent/internal/dependency/libre_hardware_monitor"
	nodeexporter "github.com/ar-imms/telemetry-agent/internal/dependency/node_exporter"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/supervisor"
)

const (
	defaultValidationEndpoint   = "127.0.0.1:4317"
	defaultGatewayEndpoint      = "127.0.0.1:4317"
	defaultStatusListenAddress  = "127.0.0.1:13134"
	defaultValidationTimeout    = 2 * time.Minute
	defaultHealthEndpoint       = "http://127.0.0.1:13133"
	defaultStartupTimeout       = 30 * time.Second
	defaultShutdownTimeout      = 10 * time.Second
	defaultStatusEndpoint       = "http://" + defaultStatusListenAddress + "/v1/status"
	defaultStatusRequestTimeout = 5 * time.Second
)

type dependencies struct {
	collectPlatform               func() (identity.PlatformInfo, error)
	runBootstrap                  bootstrapRunFunc
	downloader                    bootstrap.Downloader
	runner                        bootstrap.CommandRunner
	runSupervisor                 supervisorRunFunc
	installDependency             dependencyInstallFunc
	listDependencies              dependencyListFunc
	manageDependency              managedDependencyInstallFunc
	manageDependencyLifecycle     managedDependencyLifecycleFunc
	selectDependencies            dependencySelectionFunc
	listDependencyStatuses        dependencyStatusListFunc
	runCollectorRuntime           collectorRuntimeRunFunc
	teardownDependency            dependencyTeardownFunc
	configureDependencies         dependencyConfigureFunc
	listenStatus                  statusListenerFunc
	serveStatus                   statusServerRunFunc
	runCollectorRuntimeWithHealth collectorRuntimeHealthRunFunc
	fetchHealthStatus             healthStatusFetchFunc
}

type healthStatusFetchFunc func(
	context.Context,
	string,
) (agenthealth.Snapshot, error)

type dependencyStatusListFunc func(
	context.Context,
) ([]dependency.StatusResult, error)

type dependencySelectionFunc func(
	context.Context,
	[]dependency.Definition,
	io.Writer,
) ([]string, error)

type dependencyInstallFunc func(
	context.Context,
	string,
) (dependency.InstallResult, error)

type dependencyListFunc func(
	context.Context,
) ([]dependency.Definition, error)

type bootstrapRunFunc func(
	context.Context,
	bootstrap.Options,
	bootstrap.Downloader,
	bootstrap.CommandRunner,
) (bootstrap.Result, error)

type supervisorRunFunc func(
	context.Context,
	supervisor.Options,
) error

type dependencyTeardownFunc = agentlifecycle.DependencyTeardownFunc

type collectorRuntimeRunFunc func(
	context.Context,
	string,
	dependencyTeardownFunc,
) error

type collectorRuntimeHealthRunFunc func(
	context.Context,
	string,
	dependencyTeardownFunc,
	agenthealth.ObservationReporter,
) error

type statusListenerFunc func(
	string,
	string,
) (net.Listener, error)

type statusServerRunFunc func(
	context.Context,
	net.Listener,
	agenthealth.Provider,
) error

func main() {
	// Convert termination signals into cancellation for graceful shutdown.
	deps := defaultDependencies()

	ctx, stop := signal.NotifyContext(
		context.Background(),
		terminationSignals()...,
	)

	exitCode := run(ctx, os.Args[1:], os.Stdout, os.Stderr, deps)

	stop()
	os.Exit(exitCode)
}

// defaultDependencies wires production implementations for the agentctl binary.
func defaultDependencies() dependencies {
	return dependencies{
		collectPlatform:   identity.CollectPlatformInfo,
		runBootstrap:      bootstrap.Run,
		downloader:        bootstrap.HTTPDownloader{},
		runner:            bootstrap.OSCommandRunner{},
		runSupervisor:     supervisor.Run,
		installDependency: defaultInstallDependency,
		manageDependency: newManagedDependencyInstaller(
			identity.CollectPlatformInfo,
			defaultInstallDependency,
			bootstrap.OSCommandRunner{},
		),
		manageDependencyLifecycle: newManagedDependencyLifecycle(
			identity.CollectPlatformInfo,
			bootstrap.OSCommandRunner{},
		),
		listDependencies: defaultListDependencies,
		selectDependencies: newTerminalDependencyMultiSelector(
			func() bool {
				inputInfo, inputErr := os.Stdin.Stat()
				outputInfo, outputErr := os.Stdout.Stat()

				return inputErr == nil &&
					outputErr == nil &&
					inputInfo.Mode()&os.ModeCharDevice != 0 &&
					outputInfo.Mode()&os.ModeCharDevice != 0
			},
			runDependencyMultiSelectProgram(os.Stdin),
		),
		listDependencyStatuses: defaultListDependencyStatuses,
		runCollectorRuntime:    agentlifecycle.RunCollectorRuntime,
		configureDependencies: newTerminalDependencyConfigurer(
			func() bool {
				inputInfo, inputErr := os.Stdin.Stat()
				outputInfo, outputErr := os.Stdout.Stat()

				return inputErr == nil &&
					outputErr == nil &&
					inputInfo.Mode()&os.ModeCharDevice != 0 &&
					outputInfo.Mode()&os.ModeCharDevice != 0
			},
			runDependencyConfigureProgram(os.Stdin),
		),
		listenStatus:                  net.Listen,
		serveStatus:                   agenthealth.ServeStatus,
		runCollectorRuntimeWithHealth: agentlifecycle.RunCollectorRuntimeWithHealth,
		fetchHealthStatus:             defaultFetchHealthStatus,
	}
}

func defaultFetchHealthStatus(
	ctx context.Context,
	endpoint string,
) (agenthealth.Snapshot, error) {
	return agenthealth.FetchStatus(
		ctx,
		&http.Client{
			Timeout: defaultStatusRequestTimeout,
		},
		endpoint,
	)
}

func defaultListDependencyStatuses(
	ctx context.Context,
) ([]dependency.StatusResult, error) {
	platform, err := identity.CollectPlatformInfo()
	if err != nil {
		return nil, fmt.Errorf(
			"collect platform information: %w",
			err,
		)
	}

	catalog, err := defaultDependencyCatalog()
	if err != nil {
		return nil, fmt.Errorf(
			"build dependency catalog: %w",
			err,
		)
	}

	return (dependency.Service{
		Catalog: catalog,
		OS:      platform.OS,
	}).ListStatus(ctx)
}

// defaultListDependencies returns integrations supported by the local platform.
func defaultListDependencies(
	context.Context,
) ([]dependency.Definition, error) {
	platform, err := identity.CollectPlatformInfo()
	if err != nil {
		return nil, fmt.Errorf(
			"collect platform information: %w",
			err,
		)
	}

	catalog, err := defaultDependencyCatalog()
	if err != nil {
		return nil, fmt.Errorf(
			"build dependency catalog: %w",
			err,
		)
	}

	return catalog.List(platform.OS), nil
}

// defaultDependencyCatalog creates the built-in integrations bundled with this
// agentctl release.
func defaultDependencyCatalog() (dependency.Catalog, error) {
	windowsExporterOptions := dependency.DefaultWindowsExporterOptions()
	nodeExporterOptions := nodeexporter.DefaultOptions()
	lhmOptions := librehardwaremonitor.DefaultOptions()

	return dependency.NewCatalog([]dependency.Integration{
		{
			Definition: dependency.Definition{
				Name:        "windows-exporter",
				DisplayName: "Windows Exporter",
				Description: "Collects Windows host metrics.",
				MetricsEndpoint: "http://" +
					windowsExporterOptions.ListenAddress +
					"/metrics",
				SupportedOS: []string{"windows"},
			},
			CollectorReceiver: "prometheus/windows_exporter",
			Install: dependency.NewWindowsExporterInstaller(
				windowsExporterOptions,
				bootstrap.HTTPDownloader{},
			),
			Inspect: dependency.NewWindowsExporterInspector(
				windowsExporterOptions,
			),
			Teardown: dependency.NewWindowsExporterTeardown(
				windowsExporterOptions,
			),
			Enable: dependency.NewWindowsExporterEnabler(
				dependency.DefaultWindowsExporterOptions(),
			),
		},
		{
			Definition: dependency.Definition{
				Name:        "node-exporter",
				DisplayName: "Node Exporter",
				Description: "Collects Linux host metrics.",
				MetricsEndpoint: "http://" +
					nodeExporterOptions.ListenAddress +
					"/metrics",
				SupportedOS: []string{"linux"},
			},
			CollectorReceiver: "prometheus/node_exporter",
			Install: nodeexporter.NewInstaller(
				nodeExporterOptions,
				bootstrap.HTTPDownloader{},
			),
			Inspect: nodeexporter.NewInspector(
				nodeExporterOptions,
			),
			Teardown: nodeexporter.NewTeardown(
				nodeExporterOptions,
			),
			Enable: nodeexporter.NewEnabler(
				nodeexporter.DefaultOptions(),
			),
		},
		{
			Definition: dependency.Definition{
				Name:        "libre-hardware-monitor",
				DisplayName: "Libre Hardware Monitor",
				Description: "Collects Windows hardware metrics.",
				MetricsEndpoint: fmt.Sprintf(
					"http://127.0.0.1:%d/metrics",
					lhmOptions.ListenPort,
				),
				SupportedOS: []string{"windows"},
			},
			CollectorReceiver: "prometheus/libre_hardware_monitor",
			Install: librehardwaremonitor.NewInstaller(
				lhmOptions,
				bootstrap.HTTPDownloader{},
			),
			Inspect: librehardwaremonitor.NewInspector(
				lhmOptions,
			),
			Teardown: librehardwaremonitor.NewTeardown(
				lhmOptions,
			),
			Enable: librehardwaremonitor.NewEnabler(
				librehardwaremonitor.DefaultOptions(),
			),
		},
	})
}

// defaultInstallDependency selects the platform-safe dependency installer for
// one real CLI invocation.
func defaultInstallDependency(
	ctx context.Context,
	name string,
) (dependency.InstallResult, error) {
	platform, err := identity.CollectPlatformInfo()
	if err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"collect platform information: %w",
			err,
		)
	}

	catalog, err := defaultDependencyCatalog()
	if err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"build dependency catalog: %w",
			err,
		)
	}

	service := dependency.Service{
		Catalog: catalog,
		OS:      platform.OS,
	}

	return service.Install(ctx, name)
}

func defaultEnableDependency(
	ctx context.Context,
	name string,
	resources []agentstate.OwnedResource,
) error {
	platform, err := identity.CollectPlatformInfo()
	if err != nil {
		return fmt.Errorf(
			"collect platform information: %w",
			err,
		)
	}

	catalog, err := defaultDependencyCatalog()
	if err != nil {
		return fmt.Errorf(
			"build dependency catalog: %w",
			err,
		)
	}

	return (dependency.Service{
		Catalog: catalog,
		OS:      platform.OS,
	}).Enable(ctx, name, resources)
}

func writeRootHelp(output io.Writer) {
	fmt.Fprint(output, `Usage:
  agentctl <command> [options]

Commands:
  bootstrap   Bootstrap the OpenTelemetry Collector.
  dependency  Manage telemetry dependencies.
  run         Run the OpenTelemetry Collector under supervision.
  status      Show live Agent health.
  help        Show this help.
`)
}

func run(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	// Keep command dispatch separate from command-specific flag validation and effects.
	// usage: agentctl <bootstrap|run|dependency> [command options]
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage error: command is required")
		fmt.Fprintln(stderr)
		writeRootHelp(stderr)

		return 2
	}

	switch args[0] {
	case "bootstrap":
		return runBootstrap(ctx, args[1:], stdout, stderr, deps)

	case "run":
		return runCollector(ctx, args[1:], stdout, stderr, deps)

	case "dependency":
		return runDependency(ctx, args[1:], stdout, stderr, deps)

	case "status":
		return runStatus(ctx, args[1:], stdout, stderr, deps)

	case "help":
		writeRootHelp(stdout)
		return 0

	default:
		fmt.Fprintf(
			stderr,
			"usage error: unknown command %q\n\n",
			args[0],
		)
		writeRootHelp(stderr)

		return 2
	}
}

func runBootstrap(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	flags := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(
			stderr,
			"usage: agentctl bootstrap --config-root <path> --install-dir <path> --config-path <path> [--state-path <path>] [--gateway-endpoint <host:port>] [--validation-endpoint <host:port>] [--timeout <duration>]",
		)
	}

	configRoot := flags.String("config-root", "", "root directory containing configuration fragments")
	installDir := flags.String("install-dir", "", "Collector installation root")
	configPath := flags.String("config-path", "", "final rendered Collector configuration path")
	statePath := flags.String(
		"state-path",
		agentstate.DefaultPath(),
		"Agent lifecycle state path",
	)
	validationEndpoint := flags.String("validation-endpoint", defaultValidationEndpoint, "endpoint supplied only to Collector validation")
	gatewayEndpoint := flags.String(
		"gateway-endpoint",
		defaultGatewayEndpoint,
		"runtime OpenTelemetry Gateway endpoint",
	)
	validationTimeout := flags.Duration("timeout", defaultValidationTimeout, "Collector validation timeout")

	if err := flags.Parse(args); err != nil {
		flags.Usage()
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage error: bootstrap does not accept positional arguments")
		flags.Usage()
		return 2
	}
	if *configRoot == "" {
		return usageError(stderr, flags, "--config-root is required")
	}
	if *installDir == "" {
		return usageError(stderr, flags, "--install-dir is required")
	}
	if *configPath == "" {
		return usageError(stderr, flags, "--config-path is required")
	}

	if *gatewayEndpoint == "" {
		return usageError(
			stderr,
			flags,
			"--gateway-endpoint must not be empty",
		)
	}

	if *validationEndpoint == "" {
		return usageError(
			stderr,
			flags,
			"--validation-endpoint must not be empty",
		)
	}

	if *validationTimeout <= 0 {
		return usageError(
			stderr,
			flags,
			"--timeout must be greater than zero",
		)
	}

	collectPlatform := deps.collectPlatform
	if collectPlatform == nil {
		collectPlatform = identity.CollectPlatformInfo
	}
	platform, err := collectPlatform()
	if err != nil {
		fmt.Fprintf(stderr, "collect platform information: %v\n", err)
		return 1
	}

	stateStore := agentstate.NewFileStore(*statePath)

	state, err := stateStore.Load()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(stderr, "load Agent state: %v\n", err)

			return 1
		}

		state = agentstate.State{
			Dependencies: make(map[string]agentstate.DependencyState),
		}
	}

	catalog, err := defaultDependencyCatalog()
	if err != nil {
		fmt.Fprintf(stderr, "build dependency catalog: %v\n", err)

		return 1
	}

	receiverLayer, err := dependencyMetricsReceiverLayer(
		catalog,
		platform.OS,
		state,
	)
	if err != nil {
		fmt.Fprintf(
			stderr,
			"configure managed dependency receivers: %v\n",
			err,
		)

		return 1
	}

	layers, err := configurationLayers(*configRoot, platform.OS)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	runBootstrap := deps.runBootstrap
	if runBootstrap == nil {
		runBootstrap = bootstrap.Run
	}
	result, err := runBootstrap(ctx, bootstrap.Options{
		Platform:   platform,
		InstallDir: *installDir,
		ConfigPath: *configPath,
		ConfigInput: config.RenderInput{
			Platform: platform,
			Layers:   layers,
			InlineLayers: []config.InlineLayer{
				receiverLayer,
			},
		},
		ValidationEnvironment: []string{
			"OTEL_GATEWAY_ENDPOINT=" + *validationEndpoint,
		},
		ValidationTimeout: *validationTimeout,
	}, deps.downloader, deps.runner)
	if err != nil {
		fmt.Fprintf(stderr, "bootstrap Collector: %v\n", err)
		return 1
	}

	_, err = stateStore.Update(func(state *agentstate.State) error {
		state.Collector = agentstate.CollectorContext{
			ConfigRoot:      *configRoot,
			BinaryPath:      result.BinaryPath,
			ConfigPath:      result.ConfigPath,
			GatewayEndpoint: *gatewayEndpoint,
			HealthEndpoint:  defaultHealthEndpoint,
		}

		return nil
	})
	if err != nil {
		fmt.Fprintf(stderr, "persist Agent state: %v\n", err)

		return 1
	}

	fmt.Fprintf(stdout, "Collector version: %s\n", result.Artifact.Version)
	fmt.Fprintf(stdout, "Collector binary path: %s\n", result.BinaryPath)
	fmt.Fprintf(stdout, "Final configuration path: %s\n", result.ConfigPath)
	fmt.Fprintf(stdout, "Collector installation reused: %t\n", result.Reused)

	return 0
}

func runCollector(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(
			stderr,
			"usage: agentctl run [--state-path <path>]",
		)
	}

	statePath := flags.String(
		"state-path",
		agentstate.DefaultPath(),
		"persistent Agent state path",
	)

	if err := flags.Parse(args); err != nil {
		flags.Usage()

		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(
			stderr,
			"usage error: run does not accept positional arguments",
		)
		flags.Usage()

		return 2
	}
	if strings.TrimSpace(*statePath) == "" {
		return usageError(
			stderr,
			flags,
			"--state-path must not be empty",
		)
	}

	teardown := deps.teardownDependency
	if teardown == nil {
		teardown = defaultDependencyTeardown
	}

	runRuntimeWithHealth := deps.runCollectorRuntimeWithHealth
	if runRuntimeWithHealth == nil {
		runRuntime := deps.runCollectorRuntime
		if runRuntime == nil {
			runRuntime = agentlifecycle.RunCollectorRuntime
		}

		runRuntimeWithHealth = func(
			ctx context.Context,
			statePath string,
			teardown dependencyTeardownFunc,
			_ agenthealth.ObservationReporter,
		) error {
			return runRuntime(ctx, statePath, teardown)
		}
	}

	listenStatus := deps.listenStatus
	if listenStatus == nil {
		listenStatus = net.Listen
	}

	listener, err := listenStatus(
		"tcp",
		defaultStatusListenAddress,
	)
	if err != nil {
		fmt.Fprintf(
			stderr,
			"listen Agent status endpoint: %v\n",
			err,
		)

		return 1
	}
	defer listener.Close()

	serveStatus := deps.serveStatus
	if serveStatus == nil {
		serveStatus = agenthealth.ServeStatus
	}

	reporter := agenthealth.NewReporter(
		agenthealth.RuntimeObservation{
			CollectorState: agenthealth.CollectorStateStarting,
		},
	)

	runContext, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	statusDone := make(chan error, 1)

	go func() {
		statusDone <- serveStatus(
			runContext,
			listener,
			reporter,
		)
	}()

	runtimeErr := runRuntimeWithHealth(
		runContext,
		*statePath,
		teardown,
		reporter,
	)

	cancelRun()
	statusErr := <-statusDone

	if runtimeErr != nil {
		fmt.Fprintf(stderr, "run Collector: %v\n", runtimeErr)

		return 1
	}
	if statusErr != nil {
		fmt.Fprintf(
			stderr,
			"run Agent status endpoint: %v\n",
			statusErr,
		)

		return 1
	}

	fmt.Fprintln(stdout, "Collector stopped")

	return 0
}

func usageError(stderr io.Writer, flags *flag.FlagSet, message string) int {
	fmt.Fprintln(stderr, "usage error: "+message)
	flags.Usage()
	return 2
}

func configurationLayers(configRoot string, osName string) ([]config.Layer, error) {
	osName = strings.ToLower(strings.TrimSpace(osName))
	if osName != "linux" && osName != "windows" {
		return nil, fmt.Errorf("unsupported operating system %q", osName)
	}

	return []config.Layer{
		{Name: "base", Path: filepath.Join(configRoot, "base", "otel.yaml")},
		{Name: "profile", Path: filepath.Join(configRoot, "profiles", "laptop.yaml")},
		{Name: "os", Path: filepath.Join(configRoot, "os", osName, "otel.yaml")},
	}, nil
}

func dependencyMetricsReceiverLayer(
	catalog dependency.Catalog,
	osName string,
	state agentstate.State,
) (config.InlineLayer, error) {
	managedReceivers, err := catalog.CollectorReceivers(osName, state)
	if err != nil {
		return config.InlineLayer{}, err
	}

	receivers := append(
		[]string{"otlp", "hostmetrics"},
		managedReceivers...,
	)

	return config.MetricsReceiverLayer(receivers), nil
}

func defaultDependencyTeardown(
	ctx context.Context,
	name string,
	action agentstate.TeardownAction,
	resources []agentstate.OwnedResource,
) error {
	platform, err := identity.CollectPlatformInfo()
	if err != nil {
		return fmt.Errorf(
			"collect platform information: %w",
			err,
		)
	}

	catalog, err := defaultDependencyCatalog()
	if err != nil {
		return fmt.Errorf(
			"build dependency catalog: %w",
			err,
		)
	}

	return (dependency.Service{
		Catalog: catalog,
		OS:      platform.OS,
	}).Teardown(ctx, name, action, resources)
}

func runStatus(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(
			stderr,
			"usage: agentctl status [--endpoint <url>]",
		)
	}

	endpoint := flags.String(
		"endpoint",
		defaultStatusEndpoint,
		"local Agent status endpoint",
	)

	if err := flags.Parse(args); err != nil {
		flags.Usage()

		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(
			stderr,
			"usage error: status does not accept positional arguments",
		)
		flags.Usage()

		return 2
	}
	if strings.TrimSpace(*endpoint) == "" {
		return usageError(
			stderr,
			flags,
			"--endpoint must not be empty",
		)
	}

	fetchStatus := deps.fetchHealthStatus
	if fetchStatus == nil {
		fetchStatus = defaultFetchHealthStatus
	}

	snapshot, err := fetchStatus(ctx, *endpoint)
	if err != nil {
		fmt.Fprintf(
			stderr,
			"Agent is not running or status endpoint is unreachable: %v\n",
			err,
		)

		return 1
	}

	fmt.Fprintf(stdout, "Agent health: %s\n", snapshot.Status)
	fmt.Fprintf(
		stdout,
		"Collector state: %s\n",
		snapshot.CollectorState,
	)
	fmt.Fprintf(
		stdout,
		"Configuration generations: desired=%d activated=%d applied=%d\n",
		snapshot.DesiredGeneration,
		snapshot.ActivatedGeneration,
		snapshot.AppliedGeneration,
	)
	fmt.Fprintf(
		stdout,
		"Pending dependency teardowns: %d\n",
		snapshot.PendingTeardowns,
	)

	if snapshot.LastError != "" {
		fmt.Fprintf(stdout, "Last error: %s\n", snapshot.LastError)
	}

	return 0
}
