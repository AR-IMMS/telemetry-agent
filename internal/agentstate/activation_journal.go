package agentstate

import (
	"database/sql"

	activationdomain "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/domain"
	activationsqlite "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/sqlite"
)

func recordActivationTransition(
	tx *sql.Tx,
	previous State,
	current State,
) error {
	outcome := ""
	generation := current.ActivatedGeneration
	switch {
	case current.ActivatedGeneration > previous.ActivatedGeneration:
		outcome = "activated"
	case current.ActivatedGeneration < previous.ActivatedGeneration:
		outcome = "rolled_back"
		generation = previous.ActivatedGeneration
	}
	if outcome == "" {
		return nil
	}
	var candidate *activationdomain.CandidateMapping
	if current.PendingActivation != nil && current.PendingActivation.Generation == generation {
		candidate = current.PendingActivation.Candidate
	}
	if candidate == nil && previous.PendingActivation != nil && previous.PendingActivation.Generation == generation {
		candidate = previous.PendingActivation.Candidate
	}
	if candidate != nil {
		mapping := *candidate
		mapping.Outcome = outcome
		if outcome == "activated" {
			mapping.Stage = "activation"
		} else {
			mapping.Stage = "rollback"
			mapping.RecoveryResult = "recovered"
		}
		return activationsqlite.WriteCandidateJournal(tx, generation, mapping)
	}
	return activationsqlite.RecordActivationTransition(tx, generation, outcome)
}
