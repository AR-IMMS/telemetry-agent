package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/operations"
)

type healthFake struct{ snapshot agenthealth.Snapshot }

func (f healthFake) SnapshotContext(context.Context) agenthealth.Snapshot { return f.snapshot }

type containerFake struct {
	value ContainerObservation
	calls int
}

func (f *containerFake) Inspect(_ context.Context, id string) (ContainerObservation, error) {
	f.calls++
	if id != f.value.ID {
		return ContainerObservation{}, errors.New("wrong id")
	}
	return f.value, nil
}

type dependencyFake struct{ calls int }

func (f *dependencyFake) Inspect(context.Context, string) (DependencyObservation, error) {
	f.calls++
	return DependencyObservation{Name: "otelcol-contrib", Availability: "enabled", Health: "healthy"}, nil
}

func TestContainerInspectRequiresExactResourceIdentityAndReturnsAllowlist(t *testing.T) {
	inspector := &containerFake{value: ContainerObservation{ID: strings.Repeat("a", 64), Name: "redis", Image: "redis:7", State: "running", RestartCount: 1}}
	handler := ContainerInspect{Containers: inspector}
	request := operations.Request{OperationID: "op-1", OperationType: operations.OperationContainerInspect,
		Target:     operations.Target{Kind: "docker_container", ProviderID: "docker", ResourceID: inspector.value.ID},
		Parameters: json.RawMessage(`{"container_id":"` + inspector.value.ID + `"}`)}
	result, err := handler.Execute(context.Background(), operations.RequestMetadata{OperationID: request.OperationID, Target: request.Target}, ContainerInspectParameters{ContainerID: inspector.value.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(result) || inspector.calls != 1 || strings.Contains(string(result), "mount") || strings.Contains(string(result), "environment") {
		t.Fatalf("inspection result=%s calls=%d", result, inspector.calls)
	}
	_, err = handler.Execute(context.Background(), operations.RequestMetadata{OperationID: request.OperationID, Target: request.Target}, ContainerInspectParameters{ContainerID: "redis"})
	var operationError *operations.OperationError
	if !errors.As(err, &operationError) || operationError.Code != "TARGET_IDENTITY_MISMATCH" || inspector.calls != 1 {
		t.Fatalf("mismatched identity error=%v calls=%d", err, inspector.calls)
	}
}

func TestDependencyInspectIsLimitedToAgentManagedTarget(t *testing.T) {
	inspector := &dependencyFake{}
	handler := DependencyInspect{Dependencies: inspector}
	metadata := operations.RequestMetadata{Target: operations.Target{Kind: "managed_dependency", ProviderID: "agent", ResourceID: "otelcol-contrib"}}
	result, err := handler.Execute(context.Background(), metadata, DependencyInspectParameters{Name: "otelcol-contrib"})
	if err != nil || inspector.calls != 1 || !json.Valid(result) {
		t.Fatalf("result=%s calls=%d err=%v", result, inspector.calls, err)
	}
	metadata.Target.ProviderID = "host-service-manager"
	_, err = handler.Execute(context.Background(), metadata, DependencyInspectParameters{Name: "otelcol-contrib"})
	var operationError *operations.OperationError
	if !errors.As(err, &operationError) || operationError.Code != "UNSUPPORTED_TARGET" || inspector.calls != 1 {
		t.Fatalf("unsupported target error=%v calls=%d", err, inspector.calls)
	}
}

func TestDiagnosticsReusesBoundedHealthSnapshot(t *testing.T) {
	snapshot := agenthealth.Evaluate(agenthealth.RuntimeObservation{CollectorState: agenthealth.CollectorStateReady})
	handler := Diagnostics{Health: healthFake{snapshot: snapshot}}
	result, err := handler.Execute(context.Background(), operations.RequestMetadata{}, struct{}{})
	if err != nil || !json.Valid(result) || len(result) > maxHandlerResultBytes {
		t.Fatalf("diagnostics bytes=%d err=%v", len(result), err)
	}
}
