package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type testInspectParameters struct {
	ContainerID string `json:"container_id"`
}

type testInspectHandler struct{ calls int }

func (h *testInspectHandler) Type() OperationType { return OperationContainerInspect }
func (h *testInspectHandler) Execute(_ context.Context, _ RequestMetadata, p testInspectParameters) (json.RawMessage, error) {
	h.calls++
	return json.Marshal(map[string]string{"container_id": p.ContainerID})
}

func TestRegistryDispatchesTypedParameters(t *testing.T) {
	registry := NewRegistry()
	handler := &testInspectHandler{}
	if err := Register(registry, handler); err != nil {
		t.Fatal(err)
	}
	got, err := registry.execute(context.Background(), Request{OperationType: OperationContainerInspect, Parameters: json.RawMessage(`{"container_id":"full-id"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if handler.calls != 1 || string(got) != `{"container_id":"full-id"}` {
		t.Fatalf("typed dispatch = calls:%d result:%s", handler.calls, got)
	}
}

func TestRegistryRejectsUnknownTypedParameters(t *testing.T) {
	registry := NewRegistry()
	if err := Register(registry, &testInspectHandler{}); err != nil {
		t.Fatal(err)
	}
	_, err := registry.execute(context.Background(), Request{OperationType: OperationContainerInspect, Parameters: json.RawMessage(`{"container_id":"id","secret":"x"}`)})
	if !errors.Is(err, ErrInvalidParameters) {
		t.Fatalf("error = %v, want ErrInvalidParameters", err)
	}
}

func TestRegistryRejectsDuplicateOperationTypes(t *testing.T) {
	registry := NewRegistry()
	if err := Register(registry, &testInspectHandler{}); err != nil {
		t.Fatal(err)
	}
	if err := Register(registry, &testInspectHandler{}); !errors.Is(err, ErrDuplicateHandler) {
		t.Fatalf("duplicate registration error = %v", err)
	}
}
