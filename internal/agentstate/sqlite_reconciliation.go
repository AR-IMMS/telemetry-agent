package agentstate

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ReconciliationTarget identifies the desired/baseline state whose retries share a budget.
type ReconciliationTarget struct {
	Key                 string
	DesiredRevision     uint64
	DesiredGeneration   uint64
	BaselineFingerprint string
}

// ReconciliationAttempt is durable evidence that one attempt started before planning work.
type ReconciliationAttempt struct {
	ID        int64
	Target    ReconciliationTarget
	Epoch     uint64
	Number    uint64
	StartedAt string
}

// ReconciliationOutcome records a safe, structured terminal or pending result.
type ReconciliationOutcome struct {
	Status              string
	Stage               string
	Code                string
	Class               string
	CandidateDigest     string
	LifecycleGeneration uint64
	SafeDetail          string
	RetryAt             string
}

// ReconciliationEvidence is a persisted target status for local diagnostics.
type ReconciliationEvidence struct {
	Target              ReconciliationTarget
	Epoch               uint64
	Attempts            uint64
	Status              string
	Stage               string
	Code                string
	Class               string
	CandidateDigest     string
	LifecycleGeneration uint64
	SafeDetail          string
	RetryAt             string
	UpdatedAt           string
}

var (
	// ErrReconciliationBlocked marks a permanently blocked desired target.
	ErrReconciliationBlocked = errors.New("reconciliation target is blocked")
	// ErrReconciliationExhausted marks a target whose durable attempt budget is spent.
	ErrReconciliationExhausted = errors.New("reconciliation attempt budget exhausted")
	// ErrReconciliationBackoff means the persisted retry time has not elapsed.
	ErrReconciliationBackoff = errors.New("reconciliation retry backoff has not elapsed")
	// ErrReconciliationApplied means M2 already has authoritative applied evidence for the target.
	ErrReconciliationApplied = errors.New("reconciliation target is already applied")
	// ErrReconciliationAwaitingLifecycle means the existing M2 lifecycle still owns a pending operation.
	ErrReconciliationAwaitingLifecycle = errors.New("reconciliation target awaits M2 lifecycle completion")
)

