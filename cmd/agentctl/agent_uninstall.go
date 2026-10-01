package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentinstallation"
	"github.com/ar-imms/telemetry-agent/internal/agentlifecycle"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

type dependencyUninstallRequestFunc func(
	context.Context,
	string,
) error

type dependencyTeardownWaitFunc func(
	context.Context,
	string,
) (agentstate.State, error)

type agentRemoverFactory func(
	identity.PlatformInfo,
) (agentinstallation.Remover, error)

// managedAgentUninstaller coordinates the safe dependency teardown and final
// removal of the Agent installation.
type managedAgentUninstaller struct {
	collectPlatform     func() (identity.PlatformInfo, error)
	currentExecutable   func() (string, error)
	resolveExecutable   func(string) (string, error)
	requestUninstalls   dependencyUninstallRequestFunc
	waitForDependencies dependencyTeardownWaitFunc
	newRemover          agentRemoverFactory
}

func (u managedAgentUninstaller) Uninstall(
	ctx context.Context,
	statePath string,
	timeout time.Duration,
) error {
	if ctx == nil {
		return fmt.Errorf("Agent uninstall context is required")
	}

	statePath = strings.TrimSpace(statePath)
	if statePath == "" {
		return fmt.Errorf("Agent lifecycle state path is required")
	}
	if timeout <= 0 {
		return fmt.Errorf("Agent uninstall timeout must be positive")
	}

	currentExecutable := u.currentExecutable
	if currentExecutable == nil {
		currentExecutable = os.Executable
	}

	resolveExecutable := u.resolveExecutable
	if resolveExecutable == nil {
		resolveExecutable = filepath.EvalSymlinks
	}

	requestUninstalls := u.requestUninstalls
	if requestUninstalls == nil {
		return fmt.Errorf("dependency uninstall requester is required")
	}

	waitForDependencies := u.waitForDependencies
	if waitForDependencies == nil {
		return fmt.Errorf("dependency teardown waiter is required")
	}

	newRemover := u.newRemover
	if newRemover == nil {
		newRemover = agentinstallation.NewRemover
	}

	executablePath, err := currentExecutable()
	if err != nil {
		return fmt.Errorf("resolve current Agent executable: %w", err)
	}

	executablePath, err = resolveExecutable(executablePath)
	if err != nil {
		return fmt.Errorf("resolve current Agent executable path: %w", err)
	}

	store := agentstate.NewFileStore(statePath)
	state, err := store.Load()
	if err != nil {
		return fmt.Errorf("load Agent state: %w", err)
	}
	if state.Installation == nil {
		return fmt.Errorf("Agent installation is not recorded in state")
	}

	if err := agentinstallation.ValidateExternalUninstaller(
		executablePath,
		*state.Installation,
	); err != nil {
		return err
	}

	collectPlatform := u.collectPlatform
	if collectPlatform == nil {
		collectPlatform = identity.CollectPlatformInfo
	}

	platform, err := collectPlatform()
	if err != nil {
		return fmt.Errorf("collect platform information: %w", err)
	}
	if !strings.EqualFold(platform.OS, state.Installation.Platform) {
		return fmt.Errorf(
			"installed Agent platform %q does not match current platform %q",
			state.Installation.Platform,
			platform.OS,
		)
	}

	uninstallContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := requestUninstalls(uninstallContext, statePath); err != nil {
		return fmt.Errorf("request dependency uninstalls: %w", err)
	}

	finalState, err := waitForDependencies(uninstallContext, statePath)
	if err != nil {
		return fmt.Errorf("wait for dependency teardowns: %w", err)
	}
	if len(finalState.Dependencies) != 0 {
		return fmt.Errorf(
			"dependency teardowns are incomplete: %d dependencies remain",
			len(finalState.Dependencies),
		)
	}
	if finalState.Installation == nil {
		return fmt.Errorf(
			"Agent installation is not recorded after dependency teardown",
		)
	}

	remover, err := newRemover(platform)
	if err != nil {
		return fmt.Errorf("resolve Agent remover: %w", err)
	}

	if err := remover.Remove(
		uninstallContext,
		*finalState.Installation,
		statePath,
	); err != nil {
		return fmt.Errorf("remove Agent installation: %w", err)
	}

	return nil
}

