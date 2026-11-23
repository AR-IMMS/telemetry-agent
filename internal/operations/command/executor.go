package command

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	maxRequestBytes      = 64 * 1024
	maxExecutionDuration = 30 * time.Second
)

var subjectTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

var (
	ErrInvalidEnvelope     = errors.New("operation envelope is invalid")
	ErrOperationConflict   = errors.New("operation ID was reused with a different request")
	ErrExecutionInProgress = errors.New("operation execution is already in progress")
)

// Status describes local execution state independently from result publication.
type Status string

const (
	StatusSucceeded Status = "SUCCEEDED"
	StatusFailed    Status = "FAILED"
	StatusUnknown   Status = "UNKNOWN"
)

// Result is the versioned, bounded execution response persisted before transport acknowledgment.
type Result struct {
	SchemaVersion  string          `json:"schema_version"`
	EventID        string          `json:"event_id"`
	OperationID    string          `json:"operation_id"`
	OperationType  OperationType   `json:"operation_type"`
	InstallationID string          `json:"installation_id"`
	BindingID      string          `json:"binding_id"`
	Status         Status          `json:"status"`
	Target         Target          `json:"target"`
	StartedAt      time.Time       `json:"started_at"`
	CompletedAt    time.Time       `json:"completed_at"`
	Result         json.RawMessage `json:"result,omitempty"`
	ErrorCode      string          `json:"error_code,omitempty"`
	ErrorMessage   string          `json:"error_message,omitempty"`
	AuditRef       string          `json:"audit_ref"`
}

// StoredExecution is the deduplication and replay record returned by the execution journal.
type StoredExecution struct {
	Fingerprint string
	StartedAt   time.Time
	Result      *Result
}

// ExecutionStore atomically claims operation identity and commits terminal result, audit, and outbox.
type ExecutionStore interface {
	Claim(context.Context, Request, string, time.Time) (StoredExecution, bool, error)
	Complete(context.Context, Request, string, Result) error
}

// Executor validates trusted scope, applies idempotency, dispatches typed handlers, and durably records outcomes.
type Executor struct {
	Registry         *Registry
	Store            ExecutionStore
	InstallationID   string
	BindingID        string
	Now              func() time.Time
	executionTimeout time.Duration
}

// Execute returns the durable result for a supported, correctly bound request.
// A nil error means the terminal result and publication outbox are committed.
func (e Executor) Execute(ctx context.Context, request Request) (Result, error) {
	if ctx == nil || e.Registry == nil || e.Store == nil {
		return Result{}, fmt.Errorf("operation context, registry, and journal are required")
	}
	now := time.Now().UTC()
	if e.Now != nil {
		now = e.Now().UTC()
	}
	if err := e.validate(request, now); err != nil {
		return Result{}, err
	}
	fingerprint, err := requestFingerprint(request)
	if err != nil {
		return Result{}, err
	}
	stored, claimed, err := e.Store.Claim(ctx, request, fingerprint, now)
	if err != nil {
		return Result{}, err
	}
	if !claimed {
		if stored.Result == nil {
			return Result{}, ErrExecutionInProgress
		}
		return *stored.Result, nil
	}
	result := Result{
		SchemaVersion: "ar-imms.operation.result.v1", EventID: uuid.NewString(),
		OperationID: request.OperationID, OperationType: request.OperationType,
		InstallationID: request.InstallationID, BindingID: request.BindingID,
		Status: StatusSucceeded, Target: request.Target, StartedAt: stored.StartedAt,
		CompletedAt: now, AuditRef: request.OperationID,
	}
	executionTimeout := e.executionTimeout
	if executionTimeout <= 0 || executionTimeout > maxExecutionDuration {
		executionTimeout = maxExecutionDuration
	}
	if expiresAt, parseErr := time.Parse(time.RFC3339Nano, request.ExpiresAt); parseErr == nil {
		if remaining := expiresAt.Sub(now); remaining < executionTimeout {
			executionTimeout = remaining
		}
	}
	executionContext, cancel := context.WithTimeout(ctx, executionTimeout)
	defer cancel()
	payload, executeErr := e.Registry.execute(executionContext, request)
	result.CompletedAt = time.Now().UTC()
	if e.Now != nil {
		result.CompletedAt = e.Now().UTC()
	}
	if executeErr != nil {
		result.Status = StatusFailed
		if errors.Is(executeErr, ErrOutcomeUnknown) {
			result.Status = StatusUnknown
		}
		result.ErrorCode, result.ErrorMessage = safeOperationError(executeErr)
	} else {
		result.Result = payload
	}
	if err := e.Store.Complete(ctx, request, fingerprint, result); err != nil {
		return Result{}, fmt.Errorf("persist operation outcome and result outbox: %w", err)
	}
	return result, nil
}