// BeginReconciliationAttempt durably charges an attempt before resolver/compiler work.
func (s SQLiteStore) BeginReconciliationAttempt(target ReconciliationTarget, now time.Time, maxAttempts uint64) (ReconciliationAttempt, error) {
	if target.Key == "" || target.BaselineFingerprint == "" || maxAttempts == 0 {
		return ReconciliationAttempt{}, fmt.Errorf("reconciliation target and attempt limit are required")
	}
	sqliteUpdateMutex.Lock()
	defer sqliteUpdateMutex.Unlock()
	db, err := s.open()
	if err != nil {
		return ReconciliationAttempt{}, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return ReconciliationAttempt{}, fmt.Errorf("begin reconciliation attempt: %w", err)
	}
	defer tx.Rollback()
	nowText := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`INSERT OR IGNORE INTO reconciliation_targets(target_key,desired_revision,desired_generation,baseline_fingerprint,status,updated_at) VALUES(?,?,?,?,?,?)`, target.Key, target.DesiredRevision, target.DesiredGeneration, target.BaselineFingerprint, "Pending", nowText); err != nil {
		return ReconciliationAttempt{}, err
	}
	var epoch, count uint64
	var status, retryAt string
	if err := tx.QueryRow(`SELECT retry_epoch,attempt_count,status,COALESCE(retry_at,'') FROM reconciliation_targets WHERE target_key=?`, target.Key).Scan(&epoch, &count, &status, &retryAt); err != nil {
		return ReconciliationAttempt{}, err
	}
	if status == "Blocked" {
		return ReconciliationAttempt{}, ErrReconciliationBlocked
	}
	if status == "Applied" {
		return ReconciliationAttempt{}, ErrReconciliationApplied
	}
	var code, candidateDigest string
	if err := tx.QueryRow(`SELECT COALESCE(code,''),COALESCE(candidate_digest,'') FROM reconciliation_targets WHERE target_key=?`, target.Key).Scan(&code, &candidateDigest); err != nil {
		return ReconciliationAttempt{}, err
	}
	if status == "Pending" && code == "awaiting_m2_terminal_outcome" {
		var generation uint64
		var outcome string
		journalErr := tx.QueryRow(`SELECT generation,outcome FROM activation_journal WHERE desired_set_revision=? AND baseline_fingerprint=? AND candidate_digest=? ORDER BY generation DESC LIMIT 1`, target.DesiredRevision, target.BaselineFingerprint, candidateDigest).Scan(&generation, &outcome)
		if errors.Is(journalErr, sql.ErrNoRows) || outcome == "activated" {
			return ReconciliationAttempt{}, ErrReconciliationAwaitingLifecycle
		}
		if journalErr != nil {
			return ReconciliationAttempt{}, fmt.Errorf("read M2 terminal candidate mapping: %w", journalErr)
		}
		if outcome == "applied" {
			state, err := loadSQLiteState(tx)
			if err != nil {
				return ReconciliationAttempt{}, err
			}
			if state.AppliedGeneration < generation {
				return ReconciliationAttempt{}, fmt.Errorf("M2 applied candidate mapping is ahead of AppliedGeneration")
			}
			if _, err := tx.Exec(`UPDATE reconciliation_targets SET status='Applied',stage='terminal',code='applied',class='',lifecycle_generation=?,retry_at=NULL,safe_detail='',updated_at=? WHERE target_key=?`, generation, nowText, target.Key); err != nil {
				return ReconciliationAttempt{}, err
			}
			if err := tx.Commit(); err != nil {
				return ReconciliationAttempt{}, fmt.Errorf("persist M2-applied reconciliation target: %w", err)
			}
			return ReconciliationAttempt{}, ErrReconciliationApplied
		}
	}
	if retryAt != "" {
		retry, parseErr := time.Parse(time.RFC3339Nano, retryAt)
		if parseErr == nil && now.Before(retry) {
			return ReconciliationAttempt{}, ErrReconciliationBackoff
		}
	}
	if _, err := tx.Exec(`UPDATE reconciliation_attempts SET status='interrupted',stage='restart_recovery',code='process_interrupted',updated_at=? WHERE target_key=? AND retry_epoch=? AND status='started'`, nowText, target.Key, epoch); err != nil {
		return ReconciliationAttempt{}, err
	}
	if count >= maxAttempts {
		if _, err := tx.Exec(`UPDATE reconciliation_targets SET status='Needs Attention',updated_at=? WHERE target_key=?`, nowText, target.Key); err != nil {
			return ReconciliationAttempt{}, err
		}
		if err := tx.Commit(); err != nil {
			return ReconciliationAttempt{}, fmt.Errorf("persist exhausted reconciliation target: %w", err)
		}
		return ReconciliationAttempt{}, ErrReconciliationExhausted
	}
	count++
	result, err := tx.Exec(`INSERT INTO reconciliation_attempts(target_key,retry_epoch,attempt_number,status,started_at,updated_at) VALUES(?,?,?,'started',?,?)`, target.Key, epoch, count, nowText, nowText)
	if err != nil {
		return ReconciliationAttempt{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return ReconciliationAttempt{}, err
	}
	if _, err := tx.Exec(`UPDATE reconciliation_targets SET attempt_count=?,status='Pending',retry_at=NULL,updated_at=? WHERE target_key=?`, count, nowText, target.Key); err != nil {
		return ReconciliationAttempt{}, err
	}
	if _, err := tx.Exec(`DELETE FROM reconciliation_attempts WHERE id IN (SELECT id FROM reconciliation_attempts WHERE status<>'started' ORDER BY id DESC LIMIT -1 OFFSET 500)`); err != nil {
		return ReconciliationAttempt{}, err
	}
	if _, err := tx.Exec(`DELETE FROM reconciliation_attempts WHERE target_key IN (SELECT target_key FROM reconciliation_targets WHERE target_key NOT IN (SELECT target_key FROM reconciliation_targets ORDER BY updated_at DESC LIMIT 100)) AND status<>'started' AND target_key NOT IN (SELECT target_key FROM reconciliation_attempts WHERE status='started')`); err != nil {
		return ReconciliationAttempt{}, err
	}
	if _, err := tx.Exec(`DELETE FROM reconciliation_targets WHERE target_key NOT IN (SELECT target_key FROM reconciliation_targets ORDER BY updated_at DESC LIMIT 100) AND target_key NOT IN (SELECT target_key FROM reconciliation_attempts WHERE status='started')`); err != nil {
		return ReconciliationAttempt{}, err
	}
	if err := tx.Commit(); err != nil {
		return ReconciliationAttempt{}, fmt.Errorf("commit reconciliation attempt start: %w", err)
	}
	return ReconciliationAttempt{ID: id, Target: target, Epoch: epoch, Number: count, StartedAt: nowText}, nil
}

