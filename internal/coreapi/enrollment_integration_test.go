package coreapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/credentialstore"
	"github.com/ar-imms/telemetry-agent/internal/enrollment"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

// TestCoreEnrollmentLifecycle exercises the Agent against an isolated HTTPS
// Core deployment. It requires an authorized test Node and an administrator
// token because it creates bootstrap tokens and revokes test bindings.
func TestCoreEnrollmentLifecycle(t *testing.T) {
	if os.Getenv("AR_IMMS_CORE_ENROLLMENT_TEST") != "1" {
		t.Skip("set AR_IMMS_CORE_ENROLLMENT_TEST=1 to opt in to the isolated Core deployment")
	}
	if !canPersistPlatformCredentialForTest() {
		t.Skip("the Windows CredentialStore integration requires an elevated process")
	}
	if os.Getenv("AR_IMMS_CORE_ENROLLMENT_TEST_CONFIRM_ISOLATED") != "yes" {
		t.Fatal("isolated test confirmation is required")
	}
	origin, err := enrollment.ParseCoreOrigin(os.Getenv("AR_IMMS_CORE_ENROLLMENT_TEST_ORIGIN"))
	if err != nil {
		t.Fatalf("test Core origin: %v", err)
	}
	if !isIsolatedTestHost(origin.String()) {
		t.Fatal("Core origin host must be localhost, loopback, or use the reserved .test domain")
	}
	nodeID := strings.TrimSpace(os.Getenv("AR_IMMS_CORE_ENROLLMENT_TEST_NODE_ID"))
	if _, err := identity.ParseInstallationID(nodeID); err != nil {
		t.Fatalf("test Node ID must be a UUID: %v", err)
	}
	adminToken := strings.TrimSpace(os.Getenv("AR_IMMS_CORE_ENROLLMENT_TEST_ADMIN_TOKEN"))
	if adminToken == "" {
		t.Fatal("an authorized Core test administrator token is required")
	}

	adminHTTP := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	temp := t.TempDir()
	statePath := filepath.Join(temp, "state.db")
	installationID, err := identity.NewInstallationID(rand.Reader)
	if err != nil {
		t.Fatal("generate test installation ID")
	}
	if _, err := agentstate.NewIdentityStore(statePath).EnsureInstallationID(installationID); err != nil {
		t.Fatalf("persist test installation identity: %v", err)
	}
	repository := agentstate.NewEnrollmentStore(statePath)
	credentials := credentialstore.New(statePath)

	var lastBinding string
	t.Cleanup(func() {
		if lastBinding != "" {
			_ = revokeTestBinding(adminHTTP, origin, adminToken, lastBinding)
		}
	})

	firstToken := issueTestBootstrapToken(t, adminHTTP, origin, adminToken, nodeID)
	firstTransport := &faultTransport{base: http.DefaultTransport, dropFirstResponse: true}
	service := enrollment.Service{
		Repository: repository, Credentials: credentials,
		Client:      newEnrollmentClient(&http.Client{Transport: firstTransport, Timeout: 15 * time.Second}),
		RetryPolicy: enrollment.RetryPolicy{MaxAttempts: 1},
	}
	payload := enrollment.EnrollmentPayload{
		Installation: enrollment.Installation{ID: installationID.String()},
		Agent:        enrollment.Agent{Version: "0.1.4-r3-integration"},
		Environment:  enrollment.Environment{OS: agentOS(), Architecture: agentArch()},
		Metadata:     map[string]any{"test_run": "agent-enrollment-r3"},
	}
	_, startErr := service.Start(context.Background(), origin, payload, firstToken)
	lastBinding = firstTransport.droppedBinding
	if startErr == nil {
		t.Fatal("Start() succeeded despite simulated lost HTTP response")
	}
	pending, err := repository.LoadAttempt()
	if err != nil || pending.Status != enrollment.AttemptPending {
		t.Fatalf("pending attempt after lost response = %q, err=%v", pending.Status, err)
	}
	firstAttemptID := pending.Payload.AttemptID
	if len(firstTransport.statuses) != 1 || firstTransport.statuses[0] != http.StatusCreated {
		t.Fatalf("Core first enrollment status=%v, want 201", firstTransport.statuses)
	}

	// A new Service instance models an Agent process restart. The persisted token,
	// origin, payload, and attempt ID are loaded from SQLite and CredentialStore.
	firstTransport.dropFirstResponse = false
	service = enrollment.Service{
		Repository: repository, Credentials: credentials,
		Client:      newEnrollmentClient(&http.Client{Transport: firstTransport, Timeout: 15 * time.Second}),
		RetryPolicy: enrollment.RetryPolicy{MaxAttempts: 1},
	}
	firstResult, err := service.Resume(context.Background())
	if err != nil {
		t.Fatalf("Resume() after simulated restart: %v", err)
	}
	lastBinding = firstResult.Binding.ID
	if len(firstTransport.statuses) != 2 || firstTransport.statuses[1] != http.StatusOK {
		t.Fatalf("Core recovery statuses=%v, want [201 200]", firstTransport.statuses)
	}
	if len(firstTransport.requestHashes) != 2 || firstTransport.requestHashes[0] != firstTransport.requestHashes[1] {
		t.Fatal("recovery did not resend the exact persisted enrollment request")
	}
	registered, err := repository.LoadAttempt()
	if err != nil || registered.Status != enrollment.AttemptRegistered || registered.Payload.AttemptID != firstAttemptID {
		t.Fatalf("registered attempt state = %+v, err=%v", registered, err)
	}
	firstAccessToken, err := credentials.Get(registered.AccessCredentialRef)
	if err != nil || firstAccessToken == "" {
		t.Fatal("Agent credential was not securely stored")
	}
	if _, err := credentials.Get(registered.BootstrapCredentialRef); !errors.Is(err, enrollment.ErrCredentialNotFound) {
		t.Fatalf("completed bootstrap token cleanup error = %v", err)
	}
	assertSQLiteContainsNoSecret(t, statePath, firstToken, firstAccessToken)

	// Core owns attempt conflict semantics. Replaying the consumed token with the
	// same ID but a changed immutable Agent version must be rejected as 409.
	changed := enrollment.EnrollmentRequest{
		BootstrapToken: firstToken,
		EnrollmentPayload: enrollment.EnrollmentPayload{
			AttemptID: firstAttemptID, Installation: payload.Installation,
			Agent:       enrollment.Agent{Version: payload.Agent.Version + ".changed"},
			Environment: payload.Environment, Metadata: payload.Metadata,
		},
	}
	client := NewEnrollmentClient()
	if _, err := client.Enroll(context.Background(), origin, changed); err == nil {
		t.Fatal("Core accepted a changed request for an existing attempt ID")
	} else {
		var permanent *enrollment.PermanentError
		if !errors.As(err, &permanent) || permanent.Code != "ENROLLMENT_ATTEMPT_CONFLICT" {
			t.Fatalf("changed request error = %v, want 409 ENROLLMENT_ATTEMPT_CONFLICT", err)
		}
		assertNoSecrets(t, err.Error(), firstToken, firstAccessToken, adminToken)
	}

	if err := revokeTestBinding(adminHTTP, origin, adminToken, firstResult.Binding.ID); err != nil {
		t.Fatalf("revoke first test binding: %v", err)
	}
	lastBinding = ""
	secondToken := issueTestBootstrapToken(t, adminHTTP, origin, adminToken, nodeID)
	secondTransport := &faultTransport{base: http.DefaultTransport, dropFirstResponse: true}
	service.Client = newEnrollmentClient(&http.Client{Transport: secondTransport, Timeout: 15 * time.Second})
	_, secondStartErr := service.ReEnroll(context.Background(), origin, payload, secondToken)
	lastBinding = secondTransport.droppedBinding
	if secondStartErr == nil {
		t.Fatal("ReEnroll() succeeded despite simulated lost response")
	}
	secondPending, err := repository.LoadAttempt()
	if err != nil || secondPending.Status != enrollment.AttemptPending || secondPending.Payload.AttemptID == firstAttemptID {
		t.Fatalf("second pending attempt = %+v, err=%v", secondPending, err)
	}
	secondAttemptID := secondPending.Payload.AttemptID
	if secondTransport.droppedBinding == "" || secondTransport.statuses[0] != http.StatusCreated {
		t.Fatal("Core did not commit the second enrollment before the simulated lost response")
	}
	lastBinding = secondTransport.droppedBinding
	if err := revokeTestBinding(adminHTTP, origin, adminToken, lastBinding); err != nil {
		t.Fatalf("revoke binding whose response was lost: %v", err)
	}
	secondTransport.dropFirstResponse = false
	if _, err := service.Resume(context.Background()); err == nil {
		t.Fatal("Resume() accepted recovery after Core revoked the binding")
	} else {
		var permanent *enrollment.PermanentError
		if !errors.As(err, &permanent) || permanent.Code != "ENROLLMENT_RECOVERY_REVOKED" {
			t.Fatalf("revoked recovery error = %v, want permanent ENROLLMENT_RECOVERY_REVOKED", err)
		}
		assertNoSecrets(t, err.Error(), secondToken, firstAccessToken, adminToken)
	}
	failed, err := repository.LoadAttempt()
	if err != nil || failed.Status != enrollment.AttemptFailed || failed.FailureCode != "ENROLLMENT_RECOVERY_REVOKED" {
		t.Fatalf("revoked recovery state = %+v, err=%v", failed, err)
	}
	requestCount := len(secondTransport.statuses)
	if _, err := service.Resume(context.Background()); !errors.Is(err, enrollment.ErrAttemptFailed) {
		t.Fatalf("Resume() after permanent 410 = %v, want ErrAttemptFailed", err)
	}
	if len(secondTransport.statuses) != requestCount {
		t.Fatal("permanent 410 caused another Core request")
	}
	if _, err := credentials.Get(failed.BootstrapCredentialRef); !errors.Is(err, enrollment.ErrCredentialNotFound) {
		t.Fatalf("410 bootstrap token cleanup error = %v", err)
	}

	thirdToken := issueTestBootstrapToken(t, adminHTTP, origin, adminToken, nodeID)
	service.Client = NewEnrollmentClient()
	thirdResult, err := service.ReEnroll(context.Background(), origin, payload, thirdToken)
	if err != nil {
		t.Fatalf("authorized re-enrollment after 410: %v", err)
	}
	if thirdResult.InstallationID != firstResult.InstallationID || thirdResult.Binding.ID == firstResult.Binding.ID || thirdResult.Binding.ID == lastBinding {
		t.Fatal("re-enrollment did not preserve installation identity and create a fresh binding")
	}
	thirdAttempt, err := repository.LoadAttempt()
	if err != nil || thirdAttempt.Payload.AttemptID == secondAttemptID || thirdAttempt.Status != enrollment.AttemptRegistered {
		t.Fatalf("third attempt state = %+v, err=%v", thirdAttempt, err)
	}
	if thirdAttempt.PreviousResult == nil || thirdAttempt.PreviousResult.Binding.ID != firstResult.Binding.ID {
		t.Fatal("re-enrollment lost the last known binding history")
	}
	if _, err := credentials.Get(registered.AccessCredentialRef); err == nil {
		t.Fatal("retired access credential was not removed after replacement committed")
	}
	lastBinding = thirdResult.Binding.ID
	assertSQLiteContainsNoSecret(t, statePath, firstToken, secondToken, thirdToken, firstAccessToken)
}

