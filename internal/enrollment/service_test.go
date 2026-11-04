package enrollment

import (
	"context"
	"errors"
	"reflect"
	"strings"
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
	if result.Binding.ID != "10000000-0000-4000-8000-000000000004" {
		t.Fatalf("binding ID = %q, want test binding UUID", result.Binding.ID)
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

func TestServiceRetriesGateway502WithExactPendingAttempt(t *testing.T) {
	origin, _ := ParseCoreOrigin("https://core.example.com")
	repository := &memoryAttemptRepository{}
	credentials := memoryCredentialStore{}
	client := &recordingClient{
		errors:   []error{&RetryableError{Code: "HTTP_502"}},
		response: validResponse("", "https://core.example.com/api/v1/agent-control"),
	}
	service := Service{
		Repository: repository, Credentials: credentials, Client: client,
		RetryPolicy: RetryPolicy{MaxAttempts: 2}, Wait: func(context.Context, time.Duration) error { return nil },
	}
	result, err := service.Start(context.Background(), origin, validPayload(""), "bootstrap-token")
	if err != nil || result.InstallationID == "" {
		t.Fatalf("Start() result=%+v err=%v", result, err)
	}
	if len(client.requests) != 2 || !reflect.DeepEqual(client.requests[0], client.requests[1]) {
		t.Fatalf("retried requests differ or count=%d", len(client.requests))
	}
	if repository.attempt.Status != AttemptRegistered {
		t.Fatalf("attempt status=%q, want registered", repository.attempt.Status)
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
	if result.Binding.ID != "10000000-0000-4000-8000-000000000004" || repository.attempt.Status != AttemptRegistered {
		t.Fatalf("result=%+v status=%q, want committed enrollment", result, repository.attempt.Status)
	}
	if len(client.requests) != 1 {
		t.Fatalf("request count = %d, want exactly 1 after durable commit", len(client.requests))
	}
	credentials.fail = false
	if _, err := service.Resume(context.Background()); err != nil {
		t.Fatalf("Resume() cleanup retry error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("Resume() resent completed enrollment: requests=%d", len(client.requests))
	}
}

func TestServiceRecoversCoreCommitWhenCredentialWriteOrSQLiteCompletionFails(t *testing.T) {
	for _, failAt := range []string{"credential write", "SQLite completion"} {
		t.Run(failAt, func(t *testing.T) {
			origin, _ := ParseCoreOrigin("https://core.example.com")
			repository := &memoryAttemptRepository{failComplete: failAt == "SQLite completion"}
			var credentials CredentialStore = memoryCredentialStore{}
			if failAt == "credential write" {
				credentials = &failingPutCredentialStore{memoryCredentialStore: memoryCredentialStore{}, failOnce: true}
			}
			client := &recordingClient{response: validResponse("", "https://core.example.com/api/v1/agent-control")}
			service := Service{Repository: repository, Credentials: credentials, Client: client, RetryPolicy: RetryPolicy{MaxAttempts: 2}, Wait: func(context.Context, time.Duration) error { return nil }}
			result, err := service.Start(context.Background(), origin, validPayload(""), "bootstrap-secret")
			if err != nil {
				t.Fatalf("Start() recovery error = %v", err)
			}
			if repository.attempt.Status != AttemptRegistered || result.EnrollmentID == "" || len(client.requests) != 2 {
				t.Fatalf("status=%q result=%+v requests=%d", repository.attempt.Status, result, len(client.requests))
			}
			if !reflect.DeepEqual(client.requests[0], client.requests[1]) {
				t.Fatal("recovery changed the immutable request")
			}
		})
	}
}

func TestServiceReEnrollRequiresExplicitOperationAndPreservesPreviousCredential(t *testing.T) {
	origin, _ := ParseCoreOrigin("https://core.example.com")
	repository := &memoryAttemptRepository{}
	credentials := &failingDeleteCredentialStore{memoryCredentialStore: memoryCredentialStore{}}
	client := &recordingClient{response: validResponse("", "https://core.example.com/api/v1/agent-control")}
	service := Service{Repository: repository, Credentials: credentials, Client: client}
	if _, err := service.Start(context.Background(), origin, validPayload(""), "first-token"); err != nil {
		t.Fatal(err)
	}
	oldAttempt := repository.attempt
	if _, err := credentials.Get(oldAttempt.AccessCredentialRef); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(context.Background(), origin, validPayload(""), "new-token"); !errors.Is(err, ErrAlreadyEnrolled) {
		t.Fatalf("Start() error=%v, want ErrAlreadyEnrolled", err)
	}
	credentials.failRef = oldAttempt.AccessCredentialRef
	if _, err := service.ReEnroll(context.Background(), origin, validPayload(""), "new-token"); !errors.Is(err, ErrEnrollmentCommitted) {
		t.Fatalf("ReEnroll() error=%v, want committed with cleanup pending", err)
	}
	if repository.attempt.Payload.Installation.ID != oldAttempt.Payload.Installation.ID || repository.attempt.Payload.AttemptID == oldAttempt.Payload.AttemptID {
		t.Fatalf("re-enrollment identity/attempt = %+v", repository.attempt.Payload)
	}
	if repository.attempt.PreviousResult == nil || repository.attempt.PreviousResult.Binding.ID != oldAttempt.Result.Binding.ID {
		t.Fatalf("previous binding history was not retained: %+v", repository.attempt.PreviousResult)
	}
	if _, err := credentials.Get(oldAttempt.AccessCredentialRef); err != nil {
		t.Fatal("old credential was deleted before replacement cleanup could be retried")
	}
	if repository.attempt.AccessCredentialRef == oldAttempt.AccessCredentialRef {
		t.Fatal("replacement credential reference reused old reference")
	}
	credentials.failRef = ""
	if _, err := service.Resume(context.Background()); err != nil {
		t.Fatalf("Resume() cleanup error=%v", err)
	}
	if _, err := credentials.Get(oldAttempt.AccessCredentialRef); err == nil {
		t.Fatal("old credential remains after cleanup retry")
	}
	if len(client.requests) != 2 {
		t.Fatalf("cleanup retry resent enrollment: requests=%d", len(client.requests))
	}
}

func TestServiceTreatsRecoveryRevokedAsPermanentAndRequiresExplicitReEnrollment(t *testing.T) {
	origin, _ := ParseCoreOrigin("https://core.example.com")
	repository := &memoryAttemptRepository{}
	credentials := memoryCredentialStore{}
	client := &recordingClient{errors: []error{&PermanentError{Code: "ENROLLMENT_RECOVERY_REVOKED"}}, response: validResponse("", "https://core.example.com/api/v1/agent-control")}
	service := Service{Repository: repository, Credentials: credentials, Client: client}
	if _, err := service.Start(context.Background(), origin, validPayload(""), "revoked-token"); err == nil {
		t.Fatal("Start() accepted revoked recovery")
	}
	if repository.attempt.Status != AttemptFailed {
		t.Fatalf("status=%q, want failed", repository.attempt.Status)
	}
	count := len(client.requests)
	if _, err := service.Resume(context.Background()); !errors.Is(err, ErrAttemptFailed) {
		t.Fatalf("Resume()=%v, want permanent failure", err)
	}
	if len(client.requests) != count {
		t.Fatal("Resume retried permanently revoked recovery")
	}
	if _, err := service.Start(context.Background(), origin, validPayload(""), "fresh-token"); !errors.Is(err, ErrAttemptFailed) {
		t.Fatalf("Start()=%v, want explicit re-enrollment required", err)
	}
	if _, err := service.ReEnroll(context.Background(), origin, validPayload(""), "fresh-token"); err != nil {
		t.Fatalf("ReEnroll()=%v", err)
	}
	if repository.attempt.Status != AttemptRegistered || len(client.requests) != count+1 {
		t.Fatalf("status=%q requests=%d", repository.attempt.Status, len(client.requests))
	}
}

func TestServicePreservesPriorBindingAndCredentialAcrossRepeatedRevokedRecovery(t *testing.T) {
	origin, _ := ParseCoreOrigin("https://core.example.com")
	repository := &memoryAttemptRepository{}
	credentials := memoryCredentialStore{}
	client := &recordingClient{response: validResponse("", "https://core.example.com/api/v1/agent-control")}
	service := Service{Repository: repository, Credentials: credentials, Client: client}
	first, err := service.Start(context.Background(), origin, validPayload(""), "first-token")
	if err != nil {
		t.Fatal(err)
	}
	firstAttempt := repository.attempt
	firstCredential, err := credentials.Get(firstAttempt.AccessCredentialRef)
	if err != nil {
		t.Fatal(err)
	}
	client.errors = []error{&PermanentError{Code: "ENROLLMENT_RECOVERY_REVOKED"}}
	if _, err := service.ReEnroll(context.Background(), origin, validPayload(""), "second-token"); err == nil {
		t.Fatal("ReEnroll() accepted revoked recovery")
	}
	if _, err := credentials.Get(firstAttempt.AccessCredentialRef); err != nil {
		t.Fatal("prior credential was deleted before replacement committed")
	}
	if _, err := service.ReEnroll(context.Background(), origin, validPayload(""), "third-token"); err != nil {
		t.Fatalf("second ReEnroll() error=%v", err)
	}
	if repository.attempt.PreviousResult == nil || repository.attempt.PreviousResult.Binding.ID != first.Binding.ID {
		t.Fatalf("prior binding history was lost: %+v", repository.attempt.PreviousResult)
	}
	if repository.attempt.PreviousAccessCredentialRef != "" {
		t.Fatalf("retired credential reference was not cleared: %q", repository.attempt.PreviousAccessCredentialRef)
	}
	if _, err := credentials.Get(firstAttempt.AccessCredentialRef); err == nil {
		t.Fatal("prior credential remains after the replacement was committed")
	}
	if len(client.requests) != 3 {
		t.Fatalf("client requests=%d, want initial plus two re-enrollment attempts", len(client.requests))
	}
	if firstCredential == "" {
		t.Fatal("first Agent credential was empty")
	}
}

func TestServiceAllowsNewOriginAfterProvenUnsentRequest(t *testing.T) {
	oldOrigin, _ := ParseCoreOrigin("https://localhost:8081")
	newOrigin, _ := ParseCoreOrigin("https://core-tunnel.test")
	repository := &memoryAttemptRepository{}
	credentials := memoryCredentialStore{}
	client := &recordingClient{errors: []error{&RequestNotSentError{Code: "CORE_TLS_REQUIRED"}}, response: validResponse("", "https://core-tunnel.test/api/v1/agent-control")}
	waits := 0
	service := Service{
		Repository: repository, Credentials: credentials, Client: client,
		RetryPolicy: RetryPolicy{MaxAttempts: 3}, Wait: func(context.Context, time.Duration) error { waits++; return nil },
	}
	payload := validPayload("")
	if _, err := service.Start(context.Background(), oldOrigin, payload, "unused-token"); err == nil {
		t.Fatal("Start() accepted a request proven not sent")
	}
	if len(client.requests) != 1 {
		t.Fatalf("request-not-sent failure made %d attempts, want 1", len(client.requests))
	}
	if waits != 0 {
		t.Fatalf("request-not-sent failure waited %d times, want no retry", waits)
	}
	failed, err := repository.LoadAttempt()
	if err != nil || failed.Status != AttemptFailed || failed.FailureCode != "CORE_TLS_REQUIRED" {
		t.Fatalf("unsent attempt state=%+v err=%v", failed, err)
	}
	if _, err := credentials.Get(failed.BootstrapCredentialRef); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("unused bootstrap token was retained: %v", err)
	}
	result, err := service.Start(context.Background(), newOrigin, payload, "fresh-token")
	if err != nil || result.InstallationID != payload.Installation.ID {
		t.Fatalf("Start() on corrected Core origin result=%+v err=%v", result, err)
	}
	if repository.attempt.CoreOrigin != newOrigin.String() || repository.attempt.Payload.AttemptID == failed.Payload.AttemptID {
		t.Fatal("corrected origin did not create a fresh attempt")
	}
}

func validPayload(_ string) EnrollmentPayload {
	return EnrollmentPayload{
		Installation: Installation{ID: "10000000-0000-4000-8000-000000000002"},
		Agent:        Agent{Version: "0.1.4"},
		Environment:  Environment{OS: "linux", Architecture: "amd64", Hostname: "node-01"},
		Metadata:     map[string]any{"deployment_kind": "vm"},
	}
}

func validResponse(_ string, controlPlaneURL string) EnrollmentResponse {
	return EnrollmentResponse{
		EnrollmentID: "10000000-0000-4000-8000-000000000003", InstallationID: "10000000-0000-4000-8000-000000000002",
		Binding:      Binding{ID: "10000000-0000-4000-8000-000000000004", NodeID: "10000000-0000-4000-8000-000000000005", Status: "active"},
		Credential:   Credential{Type: "bearer", AccessToken: "agent-secret"},
		ControlPlane: ControlPlane{BaseURL: controlPlaneURL, PollIntervalSeconds: 30},
	}
}

type memoryAttemptRepository struct {
	attempt      Attempt
	created      bool
	failComplete bool
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

func (r *memoryAttemptRepository) ReplaceForReEnrollment(id string, value Attempt) error {
	allowed := r.attempt.Status == AttemptRegistered || (r.attempt.Status == AttemptFailed && r.attempt.FailureCode == "ENROLLMENT_RECOVERY_REVOKED")
	if !r.created || r.attempt.Payload.AttemptID != id || !allowed {
		return ErrAttemptExists
	}
	value.PreviousResult = r.attempt.Result
	if value.PreviousResult == nil {
		value.PreviousResult = r.attempt.PreviousResult
	}
	value.PreviousAccessCredentialRef = r.attempt.AccessCredentialRef
	if r.attempt.PreviousAccessCredentialRef != "" {
		value.PreviousAccessCredentialRef = r.attempt.PreviousAccessCredentialRef
	}
	r.attempt = value
	return nil
}

func (r *memoryAttemptRepository) CompleteAttempt(id string, result EnrollmentResult, credentialRef string) error {
	if r.failComplete {
		r.failComplete = false
		return errors.New("simulated SQLite failure")
	}
	if !r.created || r.attempt.Payload.AttemptID != id {
		return ErrNoAttempt
	}
	r.attempt.Status = AttemptRegistered
	r.attempt.Result = &result
	r.attempt.AccessCredentialRef = credentialRef
	return nil
}

func (r *memoryAttemptRepository) ClearPreviousCredential(id string) error {
	if !r.created || r.attempt.Payload.AttemptID != id {
		return ErrNoAttempt
	}
	r.attempt.PreviousAccessCredentialRef = ""
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

type failingPutCredentialStore struct {
	memoryCredentialStore
	failOnce bool
}

func (s *failingPutCredentialStore) Put(reference, value string) error {
	if s.failOnce && strings.HasPrefix(reference, "enrollment/agent/") {
		s.failOnce = false
		return errors.New("simulated credential write failure")
	}
	return s.memoryCredentialStore.Put(reference, value)
}

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
	fail    bool
	failRef string
}

func (s *failingDeleteCredentialStore) Put(reference, value string) error {
	return s.memoryCredentialStore.Put(reference, value)
}

func (s *failingDeleteCredentialStore) Get(reference string) (string, error) {
	return s.memoryCredentialStore.Get(reference)
}

func (s *failingDeleteCredentialStore) Delete(reference string) error {
	if s.fail || reference == s.failRef {
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
