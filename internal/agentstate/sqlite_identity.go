package agentstate

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/identity"
)

// EnsureInstallationID persists candidate only for an uninitialized identity
// and returns the committed UUID observed after the transaction succeeds.
func (s SQLiteStore) EnsureInstallationID(
	candidate identity.InstallationID,
) (identity.InstallationID, error) {
	validatedCandidate, err := identity.ParseInstallationID(candidate.String())
	if err != nil {
		return identity.InstallationID{}, fmt.Errorf("validate candidate installation identity: %w", err)
	}
	db, err := s.open()
	if err != nil {
		return identity.InstallationID{}, err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return identity.InstallationID{}, fmt.Errorf("begin installation identity transaction: %w", err)
	}
	defer tx.Rollback()

	initialized, stored, err := loadInstallationIdentity(tx)
	if err != nil {
		return identity.InstallationID{}, err
	}
	if !initialized {
		if stored.Valid {
			return identity.InstallationID{}, fmt.Errorf("uninitialized Agent identity contains a UUID")
		}
		if _, err := tx.Exec(
			`UPDATE installation_identity
			 SET installation_id = ?, initialized = 1
			 WHERE id = 1 AND initialized = 0 AND installation_id IS NULL`,
			validatedCandidate.String(),
		); err != nil {
			return identity.InstallationID{}, fmt.Errorf("commit candidate installation identity: %w", err)
		}
		initialized, stored, err = loadInstallationIdentity(tx)
		if err != nil {
			return identity.InstallationID{}, err
		}
	}
	if !initialized || !stored.Valid {
		return identity.InstallationID{}, fmt.Errorf("Agent installation identity is not initialized")
	}
	committed, err := identity.ParseInstallationID(stored.String)
	if err != nil {
		return identity.InstallationID{}, fmt.Errorf("validate stored installation identity: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return identity.InstallationID{}, fmt.Errorf("commit installation identity transaction: %w", err)
	}
	return committed, nil
}

func loadInstallationIdentity(tx *sql.Tx) (bool, sql.NullString, error) {
	var initialized int
	var stored sql.NullString
	if err := tx.QueryRow(
		`SELECT initialized, installation_id FROM installation_identity WHERE id = 1`,
	).Scan(&initialized, &stored); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, sql.NullString{}, fmt.Errorf("Agent installation identity row is missing")
		}
		return false, sql.NullString{}, fmt.Errorf("load Agent installation identity: %w", err)
	}
	switch initialized {
	case 0:
		if stored.Valid {
			return false, stored, nil
		}
		return false, stored, nil
	case 1:
		if !stored.Valid {
			return false, stored, fmt.Errorf("initialized Agent identity has no UUID")
		}
		return true, stored, nil
	default:
		return false, stored, fmt.Errorf("Agent identity has invalid initialization state %d", initialized)
	}
}
