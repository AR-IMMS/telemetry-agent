package agentinstallation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

// Options contains the caller-provided inputs for one packaged Agent
// installation. Paths owned by the Agent are resolved from Platform.
type Options struct {
	Platform              identity.PlatformInfo
	SourceBinaryPath      string
	ConfigRoot            string
	GatewayEndpoint       string
	HealthEndpoint        string
	ConfigInput           config.RenderInput
	ValidationEnvironment []string
	ValidationTimeout     time.Duration
}

type stateStore interface {
	Update(agentstate.StateMutation) (agentstate.State, error)
}

type serviceInstaller interface {
	Install(context.Context, Layout) error
}

type serviceInstallerFunc func(context.Context, Layout) error

func (f serviceInstallerFunc) Install(
	ctx context.Context,
	layout Layout,
) error {
	return f(ctx, layout)
}

// Installer coordinates the concrete effects of a packaged Agent installation.
type Installer struct {
	layoutFor       func(identity.PlatformInfo) (Layout, error)
	copyAgentBinary func(context.Context, string, string) error
	bootstrap       func(context.Context, bootstrap.Options) (bootstrap.Result, error)
	newStateStore   func(string) stateStore
	serviceFor      func(Layout) (serviceInstaller, error)
}

// Install stages the durable Agent binary, bootstraps the Collector, persists
// ownership and Collector context, then enables and starts the Agent service.
func (i Installer) Install(
	ctx context.Context,
	options Options,
) (Layout, error) {
	if ctx == nil {
		return Layout{}, fmt.Errorf("Agent installation context is required")
	}
	if i.layoutFor == nil {
		return Layout{}, fmt.Errorf("Agent installation layout resolver is required")
	}
	if i.copyAgentBinary == nil {
		return Layout{}, fmt.Errorf("Agent binary copier is required")
	}
	if i.bootstrap == nil {
		return Layout{}, fmt.Errorf("Collector bootstrapper is required")
	}
	if i.newStateStore == nil {
		return Layout{}, fmt.Errorf("Agent state store factory is required")
	}
	if i.serviceFor == nil {
		return Layout{}, fmt.Errorf("Agent service resolver is required")
	}
	if strings.TrimSpace(options.SourceBinaryPath) == "" {
		return Layout{}, fmt.Errorf("Agent source binary path is required")
	}
	if strings.TrimSpace(options.ConfigRoot) == "" {
		return Layout{}, fmt.Errorf("Collector configuration root is required")
	}
	if strings.TrimSpace(options.GatewayEndpoint) == "" {
		return Layout{}, fmt.Errorf("Collector gateway endpoint is required")
	}
	if strings.TrimSpace(options.HealthEndpoint) == "" {
		return Layout{}, fmt.Errorf("Collector health endpoint is required")
	}
	if options.ValidationTimeout <= 0 {
		return Layout{}, fmt.Errorf(
			"Collector validation timeout must be positive",
		)
	}

	layout, err := i.layoutFor(options.Platform)
	if err != nil {
		return Layout{}, fmt.Errorf("resolve Agent installation layout: %w", err)
	}

	service, err := i.serviceFor(layout)
	if err != nil {
		return Layout{}, fmt.Errorf("resolve Agent service installer: %w", err)
	}
	if service == nil {
		return Layout{}, fmt.Errorf("Agent service installer is required")
	}

	if err := i.copyAgentBinary(
		ctx,
		options.SourceBinaryPath,
		layout.AgentBinaryPath,
	); err != nil {
		return Layout{}, fmt.Errorf("stage Agent binary: %w", err)
	}

	result, err := i.bootstrap(ctx, bootstrap.Options{
		Platform:    options.Platform,
		InstallDir:  layout.CollectorInstallDirectory,
		ConfigPath:  layout.CollectorConfigurationPath,
		ConfigInput: options.ConfigInput,
		ValidationEnvironment: append(
			[]string(nil),
			options.ValidationEnvironment...,
		),
		ValidationTimeout: options.ValidationTimeout,
	})
	if err != nil {
		return Layout{}, fmt.Errorf("bootstrap Collector: %w", err)
	}

	store := i.newStateStore(layout.StatePath)

	_, err = store.Update(func(state *agentstate.State) error {
		if state.Dependencies == nil {
			state.Dependencies = make(
				map[string]agentstate.DependencyState,
			)
		}

		state.Collector = agentstate.CollectorContext{
			ConfigRoot:      options.ConfigRoot,
			BinaryPath:      result.BinaryPath,
			ConfigPath:      result.ConfigPath,
			GatewayEndpoint: options.GatewayEndpoint,
			HealthEndpoint:  options.HealthEndpoint,
		}

		return state.RecordAgentInstallation(layout.Installation())
	})
	if err != nil {
		return Layout{}, fmt.Errorf(
			"persist Agent installation state: %w",
			err,
		)
	}

	if err := service.Install(ctx, layout); err != nil {
		return Layout{}, fmt.Errorf("install Agent service: %w", err)
	}

	return layout, nil
}
