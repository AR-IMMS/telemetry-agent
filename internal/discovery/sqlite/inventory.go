package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/discovery"
)

// NewInventoryRepository creates the Discovery inventory adapter over the shared database.
func NewInventoryRepository(open func() (*sql.DB, error), locker sync.Locker) discovery.InventoryRepository {
	return inventorySQLite{store: database{open: open, locker: locker}}
}

type inventorySQLite struct{ store database }

func (s inventorySQLite) ReadProviderStatuses(ctx context.Context, limit int) ([]discovery.ProviderStatus, bool, error) {
	if limit <= 0 || limit > discovery.MaxProviderStatusReadLimit {
		return nil, false, fmt.Errorf("provider status read limit must be between 1 and %d", discovery.MaxProviderStatusReadLimit)
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	db, err := s.store.openDatabase()
	if err != nil {
		return nil, false, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT payload FROM discovery_provider_status ORDER BY provider_id,provider_instance_id LIMIT ?`, limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("query discovery provider status: %w", err)
	}
	defer rows.Close()
	statuses := make([]discovery.ProviderStatus, 0, limit)
	truncated := false
	for rows.Next() {
		if len(statuses) == limit {
			truncated = true
			break
		}
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, false, err
		}
		var status discovery.ProviderStatus
		if err := json.Unmarshal(payload, &status); err != nil {
			return nil, false, fmt.Errorf("decode discovery provider status: %w", err)
		}
		statuses = append(statuses, status)
	}
	if !truncated {
		if err := rows.Err(); err != nil {
			return nil, false, err
		}
	}
	return statuses, truncated, nil
}

func (s inventorySQLite) Snapshot() (discovery.InventorySnapshot, error) {
	db, err := s.store.openDatabase()
	if err != nil {
		return discovery.InventorySnapshot{}, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return discovery.InventorySnapshot{}, fmt.Errorf("begin inventory snapshot: %w", err)
	}
	defer tx.Rollback()
	var revision, writeRevision int64
	if err := tx.QueryRow(`SELECT revision,write_revision FROM inventory_meta WHERE id=1`).Scan(&revision, &writeRevision); err != nil {
		return discovery.InventorySnapshot{}, fmt.Errorf("load inventory revision: %w", err)
	}
	snapshot := discovery.InventorySnapshot{Revision: uint64(revision), WriteRevision: uint64(writeRevision), Resources: []discovery.Resource{}, Providers: []discovery.ProviderStatus{}}
	rows, err := tx.Query(`SELECT payload FROM inventory_resources ORDER BY provider_id,provider_instance_id,provider_resource_id`)
	if err != nil {
		return discovery.InventorySnapshot{}, fmt.Errorf("query inventory resources: %w", err)
	}
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			rows.Close()
			return discovery.InventorySnapshot{}, err
		}
		var resource discovery.Resource
		if err := json.Unmarshal(payload, &resource); err != nil {
			rows.Close()
			return discovery.InventorySnapshot{}, fmt.Errorf("decode inventory resource: %w", err)
		}
		snapshot.Resources = append(snapshot.Resources, resource)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return discovery.InventorySnapshot{}, err
	}
	rows.Close()
	statusRows, err := tx.Query(`SELECT payload FROM discovery_provider_status ORDER BY provider_id,provider_instance_id`)
	if err != nil {
		return discovery.InventorySnapshot{}, fmt.Errorf("query discovery provider status: %w", err)
	}
	for statusRows.Next() {
		var payload []byte
		if err := statusRows.Scan(&payload); err != nil {
			statusRows.Close()
			return discovery.InventorySnapshot{}, err
		}
		var status discovery.ProviderStatus
		if err := json.Unmarshal(payload, &status); err != nil {
			statusRows.Close()
			return discovery.InventorySnapshot{}, fmt.Errorf("decode discovery provider status: %w", err)
		}
		snapshot.Providers = append(snapshot.Providers, status)
	}
	if err := statusRows.Err(); err != nil {
		statusRows.Close()
		return discovery.InventorySnapshot{}, err
	}
	statusRows.Close()
	if err := tx.Commit(); err != nil {
		return discovery.InventorySnapshot{}, fmt.Errorf("commit inventory snapshot read: %w", err)
	}
	return snapshot, nil
}

func (s inventorySQLite) CommitProviderSnapshot(expectedWriteRevision uint64, snapshot discovery.ProviderSnapshot, resources []discovery.Resource, materialChange bool) error {
	normalized, err := discovery.NormalizeSnapshot(snapshot)
	if err != nil {
		return err
	}
	if expectedWriteRevision > uint64(^uint64(0)>>1) {
		return fmt.Errorf("inventory write revision exceeds SQLite range")
	}
	for _, resource := range resources {
		if err := resource.Key.Validate(); err != nil {
			return err
		}
		if resource.Key.Provider != normalized.ProviderID || resource.Key.ProviderInstance != normalized.ProviderInstanceID {
			return fmt.Errorf("inventory update contains another provider instance")
		}
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
		return fmt.Errorf("begin inventory update: %w", err)
	}
	defer tx.Rollback()
	var currentWriteRevision int64
	if err := tx.QueryRow(`SELECT write_revision FROM inventory_meta WHERE id=1`).Scan(&currentWriteRevision); err != nil {
		return fmt.Errorf("load inventory write revision for update: %w", err)
	}
	if uint64(currentWriteRevision) != expectedWriteRevision {
		return discovery.ErrSnapshotConflict
	}
	now := normalized.ObservedAt.UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`DELETE FROM inventory_resources WHERE provider_id=? AND provider_instance_id=?`, normalized.ProviderID, normalized.ProviderInstanceID); err != nil {
		return fmt.Errorf("replace provider inventory: %w", err)
	}
	for _, resource := range resources {
		payload, err := json.Marshal(resource)
		if err != nil {
			return fmt.Errorf("encode inventory resource: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO inventory_resources(provider_id,provider_instance_id,provider_resource_id,payload,updated_at) VALUES(?,?,?,?,?)`,
			resource.Key.Provider, resource.Key.ProviderInstance, resource.Key.ProviderResourceID, payload, now); err != nil {
			return fmt.Errorf("persist inventory resource: %w", err)
		}
	}
	status := discovery.ProviderStatus{
		ProviderID: normalized.ProviderID, ProviderInstanceID: normalized.ProviderInstanceID,
		Enabled: true, LastAttemptAt: nowTime(normalized.ObservedAt), LastSuccessAt: nowTime(normalized.ObservedAt),
		ResourceCount: len(normalized.Resources),
	}
	payload, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("encode discovery provider status: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO discovery_provider_status(provider_id,provider_instance_id,payload,updated_at) VALUES(?,?,?,?)
		ON CONFLICT(provider_id,provider_instance_id) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`,
		status.ProviderID, status.ProviderInstanceID, payload, now); err != nil {
		return fmt.Errorf("persist discovery provider status: %w", err)
	}
	materialIncrement := 0
	if materialChange {
		materialIncrement = 1
	}
	result, err := tx.Exec(`UPDATE inventory_meta SET revision=revision+?,write_revision=write_revision+1 WHERE id=1 AND write_revision=?`, materialIncrement, currentWriteRevision)
	if err != nil {
		return fmt.Errorf("advance inventory revisions: %w", err)
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		if err != nil {
			return fmt.Errorf("check inventory revision update: %w", err)
		}
		return discovery.ErrSnapshotConflict
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit inventory update: %w", err)
	}
	return nil
}

func (s inventorySQLite) RecordProviderFailure(status discovery.ProviderStatus) error {
	if status.ProviderID == "" || status.ProviderInstanceID == "" || status.LastErrorCode == "" {
		return fmt.Errorf("provider failure identity and safe error code are required")
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
		return fmt.Errorf("begin provider failure update: %w", err)
	}
	defer tx.Rollback()
	var existing []byte
	err = tx.QueryRow(`SELECT payload FROM discovery_provider_status WHERE provider_id=? AND provider_instance_id=?`, status.ProviderID, status.ProviderInstanceID).Scan(&existing)
	if err == nil {
		var previous discovery.ProviderStatus
		if err := json.Unmarshal(existing, &previous); err != nil {
			return fmt.Errorf("decode previous provider status: %w", err)
		}
		status.LastSuccessAt = previous.LastSuccessAt
		status.ResourceCount = previous.ResourceCount
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("load previous provider status: %w", err)
	}
	if _, err := tx.Exec(`UPDATE inventory_meta SET write_revision=write_revision+1 WHERE id=1`); err != nil {
		return fmt.Errorf("advance inventory write revision after provider failure: %w", err)
	}
	payload, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("encode provider failure status: %w", err)
	}
	now := status.LastAttemptAt.UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`INSERT INTO discovery_provider_status(provider_id,provider_instance_id,payload,updated_at) VALUES(?,?,?,?)
		ON CONFLICT(provider_id,provider_instance_id) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`,
		status.ProviderID, status.ProviderInstanceID, payload, now); err != nil {
		return fmt.Errorf("persist provider failure status: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit provider failure status: %w", err)
	}
	return nil
}

func nowTime(value time.Time) time.Time { return value.UTC() }