// FinishReconciliationAttempt records one attempt result and updates current target status atomically.
func (s SQLiteStore) FinishReconciliationAttempt(attempt ReconciliationAttempt, outcome ReconciliationOutcome, now time.Time) error {
	if attempt.ID == 0 || outcome.Status == "" || outcome.Stage == "" || outcome.Code == "" {
		return fmt.Errorf("attempt identity and structured outcome are required")
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
		return err
	}
	defer tx.Rollback()
	nowText := now.UTC().Format(time.RFC3339Nano)
	result, err := tx.Exec(`UPDATE reconciliation_attempts SET status=?,stage=?,code=?,class=?,candidate_digest=?,lifecycle_generation=?,safe_detail=?,retry_at=?,updated_at=? WHERE id=? AND target_key=? AND retry_epoch=? AND status='started'`, outcome.Status, outcome.Stage, outcome.Code, outcome.Class, outcome.CandidateDigest, outcome.LifecycleGeneration, outcome.SafeDetail, nullable(outcome.RetryAt), nowText, attempt.ID, attempt.Target.Key, attempt.Epoch)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return fmt.Errorf("reconciliation attempt %d is no longer active", attempt.ID)
	}
	if _, err = tx.Exec(`UPDATE reconciliation_targets SET status=?,stage=?,code=?,class=?,candidate_digest=?,lifecycle_generation=?,safe_detail=?,retry_at=?,updated_at=? WHERE target_key=? AND retry_epoch=?`, outcome.Status, outcome.Stage, outcome.Code, outcome.Class, outcome.CandidateDigest, outcome.LifecycleGeneration, outcome.SafeDetail, nullable(outcome.RetryAt), nowText, attempt.Target.Key, attempt.Epoch); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordReconciliationCandidate durably binds a compiled candidate to its active attempt before lifecycle admission.
func (s SQLiteStore) RecordReconciliationCandidate(attempt ReconciliationAttempt, digest string, now time.Time) error {
	if attempt.ID == 0 || attempt.Target.Key == "" || digest == "" {
		return fmt.Errorf("active attempt identity and candidate digest are required")
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
		return fmt.Errorf("begin reconciliation candidate identity transaction: %w", err)
	}
	defer tx.Rollback()
	nowText := now.UTC().Format(time.RFC3339Nano)
	result, err := tx.Exec(`UPDATE reconciliation_attempts SET candidate_digest=?,updated_at=? WHERE id=? AND target_key=? AND retry_epoch=? AND status='started'`, digest, nowText, attempt.ID, attempt.Target.Key, attempt.Epoch)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return fmt.Errorf("reconciliation attempt %d is no longer active", attempt.ID)
	}
	if _, err := tx.Exec(`UPDATE reconciliation_targets SET candidate_digest=?,updated_at=? WHERE target_key=? AND retry_epoch=?`, digest, nowText, attempt.Target.Key, attempt.Epoch); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reconciliation candidate identity: %w", err)
	}
	return nil
}

