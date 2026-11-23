package sqlite

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation/contracts"
)

type candidateJournalReader interface {
	CandidateOutcomeTx(*sql.Tx, uint64, string, string) (contracts.CandidateOutcome, uint64, bool, error)
}

type reconciliationSQLite struct {
	store      database
	candidates candidateJournalReader
	now        func() time.Time
}

// NewReconciliationRepository creates Configuration's SQLite reconciliation adapter.
// Runtime supplies candidate evidence through the same transaction snapshot.
func NewReconciliationRepository(open func() (*sql.DB, error), locker sync.Locker, candidates candidateJournalReader) contracts.Repository {
	return reconciliationSQLite{store: database{open: open, locker: locker}, candidates: candidates, now: time.Now}
}

// NewReconciliationRepositoryWithClock injects a clock for deterministic persistence tests.
func NewReconciliationRepositoryWithClock(open func() (*sql.DB, error), locker sync.Locker, candidates candidateJournalReader, now func() time.Time) contracts.Repository {
	if now == nil {
		now = time.Now
	}
	return reconciliationSQLite{store: database{open: open, locker: locker}, candidates: candidates, now: now}
}

func (s reconciliationSQLite) BeginAttempt(target contracts.Target) (contracts.Attempt, error) {
	if target.BaselineFingerprint == "" {
		return contracts.Attempt{}, fmt.Errorf("reconciliation target baseline fingerprint is required")
	}
	key, err := reconciliationTargetKey(target)
	if err != nil {
		return contracts.Attempt{}, err
	}
	unlock, err := s.store.lockWrites()
	if err != nil {
		return contracts.Attempt{}, err
	}
	defer unlock()
	db, err := s.store.openDatabase()
	if err != nil {
		return contracts.Attempt{}, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return contracts.Attempt{}, fmt.Errorf("begin reconciliation attempt: %w", err)
	}
	defer tx.Rollback()
	now := s.now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	if _, err := tx.Exec(`INSERT OR IGNORE INTO reconciliation_targets(target_key,desired_revision,desired_generation,baseline_fingerprint,status,updated_at) VALUES(?,?,?,?,?,?)`, key, target.DesiredRevision, target.DesiredGeneration, target.BaselineFingerprint, "Pending", nowText); err != nil {
		return contracts.Attempt{}, err
	}
	var epoch, count uint64
	var status, retryAt string
	if err := tx.QueryRow(`SELECT retry_epoch,attempt_count,status,COALESCE(retry_at,'') FROM reconciliation_targets WHERE target_key=?`, key).Scan(&epoch, &count, &status, &retryAt); err != nil {
		return contracts.Attempt{}, err
	}
	if status == string(contracts.StatusBlocked) {
		return contracts.Attempt{}, contracts.ErrAttemptBlocked
	}
	if status == string(contracts.StatusApplied) {
		return contracts.Attempt{}, contracts.ErrTargetAlreadyApplied
	}
	var code, candidateDigest string
	if err := tx.QueryRow(`SELECT COALESCE(code,''),COALESCE(candidate_digest,'') FROM reconciliation_targets WHERE target_key=?`, key).Scan(&code, &candidateDigest); err != nil {
		return contracts.Attempt{}, err
	}
	if status == string(contracts.StatusPending) && code == "awaiting_m2_terminal_outcome" {
		if s.candidates == nil {
			return contracts.Attempt{}, fmt.Errorf("Runtime candidate journal reader is required for terminal reconciliation evidence")
		}
		outcome, generation, found, err := s.candidates.CandidateOutcomeTx(tx, target.DesiredRevision, target.BaselineFingerprint, candidateDigest)
		if err != nil {
			return contracts.Attempt{}, fmt.Errorf("read Runtime terminal candidate evidence: %w", err)
		}
		if !found || outcome.Outcome == "activated" {
			return contracts.Attempt{}, contracts.ErrAwaitingLifecycle
		}
		if outcome.Outcome == "applied" {
			if _, err := tx.Exec(`UPDATE reconciliation_targets SET status='Applied',stage='terminal',code='applied',class='',lifecycle_generation=?,retry_at=NULL,safe_detail='',updated_at=? WHERE target_key=?`, generation, nowText, key); err != nil {
				return contracts.Attempt{}, err
			}
			if err := tx.Commit(); err != nil {
				return contracts.Attempt{}, fmt.Errorf("persist Runtime-applied reconciliation target: %w", err)
			}
			return contracts.Attempt{}, contracts.ErrTargetAlreadyApplied
		}
	}
	if retryAt != "" {
		retry, parseErr := time.Parse(time.RFC3339Nano, retryAt)
		if parseErr == nil && now.Before(retry) {
			return contracts.Attempt{}, contracts.ErrAttemptBackoff
		}
	}
	if _, err := tx.Exec(`UPDATE reconciliation_attempts SET status='interrupted',stage='restart_recovery',code='process_interrupted',updated_at=? WHERE target_key=? AND retry_epoch=? AND status='started'`, nowText, key, epoch); err != nil {
		return contracts.Attempt{}, err
	}
	if count >= contracts.MaxAttempts {
		if _, err := tx.Exec(`UPDATE reconciliation_targets SET status='Needs Attention',updated_at=? WHERE target_key=?`, nowText, key); err != nil {
			return contracts.Attempt{}, err
		}
		if err := tx.Commit(); err != nil {
			return contracts.Attempt{}, fmt.Errorf("persist exhausted reconciliation target: %w", err)
		}
		return contracts.Attempt{}, contracts.ErrAttemptBudgetExhausted
	}
	count++
	result, err := tx.Exec(`INSERT INTO reconciliation_attempts(target_key,retry_epoch,attempt_number,status,started_at,updated_at) VALUES(?,?,?,'started',?,?)`, key, epoch, count, nowText, nowText)
	if err != nil {
		return contracts.Attempt{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return contracts.Attempt{}, err
	}
	if _, err := tx.Exec(`UPDATE reconciliation_targets SET attempt_count=?,status='Pending',retry_at=NULL,updated_at=? WHERE target_key=?`, count, nowText, key); err != nil {
		return contracts.Attempt{}, err
	}
	if _, err := tx.Exec(`DELETE FROM reconciliation_attempts WHERE id IN (SELECT id FROM reconciliation_attempts WHERE status<>'started' ORDER BY id DESC LIMIT -1 OFFSET 500)`); err != nil {
		return contracts.Attempt{}, err
	}
	if _, err := tx.Exec(`DELETE FROM reconciliation_attempts WHERE target_key IN (SELECT target_key FROM reconciliation_targets WHERE target_key NOT IN (SELECT target_key FROM reconciliation_targets ORDER BY updated_at DESC LIMIT 100)) AND status<>'started' AND target_key NOT IN (SELECT target_key FROM reconciliation_attempts WHERE status='started')`); err != nil {
		return contracts.Attempt{}, err
	}
	if _, err := tx.Exec(`DELETE FROM reconciliation_targets WHERE target_key NOT IN (SELECT target_key FROM reconciliation_targets ORDER BY updated_at DESC LIMIT 100) AND target_key NOT IN (SELECT target_key FROM reconciliation_attempts WHERE status='started')`); err != nil {
		return contracts.Attempt{}, err
	}
	if err := tx.Commit(); err != nil {
		return contracts.Attempt{}, fmt.Errorf("commit reconciliation attempt start: %w", err)
	}
	return contracts.Attempt{ID: id, Target: target, Epoch: epoch, Number: count, StartedAt: now}, nil
}

func (s reconciliationSQLite) FinishAttempt(attempt contracts.Attempt, outcome contracts.Outcome) error {
	if attempt.ID == 0 || outcome.Status == "" || outcome.Stage == "" || outcome.Code == "" {
		return fmt.Errorf("attempt identity and structured outcome are required")
	}
	key, err := reconciliationTargetKey(attempt.Target)
	if err != nil {
		return err
	}
	unlock, err := s.store.lockWrites()
	if err != nil {
		return err
	}
	defer unlock()
	db, err := s.store.openDatabase()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	nowText := s.now().UTC().Format(time.RFC3339Nano)
	result, err := tx.Exec(`UPDATE reconciliation_attempts SET status=?,stage=?,code=?,class=?,candidate_digest=?,lifecycle_generation=?,safe_detail=?,retry_at=?,updated_at=? WHERE id=? AND target_key=? AND retry_epoch=? AND status='started'`, outcome.Status, outcome.Stage, outcome.Code, outcome.Class, outcome.CandidateDigest, outcome.Generation, outcome.SafeDetail, reconciliationNullableRetry(outcome.RetryAt), nowText, attempt.ID, key, attempt.Epoch)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return fmt.Errorf("reconciliation attempt %d is no longer active", attempt.ID)
	}
	if _, err = tx.Exec(`UPDATE reconciliation_targets SET status=?,stage=?,code=?,class=?,candidate_digest=?,lifecycle_generation=?,safe_detail=?,retry_at=?,updated_at=? WHERE target_key=? AND retry_epoch=?`, outcome.Status, outcome.Stage, outcome.Code, outcome.Class, outcome.CandidateDigest, outcome.Generation, outcome.SafeDetail, reconciliationNullableRetry(outcome.RetryAt), nowText, key, attempt.Epoch); err != nil {
		return err
	}
	return tx.Commit()
}

