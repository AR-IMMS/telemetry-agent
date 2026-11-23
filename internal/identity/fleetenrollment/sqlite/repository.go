package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/domain"
	"github.com/ar-imms/telemetry-agent/internal/platform/sqlite/migrate"
)

// Migrations returns the immutable Fleet enrollment schema migration.
func Migrations() []migrate.Migration {
	return []migrate.Migration{{Version: 15, Name: "Fleet enrollment attempts", Apply: migrateFleetAttempts}}
}

func migrateFleetAttempts(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Fleet enrollment migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var applied int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=15`).Scan(&applied); err != nil {
		return fmt.Errorf("check Fleet enrollment migration: %w", err)
	}
	if applied != 0 {
		return tx.Commit()
	}
	var existing int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='fleet_enrollment_attempt'`).Scan(&existing); err != nil {
		return fmt.Errorf("check Fleet enrollment table: %w", err)
	}
	if existing != 0 {
		return fmt.Errorf("fleet enrollment table exists without schema migration version 15")
	}
	if _, err := tx.Exec(`CREATE TABLE fleet_enrollment_attempt (id INTEGER PRIMARY KEY CHECK (id = 1), payload BLOB NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create Fleet enrollment table: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES(15, CURRENT_TIMESTAMP)`); err != nil {
		return fmt.Errorf("record Fleet enrollment migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Fleet enrollment migration: %w", err)
	}
	return nil
}

// New creates a Fleet attempt adapter over the Agent's shared SQLite database.
func New(open func() (*sql.DB, error), locker sync.Locker) *Repository {
	return &Repository{open: open, locker: locker}
}

// Repository persists one secret-free Fleet enrollment attempt.
type Repository struct {
	open   func() (*sql.DB, error)
	locker sync.Locker
}

// LoadAttempt returns the current Fleet attempt.
func (r *Repository) LoadAttempt() (domain.Attempt, error) {
	db, err := r.openDB()
	if err != nil {
		return domain.Attempt{}, err
	}
	defer func() { _ = db.Close() }()
	var content []byte
	if err := db.QueryRow(`SELECT payload FROM fleet_enrollment_attempt WHERE id=1`).Scan(&content); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Attempt{}, fmt.Errorf("load Fleet enrollment attempt: %w", os.ErrNotExist)
		}
		return domain.Attempt{}, fmt.Errorf("load Fleet enrollment attempt from SQLite: %w", err)
	}
	var attempt domain.Attempt
	if err := json.Unmarshal(content, &attempt); err != nil {
		return domain.Attempt{}, fmt.Errorf("decode Fleet enrollment attempt: %w", err)
	}
	return attempt, nil
}

// CreateAttempt atomically creates the first pending Fleet attempt.
func (r *Repository) CreateAttempt(attempt domain.Attempt) error {
	if err := validateAttempt(attempt); err != nil {
		return err
	}
	unlock, err := r.lockWrites()
	if err != nil {
		return err
	}
	defer unlock()
	db, err := r.openDB()
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	content, err := json.Marshal(attempt)
	if err != nil {
		return fmt.Errorf("encode Fleet enrollment attempt: %w", err)
	}
	if _, err := db.Exec(`INSERT INTO fleet_enrollment_attempt(id,payload,updated_at) VALUES(1,?,?)`, content, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("persist Fleet enrollment attempt: %w", err)
	}
	return nil
}

// CompleteAttempt commits the validated secret-free result and credential reference.
func (r *Repository) CompleteAttempt(id string, result domain.Result) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(result.CredentialID) == "" {
		return fmt.Errorf("fleet attempt ID and credential ID are required")
	}
	unlock, err := r.lockWrites()
	if err != nil {
		return err
	}
	defer unlock()
	db, err := r.openDB()
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Fleet enrollment completion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	attempt, err := loadAttempt(tx)
	if err != nil {
		return err
	}
	if attempt.AttemptID != id || attempt.AgentID != result.AgentID {
		return fmt.Errorf("fleet result does not match pending attempt")
	}
	if attempt.Status == domain.AttemptRegistered {
		if attempt.Result != nil && reflect.DeepEqual(*attempt.Result, result) {
			return tx.Commit()
		}
		return fmt.Errorf("fleet attempt was already completed with different state")
	}
	if attempt.Status != domain.AttemptPending {
		return fmt.Errorf("cannot complete Fleet attempt in status %q", attempt.Status)
	}
	attempt.Status, attempt.Result = domain.AttemptRegistered, &result
	if err := saveAttempt(tx, attempt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Fleet enrollment: %w", err)
	}
	return nil
}

