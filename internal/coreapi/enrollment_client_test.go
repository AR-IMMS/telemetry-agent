package coreapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
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
		if body.BootstrapToken != "do-not-log" || body.AttemptID != "10000000-0000-4000-8000-000000000001" || body.Installation.ID != "10000000-0000-4000-8000-000000000002" {
			t.Fatalf("request body = %+v", body)
		}
		return jsonResponse(http.StatusOK, `{"data":{
			"enrollment_id":"10000000-0000-4000-8000-000000000003","installation_id":"10000000-0000-4000-8000-000000000002",
			"binding":{"id":"10000000-0000-4000-8000-000000000004","node_id":"10000000-0000-4000-8000-000000000005","status":"active"},
			"credential":{"type":"bearer","access_token":"agent-token"},
			"control_plane":{"base_url":"https://core.example.test/api/v1/agent-control","poll_interval_seconds":30}
		},"meta":{"request_id":"req-1"}}`), nil
	})
	client := newEnrollmentClient(&http.Client{Transport: transport})
	result, err := client.Enroll(context.Background(), mustOrigin(t, "https://core.example.test"), enrollment.EnrollmentRequest{
		BootstrapToken: "do-not-log",
		EnrollmentPayload: enrollment.EnrollmentPayload{
			AttemptID: "10000000-0000-4000-8000-000000000001", Installation: enrollment.Installation{ID: "10000000-0000-4000-8000-000000000002"},
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

func TestEnrollmentClientRejectsSuccessWithoutRequiredResponseEnvelope(t *testing.T) {
	for _, body := range []string{
		`{"data":{}}`,
		`{"data":{},"meta":{}}`,
		`{"meta":{"request_id":"req-1"}}`,
	} {
		t.Run(body, func(t *testing.T) {
			transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusCreated, body), nil
			})
			_, err := newEnrollmentClient(&http.Client{Transport: transport}).Enroll(
				context.Background(), mustOrigin(t, "https://core.example.test"), enrollment.EnrollmentRequest{},
			)
			if err == nil {
				t.Fatal("Enroll() accepted malformed success envelope")
			}
		})
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

func TestEnrollmentClientMarksPlainHTTPResponseAsRequestNotSent(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, http.ErrSchemeMismatch
	})
	_, err := newEnrollmentClient(&http.Client{Transport: transport}).Enroll(
		context.Background(), mustOrigin(t, "https://localhost:8081"), enrollment.EnrollmentRequest{},
	)
	var notSent *enrollment.RequestNotSentError
	if !errors.As(err, &notSent) || notSent.Code != "CORE_TLS_REQUIRED" {
		t.Fatalf("Enroll() error=%v, want request-not-sent TLS classification", err)
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
		{name: "rate limited", status: http.StatusTooManyRequests, header: "2", wantRetry: true, wantCode: "TOO_MANY_REQUESTS"},
		{name: "gateway failure has uncertain outcome", status: http.StatusBadGateway, wantRetry: true, wantCode: "HTTP_502"},
		{name: "unavailable", status: http.StatusServiceUnavailable, wantRetry: true, wantCode: "ENROLLMENT_UNAVAILABLE"},
		{name: "undocumented server error is not retried", status: http.StatusInternalServerError, wantCode: "HTTP_500"},
		{name: "invalid request", status: http.StatusBadRequest, wantCode: "VALIDATION_ERROR"},
		{name: "invalid token", status: http.StatusUnauthorized, wantCode: "INVALID_BOOTSTRAP_TOKEN"},
		{name: "forbidden", status: http.StatusForbidden, wantCode: "FORBIDDEN"},
		{name: "not found", status: http.StatusNotFound, wantCode: "NOT_FOUND"},
		{name: "attempt conflict", status: http.StatusConflict, wantCode: "ENROLLMENT_ATTEMPT_CONFLICT"},
		{name: "recovery expired", status: http.StatusGone, wantCode: "ENROLLMENT_RECOVERY_EXPIRED"},
		{name: "recovery revoked", status: http.StatusGone, wantCode: "ENROLLMENT_RECOVERY_REVOKED"},
		{name: "unknown code is not exposed", status: http.StatusBadRequest, wantCode: "HTTP_400", body: `{"code":"bootstrap-secret"}`},
		{name: "mismatched problem status is not trusted", status: http.StatusBadRequest, wantCode: "HTTP_400", body: `{"status":401,"code":"INVALID_BOOTSTRAP_TOKEN"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
				body := test.body
				if body == "" {
					body = `{"status":` + strconv.Itoa(test.status) + `,"code":"` + test.wantCode + `","detail":"secret-detail"}`
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
			if strings.Contains(err.Error(), "secret-detail") {
				t.Fatalf("error exposed untrusted Problem Details text: %q", err)
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
