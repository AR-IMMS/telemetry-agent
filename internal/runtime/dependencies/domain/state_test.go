package dependencydomain

import (
	"encoding/json"
	"testing"
)

func TestDependencyStateKeepsPersistedJSONShape(t *testing.T) {
	state := DependencyState{
		Enabled: false,
		PendingTeardown: &PendingTeardown{
			Action:     TeardownActionDisable,
			Generation: 4,
		},
	}

	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal dependency state: %v", err)
	}

	const want = `{"enabled":false,"pendingTeardown":{"action":"disable","generation":4}}`
	if string(encoded) != want {
		t.Fatalf("dependency state JSON = %s, want %s", encoded, want)
	}
}
