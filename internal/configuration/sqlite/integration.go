package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	desiredstateapp "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/application"
	desiredstatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
)

// NewIntegrationRepository creates the desired-Integration adapter over the shared database.
// The opener and write lock are required; repository operations return an error if either is missing.
func NewIntegrationRepository(open func() (*sql.DB, error), locker sync.Locker) desiredstateapp.Repository {
	return integrationSQLite{store: database{open: open, locker: locker}}
}

type integrationSQLite struct{ store database }

func (s integrationSQLite) Put(instance desiredstatedomain.Instance, expected uint64) (desiredstatedomain.Instance, uint64, error) {
	if instance.Source == desiredstatedomain.SourceTemplate {
		return desiredstatedomain.Instance{}, 0, desiredstateapp.ErrTemplateOwnedInstance
	}
	if err := instance.Validate(); err != nil {
		return desiredstatedomain.Instance{}, 0, err
	}
	instance.Revision = 0
	encoded, err := json.Marshal(instance)
	if err != nil {
		return desiredstatedomain.Instance{}, 0, fmt.Errorf("encode Integration instance: %w", err)
	}
	unlock, err := s.store.lockWrites()
	if err != nil {
		return desiredstatedomain.Instance{}, 0, err
	}
	defer unlock()
	db, err := s.store.openDatabase()
	if err != nil {
		return desiredstatedomain.Instance{}, 0, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return desiredstatedomain.Instance{}, 0, fmt.Errorf("begin Integration mutation: %w", err)
	}
	defer tx.Rollback()
	var old []byte
	var current uint64
	err = tx.QueryRow(`SELECT payload, revision FROM integration_instances WHERE instance_id=?`, instance.ID).Scan(&old, &current)
	if errors.Is(err, sql.ErrNoRows) {
		if expected != 0 {
			return desiredstatedomain.Instance{}, 0, desiredstateapp.ErrRevisionConflict
		}
		current = 0
	} else if err != nil {
		return desiredstatedomain.Instance{}, 0, err
	} else if expected != current {
		return desiredstatedomain.Instance{}, 0, desiredstateapp.ErrRevisionConflict
	}
	if old != nil {
		var existing desiredstatedomain.Instance
		if err := json.Unmarshal(old, &existing); err != nil {
			return desiredstatedomain.Instance{}, 0, fmt.Errorf("decode existing Integration instance: %w", err)
		}
		if existing.Source == desiredstatedomain.SourceTemplate {
			return desiredstatedomain.Instance{}, 0, desiredstateapp.ErrTemplateOwnedInstance
		}
	}
	if old != nil && sameIntegrationIntent(old, encoded) {
		setRevision, e := desiredIntegrationRevision(tx)
		if e != nil {
			return desiredstatedomain.Instance{}, 0, e
		}
		if e = tx.Commit(); e != nil {
			return desiredstatedomain.Instance{}, 0, fmt.Errorf("commit no-op Integration mutation: %w", e)
		}
		instance.Revision = current
		return instance, setRevision, nil
	}
	if current == ^uint64(0) {
		return desiredstatedomain.Instance{}, 0, fmt.Errorf("Integration instance revision overflow")
	}
	instance.Revision = current + 1
	encoded, err = json.Marshal(instance)
	if err != nil {
		return desiredstatedomain.Instance{}, 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = tx.Exec(`INSERT INTO integration_instances(instance_id,payload,revision,updated_at) VALUES(?,?,?,?) ON CONFLICT(instance_id) DO UPDATE SET payload=excluded.payload,revision=excluded.revision,updated_at=excluded.updated_at`, instance.ID, encoded, instance.Revision, now); err != nil {
		return desiredstatedomain.Instance{}, 0, fmt.Errorf("write Integration instance: %w", err)
	}
	if _, err = tx.Exec(`UPDATE integration_desired_state SET desired_set_revision=desired_set_revision+1,updated_at=? WHERE id=1`, now); err != nil {
		return desiredstatedomain.Instance{}, 0, fmt.Errorf("advance desired Integration revision: %w", err)
	}
	setRevision, err := desiredIntegrationRevision(tx)
	if err != nil {
		return desiredstatedomain.Instance{}, 0, err
	}
	if err = tx.Commit(); err != nil {
		return desiredstatedomain.Instance{}, 0, err
	}
	return instance, setRevision, nil
}

func sameIntegrationIntent(stored, requested []byte) bool {
	var current desiredstatedomain.Instance
	if json.Unmarshal(stored, &current) != nil {
		return false
	}
	var desired desiredstatedomain.Instance
	if json.Unmarshal(requested, &desired) != nil {
		return false
	}
	if current.Source == "" {
		current.Source = desiredstatedomain.SourceLocal
	}
	if desired.Source == "" {
		desired.Source = desiredstatedomain.SourceLocal
	}
	current.Revision = 0
	desired.Revision = 0
	canonicalCurrent, err := json.Marshal(current)
	if err != nil {
		return false
	}
	canonicalDesired, err := json.Marshal(desired)
	return err == nil && string(canonicalCurrent) == string(canonicalDesired)
}

func (s integrationSQLite) Delete(id string, expected uint64) (uint64, error) {
	unlock, err := s.store.lockWrites()
	if err != nil {
		return 0, err
	}
	defer unlock()
	db, err := s.store.openDatabase()
	if err != nil {
		return 0, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var current uint64
	var payload []byte
	err = tx.QueryRow(`SELECT payload,revision FROM integration_instances WHERE instance_id=?`, id).Scan(&payload, &current)
	if errors.Is(err, sql.ErrNoRows) {
		revision, e := desiredIntegrationRevision(tx)
		if e != nil {
			return 0, e
		}
		return revision, tx.Commit()
	}
	if err != nil {
		return 0, err
	}
	var existing desiredstatedomain.Instance
	if err := json.Unmarshal(payload, &existing); err != nil {
		return 0, fmt.Errorf("decode Integration instance before delete: %w", err)
	}
	if existing.Source == desiredstatedomain.SourceTemplate {
		return 0, desiredstateapp.ErrTemplateOwnedInstance
	}
	if current != expected {
		return 0, desiredstateapp.ErrRevisionConflict
	}
	if _, err = tx.Exec(`DELETE FROM integration_instances WHERE instance_id=?`, id); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(`UPDATE integration_desired_state SET desired_set_revision=desired_set_revision+1,updated_at=? WHERE id=1`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return 0, err
	}
	revision, err := desiredIntegrationRevision(tx)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return revision, nil
}

func (s integrationSQLite) Snapshot() (desiredstateapp.Snapshot, error) {
	db, err := s.store.openDatabase()
	if err != nil {
		return desiredstateapp.Snapshot{}, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return desiredstateapp.Snapshot{}, err
	}
	defer tx.Rollback()
	revision, err := desiredIntegrationRevision(tx)
	if err != nil {
		return desiredstateapp.Snapshot{}, err
	}
	rows, err := tx.Query(`SELECT payload FROM integration_instances ORDER BY instance_id`)
	if err != nil {
		return desiredstateapp.Snapshot{}, err
	}
	defer rows.Close()
	snapshot := desiredstateapp.Snapshot{Revision: revision, Instances: []desiredstatedomain.Instance{}}
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return desiredstateapp.Snapshot{}, err
		}
		var instance desiredstatedomain.Instance
		if err := json.Unmarshal(payload, &instance); err != nil {
			return desiredstateapp.Snapshot{}, err
		}
		if instance.Source == "" {
			instance.Source = desiredstatedomain.SourceLocal
		}
		snapshot.Instances = append(snapshot.Instances, instance)
	}
	if err := rows.Err(); err != nil {
		return desiredstateapp.Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return desiredstateapp.Snapshot{}, err
	}
	return snapshot, nil
}

func desiredIntegrationRevision(q interface{ QueryRow(string, ...any) *sql.Row }) (uint64, error) {
	var revision uint64
	err := q.QueryRow(`SELECT desired_set_revision FROM integration_desired_state WHERE id=1`).Scan(&revision)
	return revision, err
}
