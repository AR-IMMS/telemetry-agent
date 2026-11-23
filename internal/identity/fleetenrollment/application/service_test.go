package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/domain"
)

func TestStartPersistsRecoveryMaterialBeforeExchangeAndProtectsCredential(t *testing.T) {
	const agentID = "11111111-1111-4111-8111-111111111111"
	repository := &memoryAttemptRepository{}
	credentials := memoryCredentialStore{}
	client := &fakeFleetClient{response: validFleetResponse(agentID)}
	service := Service{Repository: repository, Credentials: credentials, Client: client}
	result, err := service.Start(context.Background(), "https://fleet.example.test", agentID, "invitation-secret")
	if err != nil {
		t.Fatalf("Start(): %v", err)
	}
	if client.exchangeCalls != 1 || client.recoverCalls != 0 {
		t.Fatalf("exchange/recovery calls = %d/%d", client.exchangeCalls, client.recoverCalls)
	}
	if !client.persistedBeforeExchange {
		t.Fatal("Fleet request was sent before attempt, invitation and recovery proof were durably available")
	}
	if result.AgentID != agentID || result.CredentialScope != "opamp.connect" || result.OpAMPEndpoint != "wss://fleet.example.test/opamp" {
		t.Fatalf("result = %+v", result)
	}
	if _, ok := any(result).(interface{ Secret() string }); ok {
		t.Fatal("secret-bearing API leaked through result type")
	}
	attempt, err := repository.LoadAttempt()
	if err != nil || attempt.Status != StatusRegistered || attempt.Result == nil {
		t.Fatalf("stored attempt = %+v, %v", attempt, err)
	}
	if credentials[attempt.CredentialRef] != "fleet-secret" {
		t.Fatal("Fleet credential was not stored in the protected credential port")
	}
	if _, ok := credentials[attempt.InvitationRef]; ok {
		t.Fatal("invitation token remained after successful enrollment")
	}
	if _, ok := credentials[attempt.RecoveryProofRef]; ok {
		t.Fatal("recovery proof remained after successful enrollment")
	}
}

func TestStartUsesPersistedProofToRecoverLostExchangeResponse(t *testing.T) {
	const agentID = "11111111-1111-4111-8111-111111111111"
	repository := &memoryAttemptRepository{}
	credentials := memoryCredentialStore{}
	client := &fakeFleetClient{exchangeErrors: []error{errors.New("connection lost after send"), errors.New("connection lost after retry")}, response: validFleetResponse(agentID)}
	service := Service{Repository: repository, Credentials: credentials, Client: client}
	if _, err := service.Start(context.Background(), "https://fleet.example.test", agentID, "invite"); err != nil {
		t.Fatalf("Start() recovery: %v", err)
	}
	if client.exchangeCalls != 2 || client.recoverCalls != 1 || client.recoveryProof == "" {
		t.Fatalf("exchange/recovery calls=%d/%d proof-present=%t", client.exchangeCalls, client.recoverCalls, client.recoveryProof != "")
	}
	if client.exchangeAttempt != client.recoveryAttempt || client.exchangeAgent != client.recoveryAgent {
		t.Fatal("recovery did not target the original immutable attempt and Agent identity")
	}
}

func TestStartRejectsUnexpectedCredentialScopeBeforePersistingCredential(t *testing.T) {
	const agentID = "11111111-1111-4111-8111-111111111111"
	repository := &memoryAttemptRepository{}
	credentials := memoryCredentialStore{}
	response := validFleetResponse(agentID)
	response.CredentialScope = "core.api"
	client := &fakeFleetClient{response: response}
	service := Service{Repository: repository, Credentials: credentials, Client: client}
	if _, err := service.Start(context.Background(), "https://fleet.example.test", agentID, "invite"); err == nil {
		t.Fatal("Start() accepted credential with non-OpAMP scope")
	}
	for reference := range credentials {
		if strings.HasPrefix(reference, "fleet/credential/") {
			t.Fatalf("unexpected Fleet credential persisted under %q", reference)
		}
	}
}

type memoryAttemptRepository struct {
	attempt domain.Attempt
	found   bool
}

