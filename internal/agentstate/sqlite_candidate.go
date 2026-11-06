package agentstate

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/coreconfig"
)

// CandidateAdmission binds one compiled candidate to the current durable
// Integration and M2 revisions before the lifecycle begins activation.
type CandidateAdmission struct {
	DesiredSetRevision  uint64
	ExpectedGeneration  uint64
	ExpectedFingerprint string
	VerifiedFingerprint string
	Mapping             CandidateMapping
	RecoveryPath        string
}

func markGatewayAppliedInTransaction(tx *sql.Tx, generation uint64, endpoint string) error {
	if err := coreconfig.ValidateGatewayEndpoint(endpoint); err != nil {
		return err
	}
	row, exists, err := loadCoreGatewayRow(tx)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	var lastGood string
	if err := tx.QueryRow(`SELECT last_good_endpoint FROM core_gateway_runtime WHERE id=1`).Scan(&lastGood); err != nil {
		return fmt.Errorf("load last-good Gateway endpoint: %w", err)
	}
	if row.generation <= generation && row.endpoint == endpoint {
		if _, err := tx.Exec(`UPDATE core_gateway_runtime SET last_good_endpoint=?,applied_revision=?,applied_generation=? WHERE id=1`, endpoint, row.revision, generation); err != nil {
			return fmt.Errorf("persist applied Core Gateway revision: %w", err)
		}
		return nil
	}
	if lastGood == endpoint {
		if _, err := tx.Exec(`UPDATE core_gateway_runtime SET applied_generation=? WHERE id=1`, generation); err != nil {
			return fmt.Errorf("persist applied local Gateway generation: %w", err)
		}
		return nil
	}
	return fmt.Errorf("Gateway endpoint changed while generation %d was starting", generation)
}

// CandidateAdmissionResult identifies the reserved generation or an existing
// already-applied generation for an idempotent request.
type CandidateAdmissionResult struct {
	Generation     uint64
	AlreadyApplied bool
}

// ErrStaleCandidate identifies a desired revision, M2 generation, or baseline
// fingerprint that changed before the candidate was admitted.
var ErrStaleCandidate = errors.New("candidate is stale")

// ErrStaleDesiredRevision indicates that the submitted Integration revision is
// no longer the current durable desired set.
var ErrStaleDesiredRevision = fmt.Errorf("%w: desired Integration revision changed", ErrStaleCandidate)

// ErrStaleM2Generation indicates that M2 configuration state changed before
// atomic candidate admission.
var ErrStaleM2Generation = fmt.Errorf("%w: M2 desired generation changed", ErrStaleCandidate)

// ErrStaleBaseline indicates that ordered baseline inputs changed after the
// candidate was compiled.
var ErrStaleBaseline = fmt.Errorf("%w: clean baseline changed", ErrStaleCandidate)

// ErrCandidateTerminalRequired prevents candidate AppliedGeneration from
// committing without its durable activation mapping in the same transaction.
var ErrCandidateTerminalRequired = errors.New("candidate terminal mapping is required")

// AdmitCandidate atomically verifies desired revisions, reserves or reuses an
// M2 generation, and persists the existing PendingActivation record.
func (s SQLiteStore) AdmitCandidate(request CandidateAdmission) (CandidateAdmissionResult, error) {
	if request.Mapping.CandidateDigest == "" || request.Mapping.EffectiveConfigDigest == "" || request.Mapping.BaselineFingerprint == "" || request.Mapping.CollectorVersion == "" || request.Mapping.ManifestVersion == "" || request.Mapping.Platform == "" {
		return CandidateAdmissionResult{}, fmt.Errorf("candidate digests, baseline fingerprint, artifact identity, and platform are required")
	}
	if request.Mapping.DesiredSetRevision != request.DesiredSetRevision || request.Mapping.BaselineFingerprint != request.ExpectedFingerprint {
		return CandidateAdmissionResult{}, fmt.Errorf("candidate mapping does not match admission revisions")
	}
	if request.ExpectedFingerprint == "" || request.ExpectedFingerprint != request.VerifiedFingerprint || request.Mapping.BaselineFingerprint != request.ExpectedFingerprint {
		return CandidateAdmissionResult{}, ErrStaleBaseline
	}
	sqliteUpdateMutex.Lock()
	defer sqliteUpdateMutex.Unlock()
	db, err := s.open()
	if err != nil {
		return CandidateAdmissionResult{}, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return CandidateAdmissionResult{}, fmt.Errorf("begin candidate admission: %w", err)
	}
	defer tx.Rollback()
	currentDesired, err := desiredIntegrationRevision(tx)
	if err != nil {
		return CandidateAdmissionResult{}, fmt.Errorf("read desired Integration revision: %w", err)
	}
	if currentDesired != request.DesiredSetRevision {
		return CandidateAdmissionResult{}, ErrStaleDesiredRevision
	}
	state, err := loadSQLiteState(tx)
	if err != nil {
		return CandidateAdmissionResult{}, err
	}
	if state.DesiredGeneration != request.ExpectedGeneration || state.PendingActivation != nil {
		return CandidateAdmissionResult{}, ErrStaleM2Generation
	}
	if generation, outcome, found, err := findCandidateTarget(tx, request.Mapping); err != nil {
		return CandidateAdmissionResult{}, err
	} else if found && outcome == "applied" && state.AppliedGeneration >= generation {
		if err := tx.Commit(); err != nil {
			return CandidateAdmissionResult{}, fmt.Errorf("finish idempotent candidate admission: %w", err)
		}
		return CandidateAdmissionResult{Generation: generation, AlreadyApplied: true}, nil
	} else if found && outcome != "applied" && generation == state.DesiredGeneration {
		state.PendingActivation = &PendingActivation{Generation: generation, RecoveryPath: request.RecoveryPath, Candidate: &request.Mapping}
		if err := saveCandidateState(tx, state); err != nil {
			return CandidateAdmissionResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return CandidateAdmissionResult{}, fmt.Errorf("commit candidate retry admission: %w", err)
		}
		return CandidateAdmissionResult{Generation: generation}, nil
	}
	if state.DesiredGeneration == ^uint64(0) {
		return CandidateAdmissionResult{}, fmt.Errorf("M2 desired generation overflow")
	}
	generation := state.DesiredGeneration + 1
	state.DesiredGeneration = generation
	state.PendingActivation = &PendingActivation{Generation: generation, RecoveryPath: request.RecoveryPath, Candidate: &request.Mapping}
	if err := saveCandidateState(tx, state); err != nil {
		return CandidateAdmissionResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return CandidateAdmissionResult{}, fmt.Errorf("commit candidate admission: %w", err)
	}
	return CandidateAdmissionResult{Generation: generation}, nil
}

