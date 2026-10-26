package agentlifecycle

import "github.com/ar-imms/telemetry-agent/internal/agentstate"

// configurationPlan is an immutable desired-state snapshot bound to one
// generation. Its fields stay private so compilers cannot change plan identity.
type configurationPlan struct {
	revision uint64
	snapshot agentstate.State
}

func newConfigurationPlan(state agentstate.State) configurationPlan {
	// Copy nested mutable fields so the plan remains tied to the revision read
	// before configuration compilation and runtime activation begin.
	snapshot := state
	if state.Dependencies != nil {
		snapshot.Dependencies = make(
			map[string]agentstate.DependencyState,
			len(state.Dependencies),
		)
		for name, dependency := range state.Dependencies {
			if dependency.PendingTeardown != nil {
				pending := *dependency.PendingTeardown
				dependency.PendingTeardown = &pending
			}
			if dependency.Ownership != nil {
				ownership := *dependency.Ownership
				ownership.Resources = append(
					[]agentstate.OwnedResource(nil),
					ownership.Resources...,
				)
				dependency.Ownership = &ownership
			}
			snapshot.Dependencies[name] = dependency
		}
	}
	if state.Installation != nil {
		installation := *state.Installation
		installation.Ownership.Resources = append(
			[]agentstate.OwnedResource(nil),
			installation.Ownership.Resources...,
		)
		snapshot.Installation = &installation
	}

	return configurationPlan{
		revision: state.DesiredGeneration,
		snapshot: snapshot,
	}
}