func TestFaultTransportForwardsThenDropsFirstSuccessfulResponse(t *testing.T) {
	const body = `{"bootstrap_token":"test-token","enrollment_attempt_id":"attempt"}`
	forwardedBodies := make([]string, 0, 2)
	base := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		content, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		forwardedBodies = append(forwardedBodies, string(content))
		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"data":{"binding":{"id":"binding-id"}}}`)),
			Request:    request,
		}, nil
	})
	transport := &faultTransport{base: base, dropFirstResponse: true}
	for attempt := 0; attempt < 2; attempt++ {
		target, _ := url.Parse("https://core.test/api/v1/agent-enrollments")
		request, err := http.NewRequest(http.MethodPost, target.String(), strings.NewReader(body))
		if err != nil {
			t.Fatal("create test request")
		}
		response, err := transport.RoundTrip(request)
		if attempt == 0 {
			if err == nil || !strings.Contains(err.Error(), "simulated lost") {
				t.Fatalf("first successful response was not dropped: %v", err)
			}
			continue
		}
		if err != nil || response.StatusCode != http.StatusCreated {
			t.Fatalf("second response status=%v, err=%v", response, err)
		}
		_ = response.Body.Close()
	}
	if transport.droppedBinding != "binding-id" || len(transport.requestHashes) != 2 || transport.requestHashes[0] != transport.requestHashes[1] || len(forwardedBodies) != 2 || forwardedBodies[0] != body || forwardedBodies[1] != body {
		t.Fatal("fault transport did not preserve request identity and record the committed binding")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type faultTransport struct {
	base              http.RoundTripper
	dropFirstResponse bool
	dropped           bool
	statuses          []int
	requestHashes     []string
	droppedBinding    string
}

func (f *faultTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	digest := sha256.Sum256(body)
	f.requestHashes = append(f.requestHashes, hex.EncodeToString(digest[:]))
	base := f.base
	if base == nil {
		base = http.DefaultTransport
	}
	response, err := base.RoundTrip(request)
	clear(body)
	if err != nil {
		return nil, err
	}
	f.statuses = append(f.statuses, response.StatusCode)
	if f.dropFirstResponse && !f.dropped && response.StatusCode >= 200 && response.StatusCode < 300 {
		f.dropped = true
		content, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			clear(content)
			return nil, readErr
		}
		var envelope struct {
			Data struct {
				Binding struct {
					ID string `json:"id"`
				} `json:"binding"`
			} `json:"data"`
		}
		_ = json.Unmarshal(content, &envelope)
		f.droppedBinding = envelope.Data.Binding.ID
		clear(content)
		return nil, errors.New("simulated lost Core enrollment response")
	}
	return response, nil
}

func issueTestBootstrapToken(t *testing.T, client *http.Client, origin enrollment.CoreOrigin, adminToken, nodeID string) string {
	t.Helper()
	url := origin.String() + "/api/v1/nodes/" + nodeID + "/agent-bootstrap-tokens"
	request, err := http.NewRequest(http.MethodPost, url, strings.NewReader("{}"))
	if err != nil {
		t.Fatal("create bootstrap-token request")
	}
	request.Header.Set("Authorization", "Bearer "+adminToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("issue bootstrap token: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("bootstrap-token status=%d, want 201", response.StatusCode)
	}
	var envelope struct {
		Data struct {
			Token string `json:"bootstrap_token"`
			Node  string `json:"node_id"`
		} `json:"data"`
		Meta struct {
			RequestID string `json:"request_id"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&envelope); err != nil || envelope.Data.Token == "" || envelope.Data.Node != nodeID || envelope.Meta.RequestID == "" {
		t.Fatal("Core returned an invalid bootstrap-token response envelope")
	}
	return envelope.Data.Token
}

