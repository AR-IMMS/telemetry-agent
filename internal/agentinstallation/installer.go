package agentinstallation

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
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
	Platform         identity.PlatformInfo
	SourceBinaryPath string
	ConfigRoot       string
	GatewayEndpoint  string
	HealthEndpoint   string
	// OpAMPServerURL configures the default Supervisor owner on a new
	// installation. A new install requires this unless DisableSupervisor is set.
	OpAMPServerURL string
	// DisableSupervisor explicitly selects the legacy Agent Collector runtime.
	DisableSupervisor     bool
	ConfigInput           config.RenderInput
	ValidationEnvironment []string
	ValidationTimeout     time.Duration
}

type stateStore interface {
	Load() (agentstate.State, error)
	Update(agentstate.StateMutation) (agentstate.State, error)
}

type serviceInstaller interface {
	Install(context.Context, Layout) error
}

type serviceStopper interface {
	Stop(context.Context, Layout) error
}

type serviceInstallerFunc func(context.Context, Layout) error

// ValidateOpAMPServerURL accepts secure WSS URLs and loopback-only WS URLs.
func ValidateOpAMPServerURL(value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("OpAMP server URL is invalid")
	}
	if port := parsed.Port(); port != "" {
		parsedPort, portErr := strconv.Atoi(port)
		if portErr != nil || parsedPort < 1 || parsedPort > 65535 {
			return fmt.Errorf("OpAMP server URL port is invalid")
		}
	}
	switch strings.ToLower(parsed.Scheme) {
	case "wss":
		return nil
	case "ws":
		host := strings.ToLower(parsed.Hostname())
		if host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback() {
			return nil
		}
		return fmt.Errorf("cleartext OpAMP is allowed only on loopback")
	default:
		return fmt.Errorf("OpAMP server URL must use wss, or ws on loopback")
	}
}

func (f serviceInstallerFunc) Install(
	ctx context.Context,
	layout Layout,
) error {
	return f(ctx, layout)
}

// Installer coordinates the concrete effects of a packaged Agent installation.
type Installer struct {
	layoutFor         func(identity.PlatformInfo) (Layout, error)
	copyAgentBinary   func(context.Context, string, string) error
	bootstrap         func(context.Context, bootstrap.Options) (bootstrap.Result, error)
	installSupervisor func(context.Context, string, identity.PlatformInfo) (string, error)
	newStateStore     func(string) stateStore
	serviceFor        func(Layout) (serviceInstaller, error)
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
	if i.installSupervisor == nil {
		return Layout{}, fmt.Errorf("Supervisor installer is required")
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
	if options.OpAMPServerURL != "" {
		if err := ValidateOpAMPServerURL(options.OpAMPServerURL); err != nil {
			return Layout{}, err
		}
		options.OpAMPServerURL = strings.TrimSpace(options.OpAMPServerURL)
	}
	if options.DisableSupervisor && options.OpAMPServerURL != "" {
		return Layout{}, fmt.Errorf("OpAMP Supervisor cannot be disabled while an OpAMP endpoint is configured")
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
	store := i.newStateStore(layout.StatePath)
	previousState, previousErr := store.Load()
	if previousErr != nil && !os.IsNotExist(previousErr) {
		return Layout{}, fmt.Errorf("load Agent state before installation update: %w", previousErr)
	}
	opampServerURL, enabled, err := supervisorActivationForInstall(
		options.OpAMPServerURL,
		previousState,
		options.DisableSupervisor,
	)
	if err != nil {
		return Layout{}, err
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
	supervisorBinaryPath := previousState.Supervisor.BinaryPath
	if options.OpAMPServerURL != "" || enabled {
		supervisorBinaryPath, err = i.installSupervisor(ctx, layout.SupervisorInstallDirectory, options.Platform)
		if err != nil {
			return Layout{}, fmt.Errorf("install OpAMP Supervisor: %w", err)
		}
	}
	if previousState.Installation != nil {
		stopper, ok := service.(serviceStopper)
		if !ok {
			return Layout{}, fmt.Errorf("Agent service stop capability is required to replace an installed binary")
		}
		if err := stopper.Stop(ctx, layout); err != nil {
			return Layout{}, fmt.Errorf("stop existing Agent service before binary replacement: %w", err)
		}
	}
	if err := i.copyAgentBinary(ctx, options.SourceBinaryPath, layout.AgentBinaryPath); err != nil {
		if previousState.Installation != nil {
			if restartErr := service.Install(ctx, layout); restartErr != nil {
				return Layout{}, fmt.Errorf("stage Agent binary: %w; restart previous Agent service: %v", err, restartErr)
			}
		}
		return Layout{}, fmt.Errorf("stage Agent binary: %w", err)
	}

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
		state.Supervisor = agentstate.SupervisorContext{
			BinaryPath: supervisorBinaryPath, ConfigurationPath: layout.SupervisorConfigurationPath,
			StateDirectory: layout.SupervisorStateDirectory, OpAMPServerURL: opampServerURL,
			Enabled: enabled,
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

func supervisorActivationForInstall(requestedEndpoint string, previous agentstate.State, disableSupervisor bool) (string, bool, error) {
	if disableSupervisor {
		return "", false, nil
	}
	if strings.TrimSpace(requestedEndpoint) != "" {
		// Supplying the endpoint selects the default Supervisor owner for a new
		// install or explicitly migrates an existing legacy installation.
		return requestedEndpoint, true, nil
	}
	if previous.Installation != nil {
		if previous.Supervisor.Enabled {
			return previous.Supervisor.OpAMPServerURL, true, nil
		}
		return "", false, nil
	}
	return "", false, fmt.Errorf(
		"OpAMP server URL is required for a new installation using the default Supervisor runtime; explicitly disable Supervisor to select the legacy Agent runtime",
	)
}
