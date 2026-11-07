package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

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
	install dependencyInstallFunc
}

func newManagedDependencyInstaller(
	collectPlatform func() (identity.PlatformInfo, error),
	install dependencyInstallFunc,
	runner bootstrap.CommandRunner,
) managedDependencyInstallFunc {
	_ = collectPlatform // platform data is consumed by Agent Service reconciliation.
	_ = runner
	installer := managedDependencyInstaller{install: install}

	return installer.Install
}

type managedDependencyLifecycle struct {
	enable dependencyEnableFunc
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

	_ = collectPlatform // platform data is consumed by Agent Service reconciliation.
	_ = runner
	lifecycle := managedDependencyLifecycle{enable: enable}

	return lifecycle.Apply
}

// Apply changes dependency state after preserving its ownership and teardown
// rules. The Agent Service observes SQLite and performs unified convergence.
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

	store := agentstate.NewStore(statePath)

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
	lifecycleLock, err := agentstate.AcquireLifecycleLock(
		state.Collector.ConfigPath + ".lifecycle",
	)
	if err != nil {
		return fmt.Errorf("acquire Agent lifecycle lock: %w", err)
	}
	lockHeld := true
	defer func() {
		if lockHeld {
			_ = lifecycleLock.Close()
		}
	}()

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

		_, err = store.Update(
			func(state *agentstate.State) error {
				_, err := state.EnableDependency(name)

				return err
			},
		)

	case dependencyLifecycleDisable:
		_, err = store.Update(
			func(state *agentstate.State) error {
				_, err := state.DisableDependency(name)

				return err
			},
		)

	case dependencyLifecycleUninstall:
		_, err = store.Update(
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
	if err := lifecycleLock.Close(); err != nil {
		return fmt.Errorf("release Agent lifecycle lock: %w", err)
	}
	lockHeld = false

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

	store := agentstate.NewStore(statePath)

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
	lifecycleLock, err := agentstate.AcquireLifecycleLock(
		state.Collector.ConfigPath + ".lifecycle",
	)
	if err != nil {
		return dependency.InstallResult{}, fmt.Errorf("acquire Agent lifecycle lock: %w", err)
	}
	lockHeld := true
	defer func() {
		if lockHeld {
			_ = lifecycleLock.Close()
		}
	}()

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
	if err := lifecycleLock.Close(); err != nil {
		return dependency.InstallResult{}, fmt.Errorf("release Agent lifecycle lock: %w", err)
	}
	lockHeld = false

	return result, nil
}
