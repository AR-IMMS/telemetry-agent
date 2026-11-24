package ingress

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type verifierFunc func(context.Context, string) (Principal, error)

func (f verifierFunc) Verify(ctx context.Context, token string) (Principal, error) {
	return f(ctx, token)
}

func TestIngressRejectsMissingInvalidAndUnavailableCredentials(t *testing.T) {
	forwarded := 0
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { forwarded++; w.WriteHeader(http.StatusOK) }))
	defer gateway.Close()
	verifier := verifierFunc(func(_ context.Context, token string) (Principal, error) {
		switch token {
		case strings.Repeat("v", 32):
			return Principal{AgentID: "agent-1", Scope: "otlp.write"}, nil
		case strings.Repeat("u", 32):
			return Principal{}, errors.New("Fleet unavailable")
		default:
			return Principal{}, nil
		}
	})
	handler, err := NewHandler(verifier, gateway.URL, gateway.Client())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, authorization string
		want                int
	}{
		{"missing", "", http.StatusUnauthorized},
		{"invalid", "Bearer " + strings.Repeat("x", 32), http.StatusUnauthorized},
		{"unavailable", "Bearer " + strings.Repeat("u", 32), http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := testRequest("/v1/metrics", `{"resourceMetrics":[{"resource":{}}]}`, test.authorization)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d, want %d", response.Code, test.want)
			}
		})
	}
	if forwarded != 0 {
		t.Fatalf("rejected requests forwarded %d times", forwarded)
	}
}

func TestIngressOverwritesEveryClientAgentIDBeforeForwarding(t *testing.T) {
	var forwarded []byte
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer gateway.Close()
	handler, err := NewHandler(verifierFunc(func(context.Context, string) (Principal, error) {
		return Principal{AgentID: "verified-agent", Scope: "otlp.write", ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), gateway.URL, gateway.Client())
	if err != nil {
		t.Fatal(err)
	}
	body := `{"resourceMetrics":[{"resource":{"attributes":[{"key":"agent_id","value":{"stringValue":"spoofed"}},{"key":"host.name","value":{"stringValue":"host"}}]},"scopeMetrics":[{"metrics":[{"name":"m","gauge":{"dataPoints":[{"attributes":[{"key":"agent_id","value":{"stringValue":"spoofed-point"}}]}]}}]}]}]}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, testRequest("/v1/metrics", body, "Bearer "+strings.Repeat("v", 32)))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(string(forwarded), "spoofed") {
		t.Fatalf("client identity survived: %s", forwarded)
	}
	var payload map[string]any
	if err := json.Unmarshal(forwarded, &payload); err != nil {
		t.Fatal(err)
	}
	resources := payload["resourceMetrics"].([]any)
	attrs := resources[0].(map[string]any)["resource"].(map[string]any)["attributes"].([]any)
	if len(attrs) != 2 {
		t.Fatalf("resource attributes=%v", attrs)
	}
	if attrs[1].(map[string]any)["value"].(map[string]any)["stringValue"] != "verified-agent" {
		t.Fatalf("verified agent_id absent: %v", attrs)
	}
}

func TestIngressRequiresTLSAndRejectsWrongScopeExpiryAndMalformedJSON(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { t.Fatal("invalid request was forwarded") }))
	defer gateway.Close()
	principal := Principal{AgentID: "agent-1", Scope: "otlp.write"}
	handler, err := NewHandler(verifierFunc(func(context.Context, string) (Principal, error) { return principal, nil }), gateway.URL, gateway.Client())
	if err != nil {
		t.Fatal(err)
	}
	token := "Bearer " + strings.Repeat("v", 32)
	request := testRequest("/v1/metrics", `{"resourceMetrics":[]}`, token)
	request.TLS = nil
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUpgradeRequired {
		t.Fatalf("plaintext status=%d", response.Code)
	}
	principal.Scope = "opamp.connect"
	request = testRequest("/v1/metrics", `{"resourceMetrics":[]}`, token)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("wrong scope status=%d", response.Code)
	}
	principal.Scope, principal.ExpiresAt = "otlp.write", time.Now().Add(-time.Second)
	request = testRequest("/v1/metrics", `{"resourceMetrics":[]}`, token)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expired status=%d", response.Code)
	}
	principal.ExpiresAt = time.Time{}
	request = testRequest("/v1/metrics", `{"resourceMetrics":`, token)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("malformed status=%d", response.Code)
	}
}

func testRequest(path, body, authorization string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "https://gateway.example.test"+path, strings.NewReader(body))
	request.TLS = &tls.ConnectionState{}
	request.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	return request
}
