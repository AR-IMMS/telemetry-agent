package application

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/domain"
	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation/domain"
)

var (
	ErrNoAttempt           = os.ErrNotExist
	ErrAttemptExists       = errors.New("a Fleet enrollment attempt already exists")
	ErrAttemptFailed       = errors.New("fleet enrollment attempt failed permanently")
	ErrCredentialNotFound  = errors.New("fleet enrollment credential not found")
	ErrEnrollmentCommitted = errors.New("fleet enrollment committed; secret cleanup is pending")
)

const (
	StatusPending    = domain.AttemptPending
	StatusRegistered = domain.AttemptRegistered
	StatusFailed     = domain.AttemptFailed
)

// AttemptRepository stores secret-free Fleet enrollment state.
type AttemptRepository interface {
	LoadAttempt() (domain.Attempt, error)
	CreateAttempt(domain.Attempt) error
	CompleteAttempt(string, domain.Result) error
	FailAttempt(string, string) error
}

// CredentialStore stores invitation, recovery proof and issued credential outside SQLite.
type CredentialStore interface {
	Put(string, string) error
	Get(string) (string, error)
	Delete(string) error
}

// Client performs the two Fleet HTTPS exchange operations.
type Client interface {
	Exchange(context.Context, domain.Origin, domain.ExchangeRequest) (domain.EnrollmentResponse, error)
	Recover(context.Context, domain.Origin, domain.RecoveryRequest) (domain.EnrollmentResponse, error)
}

// Service coordinates Fleet exchange without changing the existing Core enrollment flow.
type Service struct {
	Repository  AttemptRepository
	Credentials CredentialStore
	Client      Client
	Entropy     io.Reader
}

// Start persists the immutable attempt and recovery proof before the first network request.
func (s Service) Start(ctx context.Context, originRaw, agentID, invitationToken string) (domain.Result, error) {
	if err := s.validate(); err != nil {
		return domain.Result{}, err
	}
	origin, err := domain.ParseOrigin(originRaw)
	if err != nil {
		return domain.Result{}, err
	}
	if _, err := installationid.ParseInstallationID(agentID); err != nil {
		return domain.Result{}, fmt.Errorf("fleet Agent ID is invalid")
	}
	if strings.TrimSpace(invitationToken) == "" || len(invitationToken) > 4096 {
		return domain.Result{}, fmt.Errorf("fleet invitation token is required and must be bounded")
	}
	if existing, err := s.Repository.LoadAttempt(); err == nil {
		if existing.Status == domain.AttemptPending || existing.Status == domain.AttemptRegistered {
			return domain.Result{}, ErrAttemptExists
		}
		if existing.Status != domain.AttemptFailed {
			return domain.Result{}, fmt.Errorf("unknown Fleet enrollment status %q", existing.Status)
		}
		return domain.Result{}, ErrAttemptFailed
	} else if !errors.Is(err, ErrNoAttempt) {
		return domain.Result{}, fmt.Errorf("load prior Fleet enrollment attempt: %w", err)
	}

	entropy := s.Entropy
	if entropy == nil {
		entropy = rand.Reader
	}
	attemptID, err := installationid.NewInstallationID(entropy)
	if err != nil {
		return domain.Result{}, fmt.Errorf("generate Fleet enrollment attempt ID: %w", err)
	}
	proofBytes := make([]byte, 32)
	if _, err := io.ReadFull(entropy, proofBytes); err != nil {
		return domain.Result{}, fmt.Errorf("generate Fleet recovery proof: %w", err)
	}
	proof := base64.RawURLEncoding.EncodeToString(proofBytes)
	clear(proofBytes)
	attempt := domain.Attempt{
		AttemptID: attemptID.String(), AgentID: agentID, Origin: origin.String(),
		InvitationRef:    "fleet/invitation/" + attemptID.String(),
		RecoveryProofRef: "fleet/recovery/" + attemptID.String(),
		CredentialRef:    "fleet/credential/" + agentID + "/" + attemptID.String(), Status: domain.AttemptPending,
		OTLPCredentialRef: "fleet/otlp/" + agentID + "/" + attemptID.String(),
	}
	if err := s.Credentials.Put(attempt.InvitationRef, invitationToken); err != nil {
		return domain.Result{}, fmt.Errorf("store Fleet invitation securely")
	}
	if err := s.Credentials.Put(attempt.RecoveryProofRef, proof); err != nil {
		cleanupErr := s.Credentials.Delete(attempt.InvitationRef)
		return domain.Result{}, errors.Join(fmt.Errorf("store Fleet recovery proof securely"), cleanupErr)
	}
	if err := s.Repository.CreateAttempt(attempt); err != nil {
		cleanupErr := errors.Join(s.Credentials.Delete(attempt.InvitationRef), s.Credentials.Delete(attempt.RecoveryProofRef))
		return domain.Result{}, errors.Join(fmt.Errorf("persist pending Fleet enrollment attempt: %w", err), cleanupErr)
	}
	return s.exchangeOrRecover(ctx, attempt)
}

