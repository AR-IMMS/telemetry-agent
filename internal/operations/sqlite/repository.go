package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/operations"
	"github.com/google/uuid"
)

type databaseOpener interface{ OpenDatabase() (*sql.DB, error) }

// Repository persists operation identity, append-only audit events, and result publication state.
type Repository struct{ open func() (*sql.DB, error) }

// RecoverInterrupted closes unfinished read-only operations before command consumption starts.
func (r *Repository) RecoverInterrupted(ctx context.Context, now time.Time) (int, error) {
	db, err := r.database()
	if err != nil {
		return 0, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin interrupted operation recovery: %w", err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT operation_id,operation_type,installation_id,binding_id,target_kind,target_provider_id,target_resource_id,started_at FROM operation_execution WHERE status='RUNNING' ORDER BY received_at,operation_id`)
	if err != nil {
		return 0, fmt.Errorf("read interrupted operations: %w", err)
	}
	type interrupted struct {
		operationID, operationType, installationID, bindingID string
		targetKind, providerID, resourceID, startedAt         string
	}
	var pending []interrupted
	for rows.Next() {
		var item interrupted
		if err := rows.Scan(&item.operationID, &item.operationType, &item.installationID, &item.bindingID,
			&item.targetKind, &item.providerID, &item.resourceID, &item.startedAt); err != nil {
			rows.Close()
			return 0, fmt.Errorf("read interrupted operation row: %w", err)
		}
		pending = append(pending, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate interrupted operations: %w", err)
	}
	rows.Close()
	completed := now.UTC()
	completedText := completed.Format(time.RFC3339Nano)
	for _, item := range pending {
		started, _ := time.Parse(time.RFC3339Nano, item.startedAt)
		status, errorCode, errorMessage := operations.StatusFailed, "EXECUTION_INTERRUPTED", "read-only operation was interrupted before a result was recorded"
		if operations.OperationType(item.operationType) == operations.OperationContainerRestart {
			status, errorCode, errorMessage = operations.StatusUnknown, "OPERATION_OUTCOME_UNKNOWN", "container restart may have taken effect before the Agent stopped; do not retry automatically"
		}
		result := operations.Result{SchemaVersion: "ar-imms.operation.result.v1", EventID: uuid.NewString(),
			OperationID: item.operationID, OperationType: operations.OperationType(item.operationType),
			InstallationID: item.installationID, BindingID: item.bindingID, Status: status,
			Target:    operations.Target{Kind: item.targetKind, ProviderID: item.providerID, ResourceID: item.resourceID},
			StartedAt: started, CompletedAt: completed, ErrorCode: errorCode,
			ErrorMessage: errorMessage, AuditRef: item.operationID}
		payload, err := json.Marshal(result)
		if err != nil {
			return 0, fmt.Errorf("encode interrupted operation result: %w", err)
		}
		storedStatus := string(result.Status)
		if result.Status == operations.StatusUnknown {
			storedStatus = "UNKNOWN_OUTCOME"
		}
		update, err := tx.ExecContext(ctx, `UPDATE operation_execution SET status=?,completed_at=?,result_json=?,error_code=?,error_message=? WHERE operation_id=? AND status='RUNNING'`,
			storedStatus, completedText, string(payload), result.ErrorCode, result.ErrorMessage, item.operationID)
		if err != nil {
			return 0, fmt.Errorf("persist interrupted operation result: %w", err)
		}
		changed, err := update.RowsAffected()
		if err != nil || changed != 1 {
			return 0, fmt.Errorf("interrupted operation changed during recovery")
		}
		var sequence int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence_number),0)+1 FROM operation_audit_event WHERE operation_id=?`, item.operationID).Scan(&sequence); err != nil {
			return 0, fmt.Errorf("read interrupted operation audit sequence: %w", err)
		}
		details, _ := json.Marshal(map[string]string{"status": string(result.Status), "error_code": result.ErrorCode})
		if _, err := tx.ExecContext(ctx, `INSERT INTO operation_audit_event(event_id,operation_id,sequence_number,event_type,occurred_at,details_json) VALUES(?,?,?,?,?,?)`,
			uuid.NewString(), item.operationID, sequence, "execution_recovered", completedText, string(details)); err != nil {
			return 0, fmt.Errorf("persist interrupted operation audit: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO operation_result_outbox(event_id,operation_id,event_version,subject,payload_json,created_at,next_attempt_at) VALUES(?,?,?,?,?,?,?)`,
			result.EventID, item.operationID, result.SchemaVersion, "arimms.ops.v1.result."+item.installationID, string(payload), completedText, completedText); err != nil {
			return 0, fmt.Errorf("persist interrupted operation outbox: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit interrupted operation recovery: %w", err)
	}
	return len(pending), nil
}

