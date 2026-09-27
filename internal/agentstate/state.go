package agentstate

import (
	"fmt"
	"strings"
)

// CollectorContext is the persisted runtime context required to render and
// supervise the local OpenTelemetry Collector.
type CollectorContext struct {
	ConfigRoot      string `json:"configRoot"`
	BinaryPath      string `json:"binaryPath"`
	ConfigPath      string `json:"configPath"`
	GatewayEndpoint string `json:"gatewayEndpoint"`
	HealthEndpoint  string `json:"healthEndpoint"`
}

// TeardownAction identifies the host-resource operation permitted after the
// Collector has acknowledged a configuration generation.
type TeardownAction string

const (
	TeardownActionDisable   TeardownAction = "disable"
	TeardownActionUninstall TeardownAction = "uninstall"
)

// PendingTeardown delays host-resource changes until the Collector has applied
// the configuration generation that removes its scrape receiver.
type PendingTeardown struct {
	Action     TeardownAction `json:"action"`
	Generation uint64         `json:"generation"`
}

// DependencyState is the desired lifecycle state of one managed dependency.
type DependencyState struct {
	Enabled         bool             `json:"enabled"`
	Ownership       *OwnershipRecord `json:"ownership,omitempty"`
	PendingTeardown *PendingTeardown `json:"pendingTeardown,omitempty"`
}

// State is the Agent-owned desired state persisted on the local machine.
type State struct {
	Collector           CollectorContext           `json:"collector"`
	DesiredGeneration   uint64                     `json:"desiredGeneration"`
	ActivatedGeneration uint64                     `json:"activatedGeneration"`
	AppliedGeneration   uint64                     `json:"appliedGeneration"`
	Dependencies        map[string]DependencyState `json:"dependencies"`
}

// OwnedResource identifies one host resource created and managed by the Agent.
type OwnedResource struct {
	Kind       string `json:"kind"`
	Identifier string `json:"identifier"`
}

// OwnershipRecord records resources that an Agent-managed installation owns.
type OwnershipRecord struct {
	Resources []OwnedResource `json:"resources"`
}

// RequiresApply reports whether the Collector must reload an activated
// configuration generation.
func (s State) RequiresApply() bool {
	return s.ActivatedGeneration > s.AppliedGeneration
}

// MarkGenerationApplied records that the Collector is healthy with one
// activated configuration generation.
func (s *State) MarkGenerationApplied(generation uint64) error {
	if generation > s.ActivatedGeneration {
		return fmt.Errorf(
			"applied generation %d exceeds activated generation %d",
			generation,
			s.ActivatedGeneration,
		)
	}
	if generation < s.AppliedGeneration {
		return fmt.Errorf(
			"applied generation %d is older than current applied generation %d",
			generation,
			s.AppliedGeneration,
		)
	}

	s.AppliedGeneration = generation

	return nil
}

// EnableDependency enables one dependency in desired state. It advances the
// generation only when the requested state actually changes.
func (s *State) EnableDependency(name string) (bool, error) {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if normalizedName == "" {
		return false, fmt.Errorf("dependency name is required")
	}

	if s.Dependencies == nil {
		s.Dependencies = make(map[string]DependencyState)
	}

	dependency := s.Dependencies[normalizedName]
	if dependency.Enabled {
		return false, nil
	}

	dependency.Enabled = true
	dependency.PendingTeardown = nil

	s.Dependencies[normalizedName] = dependency
	s.DesiredGeneration++

	return true, nil
}

// DisableDependency disables one dependency in desired state. It advances the
// generation only when the requested state actually changes.
func (s *State) DisableDependency(name string) (bool, error) {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if normalizedName == "" {
		return false, fmt.Errorf("dependency name is required")
	}

	dependency, exists := s.Dependencies[normalizedName]
	if !exists || !dependency.Enabled {
		return false, nil
	}

	dependency.Enabled = false
	s.DesiredGeneration++
	dependency.PendingTeardown = &PendingTeardown{
		Action:     TeardownActionDisable,
		Generation: s.DesiredGeneration,
	}
	s.Dependencies[normalizedName] = dependency

	return true, nil
}

