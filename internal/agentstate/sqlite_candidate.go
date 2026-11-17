package agentstate

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	platformsqlite "github.com/ar-imms/telemetry-agent/internal/platform/sqlite"
	activationapp "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/application"
	activationdomain "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/domain"
	activationsqlite "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/sqlite"
)

// ErrCandidateTerminalRequired prevents candidate AppliedGeneration from
// committing without its durable activation mapping in the same transaction.
var ErrCandidateTerminalRequired = errors.New("candidate terminal mapping is required")

// AdmitCandidate atomically verifies desired revisions, reserves or reuses an
// M2 generation, and persists the existing activationdomain.PendingActivation record.
func (s SQLiteStore) AdmitCandidate(request activationapp.CandidateAdmissionRequest) (activationapp.AdmissionResult, error) {
	if request.Mapping.CandidateDigest == "" || request.Mapping.EffectiveConfigDigest == "" || request.Mapping.BaselineFingerprint == "" || request.Mapping.CollectorVersion == "" || request.Mapping.ManifestVersion == "" || request.Mapping.Platform == "" {
		return activationapp.AdmissionResult{}, fmt.Errorf("candidate digests, baseline fingerprint, artifact identity, and platform are required")
	}
	if request.Mapping.DesiredSetRevision != request.DesiredSetRevision || request.Mapping.BaselineFingerprint != request.ExpectedFingerprint {
		return activationapp.AdmissionResult{}, fmt.Errorf("candidate mapping does not match admission revisions")
	}
	if request.ExpectedFingerprint == "" || request.ExpectedFingerprint != request.VerifiedFingerprint || request.Mapping.BaselineFingerprint != request.ExpectedFingerprint {
		return activationapp.AdmissionResult{}, activationapp.ErrStaleBaseline
	}
	locker := platformsqlite.UpdateLocker()
	locker.Lock()
	defer locker.Unlock()
	db, err := s.open()
	if err != nil {
		return activationapp.AdmissionResult{}, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return activationapp.AdmissionResult{}, fmt.Errorf("begin candidate admission: %w", err)
	}
	defer tx.Rollback()
	currentDesired, err := desiredIntegrationRevision(tx)
	if err != nil {
		return activationapp.AdmissionResult{}, fmt.Errorf("read desired Integration revision: %w", err)
	}
	if currentDesired != request.DesiredSetRevision {
		return activationapp.AdmissionResult{}, activationapp.ErrStaleDesiredRevision
	}
	state, err := loadSQLiteState(tx)
	if err != nil {
		return activationapp.AdmissionResult{}, err
	}
	if state.DesiredGeneration != request.ExpectedGeneration || state.PendingActivation != nil {
		return activationapp.AdmissionResult{}, activationapp.ErrStaleM2Generation
	}
	if generation, outcome, found, err := activationsqlite.FindCandidateTarget(tx, request.Mapping); err != nil {
		return activationapp.AdmissionResult{}, err
	} else if found && outcome == "applied" && state.AppliedGeneration >= generation {
		if err := tx.Commit(); err != nil {
			return activationapp.AdmissionResult{}, fmt.Errorf("finish idempotent candidate admission: %w", err)
		}
		return activationapp.AdmissionResult{Generation: generation, AlreadyApplied: true}, nil
	} else if found && outcome != "applied" && generation == state.DesiredGeneration {
		state.PendingActivation = &activationdomain.PendingActivation{Generation: generation, RecoveryPath: request.RecoveryPath, Candidate: &request.Mapping}
		if err := saveCandidateState(tx, state); err != nil {
			return activationapp.AdmissionResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return activationapp.AdmissionResult{}, fmt.Errorf("commit candidate retry admission: %w", err)
		}
		return activationapp.AdmissionResult{Generation: generation}, nil
	}
	if state.DesiredGeneration == ^uint64(0) {
		return activationapp.AdmissionResult{}, fmt.Errorf("M2 desired generation overflow")
	}
	generation := state.DesiredGeneration + 1
	state.DesiredGeneration = generation
	state.PendingActivation = &activationdomain.PendingActivation{Generation: generation, RecoveryPath: request.RecoveryPath, Candidate: &request.Mapping}
	if err := saveCandidateState(tx, state); err != nil {
		return activationapp.AdmissionResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return activationapp.AdmissionResult{}, fmt.Errorf("commit candidate admission: %w", err)
	}
	return activationapp.AdmissionResult{Generation: generation}, nil
}

// MarkCandidateGenerationApplied atomically commits AppliedGeneration and the
// candidate's authoritative terminal mapping after Collector readiness.
func (s SQLiteStore) MarkCandidateGenerationApplied(generation uint64, gatewayEndpoint string) error {
	locker := platformsqlite.UpdateLocker()
	locker.Lock()
	defer locker.Unlock()
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
	if err := activationsqlite.WriteCandidateJournal(tx, generation, mapping); err != nil {
		return err
	}
	if gatewayEndpoint != "" {
		if s.markGatewayApplied == nil {
			return fmt.Errorf("Configuration Gateway transaction participant is required")
		}
		if err := s.markGatewayApplied(tx, generation, gatewayEndpoint); err != nil {
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
	locker := platformsqlite.UpdateLocker()
	locker.Lock()
	defer locker.Unlock()
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
	if err := activationsqlite.WriteCandidateJournal(tx, generation, mapping); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit candidate failure outcome: %w", err)
	}
	return nil
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
