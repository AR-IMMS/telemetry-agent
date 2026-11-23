package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/identity/credentials"
	fleetdomain "github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/domain"
	opampdomain "github.com/ar-imms/telemetry-agent/internal/runtime/opamp/domain"
)

func TestRunEnrollmentFleetProductionPathPersistsAndExplicitlyProvisions(t *testing.T) {
	if !canPersistFleetCredentialForTest() {
		if os.Getenv("AR_IMMS_REQUIRE_PROTECTED_ENROLLMENT_TEST") == "1" {
			t.Fatal("required protected Fleet credential test has no elevated Windows token")
		}
		t.Skip("protected Windows credential-store integration requires an elevated process")
	}
	const invitation = "fleet-invitation-secret"
	const opampCredential = "fleet-opamp-secret"
	const opampEndpoint = "wss://fleet.example.test/v1/opamp"
	var receivedAgentID string
	var receivedAttemptID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/enrollment/exchanges" {
			t.Errorf("request = %s %s, want Fleet exchange", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var request fleetdomain.ExchangeRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode exchange request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		receivedAgentID, receivedAttemptID = request.AgentID, request.AttemptID
		if request.InvitationToken != invitation || len(request.RecoveryProof) < 32 {
			t.Errorf("exchange request omitted invitation or recovery proof")
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(fleetdomain.EnrollmentResponse{
			AgentID: request.AgentID, NodeID: "33333333-3333-4333-8333-333333333333",
			CredentialID: "credential-1", Credential: opampCredential,
			CredentialScope: "opamp.connect", OpAMPEndpoint: opampEndpoint,
		})
	}))
	defer server.Close()

	statePath := filepath.Join(t.TempDir(), "agent-state.db")
	if _, err := newAgentStateStore(statePath).Update(func(state *agentstate.State) error {
		state.Supervisor = opampdomain.SupervisorContext{Enabled: true, BinaryPath: filepath.Join(t.TempDir(), "opampsupervisor.exe")}
		return nil
	}); err != nil {
		t.Fatalf("initialize Supervisor state: %v", err)
	}

	deps := dependencies{
		readFleetInvitationToken: func(_ io.Writer) (string, error) { return invitation, nil },
		runFleetEnrollment:       runAgentFleetEnrollment,
		provisionFleetSupervisor: provisionFleetSupervisor,
	}
	var stdout, stderr bytes.Buffer
	code := runEnrollment(context.Background(), []string{
		"--fleet-origin", server.URL, "--state-path", statePath, "--provision-supervisor",
	}, &stdout, &stderr, deps)
	if code != 0 {
		t.Fatalf("runEnrollment()=%d stderr=%q", code, stderr.String())
	}
	if receivedAgentID == "" || receivedAttemptID == "" || receivedAgentID == receivedAttemptID {
		t.Fatalf("exchange identity mapping agent_id=%q attempt_id=%q", receivedAgentID, receivedAttemptID)
	}
	if strings.Contains(stdout.String()+stderr.String(), invitation) || strings.Contains(stdout.String()+stderr.String(), opampCredential) {
		t.Fatal("CLI output exposed Fleet secret material")
	}

	attempt, err := newAgentFleetEnrollmentStore(statePath).LoadAttempt()
	if err != nil {
		t.Fatalf("load durable Fleet result: %v", err)
	}
	if attempt.Status != fleetdomain.AttemptRegistered || attempt.Result == nil || attempt.Result.AgentID != receivedAgentID {
		t.Fatalf("persisted Fleet attempt=%+v", attempt)
	}
	store := credentials.New(statePath)
	if got, err := store.Get(attempt.CredentialRef); err != nil || got != opampCredential {
		t.Fatalf("protected Fleet credential=%q, err=%v", got, err)
	}
	if _, err := store.Get(attempt.InvitationRef); err == nil {
		t.Fatal("completed exchange retained the invitation token")
	}
	if _, err := store.Get(attempt.RecoveryProofRef); err == nil {
		t.Fatal("completed exchange retained the recovery proof")
	}
	state, err := newAgentStateStore(statePath).Load()
	if err != nil || state.Supervisor.OpAMPServerURL != opampEndpoint {
		t.Fatalf("explicitly provisioned Supervisor endpoint=%q, err=%v", state.Supervisor.OpAMPServerURL, err)
	}
	database, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(database)
	for _, secret := range []string{invitation, opampCredential} {
		if bytes.Contains(database, []byte(secret)) {
			t.Fatalf("SQLite state contains secret %q", secret)
		}
	}
}

