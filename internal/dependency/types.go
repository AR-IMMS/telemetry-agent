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
	// Inspect reads the current managed and runtime state without changing it.
	Inspect Inspector
}

// Inspector reads the current lifecycle state of one dependency.
type Inspector func(context.Context) (Inspection, error)

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

// Availability describes whether the Agent currently manages a dependency as
// enabled or disabled.
type Availability string

const (
	AvailabilityUnknown  Availability = "unknown"
	AvailabilityDisabled Availability = "disabled"
	AvailabilityEnabled  Availability = "enabled"
)

// Health describes the runtime condition of an enabled dependency.
type Health string

const (
	HealthUnknown   Health = "unknown"
	HealthHealthy   Health = "healthy"
	HealthUnhealthy Health = "unhealthy"
)

// Inspection is the normalized read-only state returned by a dependency
// adapter before it is presented to CLI callers.
type Inspection struct {
	Enabled bool
	Healthy bool
	Drifted bool
}

// Status is the user-facing lifecycle state of one dependency.
type Status struct {
	Availability Availability
	Health       Health
	Drifted      bool
}

// StatusResult pairs one dependency definition with its current lifecycle
// status while preserving catalog ordering.
type StatusResult struct {
	Definition      Definition
	Status          Status
	InspectionError error
}

// Status projects an inspection into the stable lifecycle vocabulary.
func (i Inspection) Status() Status {
	if !i.Enabled {
		return Status{
			Availability: AvailabilityDisabled,
			Health:       HealthUnknown,
		}
	}

	health := HealthUnhealthy
	if i.Healthy {
		health = HealthHealthy
	}

	return Status{
		Availability: AvailabilityEnabled,
		Health:       health,
		Drifted:      i.Drifted,
	}
}
