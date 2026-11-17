package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	agenthealth "github.com/ar-imms/telemetry-agent/internal/health"
	"github.com/ar-imms/telemetry-agent/internal/identity/host"
	config "github.com/ar-imms/telemetry-agent/internal/runtime/collector/configuration"
	"github.com/ar-imms/telemetry-agent/internal/runtime/dependencies"
	"io"
	"net"
	"path/filepath"
	"strings"
)

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

	if err := parseCompatibilityFlags(flags, args); err != nil {
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
	if err := runCollectorRuntime(ctx, *statePath, deps); err != nil {
		fmt.Fprintf(stderr, "run Collector: %v\n", err)

		return 1
	}

	fmt.Fprintln(stdout, "Collector stopped")

	return 0
}

func runCollectorRuntime(
	ctx context.Context,
	statePath string,
	deps dependencies,
) error {
	prepareIdentity := deps.prepareInstallationIdentity
	if prepareIdentity == nil {
		prepareIdentity = prepareAgentInstallationIdentity
	}
	if err := prepareIdentity(statePath); err != nil {
		return fmt.Errorf("prepare installation identity: %w", err)
	}

	teardown := deps.teardownDependency
	if teardown == nil {
		teardown = defaultDependencyTeardown
	}

	runRuntimeWithHealth := deps.runCollectorRuntimeWithHealth
	if runRuntimeWithHealth == nil {
		runRuntime := deps.runCollectorRuntime
		if runRuntime == nil {
			runRuntime = runAgentCollectorRuntime
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
		return fmt.Errorf("listen Agent status endpoint: %w", err)
	}
	defer listener.Close()

	serveStatus := deps.serveStatus
	if serveStatus == nil {
		serveStatus = agenthealth.ServeStatus
	}

	var evidenceSources []agenthealth.EvidenceSource
	if deps.healthEvidenceSources != nil {
		evidenceSources = deps.healthEvidenceSources(statePath)
	}
	reporter := agenthealth.NewReporterWithEvidenceSources(
		agenthealth.RuntimeObservation{
			CollectorState: agenthealth.CollectorStateStarting,
		},
		evidenceSources...,
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
		statePath,
		teardown,
		reporter,
	)

	cancelRun()
	statusErr := <-statusDone

	if runtimeErr != nil {
		return runtimeErr
	}
	if statusErr != nil {
		return fmt.Errorf("run Agent status endpoint: %w", statusErr)
	}

	return nil
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
	additional ...string,
) (config.InlineLayer, error) {
	enabledDependencies := make(dependency.EnabledSet, len(state.Dependencies))
	for name, dependencyState := range state.Dependencies {
		if dependencyState.Enabled {
			enabledDependencies[name] = struct{}{}
		}
	}
	managedReceivers, err := catalog.CollectorReceivers(osName, enabledDependencies)
	if err != nil {
		return config.InlineLayer{}, err
	}

	receivers := append(
		[]string{"otlp", "hostmetrics"},
		managedReceivers...,
	)
	receivers = append(receivers, additional...)

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