func (s reconciliationSQLite) RecordCandidate(attempt contracts.Attempt, digest string) error {
	if attempt.ID == 0 || digest == "" {
		return fmt.Errorf("active attempt identity and candidate digest are required")
	}
	key, err := reconciliationTargetKey(attempt.Target)
	if err != nil {
		return err
	}
	unlock, err := s.store.lockWrites()
	if err != nil {
		return err
	}
	defer unlock()
	db, err := s.store.openDatabase()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin reconciliation candidate identity transaction: %w", err)
	}
	defer tx.Rollback()
	nowText := s.now().UTC().Format(time.RFC3339Nano)
	result, err := tx.Exec(`UPDATE reconciliation_attempts SET candidate_digest=?,updated_at=? WHERE id=? AND target_key=? AND retry_epoch=? AND status='started'`, digest, nowText, attempt.ID, key, attempt.Epoch)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return fmt.Errorf("reconciliation attempt %d is no longer active", attempt.ID)
	}
	if _, err := tx.Exec(`UPDATE reconciliation_targets SET candidate_digest=?,updated_at=? WHERE target_key=? AND retry_epoch=?`, digest, nowText, key, attempt.Epoch); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reconciliation candidate identity: %w", err)
	}
	return nil
}

func (s reconciliationSQLite) RetryTarget(key string, now time.Time) error {
	if key == "" {
		return fmt.Errorf("reconciliation target key is required")
	}
	unlock, err := s.store.lockWrites()
	if err != nil {
		return err
	}
	defer unlock()
	db, err := s.store.openDatabase()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	updated := now.UTC().Format(time.RFC3339Nano)
	result, err := tx.Exec(`UPDATE reconciliation_targets SET retry_epoch=retry_epoch+1,attempt_count=0,status='Pending',retry_at=NULL,stage='operator_retry',code='explicit_retry',class='',safe_detail='operator requested retry',updated_at=? WHERE target_key=? AND status<>'Applied' AND NOT EXISTS (SELECT 1 FROM reconciliation_attempts WHERE target_key=? AND retry_epoch=reconciliation_targets.retry_epoch AND status='started')`, updated, key, key)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return sql.ErrNoRows
	}
	if _, err := tx.Exec(`INSERT INTO reconciliation_attempts(target_key,retry_epoch,attempt_number,status,started_at,updated_at,stage,code,safe_detail) SELECT target_key,retry_epoch,0,'operator_retry',?,?, 'operator_retry','explicit_retry','operator requested retry' FROM reconciliation_targets WHERE target_key=?`, updated, updated, key); err != nil {
		return err
	}
	return tx.Commit()
}

