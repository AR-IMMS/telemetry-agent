package enrollment

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/identity"
)

var (
	// ErrNoAttempt reports that there is no durable enrollment attempt to resume.
	ErrNoAttempt = errors.New("no enrollment attempt exists")
	// ErrAttemptExists prevents a new request from replacing a pending attempt.
	ErrAttemptExists = errors.New("an enrollment attempt already exists")
	// ErrCredentialNotFound reports missing protected secret material.
	ErrCredentialNotFound = errors.New("enrollment credential not found")
	// ErrAttemptFailed reports an attempt stopped by a permanent failure.
	ErrAttemptFailed = errors.New("enrollment attempt failed permanently")
	// ErrAlreadyEnrolled reports that this local installation has a binding.
	ErrAlreadyEnrolled = errors.New("installation is already enrolled")
	// ErrEnrollmentCommitted reports cleanup failed after the binding was durably committed.
	ErrEnrollmentCommitted = errors.New("enrollment committed; credential cleanup is pending")
)

// PermanentError represents a Core rejection that must not be retried without
// operator action or a new bootstrap token.
type PermanentError struct {
	Code string
}

// Error returns a safe error description that excludes request secrets.
func (e *PermanentError) Error() string { return "permanent Core enrollment error: " + e.Code }

// RetryableError represents a transport or Core failure that leaves the
// durable attempt pending. RetryAfter is populated when Core supplied a delay.
type RetryableError struct {
	Code       string
	RetryAfter time.Duration
}

// Error returns a safe error description that excludes request secrets.
func (e *RetryableError) Error() string { return "retryable Core enrollment error: " + e.Code }

// AttemptStatus is the durable state of one enrollment operation.
type AttemptStatus string

const (
	// AttemptPending means retries must reuse this attempt ID and payload.
	AttemptPending AttemptStatus = "pending"
	// AttemptRegistered means Core binding and credential reference were committed.
	AttemptRegistered AttemptStatus = "registered"
	// AttemptFailed means Core returned a permanent error requiring operator action.
	AttemptFailed AttemptStatus = "failed"
)

// EnrollmentResult contains non-secret registration and control-plane metadata.
type EnrollmentResult struct {
	EnrollmentID   string       `json:"enrollment_id"`
	InstallationID string       `json:"installation_id"`
	Binding        Binding      `json:"binding"`
	ControlPlane   ControlPlane `json:"control_plane"`
}

// Attempt is the durable, secret-free representation of one operation.
type Attempt struct {
	Payload                EnrollmentPayload `json:"payload"`
	CoreOrigin             string            `json:"core_origin"`
	BootstrapCredentialRef string            `json:"bootstrap_credential_ref"`
	AccessCredentialRef    string            `json:"access_credential_ref,omitempty"`
	Status                 AttemptStatus     `json:"status"`
	FailureCode            string            `json:"failure_code,omitempty"`
	Result                 *EnrollmentResult `json:"result,omitempty"`
}

// AttemptRepository stores enrollment state without plaintext credentials.
// CreateAttempt must fail if another attempt or completed binding already exists.
type AttemptRepository interface {
	// LoadAttempt returns the one durable attempt or ErrNoAttempt.
	LoadAttempt() (Attempt, error)
	// CreateAttempt atomically creates a pending attempt if none exists.
	CreateAttempt(Attempt) error
	// ReplaceFailedAttempt atomically starts a new operation after a permanent
	// rejection, preserving the same installation identity.
	ReplaceFailedAttempt(string, Attempt) error
	// CompleteAttempt atomically commits the validated binding and credential reference.
	CompleteAttempt(string, EnrollmentResult, string) error
	// FailAttempt records a permanent Core rejection without storing secret data.
	FailAttempt(string, string) error
}

// CredentialStore persists secret values outside SQLite and exposes them only
// to the enrollment application while constructing an authenticated request.
type CredentialStore interface {
	// Put stores or replaces a secret under an opaque, installation-scoped reference.
	Put(string, string) error
	// Get resolves a secret by reference.
	Get(string) (string, error)
	// Delete removes a secret after its durable state transition completes.
	Delete(string) error
}

// EnrollmentClient sends one request to the explicitly trusted Core origin.
type EnrollmentClient interface {
	// Enroll sends the attempt to Core and returns its untrusted response for validation.
	Enroll(context.Context, CoreOrigin, EnrollmentRequest) (EnrollmentResponse, error)
}

// RetryPolicy bounds one invocation while leaving a pending attempt durable
// for a later Resume after process restart or exhausted transient failures.
type RetryPolicy struct {
	// MaxAttempts includes the initial request; zero selects the safe defaults.
	MaxAttempts int
	// InitialDelay is the first exponential delay after a transient failure.
	InitialDelay time.Duration
	// MaximumDelay caps exponential backoff; Core Retry-After is honored as given.
	MaximumDelay time.Duration
}

