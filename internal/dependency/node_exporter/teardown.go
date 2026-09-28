package nodeexporter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

// nodeExporterTeardown applies safe lifecycle actions to resources owned by the
// Agent's Node Exporter installation.
type nodeExporterTeardown struct {
	options Options

	runSystemd      systemdRunner
	removeUnit      func(string) error
	removeDirectory func(string) error
}

// Teardown disables or uninstalls only recorded Node Exporter resources.
func (t nodeExporterTeardown) Teardown(
	ctx context.Context,
	action agentstate.TeardownAction,
	resources []agentstate.OwnedResource,
) error {
	if ctx == nil {
		return fmt.Errorf(
			"Node Exporter teardown context is required",
		)
	}

	if err := t.options.Validate(); err != nil {
		return fmt.Errorf(
			"validate Node Exporter teardown options: %w",
			err,
		)
	}
	if t.runSystemd == nil {
		return fmt.Errorf(
			"Node Exporter systemd runner is required",
		)
	}

	switch action {
	case agentstate.TeardownActionDisable:
		return t.disable(ctx, resources)

	case agentstate.TeardownActionUninstall:
		return t.uninstall(ctx, resources)

	default:
		return fmt.Errorf(
			"unsupported Node Exporter teardown action %q",
			action,
		)
	}
}

func (t nodeExporterTeardown) disable(
	ctx context.Context,
	resources []agentstate.OwnedResource,
) error {
	if !ownsSystemdUnit(resources, t.options.ServicePath) {
		return fmt.Errorf(
			"Node Exporter teardown does not own systemd unit %q",
			t.options.ServicePath,
		)
	}

	serviceName := filepath.Base(
		strings.TrimSpace(t.options.ServicePath),
	)

	if err := t.runSystemd(ctx, systemdCommand{
		Executable: "systemctl",
		Args: []string{
			"disable",
			"--now",
			serviceName,
		},
	}); err != nil {
		return fmt.Errorf(
			"disable Node Exporter systemd service: %w",
			err,
		)
	}

	return nil
}

func (t nodeExporterTeardown) uninstall(
	ctx context.Context,
	resources []agentstate.OwnedResource,
) error {
	if t.removeUnit == nil {
		return fmt.Errorf(
			"Node Exporter systemd unit remover is required",
		)
	}
	if t.removeDirectory == nil {
		return fmt.Errorf(
			"Node Exporter installation directory remover is required",
		)
	}
	if !ownsDirectory(resources, t.options.InstallDir) {
		return fmt.Errorf(
			"Node Exporter teardown does not own installation directory %q",
			t.options.InstallDir,
		)
	}

	if err := t.disable(ctx, resources); err != nil {
		return err
	}

	if err := t.removeUnit(t.options.ServicePath); err != nil {
		return fmt.Errorf(
			"remove Node Exporter systemd unit: %w",
			err,
		)
	}

	if err := t.removeDirectory(t.options.InstallDir); err != nil {
		return fmt.Errorf(
			"remove Node Exporter installation directory: %w",
			err,
		)
	}

	if err := t.runSystemd(ctx, systemdCommand{
		Executable: "systemctl",
		Args:       []string{"daemon-reload"},
	}); err != nil {
		return fmt.Errorf(
			"reload systemd after Node Exporter uninstall: %w",
			err,
		)
	}

	return nil
}

func ownsSystemdUnit(
	resources []agentstate.OwnedResource,
	servicePath string,
) bool {
	servicePath = strings.TrimSpace(servicePath)

	for _, resource := range resources {
		if resource.Kind == "systemd-unit" &&
			strings.TrimSpace(resource.Identifier) == servicePath {
			return true
		}
	}

	return false
}

func ownsDirectory(
	resources []agentstate.OwnedResource,
	directory string,
) bool {
	directory = strings.TrimSpace(directory)

	for _, resource := range resources {
		if resource.Kind == "directory" &&
			strings.TrimSpace(resource.Identifier) == directory {
			return true
		}
	}

	return false
}

// NewTeardown creates the production Node Exporter teardown adapter.
func NewTeardown(options Options) dependency.Teardown {
	return nodeExporterTeardown{
		options:         options,
		runSystemd:      newOSSystemdRunner(),
		removeUnit:      removeNodeExporterUnit,
		removeDirectory: os.RemoveAll,
	}.Teardown
}

func removeNodeExporterUnit(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}
