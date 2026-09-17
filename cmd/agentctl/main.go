package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	nodeexporter "github.com/ar-imms/telemetry-agent/internal/dependency/node_exporter"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/supervisor"
)

const (
	defaultValidationEndpoint = "127.0.0.1:4317"
	defaultValidationTimeout  = 2 * time.Minute
	defaultHealthEndpoint     = "http://127.0.0.1:13133"
	defaultStartupTimeout     = 30 * time.Second
	defaultShutdownTimeout    = 10 * time.Second
)

type dependencies struct {
	collectPlatform   func() (identity.PlatformInfo, error)
	runBootstrap      bootstrapRunFunc
	downloader        bootstrap.Downloader
	runner            bootstrap.CommandRunner
	runSupervisor     supervisorRunFunc
	installDependency dependencyInstallFunc
}

type dependencyInstallFunc func(
	context.Context,
	string,
) (dependency.InstallResult, error)

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
	}
}

// defaultDependencyInstallers creates the platform-specific installers bundled
// with this agentctl release.
func defaultDependencyInstallers() map[string]dependency.Installer {
	return map[string]dependency.Installer{
		"windows-exporter": dependency.NewWindowsExporterInstaller(
			dependency.DefaultWindowsExporterOptions(),
			bootstrap.HTTPDownloader{},
		),
		"node-exporter": nodeexporter.NewInstaller(
			nodeexporter.DefaultOptions(),
			bootstrap.HTTPDownloader{},
		),
	}
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

	service := dependency.Service{
		Registry:   dependency.DefaultRegistry(),
		OS:         platform.OS,
		Installers: defaultDependencyInstallers(),
	}

	return service.Install(ctx, name)
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
		fmt.Fprintln(
			stderr,
			"usage: agentctl <bootstrap|run> [command options]",
		)
		return 2
	}

	switch args[0] {
	case "bootstrap":
		return runBootstrap(ctx, args[1:], stdout, stderr, deps)

	case "run":
		return runCollector(ctx, args[1:], stdout, stderr, deps)

	case "dependency":
		return runDependency(ctx, args[1:], stdout, stderr, deps)

	default:
		fmt.Fprintln(
			stderr,
			"usage: agentctl <bootstrap|run> [command options]",
		)
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
		fmt.Fprintln(stderr, "usage: agentctl bootstrap --config-root <path> --install-dir <path> --config-path <path> [--validation-endpoint <host:port>] [--timeout <duration>]")
	}

	configRoot := flags.String("config-root", "", "root directory containing configuration fragments")
	installDir := flags.String("install-dir", "", "Collector installation root")
	configPath := flags.String("config-path", "", "final rendered Collector configuration path")
	validationEndpoint := flags.String("validation-endpoint", defaultValidationEndpoint, "endpoint supplied only to Collector validation")
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
	// Runtime receives only already-validated paths and endpoint values from the CLI.
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(
			stderr,
			"usage: agentctl run --collector-path <path> --config-path <path> --gateway-endpoint <host:port> [--health-endpoint <url>] [--startup-timeout <duration>] [--shutdown-timeout <duration>]",
		)
	}

	collectorPath := flags.String(
		"collector-path",
		"",
		"verified Collector binary path",
	)
	configPath := flags.String(
		"config-path",
		"",
		"validated final Collector configuration path",
	)
	gatewayEndpoint := flags.String(
		"gateway-endpoint",
		"",
		"OTLP gateway endpoint supplied to Collector at runtime",
	)
	healthEndpoint := flags.String(
		"health-endpoint",
		defaultHealthEndpoint,
		"Collector health endpoint URL",
	)
	startupTimeout := flags.Duration(
		"startup-timeout",
		defaultStartupTimeout,
		"maximum time to wait for Collector readiness",
	)
	shutdownTimeout := flags.Duration(
		"shutdown-timeout",
		defaultShutdownTimeout,
		"maximum graceful Collector shutdown time",
	)

	if err := flags.Parse(args); err != nil {
		flags.Usage()
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage error: run does not accept positional arguments")
		flags.Usage()
		return 2
	}
	if *collectorPath == "" {
		return usageError(stderr, flags, "--collector-path is required")
	}
	if *configPath == "" {
		return usageError(stderr, flags, "--config-path is required")
	}
	if *gatewayEndpoint == "" {
		return usageError(stderr, flags, "--gateway-endpoint is required")
	}
	if *healthEndpoint == "" {
		return usageError(stderr, flags, "--health-endpoint must not be empty")
	}
	if *startupTimeout <= 0 {
		return usageError(stderr, flags, "--startup-timeout must be greater than zero")
	}
	if *shutdownTimeout <= 0 {
		return usageError(stderr, flags, "--shutdown-timeout must be greater than zero")
	}

	runSupervisor := deps.runSupervisor
	if runSupervisor == nil {
		runSupervisor = supervisor.Run
	}

	err := runSupervisor(ctx, supervisor.Options{
		BinaryPath:      *collectorPath,
		ConfigPath:      *configPath,
		GatewayEndpoint: *gatewayEndpoint,
		HealthEndpoint:  *healthEndpoint,
		StartupTimeout:  *startupTimeout,
		ShutdownTimeout: *shutdownTimeout,
	})
	if err != nil {
		fmt.Fprintf(stderr, "run Collector: %v\n", err)
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