// RetryWait is injectable so retry timing can be tested without sleeping.
type RetryWait func(context.Context, time.Duration) error

// Service coordinates durable enrollment attempts without controlling Collector
// activation or Core Gateway configuration ownership.
type Service struct {
	Repository  AttemptRepository
	Credentials CredentialStore
	Client      EnrollmentClient
	Entropy     io.Reader
	RetryPolicy RetryPolicy
	Wait        RetryWait
}

// Start creates and sends a durable enrollment attempt. If the request fails,
// the pending attempt and protected bootstrap token remain available to Resume.
func (s Service) Start(
	ctx context.Context,
	coreOrigin CoreOrigin,
	payload EnrollmentPayload,
	bootstrapToken string,
) (EnrollmentResult, error) {
	if err := s.validate(); err != nil {
		return EnrollmentResult{}, err
	}
	if strings.TrimSpace(bootstrapToken) == "" {
		return EnrollmentResult{}, fmt.Errorf("bootstrap token is required")
	}
	validatedOrigin, err := ParseCoreOrigin(coreOrigin.String())
	if err != nil {
		return EnrollmentResult{}, err
	}
	previousAttemptID := ""
	if existing, err := s.Repository.LoadAttempt(); err == nil {
		switch existing.Status {
		case AttemptPending:
			return EnrollmentResult{}, ErrAttemptExists
		case AttemptRegistered:
			return EnrollmentResult{}, ErrAlreadyEnrolled
		case AttemptFailed:
			previousAttemptID = existing.Payload.AttemptID
		default:
			return EnrollmentResult{}, fmt.Errorf("unknown enrollment attempt status %q", existing.Status)
		}
	} else if !errors.Is(err, ErrNoAttempt) {
		return EnrollmentResult{}, fmt.Errorf("load prior enrollment attempt: %w", err)
	}
	entropy := s.Entropy
	if entropy == nil {
		entropy = rand.Reader
	}
	attemptID, err := identity.NewInstallationID(entropy)
	if err != nil {
		return EnrollmentResult{}, fmt.Errorf("generate enrollment attempt ID: %w", err)
	}
	payload.AttemptID = attemptID.String()
	if err := payload.Validate(); err != nil {
		return EnrollmentResult{}, err
	}
	tokenRef := "enrollment/bootstrap/" + payload.AttemptID
	accessRef := "enrollment/agent/" + payload.Installation.ID
	attempt := Attempt{
		Payload: payload, CoreOrigin: validatedOrigin.String(),
		BootstrapCredentialRef: tokenRef, AccessCredentialRef: accessRef,
		Status: AttemptPending,
	}
	if err := s.Credentials.Put(tokenRef, bootstrapToken); err != nil {
		return EnrollmentResult{}, fmt.Errorf("store bootstrap token securely: %w", err)
	}
	var persistErr error
	if previousAttemptID == "" {
		persistErr = s.Repository.CreateAttempt(attempt)
	} else {
		persistErr = s.Repository.ReplaceFailedAttempt(previousAttemptID, attempt)
	}
	if persistErr != nil {
		cleanupErr := s.Credentials.Delete(tokenRef)
		if cleanupErr != nil {
			return EnrollmentResult{}, errors.Join(
				fmt.Errorf("persist pending enrollment attempt: %w", persistErr),
				fmt.Errorf("remove unreferenced bootstrap token: %w", cleanupErr),
			)
		}
		return EnrollmentResult{}, fmt.Errorf("persist pending enrollment attempt: %w", persistErr)
	}
	return s.sendWithRetry(ctx, attempt)
}

// Resume retries the persisted request with its original attempt ID, payload,
// Core origin, and protected bootstrap token.
func (s Service) Resume(ctx context.Context) (EnrollmentResult, error) {
	if err := s.validate(); err != nil {
		return EnrollmentResult{}, err
	}
	attempt, err := s.Repository.LoadAttempt()
	if err != nil {
		return EnrollmentResult{}, fmt.Errorf("load enrollment attempt: %w", err)
	}
	switch attempt.Status {
	case AttemptRegistered:
		if attempt.Result == nil {
			return EnrollmentResult{}, fmt.Errorf("registered enrollment has no committed result")
		}
		if err := s.Credentials.Delete(attempt.BootstrapCredentialRef); err != nil {
			return *attempt.Result, fmt.Errorf("%w: remove completed bootstrap token: %v", ErrEnrollmentCommitted, err)
		}
		return *attempt.Result, nil
	case AttemptFailed:
		return EnrollmentResult{}, fmt.Errorf("%w: %s", ErrAttemptFailed, attempt.FailureCode)
	case AttemptPending:
		return s.sendWithRetry(ctx, attempt)
	default:
		return EnrollmentResult{}, fmt.Errorf("unknown enrollment attempt status %q", attempt.Status)
	}
}

