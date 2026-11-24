package ingress

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testAgentID = "11111111-1111-4111-8111-111111111111"

func TestFleetVerifierUsesTLSNoStoreAndOneRequestPerCheck(t *testing.T) {
	const token = "test-only-agent-otlp-bearer-secret-123456"
	checks := 0
	fleet := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checks++
		if r.TLS == nil || r.Method != http.MethodPost || r.URL.Path != "/internal/v1/otlp/credentials/introspect" {
			t.Errorf("unsafe Fleet request: %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token != token {
			t.Error("Fleet did not receive the bearer in the request body")
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema_version":1,"active":true,"agent_id":"` + testAgentID + `","credential_id":"otlp-1","scope":"otlp.write"}`))
	}))
	defer fleet.Close()
	verifier, err := NewFleetVerifier(fleet.URL, fleet.Client())
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		principal, err := verifier.Verify(context.Background(), token)
		if err != nil || principal.AgentID != testAgentID || principal.Scope != "otlp.write" {
			t.Fatalf("verified principal=%+v, err=%v", principal, err)
		}
	}
	if checks != 2 {
		t.Fatalf("Fleet checks=%d, want one per request", checks)
	}
}

func TestFleetVerifierRejectsInvalidResponsesAndRedirectsWithoutSecretLeaks(t *testing.T) {
	const token = "test-only-agent-otlp-bearer-secret-123456"
	for _, test := range []struct {
		name, body, cache string
		status            int
	}{
		{"inactive", `{"schema_version":1,"active":false}`, "no-store", http.StatusOK},
		{"invalid identity", `{"schema_version":1,"active":true,"agent_id":"spoofed","scope":"otlp.write"}`, "no-store", http.StatusOK},
		{"missing no-store", `{"schema_version":1,"active":true,"agent_id":"` + testAgentID + `","scope":"otlp.write"}`, "", http.StatusOK},
		{"redirect", ``, "", http.StatusTemporaryRedirect},
		{"unavailable", ``, "", http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			fleet := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", test.cache)
				w.Header().Set("Content-Type", "application/json")
				if test.status == http.StatusTemporaryRedirect {
					w.Header().Set("Location", "https://other.example/steal")
				}
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer fleet.Close()
			verifier, err := NewFleetVerifier(fleet.URL, fleet.Client())
			if err != nil {
				t.Fatal(err)
			}
			principal, err := verifier.Verify(context.Background(), token)
			if test.name == "inactive" {
				if err != nil || principal.AgentID != "" {
					t.Fatalf("inactive principal=%+v, err=%v", principal, err)
				}
			} else if err == nil || strings.Contains(err.Error(), token) {
				t.Fatalf("unsafe response accepted or leaked bearer: principal=%+v, err=%v", principal, err)
			}
		})
	}
}

func TestFleetVerifierRequiresHTTPSOrigin(t *testing.T) {
	for _, origin := range []string{"http://fleet.example.test", "https://user:secret@fleet.example.test", "https://fleet.example.test/path", "https://fleet.example.test?x=1"} {
		if _, err := NewFleetVerifier(origin, &http.Client{Timeout: time.Second}); err == nil {
			t.Fatalf("unsafe Fleet origin accepted: %q", origin)
		}
	}
}