// RetryReconciliationTarget opens a new audited retry epoch without changing desired state.
func (s SQLiteStore) RetryReconciliationTarget(key string, now time.Time) error {
	if key == "" {
		return fmt.Errorf("reconciliation target key is required")
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
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE reconciliation_targets SET retry_epoch=retry_epoch+1,attempt_count=0,status='Pending',retry_at=NULL,stage='operator_retry',code='explicit_retry',class='',safe_detail='operator requested retry',updated_at=? WHERE target_key=? AND status<>'Applied' AND NOT EXISTS (SELECT 1 FROM reconciliation_attempts WHERE target_key=? AND retry_epoch=reconciliation_targets.retry_epoch AND status='started')`, now.UTC().Format(time.RFC3339Nano), key, key)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return sql.ErrNoRows
	}
	if _, err := tx.Exec(`INSERT INTO reconciliation_attempts(target_key,retry_epoch,attempt_number,status,started_at,updated_at,stage,code,safe_detail) SELECT target_key,retry_epoch,0,'operator_retry',?,?, 'operator_retry','explicit_retry','operator requested retry' FROM reconciliation_targets WHERE target_key=?`, now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano), key); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordCandidateRecoveryFailure marks the reconciliation attempt for an M2 pending candidate when startup LKG recovery fails.
func (s SQLiteStore) RecordCandidateRecoveryFailure(state State, now time.Time) error {
	if state.PendingActivation == nil || state.PendingActivation.Candidate == nil {
		return fmt.Errorf("pending candidate recovery evidence is required")
	}
	pending := state.PendingActivation
	mapping := pending.Candidate
	sqliteUpdateMutex.Lock()
	defer sqliteUpdateMutex.Unlock()
	db, err := s.open()
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
	if err := tx.QueryRow(`SELECT target_key,retry_epoch FROM reconciliation_targets WHERE desired_revision=? AND baseline_fingerprint=? AND candidate_digest=? ORDER BY updated_at DESC LIMIT 1`, mapping.DesiredSetRevision, mapping.BaselineFingerprint, mapping.CandidateDigest).Scan(&key, &epoch); err != nil {
		return fmt.Errorf("find reconciliation attempt for pending candidate recovery: %w", err)
	}
	updated := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`UPDATE reconciliation_targets SET status='Needs Attention',stage='recovery',code='lkg_recovery_failed',class='activation',candidate_digest=?,lifecycle_generation=?,safe_detail='M2 last-known-good recovery failed',retry_at=NULL,updated_at=? WHERE target_key=?`, mapping.CandidateDigest, pending.Generation, updated, key); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE reconciliation_attempts SET status='Needs Attention',stage='recovery',code='lkg_recovery_failed',class='activation',candidate_digest=?,lifecycle_generation=?,safe_detail='M2 last-known-good recovery failed',retry_at=NULL,updated_at=? WHERE id=(SELECT id FROM reconciliation_attempts WHERE target_key=? AND retry_epoch=? AND candidate_digest=? ORDER BY id DESC LIMIT 1)`, mapping.CandidateDigest, pending.Generation, updated, key, epoch, mapping.CandidateDigest); err != nil {
		return err
	}
	return tx.Commit()
}

