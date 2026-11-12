package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/discovery"
)

// NewProposalRepository creates the Discovery proposal adapter over the shared database.
func NewProposalRepository(open func() (*sql.DB, error), locker sync.Locker) discovery.ProposalRepository {
	return proposalSQLite{store: database{open: open, locker: locker}}
}

type proposalSQLite struct{ store database }

func (s proposalSQLite) ProposalSnapshot() ([]discovery.Proposal, error) {
	db, err := s.store.openDatabase()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin proposal snapshot: %w", err)
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT proposal_id,provider_id,provider_instance_id,resource_id,integration_id,revision,status,payload FROM integration_proposals ORDER BY proposal_id`)
	if err != nil {
		return nil, fmt.Errorf("query monitoring proposals: %w", err)
	}
	proposals := make([]discovery.Proposal, 0)
	for rows.Next() {
		var id, provider, providerInstance, resourceID, integrationID, status string
		var revision uint64
		var payload []byte
		if err := rows.Scan(&id, &provider, &providerInstance, &resourceID, &integrationID, &revision, &status, &payload); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read monitoring proposal: %w", err)
		}
		var proposal discovery.Proposal
		if err := json.Unmarshal(payload, &proposal); err != nil {
			rows.Close()
			return nil, fmt.Errorf("decode monitoring proposal: %w", err)
		}
		if err := discovery.ValidateProposal(proposal); err != nil {
			rows.Close()
			return nil, fmt.Errorf("validate persisted monitoring proposal: %w", err)
		}
		if proposal.ID != id || proposal.Identity.Provider != provider || proposal.Identity.ProviderInstance != providerInstance ||
			proposal.Identity.ResourceID != resourceID || proposal.Identity.IntegrationID != integrationID ||
			proposal.Revision != revision || string(proposal.Status) != status {
			rows.Close()
			return nil, fmt.Errorf("%w: proposal columns disagree with payload", discovery.ErrInvalidProposal)
		}
		proposals = append(proposals, proposal)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate monitoring proposals: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close monitoring proposal rows: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit monitoring proposal snapshot: %w", err)
	}
	return proposals, nil
}

func (s proposalSQLite) PutProposal(proposal discovery.Proposal, expectedRevision uint64) error {
	if err := discovery.ValidateProposal(proposal); err != nil {
		return err
	}
	if expectedRevision > math.MaxInt64 || proposal.Revision > math.MaxInt64 ||
		(expectedRevision == 0 && proposal.Revision != 1) ||
		(expectedRevision > 0 && proposal.Revision != expectedRevision+1) {
		return fmt.Errorf("%w: invalid proposal revision transition", discovery.ErrInvalidProposal)
	}
	encoded, err := json.Marshal(proposal)
	if err != nil {
		return fmt.Errorf("encode monitoring proposal: %w", err)
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
		return fmt.Errorf("begin monitoring proposal update: %w", err)
	}
	defer tx.Rollback()
	if expectedRevision == 0 {
		var exists int
		lookupErr := tx.QueryRow(`SELECT 1 FROM integration_proposals WHERE proposal_id=?`, proposal.ID).Scan(&exists)
		if lookupErr == nil {
			return discovery.ErrProposalConflict
		}
		if !errors.Is(lookupErr, sql.ErrNoRows) {
			return fmt.Errorf("check monitoring proposal before insert: %w", lookupErr)
		}
		_, err = tx.Exec(`INSERT INTO integration_proposals(proposal_id,provider_id,provider_instance_id,resource_id,integration_id,revision,status,payload,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			proposal.ID, proposal.Identity.Provider, proposal.Identity.ProviderInstance, proposal.Identity.ResourceID, proposal.Identity.IntegrationID,
			proposal.Revision, proposal.Status, encoded, proposal.UpdatedAt.UTC().Format(time.RFC3339Nano))
	} else {
		result, updateErr := tx.Exec(`UPDATE integration_proposals SET revision=?,status=?,payload=?,updated_at=? WHERE proposal_id=? AND revision=?`,
			proposal.Revision, proposal.Status, encoded, proposal.UpdatedAt.UTC().Format(time.RFC3339Nano), proposal.ID, expectedRevision)
		err = updateErr
		if err == nil {
			var affected int64
			affected, err = result.RowsAffected()
			if err == nil && affected != 1 {
				err = discovery.ErrProposalConflict
			}
		}
	}
	if err != nil {
		return fmt.Errorf("persist monitoring proposal: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit monitoring proposal: %w", err)
	}
	return nil
}

func (s proposalSQLite) DismissProposal(id string, expectedRevision uint64, at time.Time) error {
	if id == "" || expectedRevision == 0 || expectedRevision > math.MaxInt64 || at.IsZero() {
		return discovery.ErrInvalidProposal
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
		return fmt.Errorf("begin proposal dismissal: %w", err)
	}
	defer tx.Rollback()
	var payload []byte
	err = tx.QueryRow(`SELECT payload FROM integration_proposals WHERE proposal_id=? AND revision=?`, id, expectedRevision).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		var exists int
		if lookupErr := tx.QueryRow(`SELECT 1 FROM integration_proposals WHERE proposal_id=?`, id).Scan(&exists); errors.Is(lookupErr, sql.ErrNoRows) {
			return discovery.ErrUnknownProposal
		} else if lookupErr != nil {
			return fmt.Errorf("check proposal before dismissal: %w", lookupErr)
		}
		return discovery.ErrProposalConflict
	}
	if err != nil {
		return fmt.Errorf("load proposal before dismissal: %w", err)
	}
	var proposal discovery.Proposal
	if err := json.Unmarshal(payload, &proposal); err != nil {
		return fmt.Errorf("decode proposal before dismissal: %w", err)
	}
	if proposal.Status != discovery.ProposalPending && proposal.Status != discovery.ProposalNeedsInput {
		return discovery.ErrProposalTransition
	}
	proposal.Status = discovery.ProposalDismissed
	proposal.UpdatedAt = at.UTC()
	if err := discovery.ValidateProposal(proposal); err != nil {
		return err
	}
	encoded, err := json.Marshal(proposal)
	if err != nil {
		return fmt.Errorf("encode dismissed proposal: %w", err)
	}
	result, err := tx.Exec(`UPDATE integration_proposals SET status=?,payload=?,updated_at=? WHERE proposal_id=? AND revision=?`,
		proposal.Status, encoded, proposal.UpdatedAt.Format(time.RFC3339Nano), id, expectedRevision)
	if err != nil {
		return fmt.Errorf("persist proposal dismissal: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return fmt.Errorf("check proposal dismissal: %w", err)
		}
		return discovery.ErrProposalConflict
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit proposal dismissal: %w", err)
	}
	return nil
}
