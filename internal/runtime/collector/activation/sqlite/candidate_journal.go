package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	activationdomain "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/domain"
)

type candidateJournalQueryer interface {
	QueryRow(string, ...any) *sql.Row
}

// FindCandidateTarget returns the latest journal entry for one exact immutable
// candidate target. It does not create a transaction or change its caller's
// transaction boundary.
func FindCandidateTarget(tx *sql.Tx, mapping activationdomain.CandidateMapping) (uint64, string, bool, error) {
	var generation uint64
	var outcome string
	err := tx.QueryRow(`SELECT generation, outcome FROM activation_journal WHERE desired_set_revision=? AND baseline_fingerprint=? AND candidate_digest=? AND collector_version=? AND manifest_version=? AND platform=? ORDER BY generation DESC LIMIT 1`, mapping.DesiredSetRevision, mapping.BaselineFingerprint, mapping.CandidateDigest, mapping.CollectorVersion, mapping.ManifestVersion, mapping.Platform).Scan(&generation, &outcome)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("find candidate lifecycle target: %w", err)
	}
	return generation, outcome, true, nil
}

// ReadCandidateOutcome returns the latest outcome for an exact candidate and
// rejects applied evidence that exceeds the supplied Collector applied
// generation. The caller retains ownership of generation state and snapshots.
func ReadCandidateOutcome(
	queryer candidateJournalQueryer,
	desiredRevision uint64,
	fingerprint string,
	candidateDigest string,
) (activationdomain.CandidateMapping, uint64, bool, error) {
	var mapping activationdomain.CandidateMapping
	var generation uint64
	err := queryer.QueryRow(`SELECT generation,desired_set_revision,candidate_digest,baseline_fingerprint,COALESCE(effective_config_digest,''),COALESCE(collector_version,''),COALESCE(manifest_version,''),COALESCE(platform,''),outcome,COALESCE(stage,''),COALESCE(recovery_result,'') FROM activation_journal WHERE desired_set_revision=? AND baseline_fingerprint=? AND candidate_digest=? ORDER BY generation DESC LIMIT 1`, desiredRevision, fingerprint, candidateDigest).Scan(&generation, &mapping.DesiredSetRevision, &mapping.CandidateDigest, &mapping.BaselineFingerprint, &mapping.EffectiveConfigDigest, &mapping.CollectorVersion, &mapping.ManifestVersion, &mapping.Platform, &mapping.Outcome, &mapping.Stage, &mapping.RecoveryResult)
	if errors.Is(err, sql.ErrNoRows) {
		return activationdomain.CandidateMapping{}, 0, false, nil
	}
	if err != nil {
		return activationdomain.CandidateMapping{}, 0, false, err
	}
	return mapping, generation, true, nil
}

// ValidateCandidateAppliedGeneration ensures an applied journal outcome is
// backed by the Runtime activation generation reported by its state owner.
func ValidateCandidateAppliedGeneration(mapping activationdomain.CandidateMapping, generation, appliedGeneration uint64) error {
	if mapping.Outcome == "applied" && appliedGeneration < generation {
		return fmt.Errorf("M2 journal candidate outcome is not reflected in AppliedGeneration")
	}
	return nil
}

// WriteCandidateJournal upserts Runtime-owned candidate outcome evidence while
// preserving its previous terminal evidence in history. The caller supplies
// the transaction so journal updates remain atomic with activation state.
func WriteCandidateJournal(tx *sql.Tx, generation uint64, mapping activationdomain.CandidateMapping) error {
	history, err := CandidateJournalHistory(tx, generation, mapping.Outcome, mapping.Stage, mapping.RecoveryResult)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO activation_journal (generation,outcome,updated_at,desired_set_revision,candidate_digest,baseline_fingerprint,effective_config_digest,collector_version,manifest_version,platform,stage,recovery_result,history_json) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(generation) DO UPDATE SET outcome=excluded.outcome,updated_at=excluded.updated_at,desired_set_revision=excluded.desired_set_revision,candidate_digest=excluded.candidate_digest,baseline_fingerprint=excluded.baseline_fingerprint,effective_config_digest=excluded.effective_config_digest,collector_version=excluded.collector_version,manifest_version=excluded.manifest_version,platform=excluded.platform,stage=excluded.stage,recovery_result=excluded.recovery_result,history_json=excluded.history_json`, generation, mapping.Outcome, time.Now().UTC().Format(time.RFC3339Nano), mapping.DesiredSetRevision, mapping.CandidateDigest, mapping.BaselineFingerprint, mapping.EffectiveConfigDigest, mapping.CollectorVersion, mapping.ManifestVersion, mapping.Platform, mapping.Stage, mapping.RecoveryResult, history)
	if err != nil {
		return fmt.Errorf("write candidate activation journal: %w", err)
	}
	return nil
}

// RecordActivationTransition upserts a legacy activation or rollback outcome
// without changing candidate-specific columns or their evidence history.
func RecordActivationTransition(tx *sql.Tx, generation uint64, outcome string) error {
	if _, err := tx.Exec(
		`INSERT INTO activation_journal (generation, outcome, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(generation) DO UPDATE SET outcome = excluded.outcome, updated_at = excluded.updated_at`,
		generation,
		outcome,
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("record configuration activation: %w", err)
	}
	return nil
}
