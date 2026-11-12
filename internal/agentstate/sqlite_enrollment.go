package agentstate

import (
	"github.com/ar-imms/telemetry-agent/internal/enrollment"
	enrollmentsqlite "github.com/ar-imms/telemetry-agent/internal/enrollment/sqlite"
)

func (s SQLiteStore) enrollmentRepository() enrollment.AttemptRepository {
	return enrollmentsqlite.New(s.OpenDatabase, &sqliteUpdateMutex)
}

// LoadAttempt preserves the concrete SQLite store compatibility API.
func (s SQLiteStore) LoadAttempt() (enrollment.Attempt, error) {
	return s.enrollmentRepository().LoadAttempt()
}

// CreateAttempt preserves the concrete SQLite store compatibility API.
func (s SQLiteStore) CreateAttempt(attempt enrollment.Attempt) error {
	return s.enrollmentRepository().CreateAttempt(attempt)
}

// ReplaceFailedAttempt preserves the concrete SQLite store compatibility API.
func (s SQLiteStore) ReplaceFailedAttempt(previousID string, attempt enrollment.Attempt) error {
	return s.enrollmentRepository().ReplaceFailedAttempt(previousID, attempt)
}

// ReplaceForReEnrollment preserves the concrete SQLite store compatibility API.
func (s SQLiteStore) ReplaceForReEnrollment(previousID string, attempt enrollment.Attempt) error {
	return s.enrollmentRepository().ReplaceForReEnrollment(previousID, attempt)
}

// ClearPreviousCredential preserves the concrete SQLite store compatibility API.
func (s SQLiteStore) ClearPreviousCredential(id string) error {
	return s.enrollmentRepository().ClearPreviousCredential(id)
}

// CompleteAttempt preserves the concrete SQLite store compatibility API.
func (s SQLiteStore) CompleteAttempt(id string, result enrollment.EnrollmentResult, credentialRef string) error {
	return s.enrollmentRepository().CompleteAttempt(id, result, credentialRef)
}

// FailAttempt preserves the concrete SQLite store compatibility API.
func (s SQLiteStore) FailAttempt(id, code string) error {
	return s.enrollmentRepository().FailAttempt(id, code)
}
