package agentstate

import (
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
	PendingActivation   *activationdomain.PendingActivation         `json:"pendingActivation,omitempty"`
	Dependencies        map[string]dependencydomain.DependencyState `json:"-"`
	Installation        *servicedomain.AgentInstallation            `json:"installation,omitempty"`
	dependencyOwnership map[string]*servicedomain.OwnershipRecord
}

// Clone returns an independent copy of mutable state used to prepare one
// Collector configuration plan.
func (s State) Clone() State {
	cloned := s
	if s.Dependencies != nil {
		cloned.Dependencies = make(map[string]dependencydomain.DependencyState, len(s.Dependencies))
		for name, dependency := range s.Dependencies {
			if dependency.PendingTeardown != nil {
				pending := *dependency.PendingTeardown
				dependency.PendingTeardown = &pending
			}
			cloned.Dependencies[name] = dependency
		}
	}
	if s.dependencyOwnership != nil {
		cloned.dependencyOwnership = make(map[string]*servicedomain.OwnershipRecord, len(s.dependencyOwnership))
		for name, record := range s.dependencyOwnership {
			if record == nil {
				cloned.dependencyOwnership[name] = nil
				continue
			}
			cloned.dependencyOwnership[name] = &servicedomain.OwnershipRecord{
				Resources: append([]servicedomain.OwnedResource(nil), record.Resources...),
			}
		}
	}
	if s.Installation != nil {
		installation := *s.Installation
		installation.Ownership.Resources = append([]servicedomain.OwnedResource(nil), installation.Ownership.Resources...)
		cloned.Installation = &installation
	}
	if s.PendingActivation != nil {
		pending := *s.PendingActivation
		if s.PendingActivation.Candidate != nil {
			candidate := *s.PendingActivation.Candidate
			pending.Candidate = &candidate
		}
		cloned.PendingActivation = &pending
	}
	return cloned
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

// OwnedDependencyResources returns a copy of the concrete host resources
// recorded for one dependency, or nil when the Agent owns none.
func (s State) OwnedDependencyResources(name string) []servicedomain.OwnedResource {
	return serviceapplication.DependencyResources(s.dependencyOwnershipSnapshot(), name)
}

// RecordAgentInstallation persists validated platform and ownership metadata.
func (s *State) RecordAgentInstallation(
	installation servicedomain.AgentInstallation,
) error {
	validated, err := serviceapplication.ValidateAgentInstallation(installation)
	if err != nil {
		return err
	}
	s.Installation = &validated
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
	dependencies := make(map[string]dependencydomain.DependencyState, len(s.Dependencies))
	for name, dependency := range s.Dependencies {
		dependencies[name] = dependency
	}
	return dependencyapplication.Snapshot{
		DesiredGeneration: s.DesiredGeneration,
		AppliedGeneration: s.AppliedGeneration,
		Dependencies:      dependencies,
	}
}

func (s *State) applyDependencySnapshot(snapshot dependencyapplication.Snapshot) {
	s.DesiredGeneration = snapshot.DesiredGeneration
	s.Dependencies = snapshot.Dependencies
	for name := range s.dependencyOwnership {
		if _, exists := s.Dependencies[name]; !exists {
			delete(s.dependencyOwnership, name)
		}
	}
}

func (s State) dependencyOwnershipSnapshot() serviceapplication.OwnershipSnapshot {
	dependencies := make(map[string]*servicedomain.OwnershipRecord, len(s.Dependencies))
	for name := range s.Dependencies {
		dependencies[name] = s.dependencyOwnership[name]
	}
	return serviceapplication.OwnershipSnapshot{Dependencies: dependencies}
}

func (s *State) applyDependencyOwnership(snapshot serviceapplication.OwnershipSnapshot) {
	s.dependencyOwnership = snapshot.Dependencies
}