func (r *memoryAttemptRepository) LoadAttempt() (domain.Attempt, error) {
	if !r.found {
		return domain.Attempt{}, ErrNoAttempt
	}
	return r.attempt, nil
}
func (r *memoryAttemptRepository) CreateAttempt(attempt domain.Attempt) error {
	if r.found {
		return ErrAttemptExists
	}
	r.attempt, r.found = attempt, true
	return nil
}
func (r *memoryAttemptRepository) CompleteAttempt(id string, result domain.Result) error {
	if !r.found || r.attempt.AttemptID != id {
		return errors.New("attempt mismatch")
	}
	r.attempt.Status, r.attempt.Result = StatusRegistered, &result
	return nil
}
func (r *memoryAttemptRepository) FailAttempt(id, code string) error {
	if !r.found || r.attempt.AttemptID != id {
		return errors.New("attempt mismatch")
	}
	r.attempt.Status, r.attempt.FailureCode = StatusFailed, code
	return nil
}

type memoryCredentialStore map[string]string

func (s memoryCredentialStore) Put(ref, value string) error { s[ref] = value; return nil }
func (s memoryCredentialStore) Get(ref string) (string, error) {
	v, ok := s[ref]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}
func (s memoryCredentialStore) Delete(ref string) error { delete(s, ref); return nil }

type fakeFleetClient struct {
	response                                      domain.EnrollmentResponse
	exchangeErr                                   error
	exchangeErrors                                []error
	exchangeRequests                              []domain.ExchangeRequest
	exchangeCalls, recoverCalls                   int
	exchangeAttempt, exchangeAgent                string
	recoveryAttempt, recoveryAgent, recoveryProof string
	persistedBeforeExchange                       bool
}

func (c *fakeFleetClient) Exchange(_ context.Context, _ domain.Origin, request domain.ExchangeRequest) (domain.EnrollmentResponse, error) {
	c.exchangeCalls++
	c.exchangeRequests = append(c.exchangeRequests, request)
	c.exchangeAttempt, c.exchangeAgent = request.AttemptID, request.AgentID
	c.persistedBeforeExchange = request.AttemptID != "" && request.RecoveryProof != "" && request.InvitationToken != ""
	if index := c.exchangeCalls - 1; index < len(c.exchangeErrors) && c.exchangeErrors[index] != nil {
		return domain.EnrollmentResponse{}, c.exchangeErrors[index]
	}
	if c.exchangeErr != nil {
		return domain.EnrollmentResponse{}, c.exchangeErr
	}
	return c.response, nil
}
func (c *fakeFleetClient) Recover(_ context.Context, _ domain.Origin, request domain.RecoveryRequest) (domain.EnrollmentResponse, error) {
	c.recoverCalls++
	c.recoveryAttempt, c.recoveryAgent, c.recoveryProof = request.AttemptID, request.AgentID, request.RecoveryProof
	return c.response, nil
}
func validFleetResponse(agentID string) domain.EnrollmentResponse {
	return domain.EnrollmentResponse{AgentID: agentID, NodeID: "22222222-2222-4222-8222-222222222222", CredentialID: "cred-1", Credential: "fleet-secret", CredentialScope: "opamp.connect", OpAMPEndpoint: "wss://fleet.example.test/opamp"}
}

func TestResumeRetriesSecretCleanupAfterFleetEnrollmentCommitted(t *testing.T) {
	const agentID = "11111111-1111-4111-8111-111111111111"
	repository := &memoryAttemptRepository{}
	credentials := &failDeleteCredentialStore{values: memoryCredentialStore{}, remainingFailures: 2}
	client := &fakeFleetClient{response: validFleetResponse(agentID)}
	service := Service{Repository: repository, Credentials: credentials, Client: client}

	_, err := service.Start(context.Background(), "https://fleet.example.test", agentID, "invitation-secret")
	if !errors.Is(err, ErrEnrollmentCommitted) {
		t.Fatalf("Start() error=%v, want committed-with-cleanup-pending", err)
	}
	attempt, err := repository.LoadAttempt()
	if err != nil || attempt.Status != StatusRegistered || attempt.Result == nil {
		t.Fatalf("committed attempt=%+v, err=%v", attempt, err)
	}

	result, err := service.Resume(context.Background())
	if err != nil {
		t.Fatalf("Resume() cleanup retry: %v", err)
	}
	if result.AgentID != agentID || client.exchangeCalls != 1 || client.recoverCalls != 0 {
		t.Fatalf("Resume() result=%+v, exchange/recovery=%d/%d", result, client.exchangeCalls, client.recoverCalls)
	}
	if _, err := credentials.Get(attempt.InvitationRef); err == nil {
		t.Fatal("resumed enrollment retained the invitation token")
	}
	if _, err := credentials.Get(attempt.RecoveryProofRef); err == nil {
		t.Fatal("resumed enrollment retained the recovery proof")
	}
}