// ReconciliationEvidence returns the most recently updated persisted target.
func (s SQLiteStore) ReconciliationEvidence() (ReconciliationEvidence, error) {
	db, err := s.open()
	if err != nil {
		return ReconciliationEvidence{}, err
	}
	defer db.Close()
	var e ReconciliationEvidence
	err = db.QueryRow(`SELECT target_key,desired_revision,desired_generation,baseline_fingerprint,retry_epoch,attempt_count,status,COALESCE(stage,''),COALESCE(code,''),COALESCE(class,''),COALESCE(candidate_digest,''),COALESCE(lifecycle_generation,0),COALESCE(safe_detail,''),COALESCE(retry_at,''),updated_at FROM reconciliation_targets ORDER BY updated_at DESC LIMIT 1`).Scan(&e.Target.Key, &e.Target.DesiredRevision, &e.Target.DesiredGeneration, &e.Target.BaselineFingerprint, &e.Epoch, &e.Attempts, &e.Status, &e.Stage, &e.Code, &e.Class, &e.CandidateDigest, &e.LifecycleGeneration, &e.SafeDetail, &e.RetryAt, &e.UpdatedAt)
	return e, err
}

// ReconciliationHistory returns bounded recent attempt evidence newest first.
func (s SQLiteStore) ReconciliationHistory(limit int) ([]ReconciliationAttemptRecord, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("history limit must be between 1 and 100")
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT id,target_key,retry_epoch,attempt_number,status,started_at,updated_at,COALESCE(stage,''),COALESCE(code,''),COALESCE(class,''),COALESCE(candidate_digest,''),COALESCE(lifecycle_generation,0),COALESCE(safe_detail,''),COALESCE(retry_at,'') FROM reconciliation_attempts ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReconciliationAttemptRecord
	for rows.Next() {
		var r ReconciliationAttemptRecord
		if err := rows.Scan(&r.ID, &r.TargetKey, &r.Epoch, &r.Number, &r.Status, &r.StartedAt, &r.UpdatedAt, &r.Stage, &r.Code, &r.Class, &r.CandidateDigest, &r.LifecycleGeneration, &r.SafeDetail, &r.RetryAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReconciliationAttemptRecord is one safe history row exposed to local diagnostics.
type ReconciliationAttemptRecord struct {
	ID                                                                int64
	TargetKey                                                         string
	Epoch, Number                                                     uint64
	Status, StartedAt, UpdatedAt, Stage, Code, Class, CandidateDigest string
	LifecycleGeneration                                               uint64
	SafeDetail, RetryAt                                               string
}

// CandidateOutcome reads the authoritative M2 journal mapping for one exact candidate target.
func (s SQLiteStore) CandidateOutcome(desiredRevision uint64, fingerprint, candidateDigest string) (CandidateMapping, uint64, bool, error) {
	db, err := s.open()
	if err != nil {
		return CandidateMapping{}, 0, false, err
	}
	defer db.Close()
	var mapping CandidateMapping
	var generation uint64
	err = db.QueryRow(`SELECT generation,desired_set_revision,candidate_digest,baseline_fingerprint,COALESCE(effective_config_digest,''),COALESCE(collector_version,''),COALESCE(manifest_version,''),COALESCE(platform,''),outcome,COALESCE(stage,''),COALESCE(recovery_result,'') FROM activation_journal WHERE desired_set_revision=? AND baseline_fingerprint=? AND candidate_digest=? ORDER BY generation DESC LIMIT 1`, desiredRevision, fingerprint, candidateDigest).Scan(&generation, &mapping.DesiredSetRevision, &mapping.CandidateDigest, &mapping.BaselineFingerprint, &mapping.EffectiveConfigDigest, &mapping.CollectorVersion, &mapping.ManifestVersion, &mapping.Platform, &mapping.Outcome, &mapping.Stage, &mapping.RecoveryResult)
	if errors.Is(err, sql.ErrNoRows) {
		return CandidateMapping{}, 0, false, nil
	}
	if err != nil {
		return CandidateMapping{}, 0, false, err
	}
	state, err := loadSQLiteState(db)
	if err != nil {
		return CandidateMapping{}, 0, false, err
	}
	if mapping.Outcome == "applied" && state.AppliedGeneration < generation {
		return CandidateMapping{}, 0, false, fmt.Errorf("M2 journal candidate outcome is not reflected in AppliedGeneration")
	}
	return mapping, generation, true, nil
}
func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}
