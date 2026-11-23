package activationdomain

import "testing"

func TestActivationStateMarkAppliedClearsCoveredRecoveryIntent(t *testing.T) {
	pending := &PendingActivation{Generation: 4, RecoveryPath: "last-good.yaml"}
	state := ActivationState{
		DesiredGeneration:   5,
		ActivatedGeneration: 4,
		AppliedGeneration:   3,
		PendingActivation:   pending,
	}

	updated, err := state.MarkGenerationApplied(4)
	if err != nil {
		t.Fatalf("MarkGenerationApplied() error = %v", err)
	}
	if updated.AppliedGeneration != 4 || updated.PendingActivation != nil {
		t.Fatalf("updated state = %#v, want applied generation 4 and cleared recovery intent", updated)
	}
	if state.AppliedGeneration != 3 || state.PendingActivation != pending {
		t.Fatalf("input state was mutated: %#v", state)
	}
}
