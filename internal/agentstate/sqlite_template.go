package agentstate

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/configuration"
	"github.com/ar-imms/telemetry-agent/internal/integration"
)

// TemplateRepository returns the SQLite adapter for Integration Templates.
func (s SQLiteStore) TemplateRepository() configuration.TemplateRepository {
	return templateSQLite{store: s}
}

type templateSQLite struct{ store SQLiteStore }

func (s templateSQLite) Put(template integration.IntegrationTemplate, expected uint64) (integration.IntegrationTemplate, uint64, error) {
	if template.Owner != integration.TemplateOwnerLocal {
		return integration.IntegrationTemplate{}, 0, configuration.ErrTemplateOwnerNotLocal
	}
	if err := integration.ValidateIntegrationTemplate(template); err != nil {
		return integration.IntegrationTemplate{}, 0, err
	}
	requested, err := json.Marshal(template)
	if err != nil || len(requested) > 64*1024 {
		return integration.IntegrationTemplate{}, 0, fmt.Errorf("encode Integration Template: invalid or oversized payload")
	}
	sqliteUpdateMutex.Lock()
	defer sqliteUpdateMutex.Unlock()
	db, err := s.store.open()
	if err != nil {
		return integration.IntegrationTemplate{}, 0, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return integration.IntegrationTemplate{}, 0, fmt.Errorf("begin Integration Template mutation: %w", err)
	}
	defer tx.Rollback()
	var old []byte
	var current uint64
	err = tx.QueryRow(`SELECT payload,revision FROM integration_templates WHERE template_id=?`, template.ID).Scan(&old, &current)
	if errors.Is(err, sql.ErrNoRows) {
		if expected != 0 {
			return integration.IntegrationTemplate{}, 0, configuration.ErrRevisionConflict
		}
		current = 0
	} else if err != nil {
		return integration.IntegrationTemplate{}, 0, err
	} else if expected != current {
		return integration.IntegrationTemplate{}, 0, configuration.ErrRevisionConflict
	}
	if old != nil {
		var previous integration.IntegrationTemplate
		if err := json.Unmarshal(old, &previous); err != nil {
			return integration.IntegrationTemplate{}, 0, fmt.Errorf("decode persisted Integration Template: %w", err)
		}
		if previous.Status == integration.TemplateDeleted && template.Status != integration.TemplateDeleted {
			return integration.IntegrationTemplate{}, 0, configuration.ErrTemplateDeleted
		}
		if previous.DefinitionID != template.DefinitionID {
			return integration.IntegrationTemplate{}, 0, configuration.ErrTemplateTargetImmutable
		}
		if sameTemplateIntent(old, requested) {
			setRevision, err := templateSetRevision(tx)
			if err != nil {
				return integration.IntegrationTemplate{}, 0, err
			}
			if err := tx.Commit(); err != nil {
				return integration.IntegrationTemplate{}, 0, fmt.Errorf("commit no-op Integration Template mutation: %w", err)
			}
			return previous, setRevision, nil
		}
	}
	if current >= uint64(^uint64(0)>>1) {
		return integration.IntegrationTemplate{}, 0, fmt.Errorf("Integration Template revision overflow")
	}
	template.Revision = current + 1
	requested, err = json.Marshal(template)
	if err != nil {
		return integration.IntegrationTemplate{}, 0, fmt.Errorf("encode versioned Integration Template: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`INSERT INTO integration_templates(template_id,payload,revision,status,updated_at) VALUES(?,?,?,?,?)
		ON CONFLICT(template_id) DO UPDATE SET payload=excluded.payload,revision=excluded.revision,status=excluded.status,updated_at=excluded.updated_at`,
		template.ID, requested, template.Revision, template.Status, now); err != nil {
		return integration.IntegrationTemplate{}, 0, fmt.Errorf("write Integration Template: %w", err)
	}
	if _, err := tx.Exec(`UPDATE integration_template_state SET template_set_revision=template_set_revision+1,updated_at=? WHERE id=1`, now); err != nil {
		return integration.IntegrationTemplate{}, 0, fmt.Errorf("advance Integration Template set revision: %w", err)
	}
	setRevision, err := templateSetRevision(tx)
	if err != nil {
		return integration.IntegrationTemplate{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return integration.IntegrationTemplate{}, 0, fmt.Errorf("commit Integration Template mutation: %w", err)
	}
	return template, setRevision, nil
}

func sameTemplateIntent(stored, requested []byte) bool {
	var current, desired integration.IntegrationTemplate
	if json.Unmarshal(stored, &current) != nil || json.Unmarshal(requested, &desired) != nil {
		return false
	}
	current.Revision, desired.Revision = 0, 0
	left, leftErr := json.Marshal(current)
	right, rightErr := json.Marshal(desired)
	return leftErr == nil && rightErr == nil && string(left) == string(right)
}

func (s templateSQLite) Snapshot() (configuration.TemplateSnapshot, error) {
	db, err := s.store.open()
	if err != nil {
		return configuration.TemplateSnapshot{}, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return configuration.TemplateSnapshot{}, fmt.Errorf("begin Integration Template snapshot: %w", err)
	}
	defer tx.Rollback()
	revision, err := templateSetRevision(tx)
	if err != nil {
		return configuration.TemplateSnapshot{}, err
	}
	rows, err := tx.Query(`SELECT payload FROM integration_templates ORDER BY template_id`)
	if err != nil {
		return configuration.TemplateSnapshot{}, fmt.Errorf("query Integration Templates: %w", err)
	}
	defer rows.Close()
	snapshot := configuration.TemplateSnapshot{Revision: revision, Templates: []integration.IntegrationTemplate{}}
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return configuration.TemplateSnapshot{}, err
		}
		var template integration.IntegrationTemplate
		if err := json.Unmarshal(payload, &template); err != nil {
			return configuration.TemplateSnapshot{}, fmt.Errorf("decode Integration Template snapshot: %w", err)
		}
		if err := integration.ValidateIntegrationTemplate(template); err != nil {
			return configuration.TemplateSnapshot{}, fmt.Errorf("validate persisted Integration Template: %w", err)
		}
		snapshot.Templates = append(snapshot.Templates, template)
	}
	if err := rows.Err(); err != nil {
		return configuration.TemplateSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return configuration.TemplateSnapshot{}, fmt.Errorf("finish Integration Template snapshot: %w", err)
	}
	return snapshot, nil
}

func templateSetRevision(queryer interface{ QueryRow(string, ...any) *sql.Row }) (uint64, error) {
	var revision uint64
	if err := queryer.QueryRow(`SELECT template_set_revision FROM integration_template_state WHERE id=1`).Scan(&revision); err != nil {
		return 0, fmt.Errorf("read Integration Template set revision: %w", err)
	}
	return revision, nil
}