type failDeleteCredentialStore struct {
	values            memoryCredentialStore
	remainingFailures int
}

func (s *failDeleteCredentialStore) Put(ref, value string) error    { return s.values.Put(ref, value) }
func (s *failDeleteCredentialStore) Get(ref string) (string, error) { return s.values.Get(ref) }
func (s *failDeleteCredentialStore) Delete(ref string) error {
	if s.remainingFailures > 0 {
		s.remainingFailures--
		return errors.New("simulated credential cleanup failure")
	}
	return s.values.Delete(ref)
}

func TestStartRetriesSameImmutableExchangeBeforeRecovering(t *testing.T) {
	const agentID = "11111111-1111-4111-8111-111111111111"
	repository := &memoryAttemptRepository{}
	credentials := memoryCredentialStore{}
	client := &fakeFleetClient{
		exchangeErrors: []error{errors.New("response lost after commit"), nil},
		response:       validFleetResponse(agentID),
	}
	service := Service{Repository: repository, Credentials: credentials, Client: client}
	if _, err := service.Start(context.Background(), "https://fleet.example.test", agentID, "invitation-secret"); err != nil {
		t.Fatalf("Start() retry: %v", err)
	}
	if client.exchangeCalls != 2 || client.recoverCalls != 0 {
		t.Fatalf("exchange/recovery calls=%d/%d, want same-request retry before recovery", client.exchangeCalls, client.recoverCalls)
	}
	if len(client.exchangeRequests) != 2 || client.exchangeRequests[0] != client.exchangeRequests[1] {
		t.Fatal("retry changed the immutable attempt request")
	}
}

func TestStartDoesNotRecoverDefinitiveFleetRejection(t *testing.T) {
	const agentID = "11111111-1111-4111-8111-111111111111"
	repository := &memoryAttemptRepository{}
	credentials := memoryCredentialStore{}
	client := &fakeFleetClient{
		exchangeErrors: []error{classifiedFleetError{err: errors.New("Fleet returned HTTP 401 (INVITATION_REJECTED)")}},
		response:       validFleetResponse(agentID),
	}
	service := Service{Repository: repository, Credentials: credentials, Client: client}
	if _, err := service.Start(context.Background(), "https://fleet.example.test", agentID, "invitation-secret"); err == nil {
		t.Fatal("Start() accepted a definitive Fleet rejection")
	}
	if client.exchangeCalls != 1 || client.recoverCalls != 0 {
		t.Fatalf("exchange/recovery calls=%d/%d, want no retry/recovery for definitive rejection", client.exchangeCalls, client.recoverCalls)
	}
}

type classifiedFleetError struct {
	err       error
	uncertain bool
}

func (e classifiedFleetError) Error() string          { return e.err.Error() }
func (e classifiedFleetError) Unwrap() error          { return e.err }
func (e classifiedFleetError) OutcomeUncertain() bool { return e.uncertain }

func TestStartDoesNotSendBeforeAttemptPersistenceAndCleansStoredSecretsOnFailure(t *testing.T) {
	const agentID = "11111111-1111-4111-8111-111111111111"
	credentials := memoryCredentialStore{}
	client := &fakeFleetClient{response: validFleetResponse(agentID)}
	repository := &failedCreateAttemptRepository{err: errors.New("simulated SQLite failure")}
	service := Service{Repository: repository, Credentials: credentials, Client: client}

	if _, err := service.Start(context.Background(), "https://fleet.example.test", agentID, "invitation-secret"); err == nil {
		t.Fatal("Start() succeeded with a failed durable attempt write")
	}
	if client.exchangeCalls != 0 || client.recoverCalls != 0 {
		t.Fatalf("network called before attempt persistence: exchange/recovery=%d/%d", client.exchangeCalls, client.recoverCalls)
	}
	if len(credentials) != 0 {
		t.Fatal("failed attempt persistence left invitation or recovery secret in the protected store")
	}
}

