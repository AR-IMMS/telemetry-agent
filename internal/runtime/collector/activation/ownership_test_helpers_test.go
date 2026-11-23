package agentlifecycle

import (
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	servicedomain "github.com/ar-imms/telemetry-agent/internal/runtime/service/domain"
)

func stateWithDependencyOwnership(
	t *testing.T,
	state agentstate.State,
	name string,
	resources []servicedomain.OwnedResource,
) agentstate.State {
	t.Helper()
	if err := state.RecordOwnership(name, resources); err != nil {
		t.Fatalf("record dependency ownership: %v", err)
	}
	return state
}
