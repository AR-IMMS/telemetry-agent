package agentstate

import (
	"fmt"
	"strings"

	activationdomain "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/domain"
	collectordomain "github.com/ar-imms/telemetry-agent/internal/runtime/collector/domain"
	dependencyapplication "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies/application"
	dependencydomain "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies/domain"
	opampdomain "github.com/ar-imms/telemetry-agent/internal/runtime/opamp/domain"
	serviceapplication "github.com/ar-imms/telemetry-agent/internal/runtime/service/application"
	servicedomain "github.com/ar-imms/telemetry-agent/internal/runtime/service/domain"
)

// State is the Agent-owned desired state persisted on the local machine.
type State struct {
	Collector           collectordomain.CollectorContext `json:"collector"`
	Supervisor          opampdomain.SupervisorContext    `json:"supervisor,omitempty"`
	DesiredGeneration   uint64                           `json:"desiredGeneration"`
	ActivatedGeneration uint64                           `json:"activatedGeneration"`
	AppliedGeneration   uint64                           `json:"appliedGeneration"`
	// PendingActivation supports recovery between config promotion and commit.
	PendingActivation *activationdomain.PendingActivation         `json:"pendingActivation,omitempty"`
	Dependencies      map[string]dependencydomain.DependencyState `json:"dependencies"`
	Installation      *servicedomain.AgentInstallation            `json:"installation,omitempty"`
}

// RequiresApply reports whether the Collector must reload an activated
// configuration generation.
func (s State) RequiresApply() bool {
	return s.activationState().RequiresApply()
}

// MarkGenerationApplied records that the Collector is healthy with one
// activated configuration generation.
func (s *State) MarkGenerationApplied(generation uint64) error {
	next, err := s.activationState().MarkGenerationApplied(generation)
	if err != nil {
		return err
	}
	s.applyActivationState(next)
	return nil
}

// EnableDependency enables one dependency in desired state. It advances the
// generation only when the requested state actually changes.
func (s *State) EnableDependency(name string) (bool, error) {
	next, changed, err := dependencyapplication.EnableDependency(s.dependencySnapshot(), name)
	if err != nil || !changed {
		return changed, err
	}
	s.applyDependencySnapshot(next)
	return true, nil
}

// DisableDependency disables one dependency in desired state. It advances the
// generation only when the requested state actually changes.
func (s *State) DisableDependency(name string) (bool, error) {
	next, changed, err := dependencyapplication.DisableDependency(s.dependencySnapshot(), name)
	if err != nil || !changed {
		return changed, err
	}
	s.applyDependencySnapshot(next)
	return true, nil
}

// RequestUninstall removes one dependency from desired configuration and delays
// host-resource deletion until the Collector has acknowledged that generation.
func (s *State) RequestUninstall(name string) (bool, error) {
	next, changed, err := dependencyapplication.RequestUninstall(s.dependencySnapshot(), name)
	if err != nil || !changed {
		return changed, err
	}
	s.applyDependencySnapshot(next)
	return true, nil
}

// CompleteTeardown records successful platform teardown after the Collector has
// acknowledged the generation that removed the dependency receiver.
func (s *State) CompleteTeardown(name string, generation uint64) error {
	next, err := dependencyapplication.CompleteTeardown(s.dependencySnapshot(), name, generation)
	if err != nil {
		return err
	}
	s.applyDependencySnapshot(next)
	return nil
}

// RecordOwnership records the concrete resources created by one dependency
// installation. It does not change desired Collector configuration.
func (s *State) RecordOwnership(
	name string,
	resources []servicedomain.OwnedResource,
) error {
	next, err := serviceapplication.RecordDependencyOwnership(s.dependencyOwnershipSnapshot(), name, resources)
	if err != nil {
		return err
	}
	s.applyDependencyOwnership(next)
	return nil
}

// OwnsDependency reports whether Agent state contains at least one concrete
// resource ownership record for the named dependency.
func (s State) OwnsDependency(name string) bool {
	return serviceapplication.OwnsDependency(s.dependencyOwnershipSnapshot(), name)
}

// RecordAgentInstallation persists validated platform and ownership metadata.
func (s *State) RecordAgentInstallation(
	installation servicedomain.AgentInstallation,
) error {
	platform := strings.ToLower(strings.TrimSpace(installation.Platform))
	if platform == "" {
		return fmt.Errorf("Agent installation platform is required")
	}

	serviceName := strings.TrimSpace(installation.ServiceName)
	if serviceName == "" {
		return fmt.Errorf("Agent installation service name is required")
	}

	if len(installation.Ownership.Resources) == 0 {
		return fmt.Errorf("Agent installation ownership is required")
	}

	resources := make(
		[]servicedomain.OwnedResource,
		0,
		len(installation.Ownership.Resources),
	)

	for _, resource := range installation.Ownership.Resources {
		resource.Kind = strings.TrimSpace(resource.Kind)
		resource.Identifier = strings.TrimSpace(resource.Identifier)

		if resource.Kind == "" {
			return fmt.Errorf(
				"Agent installation owned resource kind is required",
			)
		}
		if resource.Identifier == "" {
			return fmt.Errorf(
				"Agent installation owned resource identifier is required",
			)
		}

		resources = append(resources, resource)
	}

	s.Installation = &servicedomain.AgentInstallation{
		Platform:    platform,
		ServiceName: serviceName,
		BinaryPath:  installation.BinaryPath,
		Ownership: servicedomain.OwnershipRecord{
			Resources: resources,
		},
	}

	return nil
}

// RequestUninstalls removes multiple dependencies from desired configuration
// and schedules their host-resource deletion for one shared generation.
func (s *State) RequestUninstalls(
	names []string,
) ([]string, error) {
	next, changedNames, err := dependencyapplication.RequestUninstalls(s.dependencySnapshot(), names)
	if err != nil || len(changedNames) == 0 {
		return changedNames, err
	}
	s.applyDependencySnapshot(next)
	return changedNames, nil
}

func (s State) dependencySnapshot() dependencyapplication.Snapshot {
	return dependencyapplication.Snapshot{
		DesiredGeneration: s.DesiredGeneration,
		AppliedGeneration: s.AppliedGeneration,
		Dependencies:      s.Dependencies,
	}
}

func (s *State) applyDependencySnapshot(snapshot dependencyapplication.Snapshot) {
	s.DesiredGeneration = snapshot.DesiredGeneration
	s.Dependencies = snapshot.Dependencies
}

func (s State) dependencyOwnershipSnapshot() serviceapplication.OwnershipSnapshot {
	dependencies := make(map[string]*servicedomain.OwnershipRecord, len(s.Dependencies))
	for name, dependency := range s.Dependencies {
		dependencies[name] = dependency.Ownership
	}
	return serviceapplication.OwnershipSnapshot{Dependencies: dependencies}
}

func (s *State) applyDependencyOwnership(snapshot serviceapplication.OwnershipSnapshot) {
	for name, ownership := range snapshot.Dependencies {
		dependency, exists := s.Dependencies[name]
		if !exists {
			continue
		}
		dependency.Ownership = ownership
		s.Dependencies[name] = dependency
	}
}
