package dependency

import (
	"context"
	"fmt"
	"strings"
)

// WindowsExporterInstaller installs or reconciles the Windows Exporter dependency.
type WindowsExporterInstaller func(context.Context) (InstallResult, error)

// Service coordinates dependency lookup, platform checks, and installation.
type Service struct {
	// Registry lists the dependencies that this Agent release supports.
	Registry Registry

	// OS is the normalized host operating system, for example "windows".
	OS string

	// InstallWindowsExporter performs the Windows-specific installation work.
	InstallWindowsExporter WindowsExporterInstaller
}

// Install validates and installs one named dependency for the current platform.
func (s Service) Install(
	ctx context.Context,
	name string,
) (InstallResult, error) {
	// Normalize once so CLI input such as " Windows-Exporter " remains predictable.
	normalizedName := strings.ToLower(strings.TrimSpace(name))

	definition, found := s.Registry.Find(normalizedName)
	if !found {
		return InstallResult{}, fmt.Errorf(
			"unknown dependency %q",
			normalizedName,
		)
	}

	normalizedOS := strings.ToLower(strings.TrimSpace(s.OS))
	if !definition.SupportsOS(normalizedOS) {
		return InstallResult{}, fmt.Errorf(
			"dependency %q does not support operating system %q",
			definition.Name,
			normalizedOS,
		)
	}

	switch definition.Name {
	case windowsExporterDefinition.Name:
		if s.InstallWindowsExporter == nil {
			return InstallResult{}, fmt.Errorf(
				"dependency %q installer is not configured",
				definition.Name,
			)
		}

		result, err := s.InstallWindowsExporter(ctx)
		if err != nil {
			return InstallResult{}, fmt.Errorf(
				"install dependency %q: %w",
				definition.Name,
				err,
			)
		}

		// The dependency catalog owns the canonical result name.
		result.Name = definition.Name

		return result, nil

	default:
		return InstallResult{}, fmt.Errorf(
			"dependency %q has no installer",
			definition.Name,
		)
	}
}
