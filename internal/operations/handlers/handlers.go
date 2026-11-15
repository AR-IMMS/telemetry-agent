package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/operations"
)

const maxHandlerResultBytes = 32 * 1024

// HealthReader supplies the current read-only Health assessment.
type HealthReader interface {
	SnapshotContext(context.Context) agenthealth.Snapshot
}

// ContainerObservation is the handler-owned allowlist for one Docker resource.
type ContainerObservation struct {
	ID           string `json:"container_id"`
	Name         string `json:"name"`
	Image        string `json:"image"`
	State        string `json:"state"`
	Health       string `json:"health,omitempty"`
	RestartCount int    `json:"restart_count"`
	StartedAt    string `json:"started_at,omitempty"`
}

// ContainerInspector reads runtime state for an exact provider-scoped ID.
type ContainerInspector interface {
	Inspect(context.Context, string) (ContainerObservation, error)
}

// DependencyObservation is the allowlisted status for one Agent-managed dependency.
type DependencyObservation struct {
	Name         string `json:"name"`
	Availability string `json:"availability"`
	Health       string `json:"health"`
	Drifted      bool   `json:"drifted,omitempty"`
}

// DependencyInspector inspects only dependencies present in the Agent catalog.
type DependencyInspector interface {
	Inspect(context.Context, string) (DependencyObservation, error)
}

// Diagnostics returns the existing Agent Health assessment without changing its ownership.
type Diagnostics struct{ Health HealthReader }

// Type returns the stable diagnostics operation name.
func (Diagnostics) Type() operations.OperationType { return operations.OperationAgentDiagnostics }

// Execute serializes one bounded Health snapshot.
func (h Diagnostics) Execute(ctx context.Context, _ operations.RequestMetadata, _ struct{}) (json.RawMessage, error) {
	if h.Health == nil {
		return nil, &operations.OperationError{Code: "HEALTH_UNAVAILABLE", Message: "Agent health evidence is unavailable"}
	}
	return encodeBounded(h.Health.SnapshotContext(ctx))
}

// ContainerInspectParameters defines the only supported exact-container parameter.
type ContainerInspectParameters struct {
	ContainerID string `json:"container_id"`
}

// ContainerInspect reads allowlisted state for one exact Docker container.
type ContainerInspect struct{ Containers ContainerInspector }

// Type returns the stable container inspection operation name.
func (ContainerInspect) Type() operations.OperationType { return operations.OperationContainerInspect }

// Execute verifies the full target identity before calling the Docker adapter.
func (h ContainerInspect) Execute(ctx context.Context, request operations.RequestMetadata, parameters ContainerInspectParameters) (json.RawMessage, error) {
	if h.Containers == nil {
		return nil, &operations.OperationError{Code: "DOCKER_UNAVAILABLE", Message: "Docker inspection is unavailable"}
	}
	if request.Target.Kind != "docker_container" || request.Target.ProviderID != "docker" ||
		len(parameters.ContainerID) != 64 || parameters.ContainerID != request.Target.ResourceID {
		return nil, &operations.OperationError{Code: "TARGET_IDENTITY_MISMATCH", Message: "request does not identify the exact Docker container"}
	}
	inspection, err := h.Containers.Inspect(ctx, parameters.ContainerID)
	if err != nil {
		return nil, &operations.OperationError{Code: "CONTAINER_INSPECTION_FAILED", Message: "Docker could not inspect the requested container"}
	}
	if inspection.ID != parameters.ContainerID {
		return nil, &operations.OperationError{Code: "STALE_TARGET", Message: "Docker returned a different container identity"}
	}
	return encodeBounded(inspection)
}

// DependencyInspectParameters selects one Agent-managed dependency by catalog name.
type DependencyInspectParameters struct {
	Name string `json:"name"`
}

// DependencyInspect returns status for a dependency managed by this Agent.
type DependencyInspect struct{ Dependencies DependencyInspector }

// Type returns the stable dependency inspection operation name.
func (DependencyInspect) Type() operations.OperationType {
	return operations.OperationDependencyInspect
}

// Execute limits inspection to the Agent's managed-dependency identity namespace.
func (h DependencyInspect) Execute(ctx context.Context, request operations.RequestMetadata, parameters DependencyInspectParameters) (json.RawMessage, error) {
	if h.Dependencies == nil {
		return nil, &operations.OperationError{Code: "DEPENDENCY_INSPECTION_UNAVAILABLE", Message: "managed dependency inspection is unavailable"}
	}
	name := strings.TrimSpace(parameters.Name)
	if request.Target.Kind != "managed_dependency" || request.Target.ProviderID != "agent" ||
		name == "" || len(name) > 128 || name != request.Target.ResourceID {
		return nil, &operations.OperationError{Code: "UNSUPPORTED_TARGET", Message: "request does not identify an Agent-managed dependency"}
	}
	observation, err := h.Dependencies.Inspect(ctx, name)
	if err != nil {
		return nil, &operations.OperationError{Code: "DEPENDENCY_INSPECTION_FAILED", Message: "Agent could not inspect the requested managed dependency"}
	}
	if observation.Name != name {
		return nil, fmt.Errorf("dependency inspector returned a mismatched identity")
	}
	return encodeBounded(observation)
}

func encodeBounded(value any) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode operation result")
	}
	if len(encoded) > maxHandlerResultBytes {
		return nil, &operations.OperationError{Code: "RESULT_TOO_LARGE", Message: "operation result exceeds its size limit"}
	}
	return encoded, nil
}