// FailAttempt records a permanent sanitized error code for the pending attempt.
func (r *Repository) FailAttempt(id, code string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(code) == "" {
		return fmt.Errorf("fleet attempt ID and failure code are required")
	}
	unlock, err := r.lockWrites()
	if err != nil {
		return err
	}
	defer unlock()
	db, err := r.openDB()
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Fleet attempt failure: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	attempt, err := loadAttempt(tx)
	if err != nil {
		return err
	}
	if attempt.AttemptID != id {
		return fmt.Errorf("fleet failure does not match pending attempt")
	}
	if attempt.Status == domain.AttemptFailed && attempt.FailureCode == code {
		return tx.Commit()
	}
	if attempt.Status != domain.AttemptPending {
		return fmt.Errorf("cannot fail Fleet attempt in status %q", attempt.Status)
	}
	attempt.Status, attempt.FailureCode = domain.AttemptFailed, code
	if err := saveAttempt(tx, attempt); err != nil {
		return err
	}
	return tx.Commit()
}

// CurrentCredentialReference returns only the protected reference for a registered Fleet result.
func (r *Repository) CurrentCredentialReference() (string, error) {
	attempt, err := r.LoadAttempt()
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if attempt.Status == domain.AttemptRegistered && attempt.Result != nil {
		return attempt.CredentialRef, nil
	}
	return "", nil
}

func validateAttempt(attempt domain.Attempt) error {
	if _, err := domain.ParseOrigin(attempt.Origin); err != nil {
		return err
	}
	if err := (domain.ExchangeRequest{AttemptID: attempt.AttemptID, AgentID: attempt.AgentID, InvitationToken: "validation", RecoveryProof: strings.Repeat("p", 32)}).Validate(); err != nil {
		return err
	}
	if attempt.Status != domain.AttemptPending || attempt.InvitationRef == "" || attempt.RecoveryProofRef == "" || attempt.CredentialRef == "" {
		return fmt.Errorf("new Fleet attempt must be pending with protected credential references")
	}
	return nil
}
func (r *Repository) openDB() (*sql.DB, error) {
	if r == nil || r.open == nil {
		return nil, fmt.Errorf("fleet enrollment database opener is required")
	}
	return r.open()
}
func (r *Repository) lockWrites() (func(), error) {
	if r == nil || r.locker == nil {
		return nil, fmt.Errorf("fleet enrollment write lock is required")
	}
	r.locker.Lock()
	return r.locker.Unlock, nil
}

type queryer interface{ QueryRow(string, ...any) *sql.Row }

func loadAttempt(q queryer) (domain.Attempt, error) {
	var content []byte
	if err := q.QueryRow(`SELECT payload FROM fleet_enrollment_attempt WHERE id=1`).Scan(&content); err != nil {
		return domain.Attempt{}, err
	}
	var attempt domain.Attempt
	if err := json.Unmarshal(content, &attempt); err != nil {
		return domain.Attempt{}, fmt.Errorf("decode Fleet enrollment attempt: %w", err)
	}
	return attempt, nil
}
func saveAttempt(tx *sql.Tx, attempt domain.Attempt) error {
	content, err := json.Marshal(attempt)
	if err != nil {
		return fmt.Errorf("encode Fleet enrollment attempt: %w", err)
	}
	if _, err := tx.Exec(`UPDATE fleet_enrollment_attempt SET payload=?, updated_at=? WHERE id=1`, content, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("persist Fleet enrollment transition: %w", err)
	}
	return nil
}
