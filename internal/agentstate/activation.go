package agentstate

import (
	"fmt"

	activationdomain "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/domain"
)

// BeginActivation records a durable recovery intent before config promotion.
func (s *State) BeginActivation(generation uint64, recoveryPath string) error {
	if generation != s.DesiredGeneration {
		return fmt.Errorf("activation intent does not match desired generation or recovery artifact")
	}
	s.PendingActivation = &activationdomain.PendingActivation{Generation: generation, RecoveryPath: recoveryPath}
	return nil
}

// CompleteActivation clears the matching recovery intent when its revision commits.
func (s *State) CompleteActivation(generation uint64) error {
	if s.PendingActivation == nil || s.PendingActivation.Generation != generation {
		return fmt.Errorf("activation intent for generation %d is not current", generation)
	}
	s.PendingActivation = nil
	return nil
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

// MarkCandidateGenerationActivated commits an admitted candidate even when a
// newer desired generation arrived after admission. The pending candidate binds
// the generation, and the newer desired state remains outstanding for the next
// reconciliation pass.
func (s *State) MarkCandidateGenerationActivated(generation uint64) error {
	if s.PendingActivation == nil || s.PendingActivation.Generation != generation || s.PendingActivation.Candidate == nil {
		return fmt.Errorf("candidate activation intent for generation %d is not current", generation)
	}
	if generation > s.DesiredGeneration {
		return fmt.Errorf("candidate generation %d exceeds desired generation %d", generation, s.DesiredGeneration)
	}
	if generation <= s.ActivatedGeneration {
		return fmt.Errorf("candidate generation %d is not newer than activated generation %d", generation, s.ActivatedGeneration)
	}
	s.ActivatedGeneration = generation
	return nil
}

// RollbackUnappliedActivation restores the activated revision to the last
// Collector-applied revision while leaving desired state unchanged.
func (s *State) RollbackUnappliedActivation(generation uint64) error {
	if generation != s.ActivatedGeneration {
		return fmt.Errorf(
			"failed activation generation %d is not current activated generation %d",
			generation,
			s.ActivatedGeneration,
		)
	}
	if generation <= s.AppliedGeneration {
		return fmt.Errorf(
			"activation generation %d is already applied at generation %d",
			generation,
			s.AppliedGeneration,
		)
	}

	s.ActivatedGeneration = s.AppliedGeneration
	return nil
}
