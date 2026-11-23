package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	desiredapp "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/application"
	desiredstate "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	materializationapp "github.com/ar-imms/telemetry-agent/internal/configuration/materialization/application"
	materializationdomain "github.com/ar-imms/telemetry-agent/internal/configuration/materialization/domain"
	templatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/templates/domain"
	"github.com/ar-imms/telemetry-agent/internal/platform/events"
)

// NewMaterializationRepository creates the projection adapter over the shared database.
// The opener and write lock are required; repository operations return an error if either is missing.
func NewMaterializationRepository(open func() (*sql.DB, error), locker sync.Locker) materializationapp.Repository {
	return materializationSQLite{store: database{open: open, locker: locker}}
}

// NewMaterializationRepositoryWithEvents adds an optional post-commit advisory
// wakeup while preserving the durable result if the queue is full.
// The opener and write lock are required; the publisher may be nil.
func NewMaterializationRepositoryWithEvents(open func() (*sql.DB, error), locker sync.Locker, publisher interface {
	Publish(events.DesiredConfigurationChanged) error
}) materializationapp.Repository {
	return materializationSQLite{store: database{open: open, locker: locker}, publisher: publisher}
}

type materializationSQLite struct {
	store     database
	publisher interface {
		Publish(events.DesiredConfigurationChanged) error
	}
}