// MarkCandidateGenerationApplied atomically commits AppliedGeneration and the
// candidate's authoritative terminal mapping after Collector readiness.
func (s SQLiteStore) MarkCandidateGenerationApplied(generation uint64, gatewayEndpoint string) error {
	sqliteUpdateMutex.Lock()
	defer sqliteUpdateMutex.Unlock()
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin candidate applied-state transaction: %w", err)
	}
	defer tx.Rollback()
	state, err := loadSQLiteState(tx)
	if err != nil {
		return err
	}
	pending := state.PendingActivation
	if pending == nil || pending.Generation != generation || pending.Candidate == nil {
		return fmt.Errorf("candidate mapping for generation %d is not pending", generation)
	}
	mapping := *pending.Candidate
	mapping.Outcome, mapping.Stage, mapping.RecoveryResult = "applied", "readiness", "not_needed"
	if err := state.MarkGenerationApplied(generation); err != nil {
		return err
	}
	if err := saveCandidateState(tx, state); err != nil {
		return err
	}
	if err := writeCandidateJournal(tx, generation, mapping); err != nil {
		return err
	}
	if gatewayEndpoint != "" {
		if err := markGatewayAppliedInTransaction(tx, generation, gatewayEndpoint); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit candidate applied state and mapping: %w", err)
	}
	return nil
}

// CompleteCandidateFailure stores terminal failure and recovery evidence while
// clearing pending state without advancing AppliedGeneration.
func (s SQLiteStore) CompleteCandidateFailure(generation uint64, stage, outcome, recovery string) error {
	if stage == "" || outcome == "" || recovery == "" {
		return fmt.Errorf("candidate failure stage, outcome, and recovery result are required")
	}
	sqliteUpdateMutex.Lock()
	defer sqliteUpdateMutex.Unlock()
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin candidate failure transaction: %w", err)
	}
	defer tx.Rollback()
	state, err := loadSQLiteState(tx)
	if err != nil {
		return err
	}
	pending := state.PendingActivation
	if pending == nil || pending.Generation != generation || pending.Candidate == nil {
		return fmt.Errorf("candidate mapping for generation %d is not pending", generation)
	}
	mapping := *pending.Candidate
	if state.ActivatedGeneration == generation && state.AppliedGeneration < generation {
		if err := state.RollbackUnappliedActivation(generation); err != nil {
			return err
		}
	}
	state.PendingActivation = nil
	mapping.Outcome, mapping.Stage, mapping.RecoveryResult = outcome, stage, recovery
	if err := saveCandidateState(tx, state); err != nil {
		return err
	}
	if err := writeCandidateJournal(tx, generation, mapping); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit candidate failure outcome: %w", err)
	}
	return nil
}

func findCandidateTarget(tx *sql.Tx, mapping CandidateMapping) (uint64, string, bool, error) {
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

func saveCandidateState(tx *sql.Tx, state State) error {
	content, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode Agent state: %w", err)
	}
	if _, err := tx.Exec(`UPDATE agent_state SET payload=?, updated_at=? WHERE id=1`, content, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("persist candidate lifecycle state: %w", err)
	}
	return nil
}

func writeCandidateJournal(tx *sql.Tx, generation uint64, mapping CandidateMapping) error {
	history, err := candidateJournalHistory(tx, generation, mapping)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO activation_journal (generation,outcome,updated_at,desired_set_revision,candidate_digest,baseline_fingerprint,effective_config_digest,collector_version,manifest_version,platform,stage,recovery_result,history_json) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(generation) DO UPDATE SET outcome=excluded.outcome,updated_at=excluded.updated_at,desired_set_revision=excluded.desired_set_revision,candidate_digest=excluded.candidate_digest,baseline_fingerprint=excluded.baseline_fingerprint,effective_config_digest=excluded.effective_config_digest,collector_version=excluded.collector_version,manifest_version=excluded.manifest_version,platform=excluded.platform,stage=excluded.stage,recovery_result=excluded.recovery_result,history_json=excluded.history_json`, generation, mapping.Outcome, time.Now().UTC().Format(time.RFC3339Nano), mapping.DesiredSetRevision, mapping.CandidateDigest, mapping.BaselineFingerprint, mapping.EffectiveConfigDigest, mapping.CollectorVersion, mapping.ManifestVersion, mapping.Platform, mapping.Stage, mapping.RecoveryResult, history)
	if err != nil {
		return fmt.Errorf("write candidate activation journal: %w", err)
	}
	return nil
}
