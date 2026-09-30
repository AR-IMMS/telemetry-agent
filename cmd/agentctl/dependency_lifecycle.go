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

type dependencyLifecycleAction string

const (
	dependencyLifecycleEnable    dependencyLifecycleAction = "enable"
	dependencyLifecycleDisable   dependencyLifecycleAction = "disable"
	dependencyLifecycleUninstall dependencyLifecycleAction = "uninstall"
)

type dependencyEnableFunc func(
	context.Context,
	string,
	[]agentstate.OwnedResource,
) error

type managedDependencyLifecycleFunc func(
	context.Context,
	string,
	string,
	dependencyLifecycleAction,
) error
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

type managedDependencyLifecycle struct {
	collectPlatform func() (identity.PlatformInfo, error)
	runner          bootstrap.CommandRunner
	enable          dependencyEnableFunc
}

func newManagedDependencyLifecycle(
	collectPlatform func() (identity.PlatformInfo, error),
	runner bootstrap.CommandRunner,
	enablers ...dependencyEnableFunc,
) managedDependencyLifecycleFunc {
	enable := defaultEnableDependency

	if len(enablers) == 1 {
		enable = enablers[0]
	}

	lifecycle := managedDependencyLifecycle{
		collectPlatform: collectPlatform,
		runner:          runner,
		enable:          enable,
	}

	return lifecycle.Apply
}

func (l managedDependencyLifecycle) Apply(
	ctx context.Context,
	name string,
	statePath string,
	action dependencyLifecycleAction,
) error {
	statePath = strings.TrimSpace(statePath)
	if statePath == "" {
		return fmt.Errorf("Agent lifecycle state path is required")
	}

	store := agentstate.NewFileStore(statePath)

	state, err := store.Load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf(
				"bootstrap the Collector before managing dependencies",
			)
		}

		return fmt.Errorf("load Agent lifecycle state: %w", err)
	}

	if strings.TrimSpace(state.Collector.ConfigRoot) == "" ||
		strings.TrimSpace(state.Collector.BinaryPath) == "" ||
		strings.TrimSpace(state.Collector.ConfigPath) == "" ||
		strings.TrimSpace(state.Collector.GatewayEndpoint) == "" {
		return fmt.Errorf(
			"bootstrap the Collector before managing dependencies",
		)
	}

	var updatedState agentstate.State

	switch action {
	case dependencyLifecycleEnable:
		normalizedName := strings.ToLower(strings.TrimSpace(name))
		dependencyState, exists := state.Dependencies[normalizedName]

		if !exists {
			return fmt.Errorf(
				"dependency %q has no persisted Agent state",
				name,
			)
		}

		if !dependencyState.Enabled {
			if dependencyState.Ownership == nil ||
				len(dependencyState.Ownership.Resources) == 0 {
				return fmt.Errorf(
					"dependency %q is disabled without Agent-owned resources; install it again",
					name,
				)
			}

			if l.enable == nil {
				return fmt.Errorf(
					"dependency lifecycle enable executor is required",
				)
			}

			if err := l.enable(
				ctx,
				name,
				dependencyState.Ownership.Resources,
			); err != nil {
				return fmt.Errorf(
					"enable dependency %q: %w",
					name,
					err,
				)
			}
		}

		updatedState, err = store.Update(
			func(state *agentstate.State) error {
				_, err := state.EnableDependency(name)

				return err
			},
		)

	case dependencyLifecycleDisable:
		updatedState, err = store.Update(
			func(state *agentstate.State) error {
				_, err := state.DisableDependency(name)

				return err
			},
		)

	case dependencyLifecycleUninstall:
		updatedState, err = store.Update(
			func(state *agentstate.State) error {
				_, err := state.RequestUninstall(name)

				return err
			},
		)

	default:
		return fmt.Errorf(
			"unsupported dependency lifecycle action %q",
			action,
		)
	}

	if err != nil {
		return fmt.Errorf(
			"update dependency %q lifecycle state: %w",
			name,
			err,
		)
	}

	if updatedState.DesiredGeneration ==
		updatedState.ActivatedGeneration {
		return nil
	}

	collectPlatform := l.collectPlatform
	if collectPlatform == nil {
		collectPlatform = identity.CollectPlatformInfo
	}

	platform, err := collectPlatform()
	if err != nil {
		return fmt.Errorf("collect platform information: %w", err)
	}

	catalog, err := defaultDependencyCatalog()
	if err != nil {
		return fmt.Errorf("build dependency catalog: %w", err)
	}

	layers, err := configurationLayers(
		updatedState.Collector.ConfigRoot,
		platform.OS,
	)
	if err != nil {
		return err
	}

	if err := agentlifecycle.ApplyDesiredConfiguration(
		ctx,
		store,
		agentlifecycle.CollectorConfigurationOptions{
			Platform:   platform,
			Catalog:    catalog,
			Layers:     layers,
			BinaryPath: updatedState.Collector.BinaryPath,
			ConfigPath: updatedState.Collector.ConfigPath,
			ValidationEnvironment: []string{
				"OTEL_GATEWAY_ENDPOINT=" +
					updatedState.Collector.GatewayEndpoint,
			},
			Runner: l.runner,
		},
	); err != nil {
		return fmt.Errorf(
			"activate dependency lifecycle configuration: %w",
			err,
		)
	}

	return nil
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