// RequestUninstall removes one dependency from desired configuration and delays
// host-resource deletion until the Collector has acknowledged that generation.
func (s *State) RequestUninstall(name string) (bool, error) {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if normalizedName == "" {
		return false, fmt.Errorf("dependency name is required")
	}

	dependency, exists := s.Dependencies[normalizedName]
	if !exists {
		return false, fmt.Errorf(
			"dependency %q has no persisted Agent state",
			normalizedName,
		)
	}

	changed := false

	if dependency.Enabled {
		dependency.Enabled = false
		s.DesiredGeneration++
		changed = true
	}

	generation := s.DesiredGeneration
	if dependency.PendingTeardown == nil ||
		dependency.PendingTeardown.Action != TeardownActionUninstall ||
		dependency.PendingTeardown.Generation != generation {
		dependency.PendingTeardown = &PendingTeardown{
			Action:     TeardownActionUninstall,
			Generation: generation,
		}
		changed = true
	}

	s.Dependencies[normalizedName] = dependency

	return changed, nil
}

// CompleteTeardown records successful platform teardown after the Collector has
// acknowledged the generation that removed the dependency receiver.
func (s *State) CompleteTeardown(name string, generation uint64) error {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if normalizedName == "" {
		return fmt.Errorf("dependency name is required")
	}

	dependency, exists := s.Dependencies[normalizedName]
	if !exists {
		return fmt.Errorf(
			"dependency %q has no persisted Agent state",
			normalizedName,
		)
	}
	if dependency.PendingTeardown == nil {
		return fmt.Errorf(
			"dependency %q has no pending teardown",
			normalizedName,
		)
	}
	if dependency.PendingTeardown.Generation != generation {
		return fmt.Errorf(
			"dependency %q pending teardown generation is %d, not %d",
			normalizedName,
			dependency.PendingTeardown.Generation,
			generation,
		)
	}
	if s.AppliedGeneration < generation {
		return fmt.Errorf(
			"dependency %q teardown generation %d is not applied",
			normalizedName,
			generation,
		)
	}

	switch dependency.PendingTeardown.Action {
	case TeardownActionDisable:
		dependency.PendingTeardown = nil
		s.Dependencies[normalizedName] = dependency

	case TeardownActionUninstall:
		delete(s.Dependencies, normalizedName)

	default:
		return fmt.Errorf(
			"dependency %q has unsupported teardown action %q",
			normalizedName,
			dependency.PendingTeardown.Action,
		)
	}

	return nil
}

// RecordOwnership records the concrete resources created by one dependency
// installation. It does not change desired Collector configuration.
func (s *State) RecordOwnership(
	name string,
	resources []OwnedResource,
) error {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if normalizedName == "" {
		return fmt.Errorf("dependency name is required")
	}

	if len(resources) == 0 {
		return fmt.Errorf(
			"dependency %q ownership resources are required",
			normalizedName,
		)
	}

	for _, resource := range resources {
		if strings.TrimSpace(resource.Kind) == "" {
			return fmt.Errorf(
				"dependency %q owned resource kind is required",
				normalizedName,
			)
		}
		if strings.TrimSpace(resource.Identifier) == "" {
			return fmt.Errorf(
				"dependency %q owned resource identifier is required",
				normalizedName,
			)
		}
	}

	dependency, exists := s.Dependencies[normalizedName]
	if !exists {
		return fmt.Errorf(
			"dependency %q has no persisted Agent state",
			normalizedName,
		)
	}

	dependency.Ownership = &OwnershipRecord{
		Resources: append([]OwnedResource(nil), resources...),
	}
	s.Dependencies[normalizedName] = dependency

	return nil
}

// OwnsDependency reports whether Agent state contains at least one concrete
// resource ownership record for the named dependency.
func (s State) OwnsDependency(name string) bool {
	normalizedName := strings.ToLower(strings.TrimSpace(name))

	dependency, exists := s.Dependencies[normalizedName]

	return exists &&
		dependency.Ownership != nil &&
		len(dependency.Ownership.Resources) > 0
}

// MarkGenerationActivated records that the desired configuration generation
// was rendered, validated, and atomically activated.
func (s *State) MarkGenerationActivated(generation uint64) error {
	if generation != s.DesiredGeneration {
		return fmt.Errorf(
			"activated generation %d does not match desired generation %d",
			generation,
			s.DesiredGeneration,
		)
	}

	s.ActivatedGeneration = generation

	return nil
}
