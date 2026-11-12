package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/enrollment"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	identitysqlite "github.com/ar-imms/telemetry-agent/internal/identity/sqlite"
)

type repository struct {
	open   func() (*sql.DB, error)
	locker sync.Locker
}

// New creates a repository using the shared database opener and write lock.
func New(open func() (*sql.DB, error), locker sync.Locker) enrollment.AttemptRepository {
	return repository{open: open, locker: locker}
}

// LoadAttempt returns the durable enrollment attempt or ErrNoAttempt.
func (r repository) LoadAttempt() (enrollment.Attempt, error) {
	db, err := r.openDatabase()
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

// CreateAttempt atomically persists one immutable pending attempt after checking
// that the installation identity matches the shared operational database.
func (r repository) CreateAttempt(attempt enrollment.Attempt) error {
	if err := validateEnrollmentAttempt(attempt); err != nil {
		return err
	}
	unlock, err := r.lockWrites()
	if err != nil {
		return err
	}
	defer unlock()
	db, err := r.openDatabase()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin enrollment attempt creation: %w", err)
	}
	defer tx.Rollback()
	if err := checkInstallation(tx, attempt.Payload.Installation.ID); err != nil {
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

// ReplaceFailedAttempt starts a new attempt only after a permanent failure for
// the same installation identity.
func (r repository) ReplaceFailedAttempt(previousID string, attempt enrollment.Attempt) error {
	if err := validateEnrollmentAttempt(attempt); err != nil {
		return err
	}
	unlock, err := r.lockWrites()
	if err != nil {
		return err
	}
	defer unlock()
	db, err := r.openDatabase()
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

// ReplaceForReEnrollment retains prior binding metadata while beginning an
// explicitly authorized new attempt.
func (r repository) ReplaceForReEnrollment(previousID string, attempt enrollment.Attempt) error {
	if err := validateEnrollmentAttempt(attempt); err != nil {
		return err
	}
	unlock, err := r.lockWrites()
	if err != nil {
		return err
	}
	defer unlock()
	db, err := r.openDatabase()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Agent re-enrollment: %w", err)
	}
	defer tx.Rollback()
	previous, err := loadEnrollmentAttempt(tx)
	if err != nil {
		return err
	}
	allowed := previous.Status == enrollment.AttemptRegistered ||
		(previous.Status == enrollment.AttemptFailed && (previous.FailureCode == "ENROLLMENT_RECOVERY_REVOKED" || previous.FailureCode == "HTTP_502"))
	if !allowed || previous.Payload.AttemptID != previousID || previous.Payload.Installation.ID != attempt.Payload.Installation.ID {
		return fmt.Errorf("re-enrollment does not match a completed or recovery-revoked installation")
	}
	if previous.Status == enrollment.AttemptRegistered && previous.PreviousAccessCredentialRef != "" {
		return fmt.Errorf("previous re-enrollment credential cleanup must finish before another re-enrollment")
	}
	attempt.PreviousResult = previous.Result
	if attempt.PreviousResult == nil {
		attempt.PreviousResult = previous.PreviousResult
	}
	attempt.PreviousAccessCredentialRef = previous.AccessCredentialRef
	if previous.Status == enrollment.AttemptFailed && previous.PreviousAccessCredentialRef != "" {
		attempt.PreviousAccessCredentialRef = previous.PreviousAccessCredentialRef
	}
	if err := saveEnrollmentAttempt(tx, attempt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit re-enrollment attempt: %w", err)
	}
	return nil
}

// ClearPreviousCredential removes a retired credential reference after its
// protected-store deletion has succeeded.
func (r repository) ClearPreviousCredential(id string) error {
	unlock, err := r.lockWrites()
	if err != nil {
		return err
	}
	defer unlock()
	db, err := r.openDatabase()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin retired credential update: %w", err)
	}
	defer tx.Rollback()
	attempt, err := loadEnrollmentAttempt(tx)
	if err != nil {
		return err
	}
	if attempt.Payload.AttemptID != id || attempt.Status != enrollment.AttemptRegistered {
		return fmt.Errorf("retired credential update does not match registered attempt")
	}
	attempt.PreviousAccessCredentialRef = ""
	if err := saveEnrollmentAttempt(tx, attempt); err != nil {
		return err
	}
	return tx.Commit()
}

// CompleteAttempt commits the validated Core binding and protected-store
// reference in one SQLite transaction. The credential value is never passed.
func (r repository) CompleteAttempt(id string, result enrollment.EnrollmentResult, credentialRef string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(credentialRef) == "" {
		return fmt.Errorf("enrollment attempt ID and credential reference are required")
	}
	unlock, err := r.lockWrites()
	if err != nil {
		return err
	}
	defer unlock()
	db, err := r.openDatabase()
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

// FailAttempt records a permanent rejection while retaining only the original
// secret-free request and failure code.
func (r repository) FailAttempt(id, code string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(code) == "" {
		return fmt.Errorf("enrollment attempt ID and failure code are required")
	}
	unlock, err := r.lockWrites()
	if err != nil {
		return err
	}
	defer unlock()
	db, err := r.openDatabase()
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

func (r repository) openDatabase() (*sql.DB, error) {
	if r.open == nil {
		return nil, fmt.Errorf("Agent enrollment database opener is required")
	}
	return r.open()
}

func (r repository) lockWrites() (func(), error) {
	if r.locker == nil {
		return nil, fmt.Errorf("Agent enrollment write lock is required")
	}
	r.locker.Lock()
	return r.locker.Unlock, nil
}

func checkInstallation(queryer interface {
	QueryRow(string, ...any) *sql.Row
}, supplied string) error {
	committed, initialized, err := identitysqlite.ReadInstallationID(queryer)
	if err != nil {
		return err
	}
	if !initialized {
		return fmt.Errorf("Core Gateway snapshot requires an initialized installation identity")
	}
	incoming, err := identity.ParseInstallationID(supplied)
	if err != nil {
		return fmt.Errorf("validate snapshot installation identity: %w", err)
	}
	if committed.String() != incoming.String() {
		return fmt.Errorf("Core Gateway snapshot installation identity does not match local installation")
	}
	return nil
}

type stateQueryer interface {
	QueryRow(string, ...any) *sql.Row
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
