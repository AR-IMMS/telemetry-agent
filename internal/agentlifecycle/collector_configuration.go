package agentlifecycle

import (
	"context"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/config"
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

	rendered, err := config.Render(config.RenderInput{
		Platform: a.platform,
		Layers:   a.layers,
		InlineLayers: []config.InlineLayer{
			config.MetricsReceiverLayer(receivers),
		},
	})
	if err != nil {
		return fmt.Errorf(
			"render desired Collector configuration: %w",
			err,
		)
	}

	if err := bootstrap.ActivateRenderedConfig(
		ctx,
		a.runner,
		a.binaryPath,
		a.configPath,
		rendered,
		a.validationEnvironment,
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
	store agentstate.FileStore,
	options CollectorConfigurationOptions,
) error {
	activator := collectorConfigurationActivator{
		platform:              options.Platform,
		catalog:               options.Catalog,
		layers:                options.Layers,
		binaryPath:            options.BinaryPath,
		configPath:            options.ConfigPath,
		validationEnvironment: options.ValidationEnvironment,
		runner:                options.Runner,
	}

	coordinator := configurationCoordinator{
		store:    store,
		activate: activator.Activate,
	}

	return coordinator.Apply(ctx)
}
