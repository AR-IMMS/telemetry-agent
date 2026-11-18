package agentlifecycle

import (
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

// configurationPlan is an immutable desired-state snapshot bound to one
// generation. Its fields stay private so compilers cannot change plan identity.
type configurationPlan struct {
	revision uint64
	snapshot agentstate.State
}

func newConfigurationPlan(state agentstate.State) configurationPlan {
	// Clone mutable nested state so the plan remains tied to the revision read
	// before configuration compilation and runtime activation begin.
	snapshot := state.Clone()

	return configurationPlan{
		revision: state.DesiredGeneration,
		snapshot: snapshot,
	}
}
