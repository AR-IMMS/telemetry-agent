package dependency

import (
	"context"
	"fmt"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

// Service coordinates dependency lookup, platform checks, and installation.
type Service struct {
	// Catalog is the single source of dependency definitions and installers.
	Catalog Catalog

	// OS is the normalized host operating system, for example "windows".
	OS string
}

// Install validates and installs one named dependency for the current platform.
func (s Service) Install(
	ctx context.Context,
	name string,
) (InstallResult, error) {
	normalizedName := strings.ToLower(strings.TrimSpace(name))

	integration, found := s.Catalog.Find(normalizedName)
	if !found {
		return InstallResult{}, fmt.Errorf(
			"unknown dependency %q",
			normalizedName,
		)
	}

	definition := integration.Definition
	normalizedOS := strings.ToLower(strings.TrimSpace(s.OS))

	if !definition.SupportsOS(normalizedOS) {
		return InstallResult{}, fmt.Errorf(
			"dependency %q does not support operating system %q",
			definition.Name,
			normalizedOS,
		)
	}

	if integration.Install == nil {
		return InstallResult{}, fmt.Errorf(
			"dependency %q installer is not configured",
			definition.Name,
		)
	}

	result, err := integration.Install(ctx)
	if err != nil {
		return InstallResult{}, fmt.Errorf(
			"install dependency %q: %w",
			definition.Name,
			err,
		)
	}

	// The catalog owns the canonical result name.
	result.Name = definition.Name

	return result, nil
}

// Status reads one dependency's lifecycle status for the current platform.
func (s Service) Status(
	ctx context.Context,
	name string,
) (Status, error) {
	normalizedName := strings.ToLower(strings.TrimSpace(name))

	integration, found := s.Catalog.Find(normalizedName)
	if !found {
		return Status{}, fmt.Errorf(
			"unknown dependency %q",
			normalizedName,
		)
	}

	definition := integration.Definition
	normalizedOS := strings.ToLower(strings.TrimSpace(s.OS))

	if !definition.SupportsOS(normalizedOS) {
		return Status{}, fmt.Errorf(
			"dependency %q does not support operating system %q",
			definition.Name,
			normalizedOS,
		)
	}

	if integration.Inspect == nil {
		return Status{}, fmt.Errorf(
			"dependency %q inspector is not configured",
			definition.Name,
		)
	}

	inspection, err := integration.Inspect(ctx)
	if err != nil {
		return Status{}, fmt.Errorf(
			"inspect dependency %q: %w",
			definition.Name,
			err,
		)
	}

	return inspection.Status(), nil
}

// ListStatus reads lifecycle statuses for all dependencies supported by the
// current operating system, ordered by the catalog.
func (s Service) ListStatus(
	ctx context.Context,
) ([]StatusResult, error) {
	definitions := s.Catalog.List(s.OS)

	results := make([]StatusResult, 0, len(definitions))

	for _, definition := range definitions {
		status, err := s.Status(ctx, definition.Name)
		if err != nil {
			results = append(results, StatusResult{
				Definition: definition,
				Status: Status{
					Availability: AvailabilityUnknown,
					Health:       HealthUnknown,
				},
				InspectionError: err,
			})

			continue
		}

		results = append(results, StatusResult{
			Definition: definition,
			Status:     status,
		})
	}

	return results, nil
}

// Enable starts one dependency only when its integration provides an
// Agent-owned-resource enable adapter.
func (s Service) Enable(
	ctx context.Context,
	name string,
	resources []agentstate.OwnedResource,
) error {
	normalizedName := strings.ToLower(strings.TrimSpace(name))

	integration, found := s.Catalog.Find(normalizedName)
	if !found {
		return fmt.Errorf("unknown dependency %q", normalizedName)
	}

	definition := integration.Definition
	normalizedOS := strings.ToLower(strings.TrimSpace(s.OS))

	if !definition.SupportsOS(normalizedOS) {
		return fmt.Errorf(
			"dependency %q does not support operating system %q",
			definition.Name,
			normalizedOS,
		)
	}

	if integration.Enable == nil {
		return fmt.Errorf(
			"dependency %q enabler is not configured",
			definition.Name,
		)
	}

	if err := integration.Enable(ctx, resources); err != nil {
		return fmt.Errorf(
			"enable dependency %q: %w",
			definition.Name,
			err,
		)
	}

	return nil
}

// Teardown routes an approved Agent-owned resource action to one supported
// dependency integration.
func (s Service) Teardown(
	ctx context.Context,
	name string,
	action agentstate.TeardownAction,
	resources []agentstate.OwnedResource,
) error {
	normalizedName := strings.ToLower(strings.TrimSpace(name))

	integration, found := s.Catalog.Find(normalizedName)
	if !found {
		return fmt.Errorf("unknown dependency %q", normalizedName)
	}

	definition := integration.Definition
	normalizedOS := strings.ToLower(strings.TrimSpace(s.OS))

	if !definition.SupportsOS(normalizedOS) {
		return fmt.Errorf(
			"dependency %q does not support operating system %q",
			definition.Name,
			normalizedOS,
		)
	}

	if integration.Teardown == nil {
		return fmt.Errorf(
			"dependency %q teardown is not configured",
			definition.Name,
		)
	}

	if err := integration.Teardown(ctx, action, resources); err != nil {
		return fmt.Errorf(
			"teardown dependency %q: %w",
			definition.Name,
			err,
		)
	}

	return nil
}
