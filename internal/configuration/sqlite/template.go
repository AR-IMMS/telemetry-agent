package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	configurationdomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	templatesapp "github.com/ar-imms/telemetry-agent/internal/configuration/templates/application"
	templatesdomain "github.com/ar-imms/telemetry-agent/internal/configuration/templates/domain"
)

// NewTemplateRepository creates the Integration Template adapter over the shared database.
// The opener and write lock are required; repository operations return an error if either is missing.
func NewTemplateRepository(open func() (*sql.DB, error), locker sync.Locker) templatesapp.Repository {
	return templateSQLite{store: database{open: open, locker: locker}}
}

type templateSQLite struct{ store database }

func (s templateSQLite) Put(template templatesdomain.Template, expected uint64) (templatesdomain.Template, uint64, error) {
	if template.Owner != configurationdomain.TemplateOwnerLocal {
		return templatesdomain.Template{}, 0, templatesapp.ErrTemplateOwnerNotLocal
	}
	if err := template.Validate(); err != nil {
		return templatesdomain.Template{}, 0, err
	}
	requested, err := json.Marshal(template)
	if err != nil || len(requested) > 64*1024 {
		return templatesdomain.Template{}, 0, fmt.Errorf("encode Integration Template: invalid or oversized payload")
	}
	unlock, err := s.store.lockWrites()
	if err != nil {
		return templatesdomain.Template{}, 0, err
	}
	defer unlock()
	db, err := s.store.openDatabase()
	if err != nil {
		return templatesdomain.Template{}, 0, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return templatesdomain.Template{}, 0, fmt.Errorf("begin Integration Template mutation: %w", err)
	}
	defer tx.Rollback()
	var old []byte
	var current uint64
	err = tx.QueryRow(`SELECT payload,revision FROM integration_templates WHERE template_id=?`, template.ID).Scan(&old, &current)
	if errors.Is(err, sql.ErrNoRows) {
		if expected != 0 {
			return templatesdomain.Template{}, 0, templatesapp.ErrRevisionConflict
		}
		current = 0
	} else if err != nil {
		return templatesdomain.Template{}, 0, err
	} else if expected != current {
		return templatesdomain.Template{}, 0, templatesapp.ErrRevisionConflict
	}
	if old != nil {
		var previous templatesdomain.Template
		if err := json.Unmarshal(old, &previous); err != nil {
			return templatesdomain.Template{}, 0, fmt.Errorf("decode persisted Integration Template: %w", err)
		}
		if previous.Status == templatesdomain.StatusDeleted && template.Status != templatesdomain.StatusDeleted {
			return templatesdomain.Template{}, 0, templatesapp.ErrTemplateDeleted
		}
		if previous.DefinitionID != template.DefinitionID {
			return templatesdomain.Template{}, 0, templatesapp.ErrTemplateTargetImmutable
		}
		if sameTemplateIntent(old, requested) {
			setRevision, err := templateSetRevision(tx)
			if err != nil {
				return templatesdomain.Template{}, 0, err
			}
			if err := tx.Commit(); err != nil {
				return templatesdomain.Template{}, 0, fmt.Errorf("commit no-op Integration Template mutation: %w", err)
			}
			return previous, setRevision, nil
		}
	}
	if current >= uint64(^uint64(0)>>1) {
		return templatesdomain.Template{}, 0, fmt.Errorf("Integration Template revision overflow")
	}
	template.Revision = current + 1
	requested, err = json.Marshal(template)
	if err != nil {
		return templatesdomain.Template{}, 0, fmt.Errorf("encode versioned Integration Template: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`INSERT INTO integration_templates(template_id,payload,revision,status,updated_at) VALUES(?,?,?,?,?)
		ON CONFLICT(template_id) DO UPDATE SET payload=excluded.payload,revision=excluded.revision,status=excluded.status,updated_at=excluded.updated_at`,
		template.ID, requested, template.Revision, template.Status, now); err != nil {
		return templatesdomain.Template{}, 0, fmt.Errorf("write Integration Template: %w", err)
	}
	if _, err := tx.Exec(`UPDATE integration_template_state SET template_set_revision=template_set_revision+1,updated_at=? WHERE id=1`, now); err != nil {
		return templatesdomain.Template{}, 0, fmt.Errorf("advance Integration Template set revision: %w", err)
	}
	setRevision, err := templateSetRevision(tx)
	if err != nil {
		return templatesdomain.Template{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return templatesdomain.Template{}, 0, fmt.Errorf("commit Integration Template mutation: %w", err)
	}
	return template, setRevision, nil
}

func sameTemplateIntent(stored, requested []byte) bool {
	var current, desired templatesdomain.Template
	if json.Unmarshal(stored, &current) != nil || json.Unmarshal(requested, &desired) != nil {
		return false
	}
	current.Revision, desired.Revision = 0, 0
	left, leftErr := json.Marshal(current)
	right, rightErr := json.Marshal(desired)
	return leftErr == nil && rightErr == nil && string(left) == string(right)
}

func (s templateSQLite) Snapshot() (templatesapp.Snapshot, error) {
	db, err := s.store.openDatabase()
	if err != nil {
		return templatesapp.Snapshot{}, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return templatesapp.Snapshot{}, fmt.Errorf("begin Integration Template snapshot: %w", err)
	}
	defer tx.Rollback()
	revision, err := templateSetRevision(tx)
	if err != nil {
		return templatesapp.Snapshot{}, err
	}
	rows, err := tx.Query(`SELECT payload FROM integration_templates ORDER BY template_id`)
	if err != nil {
		return templatesapp.Snapshot{}, fmt.Errorf("query Integration Templates: %w", err)
	}
	defer rows.Close()
	snapshot := templatesapp.Snapshot{Revision: revision, Templates: []templatesdomain.Template{}}
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return templatesapp.Snapshot{}, err
		}
		var template templatesdomain.Template
		if err := json.Unmarshal(payload, &template); err != nil {
			return templatesapp.Snapshot{}, fmt.Errorf("decode Integration Template snapshot: %w", err)
		}
		if err := template.Validate(); err != nil {
			return templatesapp.Snapshot{}, fmt.Errorf("validate persisted Integration Template: %w", err)
		}
		snapshot.Templates = append(snapshot.Templates, template)
	}
	if err := rows.Err(); err != nil {
		return templatesapp.Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return templatesapp.Snapshot{}, fmt.Errorf("finish Integration Template snapshot: %w", err)
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
