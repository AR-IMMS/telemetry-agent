package agentstate

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation/contracts"
	activationdomain "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/domain"
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
	var mapping activationdomain.CandidateMapping
	var generation uint64
	err := queryer.QueryRow(`SELECT generation,desired_set_revision,candidate_digest,baseline_fingerprint,COALESCE(effective_config_digest,''),COALESCE(collector_version,''),COALESCE(manifest_version,''),COALESCE(platform,''),outcome,COALESCE(stage,''),COALESCE(recovery_result,'') FROM activation_journal WHERE desired_set_revision=? AND baseline_fingerprint=? AND candidate_digest=? ORDER BY generation DESC LIMIT 1`, desiredRevision, fingerprint, candidateDigest).Scan(&generation, &mapping.DesiredSetRevision, &mapping.CandidateDigest, &mapping.BaselineFingerprint, &mapping.EffectiveConfigDigest, &mapping.CollectorVersion, &mapping.ManifestVersion, &mapping.Platform, &mapping.Outcome, &mapping.Stage, &mapping.RecoveryResult)
	if errors.Is(err, sql.ErrNoRows) {
		return activationdomain.CandidateMapping{}, 0, false, nil
	}
	if err != nil {
		return activationdomain.CandidateMapping{}, 0, false, err
	}
	state, err := loadSQLiteState(queryer)
	if err != nil {
		return activationdomain.CandidateMapping{}, 0, false, err
	}
	if mapping.Outcome == "applied" && state.AppliedGeneration < generation {
		return activationdomain.CandidateMapping{}, 0, false, fmt.Errorf("M2 journal candidate outcome is not reflected in AppliedGeneration")
	}
	return mapping, generation, true, nil
}
