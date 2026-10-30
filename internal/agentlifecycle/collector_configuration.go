package agentlifecycle

import (
	"context"
	"fmt"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/coreconfig"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

// collectorConfigurationActivator renders and activates the Collector
// configuration represented by one desired dependency-state snapshot.
type collectorConfigurationActivator struct {
	platform identity.PlatformInfo
	catalog  dependency.Catalog
	layers   []config.Layer

	binaryPath            string
	configPath            string
	validationEnvironment []string
	runner                bootstrap.CommandRunner
	gatewayRepository     coreconfig.GatewayRepository
}

// CollectorConfigurationOptions supplies the dependencies required to render
// and activate a Collector configuration from persisted desired state.
type CollectorConfigurationOptions struct {
	Platform identity.PlatformInfo
	Catalog  dependency.Catalog
	Layers   []config.Layer

	BinaryPath            string
	ConfigPath            string
	ValidationEnvironment []string
	Runner                bootstrap.CommandRunner
	// GatewayRepository resolves Bootstrap/Core ownership for validation.
	GatewayRepository coreconfig.GatewayRepository
}

// Activate renders enabled dependency receivers and atomically activates the
// resulting Collector configuration after native validation succeeds.
func (a collectorConfigurationActivator) Activate(
	ctx context.Context,
	state agentstate.State,
) error {
	managedReceivers, err := a.catalog.CollectorReceivers(
		a.platform.OS,
		state,
	)
	if err != nil {
		return fmt.Errorf(
			"resolve managed dependency receivers: %w",
			err,
		)
	}

	receivers := append(
		[]string{"otlp", "hostmetrics"},
		managedReceivers...,
	)
	readinessToken, err := config.NewReadinessToken()
	if err != nil {
		return err
	}

	rendered, err := config.Render(config.RenderInput{
		Platform: a.platform,
		Layers:   a.layers,
		InlineLayers: []config.InlineLayer{
			config.MetricsReceiverLayer(receivers),
			config.ReadinessLayer(readinessToken),
		},
	})
	if err != nil {
		return fmt.Errorf(
			"render desired Collector configuration: %w",
			err,
		)
	}

	validationEnvironment := append([]string(nil), a.validationEnvironment...)
	if a.gatewayRepository != nil {
		gateway, err := a.gatewayRepository.EffectiveGateway()
		if err != nil {
			return fmt.Errorf("resolve effective Gateway endpoint for configuration validation: %w", err)
		}
		if err := coreconfig.ValidateGatewayEndpoint(gateway.Endpoint); err != nil {
			return fmt.Errorf("validate effective Gateway endpoint: %w", err)
		}
		validationEnvironment = replaceEnvironmentValue(validationEnvironment, "OTEL_GATEWAY_ENDPOINT", gateway.Endpoint)
	}

	if err := bootstrap.ActivateRenderedConfig(
		ctx,
		a.runner,
		a.binaryPath,
		a.configPath,
		rendered,
		validationEnvironment,
	); err != nil {
		return fmt.Errorf(
			"activate desired Collector configuration: %w",
			err,
		)
	}

	return nil
}

// ApplyDesiredConfiguration renders, validates, activates, and acknowledges
// the current desired dependency configuration as one generation.
func ApplyDesiredConfiguration(
	ctx context.Context,
	store agentstate.Repository,
	options CollectorConfigurationOptions,
) error {
	before, err := store.Load()
	if err != nil {
		return fmt.Errorf("load state before configuration activation: %w", err)
	}
	activator := collectorConfigurationActivator{
		platform:              options.Platform,
		catalog:               options.Catalog,
		layers:                options.Layers,
		binaryPath:            options.BinaryPath,
		configPath:            options.ConfigPath,
		validationEnvironment: options.ValidationEnvironment,
		runner:                options.Runner,
		gatewayRepository:     options.GatewayRepository,
	}

	coordinator := configurationCoordinator{
		store:    store,
		activate: activator.Activate,
		lockPath: options.ConfigPath + ".lifecycle",
		prepare: func(state agentstate.State) (string, error) {
			return bootstrap.PrepareConfigRecoveryArtifact(
				options.ConfigPath,
				state.DesiredGeneration,
			)
		},
		recover: func(path string) error {
			return bootstrap.RestoreConfigArtifact(options.ConfigPath, path)
		},
		rollback: func() error {
			return bootstrap.RestoreLastKnownGoodConfig(options.ConfigPath)
		},
	}

	if err := coordinator.Apply(ctx); err != nil {
		return err
	}
	activated, err := store.Load()
	if err != nil {
		return fmt.Errorf("load state after configuration activation: %w", err)
	}
	if before.ActivatedGeneration == activated.ActivatedGeneration {
		return nil
	}
	if gatewayRuntime, ok := options.GatewayRepository.(coreconfig.GatewayRuntimeRepository); ok {
		state, err := store.Load()
		if err != nil {
			return fmt.Errorf("load activated state for Core Gateway mapping: %w", err)
		}
		if err := gatewayRuntime.MarkGatewayActivated(state.ActivatedGeneration); err != nil {
			return fmt.Errorf("record activated Core Gateway revision: %w", err)
		}
	}
	return nil
}

func replaceEnvironmentValue(environment []string, key, value string) []string {
	updated := make([]string, 0, len(environment)+1)
	prefix := key + "="
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		updated = append(updated, entry)
	}
	return append(updated, prefix+value)
}
