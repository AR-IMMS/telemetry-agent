package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/ar-imms/telemetry-agent/internal/agentinstallation"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"io"
	"os"
	"strings"
)

func runInstall(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(
			stderr,
			"usage: agentctl install --config-root <path> [--gateway-endpoint <host:port>] [--validation-endpoint <host:port>] [--timeout <duration>]",
		)
	}

	configRoot := flags.String(
		"config-root",
		"",
		"root directory containing configuration fragments",
	)
	gatewayEndpoint := flags.String(
		"gateway-endpoint",
		defaultGatewayEndpoint,
		"runtime OpenTelemetry Gateway endpoint",
	)
	validationEndpoint := flags.String(
		"validation-endpoint",
		defaultValidationEndpoint,
		"endpoint supplied only to Collector validation",
	)
	validationTimeout := flags.Duration(
		"timeout",
		defaultValidationTimeout,
		"Collector validation timeout",
	)

	if err := flags.Parse(args); err != nil {
		flags.Usage()

		return 2
	}
	if flags.NArg() != 0 {
		return usageError(
			stderr,
			flags,
			"install does not accept positional arguments",
		)
	}
	if strings.TrimSpace(*configRoot) == "" {
		return usageError(stderr, flags, "--config-root is required")
	}
	if strings.TrimSpace(*gatewayEndpoint) == "" {
		return usageError(
			stderr,
			flags,
			"--gateway-endpoint must not be empty",
		)
	}
	if strings.TrimSpace(*validationEndpoint) == "" {
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

	layoutFor := deps.agentInstallationLayout
	if layoutFor == nil {
		layoutFor = agentinstallation.DefaultLayout
	}

	layout, err := layoutFor(platform)
	if err != nil {
		fmt.Fprintf(stderr, "resolve Agent installation layout: %v\n", err)

		return 1
	}

	stateStore := agentstate.NewStore(layout.StatePath)
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

	currentExecutable := deps.currentExecutable
	if currentExecutable == nil {
		currentExecutable = os.Executable
	}

	sourceBinaryPath, err := currentExecutable()
	if err != nil {
		fmt.Fprintf(stderr, "resolve current Agent executable: %v\n", err)

		return 1
	}

	installAgent := deps.installAgent
	if installAgent == nil {
		installAgent = agentinstallation.NewInstaller(
			deps.downloader,
			deps.runner,
		).Install
	}

	result, err := installAgent(ctx, agentinstallation.Options{
		Platform:         platform,
		SourceBinaryPath: sourceBinaryPath,
		ConfigRoot:       *configRoot,
		GatewayEndpoint:  *gatewayEndpoint,
		HealthEndpoint:   defaultHealthEndpoint,
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
	})
	if err != nil {
		fmt.Fprintf(stderr, "install Agent: %v\n", err)

		return 1
	}

	fmt.Fprintf(
		stdout,
		"Agent installation complete: %s\n",
		result.ServiceName,
	)
	fmt.Fprintf(stdout, "Agent state path: %s\n", result.StatePath)

	return 0
}

func runUninstall(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	flags := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(
			stderr,
			"usage: agentctl uninstall [--state-path <path>] [--timeout <duration>]",
		)
	}

	statePath := flags.String(
		"state-path",
		agentstate.DefaultPath(),
		"Agent lifecycle state path",
	)
	timeout := flags.Duration(
		"timeout",
		defaultUninstallTimeout,
		"maximum time to wait for safe dependency teardown",
	)

	if err := flags.Parse(args); err != nil {
		flags.Usage()

		return 2
	}
	if flags.NArg() != 0 {
		return usageError(
			stderr,
			flags,
			"uninstall does not accept positional arguments",
		)
	}
	if strings.TrimSpace(*statePath) == "" {
		return usageError(
			stderr,
			flags,
			"--state-path must not be empty",
		)
	}
	if *timeout <= 0 {
		return usageError(
			stderr,
			flags,
			"--timeout must be greater than zero",
		)
	}

	uninstallAgent := deps.uninstallAgent
	if uninstallAgent == nil {
		fmt.Fprintln(stderr, "Agent uninstaller is not configured")

		return 1
	}

	if err := uninstallAgent(ctx, *statePath, *timeout); err != nil {
		fmt.Fprintf(stderr, "uninstall Agent: %v\n", err)

		return 1
	}

	fmt.Fprintln(stdout, "Agent uninstall complete")

	return 0
}