// Resume repeats the same exchange and recovery proof from durable state.
func (s Service) Resume(ctx context.Context) (domain.Result, error) {
	if err := s.validate(); err != nil {
		return domain.Result{}, err
	}
	attempt, err := s.Repository.LoadAttempt()
	if err != nil {
		return domain.Result{}, fmt.Errorf("load Fleet enrollment attempt: %w", err)
	}
	switch attempt.Status {
	case domain.AttemptRegistered:
		if attempt.Result == nil {
			return domain.Result{}, fmt.Errorf("registered Fleet attempt has no result")
		}
		if err := s.deleteEnrollmentSecrets(attempt); err != nil {
			return *attempt.Result, fmt.Errorf("%w: %v", ErrEnrollmentCommitted, err)
		}
		return *attempt.Result, nil
	case domain.AttemptFailed:
		return domain.Result{}, fmt.Errorf("%w: %s", ErrAttemptFailed, attempt.FailureCode)
	case domain.AttemptPending:
		return s.exchangeOrRecover(ctx, attempt)
	default:
		return domain.Result{}, fmt.Errorf("unknown Fleet enrollment status %q", attempt.Status)
	}
}

func (s Service) exchangeOrRecover(ctx context.Context, attempt domain.Attempt) (domain.Result, error) {
	origin, err := domain.ParseOrigin(attempt.Origin)
	if err != nil {
		return domain.Result{}, fmt.Errorf("load trusted Fleet origin: %w", err)
	}
	invitation, err := s.Credentials.Get(attempt.InvitationRef)
	if err != nil {
		return domain.Result{}, fmt.Errorf("load protected Fleet invitation: %w", err)
	}
	proof, err := s.Credentials.Get(attempt.RecoveryProofRef)
	if err != nil {
		return domain.Result{}, fmt.Errorf("load protected Fleet recovery proof: %w", err)
	}
	request := domain.ExchangeRequest{AttemptID: attempt.AttemptID, AgentID: attempt.AgentID, InvitationToken: invitation, RecoveryProof: proof}
	if err := request.Validate(); err != nil {
		return domain.Result{}, err
	}
	response, exchangeErr := s.Client.Exchange(ctx, origin, request)
	if exchangeErr != nil && !exchangeOutcomeUncertain(exchangeErr) {
		return domain.Result{}, fmt.Errorf("fleet rejected the enrollment exchange: %w", exchangeErr)
	}
	if exchangeErr != nil {
		response, exchangeErr = s.Client.Exchange(ctx, origin, request)
		if exchangeErr != nil {
			response, err = s.Client.Recover(ctx, origin, domain.RecoveryRequest{AttemptID: attempt.AttemptID, AgentID: attempt.AgentID, RecoveryProof: proof})
			if err != nil {
				return domain.Result{}, fmt.Errorf("fleet exchange outcome is uncertain after retry; recovery did not return the original result: %w", err)
			}
		}
	}
	if err := response.Validate(attempt.AgentID); err != nil {
		return domain.Result{}, fmt.Errorf("validate Fleet enrollment response: %w", err)
	}
	if err := s.Credentials.Put(attempt.CredentialRef, response.Credential); err != nil {
		return domain.Result{}, fmt.Errorf("store Fleet OpAMP credential securely")
	}
	if response.OTLPCredential != nil {
		if attempt.OTLPCredentialRef == "" {
			return domain.Result{}, fmt.Errorf("Fleet OTLP credential reference is unavailable")
		}
		if err := s.Credentials.Put(attempt.OTLPCredentialRef, response.OTLPCredential.Token); err != nil {
			return domain.Result{}, fmt.Errorf("store Fleet OTLP credential securely")
		}
	}
	result := domain.Result{AgentID: response.AgentID, NodeID: response.NodeID, CredentialID: response.CredentialID,
		CredentialScope: response.CredentialScope, OpAMPEndpoint: response.OpAMPEndpoint, CredentialExpiresAt: response.CredentialExpiresAt}
	if response.OTLPCredential != nil {
		result.OTLPCredentialID = response.OTLPCredential.ID
		result.OTLPCredentialScope = response.OTLPCredential.Scope
		result.OTLPCredentialExpiresAt = response.OTLPCredential.ExpiresAt
	}
	if err := s.Repository.CompleteAttempt(attempt.AttemptID, result); err != nil {
		return domain.Result{}, fmt.Errorf("fleet credential is protected locally but registration state could not be committed: %w", err)
	}
	if err := s.deleteEnrollmentSecrets(attempt); err != nil {
		return result, fmt.Errorf("%w: %v", ErrEnrollmentCommitted, err)
	}
	return result, nil
}

func (s Service) deleteEnrollmentSecrets(attempt domain.Attempt) error {
	return errors.Join(s.Credentials.Delete(attempt.InvitationRef), s.Credentials.Delete(attempt.RecoveryProofRef))
}
func exchangeOutcomeUncertain(err error) bool {
	var classifier interface{ OutcomeUncertain() bool }
	if errors.As(err, &classifier) {
		return classifier.OutcomeUncertain()
	}
	return true
}
func (s Service) validate() error {
	if s.Repository == nil || s.Credentials == nil || s.Client == nil {
		return fmt.Errorf("fleet enrollment repository, protected credential store, and client are required")
	}
	return nil
}
