package agentstate

import (
	"bytes"
	"encoding/json"
	"fmt"

	activationdomain "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/domain"
	collectordomain "github.com/ar-imms/telemetry-agent/internal/runtime/collector/domain"
	dependencydomain "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies/domain"
	opampdomain "github.com/ar-imms/telemetry-agent/internal/runtime/opamp/domain"
	servicedomain "github.com/ar-imms/telemetry-agent/internal/runtime/service/domain"
)

type persistedState struct {
	Collector           collectordomain.CollectorContext    `json:"collector"`
	Supervisor          opampdomain.SupervisorContext       `json:"supervisor,omitempty"`
	DesiredGeneration   uint64                              `json:"desiredGeneration"`
	ActivatedGeneration uint64                              `json:"activatedGeneration"`
	AppliedGeneration   uint64                              `json:"appliedGeneration"`
	PendingActivation   *activationdomain.PendingActivation `json:"pendingActivation,omitempty"`
	Dependencies        map[string]persistedDependencyState `json:"dependencies"`
	Installation        *servicedomain.AgentInstallation    `json:"installation,omitempty"`
}

type persistedDependencyState struct {
	Enabled         bool                              `json:"enabled"`
	Ownership       *servicedomain.OwnershipRecord    `json:"ownership,omitempty"`
	PendingTeardown *dependencydomain.PendingTeardown `json:"pendingTeardown,omitempty"`
}

// MarshalJSON preserves the historical Agent-state JSON layout while keeping
// Service-owned dependency resources out of the Dependencies domain model.
func (s State) MarshalJSON() ([]byte, error) {
	var dependencies map[string]persistedDependencyState
	if s.Dependencies != nil {
		dependencies = make(map[string]persistedDependencyState, len(s.Dependencies))
		for name, dependency := range s.Dependencies {
			dependencies[name] = persistedDependencyState{
				Enabled:         dependency.Enabled,
				Ownership:       s.dependencyOwnership[name],
				PendingTeardown: dependency.PendingTeardown,
			}
		}
	}
	return json.Marshal(persistedState{
		Collector:           s.Collector,
		Supervisor:          s.Supervisor,
		DesiredGeneration:   s.DesiredGeneration,
		ActivatedGeneration: s.ActivatedGeneration,
		AppliedGeneration:   s.AppliedGeneration,
		PendingActivation:   s.PendingActivation,
		Dependencies:        dependencies,
		Installation:        s.Installation,
	})
}

// UnmarshalJSON reads the historical Agent-state JSON layout and separates
// Service-owned dependency resources from Dependencies domain state.
func (s *State) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("Agent state receiver is required")
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	var persisted persistedState
	if err := json.Unmarshal(data, &persisted); err != nil {
		return err
	}
	dependencies := make(map[string]dependencydomain.DependencyState, len(persisted.Dependencies))
	var ownership map[string]*servicedomain.OwnershipRecord
	for name, dependency := range persisted.Dependencies {
		dependencies[name] = dependencydomain.DependencyState{
			Enabled:         dependency.Enabled,
			PendingTeardown: dependency.PendingTeardown,
		}
		if dependency.Ownership != nil {
			if ownership == nil {
				ownership = make(map[string]*servicedomain.OwnershipRecord)
			}
			ownership[name] = dependency.Ownership
		}
	}
	*s = State{
		Collector:           persisted.Collector,
		Supervisor:          persisted.Supervisor,
		DesiredGeneration:   persisted.DesiredGeneration,
		ActivatedGeneration: persisted.ActivatedGeneration,
		AppliedGeneration:   persisted.AppliedGeneration,
		PendingActivation:   persisted.PendingActivation,
		Dependencies:        dependencies,
		Installation:        persisted.Installation,
		dependencyOwnership: ownership,
	}
	if persisted.Dependencies == nil {
		s.Dependencies = nil
		s.dependencyOwnership = nil
	}
	return nil
}
