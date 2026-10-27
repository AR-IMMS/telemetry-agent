package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"io"
	"os"
)

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

	stateStore := agentstate.NewStore(*statePath)

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