func revokeTestBinding(client *http.Client, origin enrollment.CoreOrigin, adminToken, bindingID string) error {
	request, err := http.NewRequest(http.MethodPost, origin.String()+"/api/v1/agent-bindings/"+bindingID+"/revoke", nil)
	if err != nil {
		return fmt.Errorf("create binding revocation request")
	}
	request.Header.Set("Authorization", "Bearer "+adminToken)
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("revoke test binding: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Core binding revocation returned HTTP %d", response.StatusCode)
	}
	return nil
}

func assertSQLiteContainsNoSecret(t *testing.T, path string, secrets ...string) {
	t.Helper()
	files, err := filepath.Glob(path + "*")
	if err != nil {
		t.Fatal("enumerate test SQLite files")
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read test SQLite state: %v", err)
		}
		for _, secret := range secrets {
			if secret != "" && bytes.Contains(content, []byte(secret)) {
				clear(content)
				t.Fatal("SQLite state contains enrollment secret material")
			}
		}
		clear(content)
	}
}

func assertNoSecrets(t *testing.T, value string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if secret != "" && strings.Contains(value, secret) {
			t.Fatal("enrollment error exposed secret material")
		}
	}
}

func isIsolatedTestHost(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".test") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func agentOS() string {
	switch runtime.GOOS {
	case "linux", "windows", "darwin":
		return runtime.GOOS
	default:
		return "other"
	}
}

func agentArch() string {
	switch runtime.GOARCH {
	case "amd64", "arm64", "386":
		return runtime.GOARCH
	default:
		return "other"
	}
}
