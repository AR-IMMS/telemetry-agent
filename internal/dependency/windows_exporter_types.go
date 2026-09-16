package dependency

import (
	"fmt"
	"strings"
)

// WindowsExporterOptions controls the local Windows Exporter installation.
type WindowsExporterOptions struct {
	// InstallDir is the directory where the MSI installs Windows Exporter.
	InstallDir string

	// ConfigPath is the Agent-owned exporter configuration file.
	ConfigPath string

	// ListenAddress is the loopback metrics endpoint, for example 127.0.0.1:9182.
	ListenAddress string
}

// WindowsExporterArtifact identifies one checksum-pinned MSI package.
type WindowsExporterArtifact struct {
	Version  string
	FileName string
	URL      string
	SHA256   string
}

// Validate rejects incomplete options before installation changes the machine.
func (o WindowsExporterOptions) Validate() error {
	if strings.TrimSpace(o.InstallDir) == "" {
		return fmt.Errorf("Windows Exporter installation directory is required")
	}
	if strings.TrimSpace(o.ConfigPath) == "" {
		return fmt.Errorf("Windows Exporter configuration path is required")
	}
	if strings.TrimSpace(o.ListenAddress) == "" {
		return fmt.Errorf("Windows Exporter listen address is required")
	}

	return nil
}

var windowsExporterDefinition = Definition{
	Name:        "windows-exporter",
	SupportedOS: []string{"windows"},
}

// DefaultWindowsExporterOptions returns the machine-wide locations owned by
// the Agent for a standard Windows Exporter installation.
func DefaultWindowsExporterOptions() WindowsExporterOptions {
	return WindowsExporterOptions{
		InstallDir:    `C:\Program Files\windows_exporter`,
		ConfigPath:    `C:\ProgramData\AR-IMMS\windows-exporter\config.yaml`,
		ListenAddress: "127.0.0.1:9182",
	}
}