func (s materializationSQLite) ApplyTemplateProjection(batch materializationdomain.Batch) (uint64, error) {
	if err := validateMaterializationBatch(batch); err != nil {
		return 0, err
	}
	unlock, err := s.store.lockWrites()
	if err != nil {
		return 0, err
	}
	locked := true
	defer func() {
		if locked {
			unlock()
		}
	}()
	db, err := s.store.openDatabase()
	if err != nil {
		return 0, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin template materialization: %w", err)
	}
	defer tx.Rollback()
	if err := checkMaterializationRevisions(tx, batch); err != nil {
		return 0, err
	}
	revision, err := desiredIntegrationRevision(tx)
	if err != nil {
		return 0, err
	}
	changed := false
	for _, instance := range batch.Upsert {
		if err := validateMaterializedTemplate(tx, instance); err != nil {
			return 0, err
		}
		updated, err := upsertMaterializedInstance(tx, instance)
		if err != nil {
			return 0, err
		}
		changed = changed || updated
	}
	for _, id := range batch.Suspend {
		updated, err := suspendMaterializedInstance(tx, id)
		if err != nil {
			return 0, err
		}
		changed = changed || updated
	}
	if changed {
		if revision >= uint64(^uint64(0)>>1) {
			return 0, fmt.Errorf("desired Integration revision overflow")
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.Exec(`UPDATE integration_desired_state SET desired_set_revision=desired_set_revision+1,updated_at=? WHERE id=1`, now); err != nil {
			return 0, fmt.Errorf("advance desired Integration revision after materialization: %w", err)
		}
		revision++
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit template materialization: %w", err)
	}
	unlock()
	locked = false
	if changed && s.publisher != nil {
		if err := s.publisher.Publish(events.DesiredConfigurationChanged{Revision: revision}); err != nil {
			slog.Warn("Committed desired-state wakeup was not queued", "code", "desired_event_dropped")
		}
	}
	return revision, nil
}

func validateMaterializationBatch(batch materializationdomain.Batch) error {
	return materializationdomain.ValidateBatch(batch)
}

func checkMaterializationRevisions(tx *sql.Tx, batch materializationdomain.Batch) error {
	var inventoryRevision int64
	if err := tx.QueryRow(`SELECT write_revision FROM inventory_meta WHERE id=1`).Scan(&inventoryRevision); err != nil {
		return fmt.Errorf("read inventory revision for materialization: %w", err)
	}
	templates, err := templateSetRevision(tx)
	if err != nil {
		return err
	}
	desired, err := desiredIntegrationRevision(tx)
	if err != nil {
		return err
	}
	if uint64(inventoryRevision) != batch.ExpectedInventoryWriteRevision || templates != batch.ExpectedTemplateSetRevision || desired != batch.ExpectedDesiredSetRevision {
		return materializationdomain.ErrMaterializationConflict
	}
	return nil
}

func upsertMaterializedInstance(tx *sql.Tx, instance desiredstate.Instance) (bool, error) {
	var stored []byte
	var current uint64
	err := tx.QueryRow(`SELECT payload,revision FROM integration_instances WHERE instance_id=?`, instance.ID).Scan(&stored, &current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("read generated Integration instance: %w", err)
	}
	if err == nil {
		var existing desiredstate.Instance
		if err := json.Unmarshal(stored, &existing); err != nil {
			return false, fmt.Errorf("decode generated Integration instance: %w", err)
		}
		if !sameTemplateInstanceOwner(existing, instance) {
			return false, desiredapp.ErrTemplateOwnedInstance
		}
		if sameMaterializedIntent(existing, instance) {
			return false, nil
		}
	}
	if current >= uint64(^uint64(0)>>1) {
		return false, fmt.Errorf("generated Integration instance revision overflow")
	}
	instance.Revision = current + 1
	payload, err := json.Marshal(instance)
	if err != nil {
		return false, fmt.Errorf("encode generated Integration instance: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`INSERT INTO integration_instances(instance_id,payload,revision,updated_at) VALUES(?,?,?,?)
		ON CONFLICT(instance_id) DO UPDATE SET payload=excluded.payload,revision=excluded.revision,updated_at=excluded.updated_at`,
		instance.ID, payload, instance.Revision, now); err != nil {
		return false, fmt.Errorf("persist generated Integration instance: %w", err)
	}
	return true, nil
}

func validateMaterializedTemplate(tx *sql.Tx, instance desiredstate.Instance) error {
	var payload []byte
	var revision uint64
	if err := tx.QueryRow(`SELECT payload,revision FROM integration_templates WHERE template_id=?`, instance.TemplateID).Scan(&payload, &revision); err != nil {
		return fmt.Errorf("read owning Integration Template: %w", err)
	}
	var template templatedomain.Template
	if err := json.Unmarshal(payload, &template); err != nil {
		return fmt.Errorf("decode owning Integration Template: %w", err)
	}
	if err := template.Validate(); err != nil {
		return fmt.Errorf("validate owning Integration Template: %w", err)
	}
	if template.Owner != instance.TemplateOwner || template.Revision != revision || template.Revision != instance.TemplateRevision ||
		template.Status != templatedomain.StatusEnabled || template.DefinitionID != instance.DefinitionID {
		return materializationdomain.ErrMaterializationConflict
	}
	return nil
}

func suspendMaterializedInstance(tx *sql.Tx, id string) (bool, error) {
	var payload []byte
	var revision uint64
	err := tx.QueryRow(`SELECT payload,revision FROM integration_instances WHERE instance_id=?`, id).Scan(&payload, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read Integration instance for suspension: %w", err)
	}
	var instance desiredstate.Instance
	if err := json.Unmarshal(payload, &instance); err != nil {
		return false, fmt.Errorf("decode Integration instance for suspension: %w", err)
	}
	if instance.Source != desiredstate.SourceTemplate {
		return false, desiredapp.ErrTemplateOwnedInstance
	}
	if !instance.Enabled {
		return false, nil
	}
	if revision >= uint64(^uint64(0)>>1) {
		return false, fmt.Errorf("generated Integration instance revision overflow")
	}
	instance.Enabled = false
	instance.Revision = revision + 1
	updated, err := json.Marshal(instance)
	if err != nil {
		return false, fmt.Errorf("encode suspended Integration instance: %w", err)
	}
	if _, err := tx.Exec(`UPDATE integration_instances SET payload=?,revision=?,updated_at=? WHERE instance_id=? AND revision=?`,
		updated, instance.Revision, time.Now().UTC().Format(time.RFC3339Nano), id, revision); err != nil {
		return false, fmt.Errorf("persist Integration suspension: %w", err)
	}
	return true, nil
}

func sameTemplateInstanceOwner(existing, requested desiredstate.Instance) bool {
	return existing.Source == desiredstate.SourceTemplate && existing.TemplateOwner == requested.TemplateOwner &&
		existing.TemplateID == requested.TemplateID && existing.DefinitionID == requested.DefinitionID &&
		existing.Binding != nil && requested.Binding != nil && *existing.Binding == *requested.Binding
}

func sameMaterializedIntent(existing, requested desiredstate.Instance) bool {
	existing.Revision, requested.Revision = 0, 0
	left, leftErr := json.Marshal(existing)
	right, rightErr := json.Marshal(requested)
	return leftErr == nil && rightErr == nil && string(left) == string(right)
}
