package dependency

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

// windowsExporterTeardown disables or uninstalls Agent-owned Windows Exporter
// resources. The process runner is injected so teardown remains unit-testable.
type windowsExporterTeardown struct {
	options         WindowsExporterOptions
	administrator   administratorProbe
	runProcess      func(context.Context, processCommand) error
	removeFile      func(string) error
	removeDirectory func(string) error
}

// Teardown applies one safe lifecycle action to resources recorded in state.
func (t windowsExporterTeardown) Teardown(
	ctx context.Context,
	action agentstate.TeardownAction,
	resources []agentstate.OwnedResource,
) error {
	if ctx == nil {
		return fmt.Errorf("Windows Exporter teardown context is required")
	}
	if err := t.options.Validate(); err != nil {
		return fmt.Errorf(
			"validate Windows Exporter teardown options: %w",
			err,
		)
	}
	if err := requireWindowsAdministrator(t.administrator); err != nil {
		return err
	}
	if t.runProcess == nil {
		return fmt.Errorf("Windows Exporter teardown process runner is required")
	}

	switch action {
	case agentstate.TeardownActionDisable:
		return t.disableOwnedService(ctx, resources)

	case agentstate.TeardownActionUninstall:
		return t.uninstallOwnedResources(ctx, resources)

	default:
		return fmt.Errorf(
			"unsupported Windows Exporter teardown action %q",
			action,
		)
	}
}

func (t windowsExporterTeardown) disableOwnedService(
	ctx context.Context,
	resources []agentstate.OwnedResource,
) error {
	if !ownsWindowsExporterResource(
		resources,
		"windows-service",
		windowsExporterServiceName,
	) {
		return fmt.Errorf(
			"Windows Exporter disable requires owned service %q",
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
  exit 0
}
if ($service.Status -ne 'Stopped') {
  Stop-Service -Name 'windows_exporter' -ErrorAction Stop
}
Set-Service -Name 'windows_exporter' -StartupType Disabled -ErrorAction Stop`,
		},
	}

	if err := t.runProcess(ctx, command); err != nil {
		return fmt.Errorf("disable Windows Exporter service: %w", err)
	}

	return nil
}

func ownsWindowsExporterResource(
	resources []agentstate.OwnedResource,
	kind string,
	identifier string,
) bool {
	for _, resource := range resources {
		if resource.Kind == kind && resource.Identifier == identifier {
			return true
		}
	}

	return false
}

func (t windowsExporterTeardown) uninstallOwnedResources(
	ctx context.Context,
	resources []agentstate.OwnedResource,
) error {
	if t.removeFile == nil {
		return fmt.Errorf("Windows Exporter teardown file remover is required")
	}
	if t.removeDirectory == nil {
		return fmt.Errorf(
			"Windows Exporter teardown directory remover is required",
		)
	}
	if !ownsWindowsExporterResource(
		resources,
		"msi-product",
		"windows_exporter",
	) {
		return fmt.Errorf(
			"Windows Exporter uninstall requires owned MSI product",
		)
	}
	if !ownsWindowsExporterResource(
		resources,
		"config-file",
		t.options.ConfigPath,
	) {
		return fmt.Errorf(
			"Windows Exporter uninstall requires owned configuration file %q",
			t.options.ConfigPath,
		)
	}
	if !ownsWindowsExporterResource(
		resources,
		"directory",
		t.options.InstallDir,
	) {
		return fmt.Errorf(
			"Windows Exporter uninstall requires owned installation directory %q",
			t.options.InstallDir,
		)
	}

	if err := t.disableOwnedService(ctx, resources); err != nil {
		return err
	}

	installDirectory := strings.ReplaceAll(
		t.options.InstallDir,
		"'",
		"''",
	)

	uninstallCommand := processCommand{
		Executable: "powershell.exe",
		Args: []string{
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			fmt.Sprintf(
				`$installDirectory = '%s'
$sentinelPath = Join-Path $installDirectory '.agentctl-uninstall-sentinel'

New-Item -ItemType Directory -Path $installDirectory -Force |
  Out-Null

Set-Content -LiteralPath $sentinelPath -Value 'agentctl MSI uninstall sentinel' -NoNewline

$roots = @(
  'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall',
  'HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall'
)
$products = @(
  foreach ($root in $roots) {
    Get-ChildItem -Path $root -ErrorAction SilentlyContinue |
      ForEach-Object {
        $entry = Get-ItemProperty -LiteralPath $_.PSPath
        if ($entry.DisplayName -ceq 'windows_exporter' -and
            $entry.WindowsInstaller -eq 1) {
          $entry
        }
      }
  }
)
if ($products.Count -eq 0) {
  exit 0
}
if ($products.Count -ne 1) {
  throw 'multiple Windows Exporter MSI products were found'
}
$process = Start-Process -FilePath 'msiexec.exe' -ArgumentList @('/x', $products[0].PSChildName, '/qn', '/norestart') -Wait -PassThru
if ($process.ExitCode -ne 0) {
  throw "Windows Exporter MSI uninstall failed with exit code $($process.ExitCode)"
}`,
				installDirectory,
			),
		},
	}

	if err := t.runProcess(ctx, uninstallCommand); err != nil {
		return fmt.Errorf("uninstall Windows Exporter MSI: %w", err)
	}

	if err := t.removeFile(t.options.ConfigPath); err != nil {
		return fmt.Errorf(
			"remove Windows Exporter configuration: %w",
			err,
		)
	}

	if err := t.removeDirectory(t.options.InstallDir); err != nil {
		return fmt.Errorf(
			"remove Windows Exporter installation directory: %w",
			err,
		)
	}

	return nil
}

// NewWindowsExporterTeardown creates the production teardown adapter.
func NewWindowsExporterTeardown(
	options WindowsExporterOptions,
) Teardown {
	return windowsExporterTeardown{
		options:         options,
		administrator:   isWindowsAdministrator,
		runProcess:      runOSProcess,
		removeFile:      removeWindowsExporterFile,
		removeDirectory: removeWindowsExporterDirectory,
	}.Teardown
}

func removeWindowsExporterFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}

func removeWindowsExporterDirectory(path string) error {
	return os.RemoveAll(path)
}
