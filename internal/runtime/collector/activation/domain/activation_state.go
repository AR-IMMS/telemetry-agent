package activationdomain

import "fmt"

// ActivationState contains Collector configuration generations and recovery
// intent. It owns activation invariants but no persistence or process effects.
type ActivationState struct {
	DesiredGeneration   uint64
	ActivatedGeneration uint64
	AppliedGeneration   uint64
	PendingActivation   *PendingActivation
}

// RequiresApply reports whether a newer activated generation awaits Collector
// readiness confirmation.
func (s ActivationState) RequiresApply() bool {
	return s.ActivatedGeneration > s.AppliedGeneration
}

// MarkGenerationApplied returns state acknowledging a Collector-ready
// generation and clears any covered recovery intent.
func (s ActivationState) MarkGenerationApplied(generation uint64) (ActivationState, error) {
	if generation > s.ActivatedGeneration {
		return ActivationState{}, fmt.Errorf(
			"applied generation %d exceeds activated generation %d",
			generation,
			s.ActivatedGeneration,
		)
	}
	if generation < s.AppliedGeneration {
		return ActivationState{}, fmt.Errorf(
			"applied generation %d is older than current applied generation %d",
			generation,
			s.AppliedGeneration,
		)
	}

	s.AppliedGeneration = generation
	if s.PendingActivation != nil && s.PendingActivation.Generation <= generation {
		s.PendingActivation = nil
	}

	return s, nil
}

// BeginActivation returns state recording recovery intent before config
// promotion.
func (s ActivationState) BeginActivation(generation uint64, recoveryPath string) (ActivationState, error) {
	if generation != s.DesiredGeneration {
		return ActivationState{}, fmt.Errorf("activation intent does not match desired generation or recovery artifact")
	}
	s.PendingActivation = &PendingActivation{Generation: generation, RecoveryPath: recoveryPath}
	return s, nil
}

// CompleteActivation returns state with the matching recovery intent cleared.
func (s ActivationState) CompleteActivation(generation uint64) (ActivationState, error) {
	if s.PendingActivation == nil || s.PendingActivation.Generation != generation {
		return ActivationState{}, fmt.Errorf("activation intent for generation %d is not current", generation)
	}
	s.PendingActivation = nil
	return s, nil
}

// MarkGenerationActivated returns state with a validated desired generation
// recorded as activated.
func (s ActivationState) MarkGenerationActivated(generation uint64) (ActivationState, error) {
	if generation != s.DesiredGeneration {
		return ActivationState{}, fmt.Errorf(
			"activated generation %d does not match desired generation %d",
			generation,
			s.DesiredGeneration,
		)
	}
	s.ActivatedGeneration = generation
	return s, nil
}

// MarkCandidateGenerationActivated commits an admitted candidate even when a
// newer desired generation arrived after admission. The pending candidate binds
// the generation, and newer desired state remains for the next reconciliation.
func (s ActivationState) MarkCandidateGenerationActivated(generation uint64) (ActivationState, error) {
	if s.PendingActivation == nil || s.PendingActivation.Generation != generation || s.PendingActivation.Candidate == nil {
		return ActivationState{}, fmt.Errorf("candidate activation intent for generation %d is not current", generation)
	}
	if generation > s.DesiredGeneration {
		return ActivationState{}, fmt.Errorf("candidate generation %d exceeds desired generation %d", generation, s.DesiredGeneration)
	}
	if generation <= s.ActivatedGeneration {
		return ActivationState{}, fmt.Errorf("candidate generation %d is not newer than activated generation %d", generation, s.ActivatedGeneration)
	}
	s.ActivatedGeneration = generation
	return s, nil
}

// RollbackUnappliedActivation returns state restoring the applied generation
// when the current activated generation failed before readiness.
func (s ActivationState) RollbackUnappliedActivation(generation uint64) (ActivationState, error) {
	if generation != s.ActivatedGeneration {
		return ActivationState{}, fmt.Errorf(
			"failed activation generation %d is not current activated generation %d",
			generation,
			s.ActivatedGeneration,
		)
	}
	if generation <= s.AppliedGeneration {
		return ActivationState{}, fmt.Errorf(
			"activation generation %d is already applied at generation %d",
			generation,
			s.AppliedGeneration,
		)
	}

	s.ActivatedGeneration = s.AppliedGeneration
	return s, nil
}
