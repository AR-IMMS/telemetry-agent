package agentstate

import activationdomain "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/domain"

func (s State) activationState() activationdomain.ActivationState {
	return activationdomain.ActivationState{
		DesiredGeneration:   s.DesiredGeneration,
		ActivatedGeneration: s.ActivatedGeneration,
		AppliedGeneration:   s.AppliedGeneration,
		PendingActivation:   s.PendingActivation,
	}
}

func (s *State) applyActivationState(state activationdomain.ActivationState) {
	s.DesiredGeneration = state.DesiredGeneration
	s.ActivatedGeneration = state.ActivatedGeneration
	s.AppliedGeneration = state.AppliedGeneration
	s.PendingActivation = state.PendingActivation
}

// BeginActivation records a durable recovery intent before config promotion.
func (s *State) BeginActivation(generation uint64, recoveryPath string) error {
	next, err := s.activationState().BeginActivation(generation, recoveryPath)
	if err != nil {
		return err
	}
	s.applyActivationState(next)
	return nil
}

// CompleteActivation clears the matching recovery intent when its revision commits.
func (s *State) CompleteActivation(generation uint64) error {
	next, err := s.activationState().CompleteActivation(generation)
	if err != nil {
		return err
	}
	s.applyActivationState(next)
	return nil
}

// MarkGenerationActivated records that the desired configuration generation
// was rendered, validated, and atomically activated.
func (s *State) MarkGenerationActivated(generation uint64) error {
	next, err := s.activationState().MarkGenerationActivated(generation)
	if err != nil {
		return err
	}
	s.applyActivationState(next)
	return nil
}

// MarkCandidateGenerationActivated commits an admitted candidate even when a
// newer desired generation arrived after admission. The pending candidate binds
// the generation, and the newer desired state remains outstanding for the next
// reconciliation pass.
func (s *State) MarkCandidateGenerationActivated(generation uint64) error {
	next, err := s.activationState().MarkCandidateGenerationActivated(generation)
	if err != nil {
		return err
	}
	s.applyActivationState(next)
	return nil
}

// RollbackUnappliedActivation restores the activated revision to the last
// Collector-applied revision while leaving desired state unchanged.
func (s *State) RollbackUnappliedActivation(generation uint64) error {
	next, err := s.activationState().RollbackUnappliedActivation(generation)
	if err != nil {
		return err
	}
	s.applyActivationState(next)
	return nil
}
