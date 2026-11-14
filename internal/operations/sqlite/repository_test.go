package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/operations"
)

func TestExecutionRepositoryAtomicallyPersistsTerminalAuditAndOutbox(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	repository := New(agentstate.NewSQLiteStore(path))
	request := operations.Request{SchemaVersion: operations.RequestSchemaV1, OperationID: "op-1", OperationType: operations.OperationAgentDiagnostics,
		InstallationID: "install-1", BindingID: "binding-1", RequestedAt: "2026-11-14T09:59:00Z", ExpiresAt: "2026-11-14T10:01:00Z"}
	started := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	claimed, created, err := repository.Claim(context.Background(), request, "fingerprint", started)
	if err != nil || !created || claimed.StartedAt != started {
		t.Fatalf("claim = %+v created=%t err=%v", claimed, created, err)
	}
	duplicate, created, err := repository.Claim(context.Background(), request, "fingerprint", started.Add(time.Second))
	if err != nil || created || duplicate.Result != nil {
		t.Fatalf("duplicate claim = %+v created=%t err=%v", duplicate, created, err)
	}
	payload, _ := json.Marshal(map[string]string{"state": "healthy"})
	result := operations.Result{SchemaVersion: "ar-imms.operation.result.v1", EventID: "event-1", OperationID: request.OperationID,
		OperationType: request.OperationType, InstallationID: request.InstallationID, BindingID: request.BindingID,
		Status: operations.StatusSucceeded, StartedAt: started, CompletedAt: started.Add(time.Second), Result: payload, AuditRef: request.OperationID}
	if err := repository.Complete(context.Background(), request, "fingerprint", result); err != nil {
		t.Fatal(err)
	}
	rows, err := repository.Pending(context.Background(), 10)
	if err != nil || len(rows) != 1 || rows[0].EventID != result.EventID || rows[0].Subject != "arimms.ops.v1.result.install-1" {
		t.Fatalf("pending outbox = %+v, err=%v", rows, err)
	}
	var status string
	var audits int
	var outbox int
	db, err := agentstate.NewSQLiteStore(path).OpenDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT status FROM operation_execution WHERE operation_id='op-1'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM operation_audit_event WHERE operation_id='op-1'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM operation_result_outbox WHERE operation_id='op-1'`).Scan(&outbox); err != nil {
		t.Fatal(err)
	}
	if status != "SUCCEEDED" || audits != 3 || outbox != 1 {
		t.Fatalf("committed execution status=%s audits=%d outbox=%d", status, audits, outbox)
	}
}

func TestRecoverInterruptedPersistsFailureAuditAndOutboxBeforeReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	repository := New(agentstate.NewSQLiteStore(path))
	request := operations.Request{SchemaVersion: operations.RequestSchemaV1, OperationID: "op-interrupted", OperationType: operations.OperationAgentDiagnostics,
		InstallationID: "install-1", BindingID: "binding-1", RequestedAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
		ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)}
	started := time.Now().UTC().Add(-time.Second)
	if _, created, err := repository.Claim(context.Background(), request, "fingerprint", started); err != nil || !created {
		t.Fatalf("claim interrupted operation created=%t err=%v", created, err)
	}
	completed := time.Now().UTC()
	count, err := repository.RecoverInterrupted(context.Background(), completed)
	if err != nil || count != 1 {
		t.Fatalf("recover count=%d err=%v", count, err)
	}
	count, err = repository.RecoverInterrupted(context.Background(), completed.Add(time.Second))
	if err != nil || count != 0 {
		t.Fatalf("second recovery count=%d err=%v", count, err)
	}
	duplicate, claimed, err := repository.Claim(context.Background(), request, "fingerprint", completed.Add(time.Second))
	if err != nil || claimed || duplicate.Result == nil || duplicate.Result.Status != operations.StatusFailed || duplicate.Result.ErrorCode != "EXECUTION_INTERRUPTED" {
		t.Fatalf("replayed interrupted execution=%+v claimed=%t err=%v", duplicate, claimed, err)
	}
	events, err := repository.Pending(context.Background(), 10)
	if err != nil || len(events) != 1 || events[0].OperationID != request.OperationID {
		t.Fatalf("recovered result outbox=%+v err=%v", events, err)
	}
	var status string
	var audits int
	db, err := agentstate.NewSQLiteStore(path).OpenDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT status FROM operation_execution WHERE operation_id=?`, request.OperationID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM operation_audit_event WHERE operation_id=?`, request.OperationID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if status != "FAILED" || audits != 3 {
		t.Fatalf("recovered status=%s audit events=%d", status, audits)
	}
}
