package sqlite

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/identity"
)

type repository struct {
	open func() (*sql.DB, error)
}

// New creates an installation identity repository using the supplied database opener.
func New(open func() (*sql.DB, error)) identity.InstallationIdentityRepository {
	return repository{open: open}
}

// EnsureInstallationID stores the candidate only for an uninitialized identity
// and returns the committed UUID observed after the transaction succeeds.
func (r repository) EnsureInstallationID(candidate identity.InstallationID) (identity.InstallationID, error) {
	validatedCandidate, err := identity.ParseInstallationID(candidate.String())
	if err != nil {
		return identity.InstallationID{}, fmt.Errorf("validate candidate installation identity: %w", err)
	}
	if r.open == nil {
		return identity.InstallationID{}, fmt.Errorf("Agent identity database opener is required")
	}
	db, err := r.open()
	if err != nil {
		return identity.InstallationID{}, err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return identity.InstallationID{}, fmt.Errorf("begin installation identity transaction: %w", err)
	}
	defer tx.Rollback()

	committed, initialized, err := ReadInstallationID(tx)
	if err != nil {
		return identity.InstallationID{}, err
	}
	if !initialized {
		if _, err := tx.Exec(
			`UPDATE installation_identity
			 SET installation_id = ?, initialized = 1
			 WHERE id = 1 AND initialized = 0 AND installation_id IS NULL`,
			validatedCandidate.String(),
		); err != nil {
			return identity.InstallationID{}, fmt.Errorf("commit candidate installation identity: %w", err)
		}
		committed, initialized, err = ReadInstallationID(tx)
		if err != nil {
			return identity.InstallationID{}, err
		}
	}
	if !initialized {
		return identity.InstallationID{}, fmt.Errorf("Agent installation identity is not initialized")
	}
	if err := tx.Commit(); err != nil {
		return identity.InstallationID{}, fmt.Errorf("commit installation identity transaction: %w", err)
	}
	return committed, nil
}

// ReadInstallationID reads and validates the persisted identity within the
// caller's transaction so related state updates can share the same snapshot.
func ReadInstallationID(queryer interface {
	QueryRow(string, ...any) *sql.Row
}) (identity.InstallationID, bool, error) {
	var initialized int
	var stored sql.NullString
	if err := queryer.QueryRow(
		`SELECT initialized, installation_id FROM installation_identity WHERE id = 1`,
	).Scan(&initialized, &stored); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identity.InstallationID{}, false, fmt.Errorf("Agent identity row is missing")
		}
		return identity.InstallationID{}, false, fmt.Errorf("load Agent installation identity: %w", err)
	}
	switch initialized {
	case 0:
		if stored.Valid {
			return identity.InstallationID{}, false, fmt.Errorf("uninitialized Agent identity contains a UUID")
		}
		return identity.InstallationID{}, false, nil
	case 1:
		if !stored.Valid {
			return identity.InstallationID{}, false, fmt.Errorf("initialized Agent identity has no UUID")
		}
		id, err := identity.ParseInstallationID(stored.String)
		if err != nil {
			return identity.InstallationID{}, false, fmt.Errorf("validate stored installation identity: %w", err)
		}
		return id, true, nil
	default:
		return identity.InstallationID{}, false, fmt.Errorf("Agent identity has invalid initialization state %d", initialized)
	}
}