type dependencyUninstallAllRequestFunc func(
	context.Context,
	agentstate.FileStore,
	agentlifecycle.CollectorConfigurationOptions,
) ([]string, error)

type managedAgentUninstallRequester struct {
	collectPlatform func() (identity.PlatformInfo, error)
	buildCatalog    func() (dependency.Catalog, error)
	layersFor       func(string, string) ([]config.Layer, error)
	runner          bootstrap.CommandRunner
	requestAll      dependencyUninstallAllRequestFunc
}

func (r managedAgentUninstallRequester) Request(
	ctx context.Context,
	statePath string,
) error {
	if ctx == nil {
		return fmt.Errorf("dependency uninstall request context is required")
	}

	statePath = strings.TrimSpace(statePath)
	if statePath == "" {
		return fmt.Errorf("Agent lifecycle state path is required")
	}

	store := agentstate.NewFileStore(statePath)
	state, err := store.Load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf(
				"Agent lifecycle state was not found; install the Agent first",
			)
		}

		return fmt.Errorf("load Agent state: %w", err)
	}

	if strings.TrimSpace(state.Collector.ConfigRoot) == "" ||
		strings.TrimSpace(state.Collector.BinaryPath) == "" ||
		strings.TrimSpace(state.Collector.ConfigPath) == "" ||
		strings.TrimSpace(state.Collector.GatewayEndpoint) == "" {
		return fmt.Errorf(
			"Collector context is not recorded; bootstrap the Collector first",
		)
	}

	collectPlatform := r.collectPlatform
	if collectPlatform == nil {
		collectPlatform = identity.CollectPlatformInfo
	}

	platform, err := collectPlatform()
	if err != nil {
		return fmt.Errorf("collect platform information: %w", err)
	}

	buildCatalog := r.buildCatalog
	if buildCatalog == nil {
		buildCatalog = defaultDependencyCatalog
	}

	catalog, err := buildCatalog()
	if err != nil {
		return fmt.Errorf("build dependency catalog: %w", err)
	}

	layersFor := r.layersFor
	if layersFor == nil {
		layersFor = configurationLayers
	}

	layers, err := layersFor(state.Collector.ConfigRoot, platform.OS)
	if err != nil {
		return fmt.Errorf("resolve Collector configuration layers: %w", err)
	}

	runner := r.runner
	if runner == nil {
		runner = bootstrap.OSCommandRunner{}
	}

	requestAll := r.requestAll
	if requestAll == nil {
		requestAll = agentlifecycle.RequestAllDependencyUninstalls
	}

	if _, err := requestAll(
		ctx,
		store,
		agentlifecycle.CollectorConfigurationOptions{
			Platform:   platform,
			Catalog:    catalog,
			Layers:     layers,
			BinaryPath: state.Collector.BinaryPath,
			ConfigPath: state.Collector.ConfigPath,
			ValidationEnvironment: []string{
				"OTEL_GATEWAY_ENDPOINT=" +
					state.Collector.GatewayEndpoint,
			},
			Runner: runner,
		},
	); err != nil {
		return fmt.Errorf("request all dependency uninstalls: %w", err)
	}

	return nil
}

const dependencyTeardownPollInterval = 250 * time.Millisecond

func newManagedAgentUninstaller(
	collectPlatform func() (identity.PlatformInfo, error),
	currentExecutable func() (string, error),
	runner bootstrap.CommandRunner,
) agentUninstallFunc {
	requester := managedAgentUninstallRequester{
		collectPlatform: collectPlatform,
		runner:          runner,
	}

	uninstaller := managedAgentUninstaller{
		collectPlatform:   collectPlatform,
		currentExecutable: currentExecutable,
		requestUninstalls: requester.Request,
		waitForDependencies: func(
			ctx context.Context,
			statePath string,
		) (agentstate.State, error) {
			return agentlifecycle.WaitForDependenciesRemoved(
				ctx,
				agentstate.NewFileStore(statePath),
				dependencyTeardownPollInterval,
			)
		},
		newRemover: agentinstallation.NewRemover,
	}

	return uninstaller.Uninstall
}
