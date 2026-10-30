package enrollment

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestServicePersistsAttemptBeforeSendingAndResumesSameRequest(t *testing.T) {
	origin, err := ParseCoreOrigin("https://core.example.com")
	if err != nil {
		t.Fatal(err)
	}
	repository := &memoryAttemptRepository{}
	credentials := memoryCredentialStore{}
	client := &recordingClient{response: validResponse("installation-1", "https://core.example.com/api/v1/agent-control")}
	service := Service{Repository: repository, Credentials: credentials, Client: client}
	payload := validPayload("installation-1")

	client.beforeRequest = func() {
		if !repository.created {
			t.Error("Core request started before pending attempt was persisted")
		}
	}
	result, err := service.Start(context.Background(), origin, payload, "bootstrap-secret")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if result.Binding.ID != "binding-1" {
		t.Fatalf("binding ID = %q, want binding-1", result.Binding.ID)
	}
	if got := client.requests[0].EnrollmentPayload.AttemptID; got == "" {
		t.Fatal("attempt ID was not generated")
	}
	if _, exists := credentials[repository.attempt.BootstrapCredentialRef]; exists {
		t.Fatal("bootstrap credential should be deleted after commit")
	}
	if repository.attempt.Status != AttemptRegistered {
		t.Fatalf("attempt status = %q, want registered", repository.attempt.Status)
	}
}

func TestServiceRetriesLostResponseUsingPersistedAttemptAndPayload(t *testing.T) {
	origin, _ := ParseCoreOrigin("https://core.example.com")
	repository := &memoryAttemptRepository{}
	credentials := memoryCredentialStore{}
	client := &recordingClient{errors: []error{errors.New("connection lost after request")}, response: validResponse("installation-1", "https://core.example.com/api/v1/agent-control")}
	service := Service{Repository: repository, Credentials: credentials, Client: client, RetryPolicy: RetryPolicy{MaxAttempts: 1}}
	payload := validPayload("installation-1")

	if _, err := service.Start(context.Background(), origin, payload, "bootstrap-secret"); err == nil {
		t.Fatal("Start() error = nil, want transport failure")
	}
	persisted := repository.attempt

	// Simulate a process restart by constructing a new service over durable ports.
	restarted := Service{Repository: repository, Credentials: credentials, Client: client, RetryPolicy: RetryPolicy{MaxAttempts: 3}}
	if _, err := restarted.Resume(context.Background()); err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(client.requests))
	}
	if !reflect.DeepEqual(client.requests[0], client.requests[1]) {
		t.Fatalf("retry request changed: first=%+v second=%+v", client.requests[0], client.requests[1])
	}
	if client.requests[0].EnrollmentPayload.AttemptID != persisted.Payload.AttemptID {
		t.Fatal("retry attempt ID differs from persisted attempt")
	}
}

