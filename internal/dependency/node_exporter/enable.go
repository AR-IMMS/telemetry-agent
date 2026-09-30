package nodeexporter

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

// nodeExporterEnabler starts the Agent-owned Node Exporter systemd unit.
type nodeExporterEnabler struct {
	options    Options
	runSystemd systemdRunner
}

// Enable restores systemd enablement and starts the owned Node Exporter unit.
func (e nodeExporterEnabler) Enable(
	ctx context.Context,
	resources []agentstate.OwnedResource,
) error {
	if ctx == nil {
		return fmt.Errorf("Node Exporter enable context is required")
	}

	if err := e.options.Validate(); err != nil {
		return fmt.Errorf(
			"validate Node Exporter enable options: %w",
			err,
		)
	}
	if e.runSystemd == nil {
		return fmt.Errorf("Node Exporter systemd runner is required")
	}
	if !ownsSystemdUnit(resources, e.options.ServicePath) {
		return fmt.Errorf(
			"Node Exporter enable requires owned systemd unit %q",
			e.options.ServicePath,
		)
	}

	serviceName := filepath.Base(
		strings.TrimSpace(e.options.ServicePath),
	)

	if err := e.runSystemd(ctx, systemdCommand{
		Executable: "systemctl",
		Args: []string{
			"enable",
			"--now",
			serviceName,
		},
	}); err != nil {
		return fmt.Errorf(
			"enable Node Exporter systemd service: %w",
			err,
		)
	}

	return nil
}

// NewEnabler creates the production Node Exporter enable adapter.
func NewEnabler(options Options) dependency.Enabler {
	return nodeExporterEnabler{
		options:    options,
		runSystemd: newOSSystemdRunner(),
	}.Enable
}
