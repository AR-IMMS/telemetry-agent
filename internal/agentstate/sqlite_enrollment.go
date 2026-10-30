package agentstate

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/enrollment"
)

// LoadAttempt returns the durable enrollment attempt or ErrNoAttempt.
func (s SQLiteStore) LoadAttempt() (enrollment.Attempt, error) {
	db, err := s.open()
	if err != nil {
		return enrollment.Attempt{}, err
	}
	defer db.Close()
	attempt, err := loadEnrollmentAttempt(db)
	if errors.Is(err, os.ErrNotExist) {
		return enrollment.Attempt{}, enrollment.ErrNoAttempt
	}
	return attempt, err
}

// CreateAttempt atomically persists one immutable attempt if the installation
// has no previous attempt or binding. Secret values are never accepted here.
func (s SQLiteStore) CreateAttempt(attempt enrollment.Attempt) error {
	if err := validateEnrollmentAttempt(attempt); err != nil {
		return err
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
		return fmt.Errorf("begin enrollment attempt creation: %w", err)
	}
	defer tx.Rollback()
	if err := checkGatewayInstallation(tx, attempt.Payload.Installation.ID); err != nil {
		return fmt.Errorf("validate enrollment installation: %w", err)
	}
	if _, err := loadEnrollmentAttempt(tx); err == nil {
		return enrollment.ErrAttemptExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := saveEnrollmentAttempt(tx, attempt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pending enrollment attempt: %w", err)
	}
	return nil
}

// ReplaceFailedAttempt creates a new operation only after a permanent failure
// and only for the same durable installation identity.
func (s SQLiteStore) ReplaceFailedAttempt(previousID string, attempt enrollment.Attempt) error {
	if err := validateEnrollmentAttempt(attempt); err != nil {
		return err
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
		return fmt.Errorf("begin failed enrollment replacement: %w", err)
	}
	defer tx.Rollback()
	previous, err := loadEnrollmentAttempt(tx)
	if err != nil {
		return err
	}
	if previous.Status != enrollment.AttemptFailed || previous.Payload.AttemptID != previousID ||
		previous.Payload.Installation.ID != attempt.Payload.Installation.ID {
		return fmt.Errorf("failed enrollment replacement does not match prior attempt")
	}
	if err := saveEnrollmentAttempt(tx, attempt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replacement enrollment attempt: %w", err)
	}
	return nil
}

// CompleteAttempt commits the validated Core binding and the protected-store
// reference in one SQLite transaction. The credential value is never passed.
func (s SQLiteStore) CompleteAttempt(id string, result enrollment.EnrollmentResult, credentialRef string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(credentialRef) == "" {
		return fmt.Errorf("enrollment attempt ID and credential reference are required")
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
		return fmt.Errorf("begin enrollment completion: %w", err)
	}
	defer tx.Rollback()
	attempt, err := loadEnrollmentAttempt(tx)
	if err != nil {
		return fmt.Errorf("load pending enrollment attempt: %w", err)
	}
	if attempt.Payload.AttemptID != id || attempt.Payload.Installation.ID != result.InstallationID {
		return fmt.Errorf("Core enrollment result does not match the pending attempt")
	}
	if attempt.Status == enrollment.AttemptRegistered {
		if attempt.AccessCredentialRef == credentialRef && attempt.Result != nil && *attempt.Result == result {
			return tx.Commit()
		}
		return fmt.Errorf("enrollment attempt was already completed with different state")
	}
	if attempt.Status != enrollment.AttemptPending {
		return fmt.Errorf("cannot complete enrollment in status %q", attempt.Status)
	}
	attempt.Status = enrollment.AttemptRegistered
	attempt.Result = &result
	attempt.AccessCredentialRef = credentialRef
	if err := saveEnrollmentAttempt(tx, attempt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit enrollment binding: %w", err)
	}
	return nil
}

// FailAttempt records a permanent rejection while preserving the original
// request for diagnosis without retaining the bootstrap secret in SQLite.
func (s SQLiteStore) FailAttempt(id, code string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(code) == "" {
		return fmt.Errorf("enrollment attempt ID and failure code are required")
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
		return fmt.Errorf("begin enrollment failure update: %w", err)
	}
	defer tx.Rollback()
	attempt, err := loadEnrollmentAttempt(tx)
	if err != nil {
		return err
	}
	if attempt.Payload.AttemptID != id {
		return fmt.Errorf("enrollment failure does not match pending attempt")
	}
	if attempt.Status == enrollment.AttemptFailed && attempt.FailureCode == code {
		return tx.Commit()
	}
	if attempt.Status != enrollment.AttemptPending {
		return fmt.Errorf("cannot fail enrollment in status %q", attempt.Status)
	}
	attempt.Status, attempt.FailureCode = enrollment.AttemptFailed, code
	if err := saveEnrollmentAttempt(tx, attempt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit permanent enrollment failure: %w", err)
	}
	return nil
}

func validateEnrollmentAttempt(attempt enrollment.Attempt) error {
	if err := attempt.Payload.Validate(); err != nil {
		return err
	}
	if attempt.Status != enrollment.AttemptPending || strings.TrimSpace(attempt.BootstrapCredentialRef) == "" ||
		strings.TrimSpace(attempt.AccessCredentialRef) == "" {
		return fmt.Errorf("new enrollment attempt must be pending with protected credential references")
	}
	if _, err := enrollment.ParseCoreOrigin(attempt.CoreOrigin); err != nil {
		return err
	}
	return nil
}

func loadEnrollmentAttempt(queryer stateQueryer) (enrollment.Attempt, error) {
	var payload []byte
	if err := queryer.QueryRow(`SELECT payload FROM enrollment_attempt WHERE id=1`).Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return enrollment.Attempt{}, fmt.Errorf("load enrollment attempt: %w", os.ErrNotExist)
		}
		return enrollment.Attempt{}, fmt.Errorf("load enrollment attempt from SQLite: %w", err)
	}
	var attempt enrollment.Attempt
	if err := json.Unmarshal(payload, &attempt); err != nil {
		return enrollment.Attempt{}, fmt.Errorf("decode enrollment attempt: %w", err)
	}
	return attempt, nil
}

func saveEnrollmentAttempt(tx *sql.Tx, attempt enrollment.Attempt) error {
	content, err := json.Marshal(attempt)
	if err != nil {
		return fmt.Errorf("encode enrollment attempt: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO enrollment_attempt (id, payload, updated_at) VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET payload=excluded.payload, updated_at=excluded.updated_at`,
		content, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("persist enrollment attempt: %w", err)
	}
	return nil
}