func TestServiceRetriesTransientFailureWithExponentialDelay(t *testing.T) {
	origin, _ := ParseCoreOrigin("https://core.example.com")
	repository := &memoryAttemptRepository{}
	credentials := memoryCredentialStore{}
	client := &recordingClient{
		errors:   []error{&RetryableError{Code: "ENROLLMENT_UNAVAILABLE"}, &RetryableError{Code: "ENROLLMENT_UNAVAILABLE"}},
		response: validResponse("installation-1", "https://core.example.com/api/v1/agent-control"),
	}
	var delays []time.Duration
	service := Service{
		Repository: repository, Credentials: credentials, Client: client,
		RetryPolicy: RetryPolicy{MaxAttempts: 3, InitialDelay: time.Second, MaximumDelay: 4 * time.Second},
		Wait:        func(_ context.Context, delay time.Duration) error { delays = append(delays, delay); return nil },
	}
	if _, err := service.Start(context.Background(), origin, validPayload("installation-1"), "bootstrap-secret"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if len(client.requests) != 3 || len(delays) != 2 || delays[0] != time.Second || delays[1] != 2*time.Second {
		t.Fatalf("requests=%d delays=%v, want 3 requests with 1s and 2s delays", len(client.requests), delays)
	}
}

func TestServiceHonorsRetryAfterAndStopsPermanentRejections(t *testing.T) {
	origin, _ := ParseCoreOrigin("https://core.example.com")
	repository := &memoryAttemptRepository{}
	credentials := memoryCredentialStore{}
	client := &recordingClient{errors: []error{
		&RetryableError{Code: "RATE_LIMIT_EXCEEDED", RetryAfter: 17 * time.Second},
		&PermanentError{Code: "INVALID_BOOTSTRAP_TOKEN"},
	}, response: validResponse("installation-1", "https://core.example.com/api/v1/agent-control")}
	var delays []time.Duration
	service := Service{
		Repository: repository, Credentials: credentials, Client: client,
		RetryPolicy: RetryPolicy{MaxAttempts: 5},
		Wait:        func(_ context.Context, delay time.Duration) error { delays = append(delays, delay); return nil },
	}
	if _, err := service.Start(context.Background(), origin, validPayload("installation-1"), "bootstrap-secret"); err == nil {
		t.Fatal("Start() error = nil, want permanent authorization failure")
	}
	if len(delays) != 1 || delays[0] != 17*time.Second {
		t.Fatalf("retry delays = %v, want [17s]", delays)
	}
	if repository.attempt.Status != AttemptFailed {
		t.Fatalf("attempt status = %q, want failed", repository.attempt.Status)
	}
	if _, exists := credentials[repository.attempt.BootstrapCredentialRef]; exists {
		t.Fatal("permanently rejected bootstrap token was not deleted")
	}
	requestCount := len(client.requests)
	if _, err := service.Resume(context.Background()); !errors.Is(err, ErrAttemptFailed) {
		t.Fatalf("Resume() error = %v, want ErrAttemptFailed", err)
	}
	if len(client.requests) != requestCount {
		t.Fatal("Resume() retried a permanently rejected attempt")
	}
	if _, err := service.Start(context.Background(), origin, validPayload("installation-1"), "replacement-token"); err != nil {
		t.Fatalf("Start() with a newly issued token after permanent failure: %v", err)
	}
	if repository.attempt.Status != AttemptRegistered || len(client.requests) != requestCount+1 {
		t.Fatalf("new operator attempt was not completed: status=%q requests=%d", repository.attempt.Status, len(client.requests))
	}
}

func TestServiceRejectsUntrustedControlPlaneBeforePersistingCredential(t *testing.T) {
	origin, _ := ParseCoreOrigin("https://core.example.com")
	repository := &memoryAttemptRepository{}
	credentials := memoryCredentialStore{}
	client := &recordingClient{response: validResponse("installation-1", "https://attacker.example/control")}
	service := Service{Repository: repository, Credentials: credentials, Client: client}
	if _, err := service.Start(context.Background(), origin, validPayload("installation-1"), "bootstrap-secret"); err == nil {
		t.Fatal("Start() error = nil, want untrusted control-plane response")
	}
	if _, exists := credentials[repository.attempt.AccessCredentialRef]; exists {
		t.Fatal("untrusted response credential was persisted")
	}
	if repository.attempt.Status == AttemptRegistered {
		t.Fatal("untrusted response marked attempt registered")
	}
}

func TestServiceDoesNotResendAfterCommitWhenBootstrapCleanupFails(t *testing.T) {
	origin, _ := ParseCoreOrigin("https://core.example.com")
	repository := &memoryAttemptRepository{}
	credentials := &failingDeleteCredentialStore{memoryCredentialStore: memoryCredentialStore{}, fail: true}
	client := &recordingClient{response: validResponse("installation-1", "https://core.example.com/api/v1/agent-control")}
	service := Service{Repository: repository, Credentials: credentials, Client: client, RetryPolicy: RetryPolicy{MaxAttempts: 3}}

	result, err := service.Start(context.Background(), origin, validPayload("installation-1"), "bootstrap-secret")
	if !errors.Is(err, ErrEnrollmentCommitted) {
		t.Fatalf("Start() error = %v, want ErrEnrollmentCommitted", err)
	}
	if result.Binding.ID != "binding-1" || repository.attempt.Status != AttemptRegistered {
		t.Fatalf("result=%+v status=%q, want committed enrollment", result, repository.attempt.Status)
	}
	if len(client.requests) != 1 {
		t.Fatalf("request count = %d, want exactly 1 after durable commit", len(client.requests))
	}
}

func validPayload(installationID string) EnrollmentPayload {
	return EnrollmentPayload{
		Installation: Installation{ID: installationID},
		Agent:        Agent{Version: "0.1.4"},
		Environment:  Environment{OS: "linux", Architecture: "amd64", Hostname: "node-01"},
		Metadata:     map[string]any{"deployment_kind": "vm"},
	}
}

func validResponse(installationID, controlPlaneURL string) EnrollmentResponse {
	return EnrollmentResponse{
		EnrollmentID: "enrollment-1", InstallationID: installationID,
		Binding:      Binding{ID: "binding-1", NodeID: "node-1", Status: "active"},
		Credential:   Credential{Type: "bearer", AccessToken: "agent-secret"},
		ControlPlane: ControlPlane{BaseURL: controlPlaneURL, PollIntervalSeconds: 30},
	}
}

type memoryAttemptRepository struct {
	attempt Attempt
	created bool
}

func (r *memoryAttemptRepository) LoadAttempt() (Attempt, error) {
	if !r.created {
		return Attempt{}, ErrNoAttempt
	}
	return r.attempt, nil
}

func (r *memoryAttemptRepository) CreateAttempt(value Attempt) error {
	if r.created {
		return ErrAttemptExists
	}
	r.attempt, r.created = value, true
	return nil
}

func (r *memoryAttemptRepository) ReplaceFailedAttempt(id string, value Attempt) error {
	if !r.created || r.attempt.Payload.AttemptID != id || r.attempt.Status != AttemptFailed ||
		r.attempt.Payload.Installation.ID != value.Payload.Installation.ID {
		return ErrAttemptExists
	}
	r.attempt = value
	return nil
}

func (r *memoryAttemptRepository) CompleteAttempt(id string, result EnrollmentResult, credentialRef string) error {
	if !r.created || r.attempt.Payload.AttemptID != id {
		return ErrNoAttempt
	}
	r.attempt.Status = AttemptRegistered
	r.attempt.Result = &result
	r.attempt.AccessCredentialRef = credentialRef
	return nil
}

func (r *memoryAttemptRepository) FailAttempt(id, code string) error {
	if !r.created || r.attempt.Payload.AttemptID != id {
		return ErrNoAttempt
	}
	r.attempt.Status, r.attempt.FailureCode = AttemptFailed, code
	return nil
}

type memoryCredentialStore map[string]string

func (s memoryCredentialStore) Put(reference, value string) error {
	s[reference] = value
	return nil
}

func (s memoryCredentialStore) Get(reference string) (string, error) {
	value, ok := s[reference]
	if !ok {
		return "", ErrCredentialNotFound
	}
	return value, nil
}

func (s memoryCredentialStore) Delete(reference string) error {
	delete(s, reference)
	return nil
}

type failingDeleteCredentialStore struct {
	memoryCredentialStore
	fail bool
}

func (s *failingDeleteCredentialStore) Put(reference, value string) error {
	return s.memoryCredentialStore.Put(reference, value)
}

func (s *failingDeleteCredentialStore) Get(reference string) (string, error) {
	return s.memoryCredentialStore.Get(reference)
}

func (s *failingDeleteCredentialStore) Delete(reference string) error {
	if s.fail {
		return errors.New("simulated credential cleanup failure")
	}
	return s.memoryCredentialStore.Delete(reference)
}

type recordingClient struct {
	response      EnrollmentResponse
	errors        []error
	requests      []EnrollmentRequest
	beforeRequest func()
}

func (c *recordingClient) Enroll(_ context.Context, _ CoreOrigin, request EnrollmentRequest) (EnrollmentResponse, error) {
	if c.beforeRequest != nil {
		c.beforeRequest()
	}
	c.requests = append(c.requests, request)
	if len(c.errors) > 0 {
		err := c.errors[0]
		c.errors = c.errors[1:]
		return EnrollmentResponse{}, err
	}
	return c.response, nil
}
