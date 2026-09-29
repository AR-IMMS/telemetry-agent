package dependency

import (
	"context"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

// windowsExporterEnabler starts only the Agent-owned Windows Exporter service.
type windowsExporterEnabler struct {
	options       WindowsExporterOptions
	administrator administratorProbe
	runProcess    func(context.Context, processCommand) error
}

// Enable starts the recorded Windows Exporter service and restores automatic
// service startup.
func (e windowsExporterEnabler) Enable(
	ctx context.Context,
	resources []agentstate.OwnedResource,
) error {
	if ctx == nil {
		return fmt.Errorf("Windows Exporter enable context is required")
	}

	if err := e.options.Validate(); err != nil {
		return fmt.Errorf(
			"validate Windows Exporter enable options: %w",
			err,
		)
	}
	if err := requireWindowsAdministrator(e.administrator); err != nil {
		return err
	}
	if e.runProcess == nil {
		return fmt.Errorf(
			"Windows Exporter enable process runner is required",
		)
	}
	if !ownsWindowsExporterResource(
		resources,
		"windows-service",
		windowsExporterServiceName,
	) {
		return fmt.Errorf(
			"Windows Exporter enable requires owned service %q",
			windowsExporterServiceName,
		)
	}

	command := processCommand{
		Executable: "powershell.exe",
		Args: []string{
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			`$service = Get-Service -Name 'windows_exporter' -ErrorAction SilentlyContinue
if ($null -eq $service) {
  throw 'Windows Exporter service is missing'
}
Set-Service -Name 'windows_exporter' -StartupType Automatic -ErrorAction Stop
if ($service.Status -ne 'Running') {
  Start-Service -Name 'windows_exporter' -ErrorAction Stop
}`,
		},
	}

	if err := e.runProcess(ctx, command); err != nil {
		return fmt.Errorf("enable Windows Exporter service: %w", err)
	}

	return nil
}

// NewWindowsExporterEnabler creates the production Windows Exporter enable
// adapter.
func NewWindowsExporterEnabler(
	options WindowsExporterOptions,
) Enabler {
	return windowsExporterEnabler{
		options:       options,
		administrator: isWindowsAdministrator,
		runProcess:    runOSProcess,
	}.Enable
}
