package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// OperationType names a versioned, explicitly registered Agent capability.
type OperationType string

const (
	// OperationAgentDiagnostics returns a bounded Health snapshot.
	OperationAgentDiagnostics OperationType = "agent.diagnostics"
	// OperationContainerInspect returns allowlisted state for one exact container.
	OperationContainerInspect OperationType = "container.inspect"
	// OperationDependencyInspect returns status for one Agent-managed dependency.
	OperationDependencyInspect OperationType = "dependency.inspect"
	// OperationContainerRestart restarts one explicitly opted-in Docker container.
	OperationContainerRestart OperationType = "container.restart"
)

const RequestSchemaV1 = "ar-imms.operation.request.v1"

var (
	ErrDuplicateHandler  = errors.New("operation handler already registered")
	ErrUnsupportedType   = errors.New("operation type is unsupported")
	ErrInvalidParameters = errors.New("operation parameters are invalid")
	ErrOutcomeUnknown    = errors.New("operation outcome is unknown")
)

// Request carries a bounded operation envelope decoded from the versioned transport contract.
type Request struct {
	SchemaVersion    string          `json:"schema_version"`
	OperationID      string          `json:"operation_id"`
	OperationType    OperationType   `json:"operation_type"`
	InstallationID   string          `json:"installation_id"`
	BindingID        string          `json:"binding_id"`
	Target           Target          `json:"target"`
	Parameters       json.RawMessage `json:"parameters"`
	RequestedBy      string          `json:"requested_by"`
	AuthorizationRef string          `json:"authorization_ref"`
	RequestedAt      string          `json:"requested_at"`
	ExpiresAt        string          `json:"expires_at"`
}

// Target identifies a resource without display-name fallback.
type Target struct {
	Kind       string `json:"kind,omitempty"`
	ProviderID string `json:"provider_id,omitempty"`
	ResourceID string `json:"resource_id,omitempty"`
}

// RequestMetadata contains correlation fields available to typed handlers.
type RequestMetadata struct {
	OperationID string
	Target      Target
}

// OutboxEvent is the transport-neutral persisted result awaiting publication.
type OutboxEvent struct {
	EventID     string
	OperationID string
	Version     string
	Subject     string
	Payload     []byte
	Attempts    int
	CreatedAt   time.Time
}

// Handler is the type-safe operation implementation consumed by Register.
type Handler[P any] interface {
	Type() OperationType
	Execute(context.Context, RequestMetadata, P) (json.RawMessage, error)
}

type registeredHandler interface {
	execute(context.Context, Request) (json.RawMessage, error)
}

type typedHandler[P any] struct{ handler Handler[P] }

func (h typedHandler[P]) execute(ctx context.Context, request Request) (json.RawMessage, error) {
	if len(request.Parameters) == 0 {
		request.Parameters = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(request.Parameters))
	decoder.DisallowUnknownFields()
	var parameters P
	if err := decoder.Decode(&parameters); err != nil {
		return nil, fmt.Errorf("%w: decode typed parameters", ErrInvalidParameters)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing parameter data", ErrInvalidParameters)
	}
	return h.handler.Execute(ctx, RequestMetadata{OperationID: request.OperationID, Target: request.Target}, parameters)
}

// Registry is an immutable-after-composition catalog of supported handlers.
type Registry struct {
	handlers map[OperationType]registeredHandler
}

// NewRegistry creates an empty operation registry.
func NewRegistry() *Registry { return &Registry{handlers: make(map[OperationType]registeredHandler)} }

// Register adds one typed handler and rejects empty or duplicate operation types.
func Register[P any](registry *Registry, handler Handler[P]) error {
	if registry == nil || handler == nil || handler.Type() == "" {
		return fmt.Errorf("operation registry and typed handler are required")
	}
	if _, exists := registry.handlers[handler.Type()]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateHandler, handler.Type())
	}
	registry.handlers[handler.Type()] = typedHandler[P]{handler: handler}
	return nil
}

func (r *Registry) execute(ctx context.Context, request Request) (json.RawMessage, error) {
	if r == nil {
		return nil, ErrUnsupportedType
	}
	handler, exists := r.handlers[request.OperationType]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedType, request.OperationType)
	}
	return handler.execute(ctx, request)
}