func (s Service) sendWithRetry(ctx context.Context, attempt Attempt) (EnrollmentResult, error) {
	policy := s.retryPolicy()
	var lastErr error
	for requestNumber := 1; requestNumber <= policy.MaxAttempts; requestNumber++ {
		result, err := s.sendOnce(ctx, attempt)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if errors.Is(err, ErrEnrollmentCommitted) {
			return result, err
		}
		var permanent *PermanentError
		if errors.As(err, &permanent) || ctx.Err() != nil || requestNumber == policy.MaxAttempts {
			return EnrollmentResult{}, err
		}
		delay := retryDelay(policy, requestNumber)
		var retryable *RetryableError
		if errors.As(err, &retryable) && retryable.RetryAfter > 0 {
			delay = retryable.RetryAfter
		}
		wait := s.Wait
		if wait == nil {
			wait = waitForRetry
		}
		if err := wait(ctx, delay); err != nil {
			return EnrollmentResult{}, errors.Join(lastErr, fmt.Errorf("wait before enrollment retry: %w", err))
		}
	}
	return EnrollmentResult{}, lastErr
}

func (s Service) sendOnce(ctx context.Context, attempt Attempt) (EnrollmentResult, error) {
	origin, err := ParseCoreOrigin(attempt.CoreOrigin)
	if err != nil {
		return EnrollmentResult{}, fmt.Errorf("load persisted Core origin: %w", err)
	}
	bootstrapToken, err := s.Credentials.Get(attempt.BootstrapCredentialRef)
	if err != nil {
		return EnrollmentResult{}, fmt.Errorf("load protected bootstrap token: %w", err)
	}
	request := EnrollmentRequest{BootstrapToken: bootstrapToken, EnrollmentPayload: attempt.Payload}
	response, err := s.Client.Enroll(ctx, origin, request)
	if err != nil {
		var permanent *PermanentError
		if errors.As(err, &permanent) {
			if failErr := s.Repository.FailAttempt(attempt.Payload.AttemptID, permanent.Code); failErr != nil {
				return EnrollmentResult{}, errors.Join(err, fmt.Errorf("persist permanent enrollment failure: %w", failErr))
			}
			if deleteErr := s.Credentials.Delete(attempt.BootstrapCredentialRef); deleteErr != nil {
				return EnrollmentResult{}, errors.Join(err, fmt.Errorf("remove rejected bootstrap token: %w", deleteErr))
			}
		}
		return EnrollmentResult{}, fmt.Errorf("send Core enrollment request: %w", err)
	}
	if err := response.Validate(attempt.Payload.Installation.ID, origin); err != nil {
		return EnrollmentResult{}, fmt.Errorf("validate Core enrollment response: %w", err)
	}
	if err := s.Credentials.Put(attempt.AccessCredentialRef, response.Credential.AccessToken); err != nil {
		return EnrollmentResult{}, fmt.Errorf("store Agent credential securely: %w", err)
	}
	result := EnrollmentResult{
		EnrollmentID: response.EnrollmentID, InstallationID: response.InstallationID,
		Binding: response.Binding, ControlPlane: response.ControlPlane,
	}
	if err := s.Repository.CompleteAttempt(attempt.Payload.AttemptID, result, attempt.AccessCredentialRef); err != nil {
		return EnrollmentResult{}, fmt.Errorf("commit Core enrollment binding: %w", err)
	}
	if err := s.Credentials.Delete(attempt.BootstrapCredentialRef); err != nil {
		return result, fmt.Errorf("%w: remove completed bootstrap token: %v", ErrEnrollmentCommitted, err)
	}
	return result, nil
}

func (s Service) retryPolicy() RetryPolicy {
	policy := s.RetryPolicy
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 5
	}
	if policy.InitialDelay <= 0 {
		policy.InitialDelay = time.Second
	}
	if policy.MaximumDelay <= 0 {
		policy.MaximumDelay = 30 * time.Second
	}
	if policy.MaximumDelay < policy.InitialDelay {
		policy.MaximumDelay = policy.InitialDelay
	}
	return policy
}

func retryDelay(policy RetryPolicy, failedRequest int) time.Duration {
	delay := policy.InitialDelay
	for count := 1; count < failedRequest && delay < policy.MaximumDelay; count++ {
		if delay > policy.MaximumDelay/2 {
			return policy.MaximumDelay
		}
		delay *= 2
	}
	if delay > policy.MaximumDelay {
		return policy.MaximumDelay
	}
	return delay
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s Service) validate() error {
	if s.Repository == nil || s.Credentials == nil || s.Client == nil {
		return fmt.Errorf("enrollment repository, credential store, and client are required")
	}
	return nil
}