type failedCreateAttemptRepository struct {
	memoryAttemptRepository
	err error
}

func (r *failedCreateAttemptRepository) CreateAttempt(domain.Attempt) error { return r.err }

func TestResumeRetriesAfterFleetCredentialStoreFailure(t *testing.T) {
	const agentID = "11111111-1111-4111-8111-111111111111"
	repository := &memoryAttemptRepository{}
	credentials := &failCredentialPutStore{values: memoryCredentialStore{}, failOnce: true}
	client := &fakeFleetClient{response: validFleetResponse(agentID)}
	service := Service{Repository: repository, Credentials: credentials, Client: client}

	if _, err := service.Start(context.Background(), "https://fleet.example.test", agentID, "invitation-secret"); err == nil {
		t.Fatal("Start() succeeded after the Fleet credential could not be protected locally")
	}
	attempt, err := repository.LoadAttempt()
	if err != nil || attempt.Status != StatusPending {
		t.Fatalf("attempt after credential-store failure=%+v err=%v", attempt, err)
	}
	if _, ok := credentials.values[attempt.InvitationRef]; !ok {
		t.Fatal("failed credential write removed the invitation needed for safe retry")
	}
	if _, ok := credentials.values[attempt.RecoveryProofRef]; !ok {
		t.Fatal("failed credential write removed the recovery proof needed for safe retry")
	}

	credentials.failOnce = false
	if _, err := service.Resume(context.Background()); err != nil {
		t.Fatalf("Resume() after credential-store recovery: %v", err)
	}
	attempt, err = repository.LoadAttempt()
	if err != nil || attempt.Status != StatusRegistered {
		t.Fatalf("resumed attempt=%+v err=%v", attempt, err)
	}
	if _, ok := credentials.values[attempt.CredentialRef]; !ok {
		t.Fatal("resumed enrollment did not protect the Fleet credential")
	}
	if _, ok := credentials.values[attempt.InvitationRef]; ok {
		t.Fatal("resumed enrollment retained the invitation token")
	}
	if _, ok := credentials.values[attempt.RecoveryProofRef]; ok {
		t.Fatal("resumed enrollment retained the recovery proof")
	}
}

type failCredentialPutStore struct {
	values   memoryCredentialStore
	failOnce bool
}

func (s *failCredentialPutStore) Put(ref, value string) error {
	if s.failOnce && strings.HasPrefix(ref, "fleet/credential/") {
		s.failOnce = false
		return errors.New("simulated protected credential store failure")
	}
	return s.values.Put(ref, value)
}
func (s *failCredentialPutStore) Get(ref string) (string, error) { return s.values.Get(ref) }
func (s *failCredentialPutStore) Delete(ref string) error        { return s.values.Delete(ref) }

func TestStartRecoversAfterRetryReturnsDefinitiveResponseToAmbiguousFirstAttempt(t *testing.T) {
	const agentID = "11111111-1111-4111-8111-111111111111"
	repository := &memoryAttemptRepository{}
	credentials := memoryCredentialStore{}
	client := &fakeFleetClient{
		exchangeErrors: []error{
			errors.New("exchange outcome unknown"),
			classifiedFleetError{err: errors.New("Fleet returned HTTP 409 (ENROLLMENT_CONFLICT)")},
		},
		response: validFleetResponse(agentID),
	}
	service := Service{Repository: repository, Credentials: credentials, Client: client}
	if _, err := service.Start(context.Background(), "https://fleet.example.test", agentID, "invitation-secret"); err != nil {
		t.Fatalf("Start() recovery after ambiguous first exchange: %v", err)
	}
	if client.exchangeCalls != 2 || client.recoverCalls != 1 {
		t.Fatalf("exchange/recovery calls=%d/%d, want recovery after uncertain first request", client.exchangeCalls, client.recoverCalls)
	}
}
