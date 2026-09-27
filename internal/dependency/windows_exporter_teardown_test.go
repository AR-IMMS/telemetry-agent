package dependency

import (
	"context"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func TestWindowsExporterTeardownDisablesOwnedService(
	t *testing.T,
) {
	var commands []processCommand

	teardown := windowsExporterTeardown{
		options: DefaultWindowsExporterOptions(),
		administrator: func() (bool, error) {
			return true, nil
		},
		runProcess: func(
			_ context.Context,
			command processCommand,
		) error {
			commands = append(commands, command)

			return nil
		},
	}

	err := teardown.Teardown(
		context.Background(),
		agentstate.TeardownActionDisable,
		[]agentstate.OwnedResource{
			{
				Kind:       "windows-service",
				Identifier: windowsExporterServiceName,
			},
		},
	)
	if err != nil {
		t.Fatalf("Teardown() error = %v", err)
	}

	if len(commands) != 1 {
		t.Fatalf("command count = %d, want 1", len(commands))
	}

	command := commands[0]
	if command.Executable != "powershell.exe" {
		t.Fatalf(
			"command executable = %q, want powershell.exe",
			command.Executable,
		)
	}

	script := command.Args[len(command.Args)-1]
	for _, want := range []string{
		"Get-Service -Name 'windows_exporter'",
		"Stop-Service -Name 'windows_exporter'",
		"Set-Service -Name 'windows_exporter' -StartupType Disabled",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("PowerShell script = %q, want %q", script, want)
		}
	}
}

func TestWindowsExporterTeardownUninstallsOwnedResources(
	t *testing.T,
) {
	options := DefaultWindowsExporterOptions()
	var steps []string

	teardown := windowsExporterTeardown{
		options: options,
		administrator: func() (bool, error) {
			return true, nil
		},
		runProcess: func(
			_ context.Context,
			command processCommand,
		) error {
			script := command.Args[len(command.Args)-1]

			switch {
			case strings.Contains(
				script,
				"Set-Service -Name 'windows_exporter' -StartupType Disabled",
			):
				steps = append(steps, "disable-service")

			case strings.Contains(
				script,
				"Start-Process -FilePath 'msiexec.exe'",
			):
				steps = append(steps, "uninstall-msi")

			default:
				t.Fatalf("unexpected PowerShell script: %q", script)
			}

			return nil
		},
		removeFile: func(path string) error {
			if path != options.ConfigPath {
				t.Fatalf(
					"config path = %q, want %q",
					path,
					options.ConfigPath,
				)
			}

			steps = append(steps, "remove-config")

			return nil
		},

		removeDirectory: func(path string) error {
			if path != options.InstallDir {
				t.Fatalf(
					"installation directory = %q, want %q",
					path,
					options.InstallDir,
				)
			}

			steps = append(steps, "remove-install-directory")

			return nil
		},
	}

	err := teardown.Teardown(
		context.Background(),
		agentstate.TeardownActionUninstall,
		[]agentstate.OwnedResource{
			{
				Kind:       "windows-service",
				Identifier: windowsExporterServiceName,
			},
			{
				Kind:       "msi-product",
				Identifier: "windows_exporter",
			},
			{
				Kind:       "config-file",
				Identifier: options.ConfigPath,
			},
			{
				Kind:       "directory",
				Identifier: options.InstallDir,
			},
		},
	)
	if err != nil {
		t.Fatalf("Teardown() error = %v", err)
	}

	if got := strings.Join(steps, ","); got !=
		"disable-service,uninstall-msi,remove-config,remove-install-directory" {
		t.Fatalf(
			"steps = %q, want disable-service,uninstall-msi,remove-config,remove-install-directory",
			got,
		)
	}
}

func TestWindowsExporterTeardownUninstallPreservesAndRemovesOwnedInstallDirectory(
	t *testing.T,
) {
	options := DefaultWindowsExporterOptions()
	var steps []string

	teardown := windowsExporterTeardown{
		options: options,
		administrator: func() (bool, error) {
			return true, nil
		},
		runProcess: func(
			_ context.Context,
			command processCommand,
		) error {
			script := command.Args[len(command.Args)-1]

			switch {
			case strings.Contains(
				script,
				"Set-Service -Name 'windows_exporter' -StartupType Disabled",
			):
				steps = append(steps, "disable-service")

			case strings.Contains(
				script,
				"Start-Process -FilePath 'msiexec.exe'",
			):
				for _, want := range []string{
					".agentctl-uninstall-sentinel",
					"Set-Content -LiteralPath $sentinelPath",
				} {
					if !strings.Contains(script, want) {
						t.Fatalf(
							"MSI uninstall script = %q, want %q",
							script,
							want,
						)
					}
				}

				steps = append(steps, "uninstall-msi")

			default:
				t.Fatalf("unexpected PowerShell script: %q", script)
			}

			return nil
		},
		removeFile: func(path string) error {
			if path != options.ConfigPath {
				t.Fatalf(
					"config path = %q, want %q",
					path,
					options.ConfigPath,
				)
			}

			steps = append(steps, "remove-config")

			return nil
		},
		removeDirectory: func(path string) error {
			if path != options.InstallDir {
				t.Fatalf(
					"install directory = %q, want %q",
					path,
					options.InstallDir,
				)
			}

			steps = append(steps, "remove-install-directory")

			return nil
		},
	}

	err := teardown.Teardown(
		context.Background(),
		agentstate.TeardownActionUninstall,
		[]agentstate.OwnedResource{
			{
				Kind:       "windows-service",
				Identifier: windowsExporterServiceName,
			},
			{
				Kind:       "msi-product",
				Identifier: "windows_exporter",
			},
			{
				Kind:       "config-file",
				Identifier: options.ConfigPath,
			},
			{
				Kind:       "directory",
				Identifier: options.InstallDir,
			},
		},
	)
	if err != nil {
		t.Fatalf("Teardown() error = %v", err)
	}

	if got := strings.Join(steps, ","); got !=
		"disable-service,uninstall-msi,remove-config,remove-install-directory" {
		t.Fatalf(
			"steps = %q, want disable-service,uninstall-msi,remove-config,remove-install-directory",
			got,
		)
	}
}