func (s reconciliationSQLite) RecordRecoveryFailure(failure contracts.RecoveryFailure) error {
	if failure.BaselineFingerprint == "" || failure.CandidateDigest == "" {
		return fmt.Errorf("pending candidate recovery evidence is required")
	}
	unlock, err := s.store.lockWrites()
	if err != nil {
		return err
	}
	defer unlock()
	db, err := s.store.openDatabase()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var key string
	var epoch uint64
	if err := tx.QueryRow(`SELECT target_key,retry_epoch FROM reconciliation_targets WHERE desired_revision=? AND baseline_fingerprint=? AND candidate_digest=? ORDER BY updated_at DESC LIMIT 1`, failure.DesiredRevision, failure.BaselineFingerprint, failure.CandidateDigest).Scan(&key, &epoch); err != nil {
		return fmt.Errorf("find reconciliation attempt for pending candidate recovery: %w", err)
	}
	updated := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`UPDATE reconciliation_targets SET status='Needs Attention',stage='recovery',code='lkg_recovery_failed',class='activation',candidate_digest=?,lifecycle_generation=?,safe_detail='M2 last-known-good recovery failed',retry_at=NULL,updated_at=? WHERE target_key=?`, failure.CandidateDigest, failure.Generation, updated, key); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE reconciliation_attempts SET status='Needs Attention',stage='recovery',code='lkg_recovery_failed',class='activation',candidate_digest=?,lifecycle_generation=?,safe_detail='M2 last-known-good recovery failed',retry_at=NULL,updated_at=? WHERE id=(SELECT id FROM reconciliation_attempts WHERE target_key=? AND retry_epoch=? AND candidate_digest=? ORDER BY id DESC LIMIT 1)`, failure.CandidateDigest, failure.Generation, updated, key, epoch, failure.CandidateDigest); err != nil {
		return err
	}
	return tx.Commit()
}

func (s reconciliationSQLite) Evidence() (contracts.TargetEvidence, error) {
	db, err := s.store.openDatabase()
	if err != nil {
		return contracts.TargetEvidence{}, err
	}
	defer db.Close()
	var e contracts.TargetEvidence
	var status, class string
	err = db.QueryRow(`SELECT target_key,desired_revision,desired_generation,baseline_fingerprint,retry_epoch,attempt_count,status,COALESCE(stage,''),COALESCE(code,''),COALESCE(class,''),COALESCE(candidate_digest,''),COALESCE(lifecycle_generation,0),COALESCE(safe_detail,''),COALESCE(retry_at,''),updated_at FROM reconciliation_targets ORDER BY updated_at DESC LIMIT 1`).Scan(&e.TargetKey, &e.Target.DesiredRevision, &e.Target.DesiredGeneration, &e.Target.BaselineFingerprint, &e.Epoch, &e.Attempts, &status, &e.Stage, &e.Code, &class, &e.CandidateDigest, &e.Generation, &e.SafeDetail, &e.RetryAt, &e.UpdatedAt)
	e.Status, e.Class = contracts.Status(status), contracts.FailureClass(class)
	return e, err
}

func (s reconciliationSQLite) History(limit int) ([]contracts.AttemptRecord, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("history limit must be between 1 and 100")
	}
	db, err := s.store.openDatabase()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT id,target_key,retry_epoch,attempt_number,status,started_at,updated_at,COALESCE(stage,''),COALESCE(code,''),COALESCE(class,''),COALESCE(candidate_digest,''),COALESCE(lifecycle_generation,0),COALESCE(safe_detail,''),COALESCE(retry_at,'') FROM reconciliation_attempts ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contracts.AttemptRecord
	for rows.Next() {
		var r contracts.AttemptRecord
		if err := rows.Scan(&r.ID, &r.TargetKey, &r.Epoch, &r.Number, &r.Status, &r.StartedAt, &r.UpdatedAt, &r.Stage, &r.Code, &r.Class, &r.CandidateDigest, &r.Generation, &r.SafeDetail, &r.RetryAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s reconciliationSQLite) Lookup(revision uint64, fingerprint, digest string) (contracts.CandidateOutcome, uint64, bool, error) {
	db, err := s.store.openDatabase()
	if err != nil {
		return contracts.CandidateOutcome{}, 0, false, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return contracts.CandidateOutcome{}, 0, false, err
	}
	defer tx.Rollback()
	if s.candidates == nil {
		return contracts.CandidateOutcome{}, 0, false, fmt.Errorf("Runtime candidate journal reader is required")
	}
	outcome, generation, found, err := s.candidates.CandidateOutcomeTx(tx, revision, fingerprint, digest)
	if err != nil {
		return contracts.CandidateOutcome{}, 0, false, err
	}
	if err := tx.Commit(); err != nil {
		return contracts.CandidateOutcome{}, 0, false, err
	}
	return outcome, generation, found, nil
}

func reconciliationTargetKey(target contracts.Target) (string, error) {
	encoded, err := json.Marshal(target)
	if err != nil {
		return "", fmt.Errorf("encode reconciliation target: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func reconciliationNullableRetry(at time.Time) any {
	if at.IsZero() {
		return nil
	}
	return at.UTC().Format(time.RFC3339Nano)
}

var _ contracts.Repository = reconciliationSQLite{}