// New creates an Operations repository over the Agent's existing SQLite store.
func New(store databaseOpener) *Repository {
	if store == nil {
		return &Repository{}
	}
	return &Repository{open: store.OpenDatabase}
}

// Claim creates a durable RUNNING record and initial receipt/start audit events.
func (r *Repository) Claim(ctx context.Context, request operations.Request, fingerprint string, now time.Time) (operations.StoredExecution, bool, error) {
	db, err := r.database()
	if err != nil {
		return operations.StoredExecution{}, false, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return operations.StoredExecution{}, false, fmt.Errorf("begin operation claim: %w", err)
	}
	defer tx.Rollback()
	var existingFingerprint, status, started string
	var resultJSON sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT request_hash,status,started_at,result_json FROM operation_execution WHERE operation_id=?`, request.OperationID).
		Scan(&existingFingerprint, &status, &started, &resultJSON)
	if err == nil {
		if existingFingerprint != fingerprint {
			return operations.StoredExecution{}, false, operations.ErrOperationConflict
		}
		stored := operations.StoredExecution{Fingerprint: fingerprint}
		stored.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
		if resultJSON.Valid {
			var result operations.Result
			if err := json.Unmarshal([]byte(resultJSON.String), &result); err != nil {
				return operations.StoredExecution{}, false, fmt.Errorf("decode stored operation result: %w", err)
			}
			stored.Result = &result
		}
		if err := tx.Commit(); err != nil {
			return operations.StoredExecution{}, false, fmt.Errorf("finish duplicate operation lookup: %w", err)
		}
		return stored, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return operations.StoredExecution{}, false, fmt.Errorf("read operation execution identity: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, request.ExpiresAt)
	if err != nil {
		return operations.StoredExecution{}, false, fmt.Errorf("parse operation expiry: %w", err)
	}
	startedAt := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO operation_execution(
		operation_id,operation_type,schema_version,request_hash,installation_id,binding_id,
		target_kind,target_provider_id,target_resource_id,requested_by,authorization_ref,status,
		received_at,started_at,expires_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,'RUNNING',?,?,?)`,
		request.OperationID, request.OperationType, request.SchemaVersion, fingerprint,
		request.InstallationID, request.BindingID, request.Target.Kind, request.Target.ProviderID,
		request.Target.ResourceID, request.RequestedBy, request.AuthorizationRef, startedAt, startedAt,
		expiresAt.UTC().Format(time.RFC3339Nano)); err != nil {
		return operations.StoredExecution{}, false, fmt.Errorf("persist operation execution claim: %w", err)
	}
	for index, eventType := range []string{"operation_received", "execution_started"} {
		details, _ := json.Marshal(map[string]string{
			"operation_type": string(request.OperationType), "requested_by": request.RequestedBy,
			"authorization_ref": request.AuthorizationRef, "target_kind": request.Target.Kind,
			"target_provider_id": request.Target.ProviderID, "target_resource_id": request.Target.ResourceID,
		})
		if _, err := tx.ExecContext(ctx, `INSERT INTO operation_audit_event(event_id,operation_id,sequence_number,event_type,occurred_at,details_json) VALUES(?,?,?,?,?,?)`,
			uuid.NewString(), request.OperationID, index+1, eventType, startedAt, string(details)); err != nil {
			return operations.StoredExecution{}, false, fmt.Errorf("persist operation start audit: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return operations.StoredExecution{}, false, fmt.Errorf("commit operation execution claim: %w", err)
	}
	return operations.StoredExecution{Fingerprint: fingerprint, StartedAt: now.UTC()}, true, nil
}

// Complete commits terminal state, final audit, and the result outbox row atomically.
func (r *Repository) Complete(ctx context.Context, request operations.Request, fingerprint string, result operations.Result) error {
	db, err := r.database()
	if err != nil {
		return err
	}
	defer db.Close()
	payload, err := json.Marshal(result)
	if err != nil || len(payload) > 64*1024 {
		return fmt.Errorf("encode bounded operation result")
	}
	details, err := json.Marshal(map[string]string{"status": string(result.Status), "error_code": result.ErrorCode})
	if err != nil {
		return fmt.Errorf("encode operation audit evidence: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin operation outcome transaction: %w", err)
	}
	defer tx.Rollback()
	completed := result.CompletedAt.UTC().Format(time.RFC3339Nano)
	storedStatus := string(result.Status)
	if result.Status == operations.StatusUnknown {
		storedStatus = "UNKNOWN_OUTCOME"
	}
	update, err := tx.ExecContext(ctx, `UPDATE operation_execution SET status=?,completed_at=?,result_json=?,error_code=?,error_message=? WHERE operation_id=? AND request_hash=? AND status='RUNNING'`,
		storedStatus, completed, string(payload), result.ErrorCode, result.ErrorMessage, request.OperationID, fingerprint)
	if err != nil {
		return fmt.Errorf("persist operation terminal result: %w", err)
	}
	rows, err := update.RowsAffected()
	if err != nil || rows != 1 {
		return fmt.Errorf("operation execution is no longer claimable")
	}
	var sequence int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence_number),0)+1 FROM operation_audit_event WHERE operation_id=?`, request.OperationID).Scan(&sequence); err != nil {
		return fmt.Errorf("read next operation audit sequence: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO operation_audit_event(event_id,operation_id,sequence_number,event_type,occurred_at,details_json) VALUES(?,?,?,?,?,?)`,
		uuid.NewString(), request.OperationID, sequence, "execution_completed", completed, string(details)); err != nil {
		return fmt.Errorf("persist operation completion audit: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO operation_result_outbox(event_id,operation_id,event_version,subject,payload_json,created_at,next_attempt_at) VALUES(?,?,?,?,?,?,?)`,
		result.EventID, request.OperationID, result.SchemaVersion, "arimms.ops.v1.result."+request.InstallationID, string(payload), completed, completed); err != nil {
		return fmt.Errorf("persist operation result outbox: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit operation outcome, audit, and outbox: %w", err)
	}
	return nil
}

