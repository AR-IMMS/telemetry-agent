package command

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type executionStoreFake struct {
	records map[string]StoredExecution
	claims  int
	results int
}

func (s *executionStoreFake) Claim(_ context.Context, request Request, fingerprint string, now time.Time) (StoredExecution, bool, error) {
	if s.records == nil {
		s.records = make(map[string]StoredExecution)
	}
	s.claims++
	if existing, ok := s.records[request.OperationID]; ok {
		if existing.Fingerprint != fingerprint {
			return StoredExecution{}, false, ErrOperationConflict
		}
		if existing.Result == nil {
			return StoredExecution{}, false, ErrExecutionInProgress
		}
		return existing, false, nil
	}
	entry := StoredExecution{Fingerprint: fingerprint, StartedAt: now}
	s.records[request.OperationID] = entry
	return entry, true, nil
}

func (s *executionStoreFake) Complete(_ context.Context, request Request, fingerprint string, result Result) error {
	s.results++
	copy := result
	s.records[request.OperationID] = StoredExecution{Fingerprint: fingerprint, StartedAt: result.StartedAt, Result: &copy}
	return nil
}

func TestExecutorReusesDurableResultForDuplicateDelivery(t *testing.T) {
	registry := NewRegistry()
	handler := &testInspectHandler{}
	if err := Register(registry, handler); err != nil {
		t.Fatal(err)
	}
	store := &executionStoreFake{}
	executor := Executor{Registry: registry, Store: store, InstallationID: "install-1", BindingID: "binding-1", Now: func() time.Time { return time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC) }}
	request := validRequest()
	first, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if handler.calls != 1 || store.results != 1 || first.EventID != second.EventID {
		t.Fatalf("duplicate execution calls=%d commits=%d events=%q/%q", handler.calls, store.results, first.EventID, second.EventID)
	}
}

func TestExecutorRejectsSameIDWithDifferentRequestFingerprint(t *testing.T) {
	registry := NewRegistry()
	if err := Register(registry, &testInspectHandler{}); err != nil {
		t.Fatal(err)
	}
	store := &executionStoreFake{}
	executor := Executor{Registry: registry, Store: store, InstallationID: "install-1", BindingID: "binding-1", Now: fixedNow}
	request := validRequest()
	if _, err := executor.Execute(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.Parameters = json.RawMessage(`{"container_id":"different"}`)
	if _, err := executor.Execute(context.Background(), request); !errors.Is(err, ErrOperationConflict) {
		t.Fatalf("fingerprint conflict error = %v", err)
	}
}

func TestExecutorRejectsWrongBindingBeforeClaim(t *testing.T) {
	registry := NewRegistry()
	if err := Register(registry, &testInspectHandler{}); err != nil {
		t.Fatal(err)
	}
	store := &executionStoreFake{}
	executor := Executor{Registry: registry, Store: store, InstallationID: "install-1", BindingID: "binding-1", Now: fixedNow}
	request := validRequest()
	request.BindingID = "old-binding"
	if _, err := executor.Execute(context.Background(), request); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("wrong binding error = %v", err)
	}
	if store.claims != 0 {
		t.Fatalf("wrong-binding request claimed %d times", store.claims)
	}
}

func TestExecutorRejectsExpiredRequestBeforeClaim(t *testing.T) {
	registry := NewRegistry()
	if err := Register(registry, &testInspectHandler{}); err != nil {
		t.Fatal(err)
	}
	store := &executionStoreFake{}
	executor := Executor{Registry: registry, Store: store, InstallationID: "install-1", BindingID: "binding-1", Now: fixedNow}
	request := validRequest()
	request.ExpiresAt = fixedNow().Add(-time.Second).Format(time.RFC3339Nano)
	if _, err := executor.Execute(context.Background(), request); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("expired request error = %v", err)
	}
	if store.claims != 0 {
		t.Fatalf("expired request claimed %d times", store.claims)
	}
}

func TestExecutorBoundsHandlerExecutionAndPersistsTimeout(t *testing.T) {
	registry := NewRegistry()
	if err := Register(registry, timeoutHandler{}); err != nil {
		t.Fatal(err)
	}
	store := &executionStoreFake{}
	executor := Executor{Registry: registry, Store: store, InstallationID: "install-1", BindingID: "binding-1", Now: fixedNow, executionTimeout: 10 * time.Millisecond}
	request := validRequest()
	request.OperationType = OperationAgentDiagnostics
	request.Parameters = []byte(`{}`)
	started := time.Now()
	result, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > time.Second || result.Status != StatusFailed || result.ErrorCode != "EXECUTION_TIMED_OUT" {
		t.Fatalf("timeout result=%+v duration=%s", result, time.Since(started))
	}
}

