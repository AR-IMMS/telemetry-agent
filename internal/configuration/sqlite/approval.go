package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	desiredstateapp "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/application"
	"github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	"github.com/ar-imms/telemetry-agent/internal/configuration/proposals/application"
	proposaldomain "github.com/ar-imms/telemetry-agent/internal/configuration/proposals/domain"
)

// NewApprovalRepository creates the atomic proposal-approval adapter over the shared database.
// The opener and write lock are required; repository operations return an error if either is missing.
func NewApprovalRepository(open func() (*sql.DB, error), locker sync.Locker) application.ApprovalRepository {
	return approvalSQLite{store: database{open: open, locker: locker}}
}

type approvalSQLite struct{ store database }

func (s approvalSQLite) CommitApproval(commit application.ApprovalCommit) (domain.Instance, uint64, error) {
	return s.CommitApprovalContext(context.Background(), commit)
}

// CommitApprovalContext atomically persists approval with caller cancellation.
func (s approvalSQLite) CommitApprovalContext(ctx context.Context, commit application.ApprovalCommit) (domain.Instance, uint64, error) {
	if err := ctx.Err(); err != nil {
		return domain.Instance{}, 0, err
	}
	if commit.ProposalID == "" || commit.ProposalRevision == 0 || commit.ProposalRevision > math.MaxInt64 {
		return domain.Instance{}, 0, proposaldomain.ErrInvalidProposal
	}
	unlock, err := s.store.lockWrites()
	if err != nil {
		return domain.Instance{}, 0, err
	}
	defer unlock()
	db, err := s.store.openDatabase()
	if err != nil {
		return domain.Instance{}, 0, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Instance{}, 0, fmt.Errorf("begin monitoring proposal approval: %w", err)
	}
	defer tx.Rollback()
	var payload []byte
	var persistedRevision uint64
	err = tx.QueryRowContext(ctx, `SELECT payload,revision FROM integration_proposals WHERE proposal_id=?`, commit.ProposalID).Scan(&payload, &persistedRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Instance{}, 0, proposaldomain.ErrUnknownProposal
	}
	if err != nil {
		return domain.Instance{}, 0, fmt.Errorf("load proposal for approval: %w", err)
	}
	var proposal proposaldomain.Proposal
	if err := json.Unmarshal(payload, &proposal); err != nil {
		return domain.Instance{}, 0, fmt.Errorf("decode proposal for approval: %w", err)
	}
	if persistedRevision != commit.ProposalRevision || proposal.Revision != commit.ProposalRevision {
		return domain.Instance{}, 0, application.ErrApprovalConflict
	}
	if proposal.Status == proposaldomain.ProposalApproved {
		return alreadyApproved(ctx, tx, proposal, commit)
	}
	if proposal.Status != proposaldomain.ProposalPending && proposal.Status != proposaldomain.ProposalNeedsInput {
		return domain.Instance{}, 0, application.ErrApprovalNotEligible
	}
	if proposal.CandidateDigest != commit.ExpectedCandidateDigest || commit.VerifiedCandidateDigest == "" ||
		(proposal.CandidateDigest != "" && proposal.CandidateDigest != commit.VerifiedCandidateDigest) {
		return domain.Instance{}, 0, application.ErrApprovalStale
	}
	if commit.InventoryWriteRevision > math.MaxInt64 {
		return domain.Instance{}, 0, application.ErrApprovalStale
	}
	var writeRevision uint64
	if err := tx.QueryRowContext(ctx, `SELECT write_revision FROM inventory_meta WHERE id=1`).Scan(&writeRevision); err != nil {
		return domain.Instance{}, 0, fmt.Errorf("read inventory approval token: %w", err)
	}
	if writeRevision != commit.InventoryWriteRevision {
		return domain.Instance{}, 0, application.ErrApprovalStale
	}
	var desiredRevision uint64
	if err := tx.QueryRowContext(ctx, `SELECT desired_set_revision FROM integration_desired_state WHERE id=1`).Scan(&desiredRevision); err != nil {
		return domain.Instance{}, 0, fmt.Errorf("read desired Integration revision: %w", err)
	}
	if desiredRevision != commit.ExpectedDesiredRevision {
		return domain.Instance{}, 0, desiredstateapp.ErrRevisionConflict
	}
	instance := commit.Instance
	if err := validateApprovalInstance(proposal, instance); err != nil {
		return domain.Instance{}, 0, err
	}
	if err := ensureUniqueResourceBinding(ctx, tx, instance); err != nil {
		return domain.Instance{}, 0, err
	}
	var currentPayload []byte
	var currentRevision uint64
	err = tx.QueryRowContext(ctx, `SELECT payload,revision FROM integration_instances WHERE instance_id=?`, instance.ID).Scan(&currentPayload, &currentRevision)
	if errors.Is(err, sql.ErrNoRows) {
		if proposal.ApprovedInstanceRevision != 0 {
			return domain.Instance{}, 0, application.ErrApprovalConflict
		}
		currentRevision = 0
	} else if err != nil {
		return domain.Instance{}, 0, fmt.Errorf("read existing approved Integration: %w", err)
	} else if currentRevision != proposal.ApprovedInstanceRevision {
		return domain.Instance{}, 0, application.ErrApprovalConflict
	} else if currentRevision > 0 {
		var current domain.Instance
		if err := json.Unmarshal(currentPayload, &current); err != nil {
			return domain.Instance{}, 0, fmt.Errorf("decode existing approved Integration: %w", err)
		}
		if current.Binding == nil || *current.Binding != *instance.Binding {
			return domain.Instance{}, 0, application.ErrApprovalBindingConflict
		}
	}
	if currentRevision == math.MaxInt64 || desiredRevision == math.MaxInt64 {
		return domain.Instance{}, 0, fmt.Errorf("approval revision overflow")
	}
	instance.Revision = currentRevision + 1
	instancePayload, err := json.Marshal(instance)
	if err != nil {
		return domain.Instance{}, 0, fmt.Errorf("encode approved Integration: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO integration_instances(instance_id,payload,revision,updated_at) VALUES(?,?,?,?)
		ON CONFLICT(instance_id) DO UPDATE SET payload=excluded.payload,revision=excluded.revision,updated_at=excluded.updated_at`,
		instance.ID, instancePayload, instance.Revision, now); err != nil {
		return domain.Instance{}, 0, fmt.Errorf("persist approved Integration: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE integration_desired_state SET desired_set_revision=desired_set_revision+1,updated_at=? WHERE id=1 AND desired_set_revision=?`, now, desiredRevision)
	if err != nil {
		return domain.Instance{}, 0, fmt.Errorf("advance desired Integration revision for approval: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return domain.Instance{}, 0, fmt.Errorf("check desired Integration revision update: %w", err)
		}
		return domain.Instance{}, 0, desiredstateapp.ErrRevisionConflict
	}
	proposal.Status = proposaldomain.ProposalApproved
	proposal.ApprovedInstanceID = instance.ID
	proposal.ApprovedInstanceRevision = instance.Revision
	proposal.ApprovedCandidateDigest = commit.VerifiedCandidateDigest
	proposal.UpdatedAt = time.Now().UTC()
	proposalPayload, err := json.Marshal(proposal)
	if err != nil {
		return domain.Instance{}, 0, fmt.Errorf("encode approved proposal: %w", err)
	}
	result, err = tx.ExecContext(ctx, `UPDATE integration_proposals SET status=?,payload=?,updated_at=? WHERE proposal_id=? AND revision=? AND status IN ('pending','needs_input')`,
		proposal.Status, proposalPayload, proposal.UpdatedAt.Format(time.RFC3339Nano), proposal.ID, commit.ProposalRevision)
	if err != nil {
		return domain.Instance{}, 0, fmt.Errorf("persist proposal approval: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return domain.Instance{}, 0, fmt.Errorf("check proposal approval update: %w", err)
		}
		return domain.Instance{}, 0, application.ErrApprovalConflict
	}
	if err := tx.Commit(); err != nil {
		return domain.Instance{}, 0, fmt.Errorf("commit proposal approval and desired Integration: %w", err)
	}
	return instance, desiredRevision + 1, nil
}

func alreadyApproved(ctx context.Context, tx *sql.Tx, proposal proposaldomain.Proposal, commit application.ApprovalCommit) (domain.Instance, uint64, error) {
	if proposal.ApprovedInstanceID == "" || proposal.ApprovedInstanceID != commit.Instance.ID || proposal.ApprovedCandidateDigest == "" {
		return domain.Instance{}, 0, application.ErrApprovalConflict
	}
	var payload []byte
	var revision, desiredRevision uint64
	if err := tx.QueryRowContext(ctx, `SELECT payload,revision FROM integration_instances WHERE instance_id=?`, proposal.ApprovedInstanceID).Scan(&payload, &revision); err != nil {
		return domain.Instance{}, 0, application.ErrApprovalConflict
	}
	if revision != proposal.ApprovedInstanceRevision {
		return domain.Instance{}, 0, application.ErrApprovalConflict
	}
	if err := tx.QueryRowContext(ctx, `SELECT desired_set_revision FROM integration_desired_state WHERE id=1`).Scan(&desiredRevision); err != nil {
		return domain.Instance{}, 0, fmt.Errorf("read desired Integration revision for idempotent approval: %w", err)
	}
	var instance domain.Instance
	if err := json.Unmarshal(payload, &instance); err != nil {
		return domain.Instance{}, 0, fmt.Errorf("decode idempotent approved Integration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.Instance{}, 0, fmt.Errorf("commit idempotent proposal approval read: %w", err)
	}
	return instance, desiredRevision, nil
}

func validateApprovalInstance(proposal proposaldomain.Proposal, instance domain.Instance) error {
	if err := instance.Validate(); err != nil {
		return fmt.Errorf("validate approved Integration: %w", err)
	}
	want := domain.ResourceBinding{Provider: proposal.Identity.Provider, ProviderInstance: proposal.Identity.ProviderInstance, ResourceID: proposal.Identity.ResourceID}
	if instance.Source != domain.SourceDiscovery || instance.Binding == nil || *instance.Binding != want || instance.DefinitionID != proposal.Identity.IntegrationID {
		return application.ErrApprovalStale
	}
	return nil
}

func ensureUniqueResourceBinding(ctx context.Context, tx *sql.Tx, instance domain.Instance) error {
	rows, err := tx.QueryContext(ctx, `SELECT instance_id,payload FROM integration_instances ORDER BY instance_id`)
	if err != nil {
		return fmt.Errorf("check desired resource bindings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var payload []byte
		if err := rows.Scan(&id, &payload); err != nil {
			return fmt.Errorf("read desired resource binding: %w", err)
		}
		if id == instance.ID {
			continue
		}
		var current domain.Instance
		if err := json.Unmarshal(payload, &current); err != nil {
			return fmt.Errorf("decode desired resource binding: %w", err)
		}
		if current.Binding != nil && instance.Binding != nil && *current.Binding == *instance.Binding {
			return application.ErrApprovalBindingConflict
		}
		if current.DefinitionID == instance.DefinitionID && sameConfiguration(current.Configuration, instance.Configuration) {
			return application.ErrApprovalDuplicate
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate desired resource bindings: %w", err)
	}
	return nil
}

func sameConfiguration(left, right map[string]any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}