// Pending returns due result events in creation order, bounded by limit.
func (r *Repository) Pending(ctx context.Context, limit int) ([]operations.OutboxEvent, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("outbox batch limit must be between 1 and 100")
	}
	db, err := r.database()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT event_id,operation_id,event_version,subject,payload_json,attempt_count,created_at FROM operation_result_outbox WHERE published_at IS NULL AND next_attempt_at<=? ORDER BY created_at,event_id LIMIT ?`, time.Now().UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, fmt.Errorf("query pending operation results: %w", err)
	}
	defer rows.Close()
	var events []operations.OutboxEvent
	for rows.Next() {
		var event operations.OutboxEvent
		var payload, created string
		if err := rows.Scan(&event.EventID, &event.OperationID, &event.Version, &event.Subject, &payload, &event.Attempts, &created); err != nil {
			return nil, fmt.Errorf("read pending operation result: %w", err)
		}
		event.Payload = []byte(payload)
		event.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending operation results: %w", err)
	}
	return events, nil
}

// MarkPublished closes one outbox event only after the transport confirms PubAck.
func (r *Repository) MarkPublished(ctx context.Context, eventID string, at time.Time) error {
	return r.updateOutbox(ctx, `UPDATE operation_result_outbox SET published_at=?,attempt_count=attempt_count+1 WHERE event_id=? AND published_at IS NULL`, at.UTC().Format(time.RFC3339Nano), eventID)
}

// MarkFailed schedules one bounded retry without creating a second event.
func (r *Repository) MarkFailed(ctx context.Context, eventID, code string, next time.Time) error {
	if len(code) > 64 {
		code = code[:64]
	}
	return r.updateOutbox(ctx, `UPDATE operation_result_outbox SET attempt_count=attempt_count+1,last_error_code=?,next_attempt_at=? WHERE event_id=? AND published_at IS NULL`, code, next.UTC().Format(time.RFC3339Nano), eventID)
}

func (r *Repository) updateOutbox(ctx context.Context, query string, values ...any) error {
	db, err := r.database()
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := db.ExecContext(ctx, query, values...)
	if err != nil {
		return fmt.Errorf("update operation result outbox: %w", err)
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return fmt.Errorf("operation result outbox event is not pending")
	}
	return nil
}

func (r *Repository) database() (*sql.DB, error) {
	if r == nil || r.open == nil {
		return nil, fmt.Errorf("Agent SQLite database is unavailable")
	}
	db, err := r.open()
	if err != nil {
		return nil, fmt.Errorf("open Agent Operations database: %w", err)
	}
	return db, nil
}
