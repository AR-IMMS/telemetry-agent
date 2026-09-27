package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentlifecycle"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

// managedDependencyInstallFunc installs one dependency after confirming that
// Collector context has been persisted by a successful bootstrap.
type managedDependencyInstallFunc func(
	context.Context,
	string,
	string,
) (dependency.InstallResult, error)

type managedDependencyInstaller struct {
	collectPlatform func() (identity.PlatformInfo, error)
	install         dependencyInstallFunc
	runner          bootstrap.CommandRunner
}

func newManagedDependencyInstaller(
	collectPlatform func() (identity.PlatformInfo, error),
	install dependencyInstallFunc,
	runner bootstrap.CommandRunner,
) managedDependencyInstallFunc {
	installer := managedDependencyInstaller{
		collectPlatform: collectPlatform,
		install:         install,
		runner:          runner,
	}

	return installer.Install
}

// Install refuses to create machine resources until bootstrap has persisted
// the Collector context required to activate managed receiver configuration.
func (i managedDependencyInstaller) Install(
	ctx context.Context,
	name string,
	statePath string,
) (dependency.InstallResult, error) {
	statePath = strings.TrimSpace(statePath)
	if statePath == "" {
		return dependency.InstallResult{}, fmt.Errorf(
			"Agent lifecycle state path is required",
		)
	}

	store := agentstate.NewFileStore(statePath)

	state, err := store.Load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return dependency.InstallResult{}, fmt.Errorf(
				"bootstrap the Collector before managing dependencies",
			)
		}

		return dependency.InstallResult{}, fmt.Errorf(
			"load Agent lifecycle state: %w",
			err,
		)
	}

	if strings.TrimSpace(state.Collector.ConfigRoot) == "" ||
		strings.TrimSpace(state.Collector.BinaryPath) == "" ||
		strings.TrimSpace(state.Collector.ConfigPath) == "" ||
		strings.TrimSpace(state.Collector.GatewayEndpoint) == "" {
		return dependency.InstallResult{}, fmt.Errorf(
			"bootstrap the Collector before managing dependencies",
		)
	}

	if i.install == nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"dependency installer is not configured",
		)
	}

	collectPlatform := i.collectPlatform
	if collectPlatform == nil {
		collectPlatform = identity.CollectPlatformInfo
	}

	platform, err := collectPlatform()
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

	layers, err := configurationLayers(
		state.Collector.ConfigRoot,
		platform.OS,
	)
	if err != nil {
		return dependency.InstallResult{}, err
	}

	result, err := i.install(ctx, name)
	if err != nil {
		return dependency.InstallResult{}, err
	}

	_, err = store.Update(func(state *agentstate.State) error {
		_, err := state.EnableDependency(name)
		if err != nil {
			return err
		}

		if len(result.OwnedResources) == 0 {
			return nil
		}

		return state.RecordOwnership(
			name,
			result.OwnedResources,
		)
	})

	if err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"enable dependency %q in Agent state: %w",
			name,
			err,
		)
	}

	if err := agentlifecycle.ApplyDesiredConfiguration(
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
			Runner: i.runner,
		},
	); err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"activate dependency configuration: %w",
			err,
		)
	}

	return result, nil
}