func TestFleetEnrollmentRecoversCommittedExchangeAfterCommandRestart(t *testing.T) {
	if !canPersistFleetCredentialForTest() {
		if os.Getenv("AR_IMMS_REQUIRE_PROTECTED_ENROLLMENT_TEST") == "1" {
			t.Fatal("required protected Fleet credential test has no elevated Windows token")
		}
		t.Skip("protected Windows credential-store integration requires an elevated process")
	}
	const invitation = "restart-test-invitation-secret"
	const credential = "restart-test-opamp-credential"
	const opampEndpoint = "wss://fleet.example.test/v1/opamp"

	statePath := filepath.Join(t.TempDir(), "agent-state.db")
	repository := newAgentFleetEnrollmentStore(statePath)
	store := credentials.New(statePath)
	t.Cleanup(func() {
		if attempt, err := repository.LoadAttempt(); err == nil {
			_ = store.Delete(attempt.InvitationRef)
			_ = store.Delete(attempt.RecoveryProofRef)
			_ = store.Delete(attempt.CredentialRef)
		}
	})

	var committedRequest fleetdomain.ExchangeRequest
	var committedResponse fleetdomain.EnrollmentResponse
	exchangeCalls := 0
	recoveryCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/enrollment/exchanges":
			var request fleetdomain.ExchangeRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode exchange: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			exchangeCalls++
			if exchangeCalls == 1 {
				committedRequest = request
				committedResponse = fleetdomain.EnrollmentResponse{
					AgentID: request.AgentID, NodeID: "33333333-3333-4333-8333-333333333333",
					CredentialID: "restart-credential", Credential: credential,
					CredentialScope: "opamp.connect", OpAMPEndpoint: opampEndpoint,
				}
				hijacker, ok := w.(http.Hijacker)
				if !ok {
					t.Fatal("HTTP test server does not support response-drop simulation")
				}
				connection, _, err := hijacker.Hijack()
				if err != nil {
					t.Errorf("hijack committed response: %v", err)
					return
				}
				_ = connection.Close()
				return
			}
			if request != committedRequest {
				t.Errorf("exchange retry changed the immutable request")
			}
			http.Error(w, `{"code":"TEMPORARY_UNAVAILABLE"}`, http.StatusServiceUnavailable)
		case "/v1/enrollment/recoveries":
			var request fleetdomain.RecoveryRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode recovery: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			recoveryCalls++
			if request.AttemptID != committedRequest.AttemptID || request.AgentID != committedRequest.AgentID || request.RecoveryProof != committedRequest.RecoveryProof {
				t.Errorf("recovery did not prove the committed exchange attempt")
				http.Error(w, `{"code":"RECOVERY_REJECTED"}`, http.StatusUnauthorized)
				return
			}
			if recoveryCalls == 1 {
				http.Error(w, `{"code":"TEMPORARY_UNAVAILABLE"}`, http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(committedResponse)
		default:
			t.Errorf("unexpected Fleet route %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if _, err := runAgentFleetEnrollment(context.Background(), statePath, server.URL, invitation, false); err == nil {
		t.Fatal("initial command unexpectedly recovered while Fleet recovery was unavailable")
	}
	attempt, err := repository.LoadAttempt()
	if err != nil || attempt.Status != fleetdomain.AttemptPending || attempt.AttemptID != committedRequest.AttemptID {
		t.Fatalf("pending attempt after simulated command exit=%+v err=%v", attempt, err)
	}
	if saved, err := store.Get(attempt.InvitationRef); err != nil || saved != invitation {
		t.Fatal("restart state did not retain the invitation in protected storage")
	}
	if saved, err := store.Get(attempt.RecoveryProofRef); err != nil || saved != committedRequest.RecoveryProof {
		t.Fatal("restart state did not retain the matching recovery proof in protected storage")
	}

	result, err := runAgentFleetEnrollment(context.Background(), statePath, server.URL, "", true)
	if err != nil {
		t.Fatalf("resumed command after restart: %v", err)
	}
	if result.AgentID != committedRequest.AgentID || result.CredentialID != committedResponse.CredentialID || exchangeCalls != 4 || recoveryCalls != 2 {
		t.Fatalf("resume result=%+v exchange/recovery calls=%d/%d", result, exchangeCalls, recoveryCalls)
	}
	attempt, err = repository.LoadAttempt()
	if err != nil || attempt.Status != fleetdomain.AttemptRegistered || attempt.Result == nil {
		t.Fatalf("registered attempt after resume=%+v err=%v", attempt, err)
	}
	if saved, err := store.Get(attempt.CredentialRef); err != nil || saved != credential {
		t.Fatal("resumed command did not protect the original Fleet credential")
	}
	if _, err := store.Get(attempt.InvitationRef); err == nil {
		t.Fatal("resumed command retained the invitation token")
	}
	if _, err := store.Get(attempt.RecoveryProofRef); err == nil {
		t.Fatal("resumed command retained the recovery proof")
	}
	database, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(database)
	for _, secret := range []string{invitation, committedRequest.RecoveryProof, credential} {
		if strings.Contains(string(database), secret) {
			t.Fatal("SQLite state contains Fleet secret material after recovery")
		}
	}
}