func TestExecutorPersistsAndReplaysUnknownMutationOutcomeWithoutReexecution(t *testing.T) {
	registry := NewRegistry()
	handler := &unknownRestartHandler{}
	if err := Register(registry, handler); err != nil {
		t.Fatal(err)
	}
	store := &executionStoreFake{}
	executor := Executor{Registry: registry, Store: store, InstallationID: "install-1", BindingID: "binding-1", Now: fixedNow}
	request := validRequest()
	request.OperationType = OperationContainerRestart
	request.Target.ResourceID = strings.Repeat("a", 64)
	request.Parameters = json.RawMessage(`{}`)
	first, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != StatusUnknown || first.ErrorCode != "OPERATION_OUTCOME_UNKNOWN" || handler.calls != 1 ||
		second.EventID != first.EventID || store.results != 1 {
		t.Fatalf("first=%+v second=%+v handler calls=%d persisted=%d", first, second, handler.calls, store.results)
	}
}

func TestExecutorRecordsCompletionTimeAfterHandlerFinishes(t *testing.T) {
	registry := NewRegistry()
	if err := Register(registry, &testInspectHandler{}); err != nil {
		t.Fatal(err)
	}
	store := &executionStoreFake{}
	start, completed, calls := fixedNow(), fixedNow().Add(3*time.Second), 0
	executor := Executor{Registry: registry, Store: store, InstallationID: "install-1", BindingID: "binding-1", Now: func() time.Time {
		calls++
		if calls == 1 {
			return start
		}
		return completed
	}}
	result, err := executor.Execute(context.Background(), validRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !result.CompletedAt.Equal(completed) {
		t.Fatalf("completed_at=%s, want handler completion time %s", result.CompletedAt, completed)
	}
}

type unknownRestartHandler struct{ calls int }

func (*unknownRestartHandler) Type() OperationType { return OperationContainerRestart }

func (h *unknownRestartHandler) Execute(context.Context, RequestMetadata, struct{}) (json.RawMessage, error) {
	h.calls++
	return nil, ErrOutcomeUnknown
}

type timeoutHandler struct{}

func (timeoutHandler) Type() OperationType { return OperationAgentDiagnostics }

func (timeoutHandler) Execute(ctx context.Context, _ RequestMetadata, _ struct{}) (json.RawMessage, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestExecutorRejectsOversizedAuditAndTargetFieldsBeforeClaim(t *testing.T) {
	for name, mutate := range map[string]func(*Request){
		"actor":                   func(request *Request) { request.RequestedBy = strings.Repeat("a", 257) },
		"authorization reference": func(request *Request) { request.AuthorizationRef = strings.Repeat("a", 257) },
		"target kind":             func(request *Request) { request.Target.Kind = strings.Repeat("a", 65) },
		"provider ID":             func(request *Request) { request.Target.ProviderID = strings.Repeat("a", 129) },
		"resource ID":             func(request *Request) { request.Target.ResourceID = strings.Repeat("a", 257) },
	} {
		t.Run(name, func(t *testing.T) {
			registry := NewRegistry()
			if err := Register(registry, &testInspectHandler{}); err != nil {
				t.Fatal(err)
			}
			store := &executionStoreFake{}
			executor := Executor{Registry: registry, Store: store, InstallationID: "install-1", BindingID: "binding-1", Now: fixedNow}
			request := validRequest()
			mutate(&request)
			if _, err := executor.Execute(context.Background(), request); !errors.Is(err, ErrInvalidEnvelope) {
				t.Fatalf("oversized field error = %v", err)
			}
			if store.claims != 0 {
				t.Fatalf("invalid request claimed %d times", store.claims)
			}
		})
	}
}

func validRequest() Request {
	return Request{SchemaVersion: RequestSchemaV1, OperationID: "op-1", OperationType: OperationContainerInspect,
		InstallationID: "install-1", BindingID: "binding-1", Target: Target{Kind: "docker_container", ProviderID: "docker", ResourceID: "full-id"},
		Parameters: json.RawMessage(`{"container_id":"full-id"}`), RequestedAt: "2026-11-14T09:59:00Z", ExpiresAt: "2026-11-14T10:01:00Z"}
}

func fixedNow() time.Time { return time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC) }
