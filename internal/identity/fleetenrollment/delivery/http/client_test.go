package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/domain"
)

func TestClientUsesContractExchangeAndRecoveryRoutesAndNoStore(t *testing.T) {
	var exchangeBody, recoveryBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, _ := io.ReadAll(r.Body)
		body := strings.NewReader(string(content))
		switch r.URL.Path {
		case "/v1/enrollment/exchanges":
			data, _ := io.ReadAll(body)
			exchangeBody = string(data)
			if r.Method != http.MethodPost {
				t.Errorf("exchange method=%s", r.Method)
			}
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(response())
		case "/v1/enrollment/recoveries":
			data, _ := io.ReadAll(body)
			recoveryBody = string(data)
			if r.Method != http.MethodPost {
				t.Errorf("recovery method=%s", r.Method)
			}
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(response())
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	origin, err := domain.ParseOrigin(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(server.Client())
	request := domain.ExchangeRequest{AttemptID: "11111111-1111-4111-8111-111111111111", AgentID: "22222222-2222-4222-8222-222222222222", InvitationToken: "invite", RecoveryProof: strings.Repeat("p", 32)}
	if _, err := client.Exchange(context.Background(), origin, request); err != nil {
		t.Fatalf("Exchange(): %v", err)
	}
	if !strings.Contains(exchangeBody, `"agent_id":"22222222-2222-4222-8222-222222222222"`) || strings.Contains(exchangeBody, "instance_uid") {
		t.Fatalf("exchange body has wrong identity contract: %s", exchangeBody)
	}
	if _, err := client.Recover(context.Background(), origin, domain.RecoveryRequest{AttemptID: request.AttemptID, AgentID: request.AgentID, RecoveryProof: request.RecoveryProof}); err != nil {
		t.Fatalf("Recover(): %v", err)
	}
	if !strings.Contains(recoveryBody, `"recovery_proof"`) || strings.Contains(recoveryBody, "invitation_token") {
		t.Fatalf("recovery body has wrong secret contract: %s", recoveryBody)
	}
}

func TestClientRejectsCredentialResponseWithoutNoStore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(response())
	}))
	defer server.Close()
	origin, _ := domain.ParseOrigin(server.URL)
	_, err := NewClient(server.Client()).Exchange(context.Background(), origin, domain.ExchangeRequest{
		AttemptID: "11111111-1111-4111-8111-111111111111", AgentID: "22222222-2222-4222-8222-222222222222",
		InvitationToken: "invite", RecoveryProof: strings.Repeat("p", 32),
	})
	if err == nil || !strings.Contains(err.Error(), "no-store") {
		t.Fatalf("Exchange() error=%v, want no-store rejection", err)
	}
}

func TestClientProblemErrorDoesNotExposeResponseDetails(t *testing.T) {
	const secretDetail = "invitation-token-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"status":409,"code":"ENROLLMENT_CONFLICT","detail":"` + secretDetail + `"}`))
	}))
	defer server.Close()
	origin, _ := domain.ParseOrigin(server.URL)
	_, err := NewClient(server.Client()).Exchange(context.Background(), origin, domain.ExchangeRequest{
		AttemptID: "11111111-1111-4111-8111-111111111111", AgentID: "22222222-2222-4222-8222-222222222222",
		InvitationToken: "invite", RecoveryProof: strings.Repeat("p", 32),
	})
	if err == nil || strings.Contains(err.Error(), secretDetail) {
		t.Fatalf("error leaks detail: %v", err)
	}
	var problem ProblemError
	if !errors.As(err, &problem) || problem.Code != "ENROLLMENT_CONFLICT" {
		t.Fatalf("sanitized error=%v, want stable problem code", err)
	}
}

func response() domain.EnrollmentResponse {
	return domain.EnrollmentResponse{AgentID: "22222222-2222-4222-8222-222222222222", NodeID: "33333333-3333-4333-8333-333333333333", CredentialID: "cred", Credential: "secret", CredentialScope: "opamp.connect", OpAMPEndpoint: "wss://fleet.example.test/opamp"}
}

func TestClientDoesNotFollowCredentialBearingRedirect(t *testing.T) {
	var redirectedRequests int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedRequests++
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(response())
	}))
	defer target.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL+"/capture")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer server.Close()

	origin, err := domain.ParseOrigin(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewClient(server.Client()).Exchange(context.Background(), origin, domain.ExchangeRequest{
		AttemptID: "11111111-1111-4111-8111-111111111111", AgentID: "22222222-2222-4222-8222-222222222222",
		InvitationToken: "invitation-secret", RecoveryProof: strings.Repeat("p", 32),
	})
	if err == nil {
		t.Fatal("Exchange followed a redirect carrying invitation credentials")
	}
	var problem ProblemError
	if !errors.As(err, &problem) || problem.Status != http.StatusTemporaryRedirect {
		t.Fatalf("Exchange() error=%v, want sanitized HTTP 307 response", err)
	}
	if redirectedRequests != 0 {
		t.Fatalf("redirect target received %d credential-bearing request(s)", redirectedRequests)
	}
}

func TestProblemErrorClassifiesOnlyAmbiguousResponsesAsUncertain(t *testing.T) {
	for _, test := range []struct {
		status    int
		uncertain bool
	}{
		{status: http.StatusTemporaryRedirect, uncertain: false},
		{status: http.StatusUnauthorized, uncertain: false},
		{status: http.StatusRequestTimeout, uncertain: true},
		{status: http.StatusTooEarly, uncertain: true},
		{status: http.StatusTooManyRequests, uncertain: true},
		{status: http.StatusBadGateway, uncertain: true},
	} {
		if got := (ProblemError{Status: test.status}).OutcomeUncertain(); got != test.uncertain {
			t.Errorf("status %d uncertain=%t, want %t", test.status, got, test.uncertain)
		}
	}
}
