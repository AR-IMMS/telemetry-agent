package agentstate

import (
	"database/sql"

	"github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation/contracts"
	activationdomain "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/domain"
	activationsqlite "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/sqlite"
)

type candidateOutcomeQueryer interface {
	QueryRow(string, ...any) *sql.Row
}

// CandidateOutcome reads the latest validated Runtime outcome for one exact target.
func (s SQLiteStore) CandidateOutcome(desiredRevision uint64, fingerprint, candidateDigest string) (activationdomain.CandidateMapping, uint64, bool, error) {
	db, err := s.open()
	if err != nil {
		return activationdomain.CandidateMapping{}, 0, false, err
	}
	defer db.Close()
	return readCandidateOutcome(db, desiredRevision, fingerprint, candidateDigest)
}

// CandidateOutcomeTx reads Runtime journal evidence using Configuration's active
// transaction, preserving the shared snapshot for reconciliation decisions.
func (s SQLiteStore) CandidateOutcomeTx(tx *sql.Tx, desiredRevision uint64, fingerprint, candidateDigest string) (contracts.CandidateOutcome, uint64, bool, error) {
	mapping, generation, found, err := readCandidateOutcome(tx, desiredRevision, fingerprint, candidateDigest)
	if err != nil || !found {
		return contracts.CandidateOutcome{}, generation, found, err
	}
	return contracts.CandidateOutcome{Outcome: mapping.Outcome, Stage: mapping.Stage, RecoveryResult: mapping.RecoveryResult}, generation, true, nil
}

func readCandidateOutcome(queryer candidateOutcomeQueryer, desiredRevision uint64, fingerprint, candidateDigest string) (activationdomain.CandidateMapping, uint64, bool, error) {
	mapping, generation, found, err := activationsqlite.ReadCandidateOutcome(queryer, desiredRevision, fingerprint, candidateDigest)
	if err != nil || !found {
		return activationdomain.CandidateMapping{}, generation, found, err
	}
	state, err := loadSQLiteState(queryer)
	if err != nil {
		return activationdomain.CandidateMapping{}, 0, false, err
	}
	if err := activationsqlite.ValidateCandidateAppliedGeneration(mapping, generation, state.AppliedGeneration); err != nil {
		return activationdomain.CandidateMapping{}, 0, false, err
	}
	return mapping, generation, true, nil
}
