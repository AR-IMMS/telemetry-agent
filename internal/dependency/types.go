package dependency

import (
	"context"
	"strings"
)

// Definition describes one built-in external dependency supported by agentctl.
type Definition struct {
	Name        string
	DisplayName string
	Description string
	SupportedOS []string
}

// InstallResult reports the outcome of a dependency installation attempt.
type InstallResult struct {
	Name   string
	Reused bool
}

// Installer installs or reconciles one dependency.
type Installer func(context.Context) (InstallResult, error)

// Integration binds one dependency definition to its concrete behavior.
type Integration struct {
	Definition Definition
	Install    Installer
}

// WindowsExporterInstaller preserves the existing Windows Exporter constructor
// contract while using the generic installer function type.
type WindowsExporterInstaller = Installer

// SupportsOS reports whether the dependency can run on the supplied OS name.
func (d Definition) SupportsOS(osName string) bool {
	normalizedOS := strings.ToLower(strings.TrimSpace(osName))

	for _, supportedOS := range d.SupportedOS {
		if normalizedOS == strings.ToLower(strings.TrimSpace(supportedOS)) {
			return true
		}
	}

	return false
}