func (e Executor) validate(request Request, now time.Time) error {
	if len(request.Parameters) > maxRequestBytes || request.SchemaVersion != RequestSchemaV1 ||
		!subjectTokenPattern.MatchString(request.OperationID) ||
		!subjectTokenPattern.MatchString(request.InstallationID) ||
		!subjectTokenPattern.MatchString(request.BindingID) || request.InstallationID != e.InstallationID ||
		request.BindingID != e.BindingID || request.OperationType == "" ||
		!e.Registry.supports(request.OperationType) || len(request.RequestedBy) > 256 ||
		len(request.AuthorizationRef) > 256 || len(request.Target.Kind) > 64 ||
		len(request.Target.ProviderID) > 128 || len(request.Target.ResourceID) > 256 {
		return ErrInvalidEnvelope
	}
	encoded, err := json.Marshal(request)
	if err != nil || len(encoded) > maxRequestBytes {
		return fmt.Errorf("%w: request exceeds size limit", ErrInvalidEnvelope)
	}
	requestedAt, err := time.Parse(time.RFC3339Nano, request.RequestedAt)
	if err != nil {
		return fmt.Errorf("%w: invalid requested_at", ErrInvalidEnvelope)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, request.ExpiresAt)
	if err != nil || !expiresAt.After(requestedAt) || !expiresAt.After(now) {
		return fmt.Errorf("%w: expired or invalid expiry", ErrInvalidEnvelope)
	}
	if len(request.Parameters) > 0 && !json.Valid(request.Parameters) {
		return fmt.Errorf("%w: parameters are not JSON", ErrInvalidEnvelope)
	}
	return nil
}

func (r *Registry) supports(operationType OperationType) bool {
	if r == nil {
		return false
	}
	_, ok := r.handlers[operationType]
	return ok
}

func requestFingerprint(request Request) (string, error) {
	var normalized any
	if len(request.Parameters) == 0 {
		normalized = map[string]any{}
	} else if err := json.Unmarshal(request.Parameters, &normalized); err != nil {
		return "", fmt.Errorf("decode operation parameters for fingerprint: %w", err)
	}
	request.Parameters, _ = json.Marshal(normalized)
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encode canonical operation request: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func safeOperationError(err error) (string, string) {
	if errors.Is(err, ErrOutcomeUnknown) {
		return "OPERATION_OUTCOME_UNKNOWN", "operation was issued but its outcome could not be verified; do not retry automatically"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "EXECUTION_TIMED_OUT", "operation exceeded its execution deadline"
	}
	if errors.Is(err, context.Canceled) {
		return "EXECUTION_CANCELLED", "operation was cancelled before completion"
	}
	var operationError *OperationError
	if errors.As(err, &operationError) && subjectTokenPattern.MatchString(operationError.Code) {
		message := strings.TrimSpace(operationError.Message)
		if len(message) > 256 {
			message = message[:256]
		}
		return operationError.Code, message
	}
	return "OPERATION_FAILED", "operation execution failed"
}

// OperationError is a stable, sanitized failure produced by an operation handler.
type OperationError struct {
	Code    string
	Message string
}

// Error returns the public operation failure message.
func (e *OperationError) Error() string {
	if e == nil {
		return "operation failed"
	}
	return e.Message
}
