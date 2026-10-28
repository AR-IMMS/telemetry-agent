package main

import (
	"context"
	"fmt"
	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/agentinstallation"
	"github.com/ar-imms/telemetry-agent/internal/agentlifecycle"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	librehardwaremonitor "github.com/ar-imms/telemetry-agent/internal/dependency/libre_hardware_monitor"
	nodeexporter "github.com/ar-imms/telemetry-agent/internal/dependency/node_exporter"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/supervisor"
	"io"
	"net"
	"net/http"
	"os"
	"time"
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
	defaultUninstallTimeout     = 5 * time.Minute
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
	prepareInstallationIdentity   func(string) error
	teardownDependency            dependencyTeardownFunc
	configureDependencies         dependencyConfigureFunc
	listenStatus                  statusListenerFunc
	serveStatus                   statusServerRunFunc
	runCollectorRuntimeWithHealth collectorRuntimeHealthRunFunc
	fetchHealthStatus             healthStatusFetchFunc
	agentInstallationLayout       func(
		identity.PlatformInfo,
	) (agentinstallation.Layout, error)

	currentExecutable func() (string, error)

	installAgent func(
		context.Context,
		agentinstallation.Options,
	) (agentinstallation.Layout, error)
	uninstallAgent agentUninstallFunc
}

type agentUninstallFunc func(
	context.Context,
	string,
	time.Duration,
) error

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

// defaultDependencies wires production implementations for the agentctl binary.
func defaultDependencies() dependencies {
	downloader := bootstrap.HTTPDownloader{}
	runner := bootstrap.OSCommandRunner{}
	agentInstaller := agentinstallation.NewInstaller(
		downloader,
		runner,
	)

	agentUninstaller := newManagedAgentUninstaller(
		identity.CollectPlatformInfo,
		os.Executable,
		runner,
	)

	return dependencies{
		collectPlatform: identity.CollectPlatformInfo,
		runBootstrap:    bootstrap.Run,
		downloader:      downloader,
		runner:          runner,
		runSupervisor:   supervisor.Run,

		agentInstallationLayout: agentinstallation.DefaultLayout,
		uninstallAgent:          agentUninstaller,
		currentExecutable:       os.Executable,
		installAgent:            agentInstaller.Install,

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
		listDependencyStatuses:      defaultListDependencyStatuses,
		runCollectorRuntime:         agentlifecycle.RunCollectorRuntime,
		prepareInstallationIdentity: prepareAgentInstallationIdentity,
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
