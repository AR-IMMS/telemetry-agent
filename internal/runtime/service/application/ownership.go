package application

import (
	"fmt"
	"strings"

	servicedomain "github.com/ar-imms/telemetry-agent/internal/runtime/service/domain"
)

// OwnershipSnapshot contains each known managed dependency and its recorded
// host resources. A nil record means the dependency exists but owns no resources.
type OwnershipSnapshot struct {
	// Dependencies maps each managed dependency name to its ownership record.
	Dependencies map[string]*servicedomain.OwnershipRecord
}

// RecordDependencyOwnership records validated resources for one existing
// dependency without changing its configuration generation.
func RecordDependencyOwnership(
	current OwnershipSnapshot,
	name string,
	resources []servicedomain.OwnedResource,
) (OwnershipSnapshot, error) {
	normalizedName := normalizeName(name)
	if normalizedName == "" {
		return current, fmt.Errorf("dependency name is required")
	}
	if len(resources) == 0 {
		return current, fmt.Errorf("dependency %q ownership resources are required", normalizedName)
	}
	for _, resource := range resources {
		if strings.TrimSpace(resource.Kind) == "" {
			return current, fmt.Errorf("dependency %q owned resource kind is required", normalizedName)
		}
		if strings.TrimSpace(resource.Identifier) == "" {
			return current, fmt.Errorf("dependency %q owned resource identifier is required", normalizedName)
		}
	}
	if _, exists := current.Dependencies[normalizedName]; !exists {
		return current, fmt.Errorf("dependency %q has no persisted Agent state", normalizedName)
	}

	next := cloneOwnershipSnapshot(current)
	next.Dependencies[normalizedName] = &servicedomain.OwnershipRecord{
		Resources: append([]servicedomain.OwnedResource(nil), resources...),
	}
	return next, nil
}

// OwnsDependency reports whether a known dependency has a concrete resource
// ownership record that permits safe cleanup of those resources.
func OwnsDependency(current OwnershipSnapshot, name string) bool {
	record, exists := current.Dependencies[normalizeName(name)]
	return exists && record != nil && len(record.Resources) > 0
}

// DependencyResources returns a defensive copy of the resources owned by one
// dependency, or nil when its ownership record is absent.
func DependencyResources(current OwnershipSnapshot, name string) []servicedomain.OwnedResource {
	record := current.Dependencies[normalizeName(name)]
	if record == nil {
		return nil
	}
	return append([]servicedomain.OwnedResource(nil), record.Resources...)
}

// ValidateAgentInstallation normalizes and validates the platform service and
// concrete owned resources recorded for an installed Agent.
func ValidateAgentInstallation(
	installation servicedomain.AgentInstallation,
) (servicedomain.AgentInstallation, error) {
	installation.Platform = normalizeName(installation.Platform)
	if installation.Platform == "" {
		return servicedomain.AgentInstallation{}, fmt.Errorf("Agent installation platform is required")
	}
	installation.ServiceName = strings.TrimSpace(installation.ServiceName)
	if installation.ServiceName == "" {
		return servicedomain.AgentInstallation{}, fmt.Errorf("Agent installation service name is required")
	}
	if len(installation.Ownership.Resources) == 0 {
		return servicedomain.AgentInstallation{}, fmt.Errorf("Agent installation ownership is required")
	}

	resources := make([]servicedomain.OwnedResource, 0, len(installation.Ownership.Resources))
	for _, resource := range installation.Ownership.Resources {
		resource.Kind = strings.TrimSpace(resource.Kind)
		resource.Identifier = strings.TrimSpace(resource.Identifier)
		if resource.Kind == "" {
			return servicedomain.AgentInstallation{}, fmt.Errorf("Agent installation owned resource kind is required")
		}
		if resource.Identifier == "" {
			return servicedomain.AgentInstallation{}, fmt.Errorf("Agent installation owned resource identifier is required")
		}
		resources = append(resources, resource)
	}
	installation.Ownership.Resources = resources
	return installation, nil
}

func cloneOwnershipSnapshot(current OwnershipSnapshot) OwnershipSnapshot {
	next := OwnershipSnapshot{
		Dependencies: make(map[string]*servicedomain.OwnershipRecord, len(current.Dependencies)),
	}
	for name, record := range current.Dependencies {
		if record == nil {
			next.Dependencies[name] = nil
			continue
		}
		next.Dependencies[name] = &servicedomain.OwnershipRecord{
			Resources: append([]servicedomain.OwnedResource(nil), record.Resources...),
		}
	}
	return next
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
