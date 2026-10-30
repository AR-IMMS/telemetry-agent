package coreapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/enrollment"
)

func TestEnrollmentClientPostsContractAndAcceptsRecoveredResult(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != "https://core.example.test/api/v1/agent-enrollments" {
			t.Fatalf("request = %s %s", request.Method, request.URL)
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q", request.Header.Get("Content-Type"))
		}
		var body enrollment.EnrollmentRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.BootstrapToken != "do-not-log" || body.AttemptID != "attempt-1" || body.Installation.ID != "installation-1" {
			t.Fatalf("request body = %+v", body)
		}
		return jsonResponse(http.StatusOK, `{"data":{
			"enrollment_id":"enrollment-1","installation_id":"installation-1",
			"binding":{"id":"binding-1","node_id":"node-1","status":"active"},
			"credential":{"type":"bearer","access_token":"agent-token"},
			"control_plane":{"base_url":"https://core.example.test/api/v1/agent-control","poll_interval_seconds":30}
		}}`), nil
	})
	client := newEnrollmentClient(&http.Client{Transport: transport})
	result, err := client.Enroll(context.Background(), mustOrigin(t, "https://core.example.test"), enrollment.EnrollmentRequest{
		BootstrapToken: "do-not-log",
		EnrollmentPayload: enrollment.EnrollmentPayload{
			AttemptID: "attempt-1", Installation: enrollment.Installation{ID: "installation-1"},
			Agent:       enrollment.Agent{Version: "0.1.4"},
			Environment: enrollment.Environment{OS: "linux", Architecture: "amd64"},
		},
	})
	if err != nil {
		t.Fatalf("Enroll() error = %v", err)
	}
	if result.Credential.AccessToken != "agent-token" {
		t.Fatalf("access token = %q", result.Credential.AccessToken)
	}
}

func TestEnrollmentClientNeverFollowsRedirectWithEnrollmentRequest(t *testing.T) {
	var targetCalls int
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "core.example.test" {
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{"https://attacker.example/steal"}},
				Body:       io.NopCloser(strings.NewReader("")), Request: request,
			}, nil
		}
		targetCalls++
		return jsonResponse(http.StatusOK, `{}`), nil
	})
	_, err := newEnrollmentClient(&http.Client{Transport: transport}).Enroll(
		context.Background(), mustOrigin(t, "https://core.example.test"), enrollment.EnrollmentRequest{BootstrapToken: "secret"},
	)
	if err == nil {
		t.Fatal("Enroll() error = nil, want redirect rejection")
	}
	if targetCalls != 0 {
		t.Fatalf("redirect target received %d requests", targetCalls)
	}
}

func TestEnrollmentClientClassifiesRetryableAndPermanentProblems(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		header    string
		wantRetry bool
		wantCode  string
		body      string
	}{
		{name: "rate limited", status: http.StatusTooManyRequests, header: "2", wantRetry: true, wantCode: "RATE_LIMIT_EXCEEDED"},
		{name: "unavailable", status: http.StatusServiceUnavailable, wantRetry: true, wantCode: "ENROLLMENT_UNAVAILABLE"},
		{name: "invalid token", status: http.StatusUnauthorized, wantCode: "INVALID_BOOTSTRAP_TOKEN"},
		{name: "unknown code is not exposed", status: http.StatusBadRequest, wantCode: "HTTP_400", body: `{"code":"bootstrap-secret"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
				body := test.body
				if body == "" {
					body = `{"code":"` + test.wantCode + `"}`
				}
				response := jsonResponse(test.status, body)
				if test.header != "" {
					response.Header.Set("Retry-After", test.header)
				}
				return response, nil
			})
			_, err := newEnrollmentClient(&http.Client{Transport: transport}).Enroll(
				context.Background(), mustOrigin(t, "https://core.example.test"), enrollment.EnrollmentRequest{},
			)
			if err == nil {
				t.Fatal("Enroll() error = nil, want API error")
			}
			var retry *enrollment.RetryableError
			var permanent *enrollment.PermanentError
			if test.wantRetry {
				if !errors.As(err, &retry) {
					t.Fatalf("error %T is not RetryableError", err)
				}
				if test.header == "2" && retry.RetryAfter != 2*time.Second {
					t.Fatalf("RetryAfter = %s, want 2s", retry.RetryAfter)
				}
			} else if !errors.As(err, &permanent) {
				t.Fatalf("error %T is not PermanentError", err)
			}
			if !strings.Contains(err.Error(), test.wantCode) {
				t.Fatalf("error %q does not include code %q", err, test.wantCode)
			}
			if strings.Contains(err.Error(), "bootstrap-secret") {
				t.Fatalf("error exposed server-supplied secret-like code: %q", err)
			}
		})
	}
}

func jsonResponse(status int, value string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(value)),
	}
}

func mustOrigin(t *testing.T, value string) enrollment.CoreOrigin {
	t.Helper()
	origin, err := enrollment.ParseCoreOrigin(value)
	if err != nil {
		t.Fatal(err)
	}
	return origin
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }
