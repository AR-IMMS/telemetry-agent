package supervisor

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestWaitForReadinessReturnsWhenEndpointIsHealthy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client := readinessClient(func(int) *http.Response { return readinessResponse(http.StatusOK, "") })
	if err := waitForReadiness(ctx, client, "http://127.0.0.1:13133", time.Millisecond, ""); err != nil {
		t.Fatalf("waitForReadiness() error = %v", err)
	}
}

func TestWaitForReadinessRetriesUntilEndpointBecomesHealthy(t *testing.T) {
	attempts := 0
	client := readinessClient(func(int) *http.Response {
		attempts++
		if attempts < 3 {
			return readinessResponse(http.StatusServiceUnavailable, "")
		}
		return readinessResponse(http.StatusOK, "")
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := waitForReadiness(ctx, client, "http://127.0.0.1:13133", time.Millisecond, ""); err != nil {
		t.Fatalf("waitForReadiness() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("readiness attempts = %d, want 3", attempts)
	}
}

func TestWaitForReadinessRejectsAnotherGenerationBody(t *testing.T) {
	responses := []string{"old-generation", "candidate-generation"}
	attempt := 0
	client := readinessClient(func(int) *http.Response {
		body := responses[attempt]
		attempt++
		return readinessResponse(http.StatusOK, body)
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := waitForReadiness(ctx, client, "http://127.0.0.1:13133", time.Millisecond, "candidate-generation"); err != nil {
		t.Fatalf("waitForReadiness() error = %v", err)
	}
	if attempt != 2 {
		t.Fatalf("readiness attempts = %d, want 2", attempt)
	}
}

func TestWaitForReadinessDoesNotAcceptUnrelatedHTTP200(t *testing.T) {
	client := readinessClient(func(int) *http.Response { return readinessResponse(http.StatusOK, "unrelated") })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := waitForReadiness(ctx, client, "http://127.0.0.1:13133", time.Millisecond, "candidate-token")
	if err == nil || !strings.Contains(err.Error(), "response body did not match") {
		t.Fatalf("waitForReadiness() error = %v, want body mismatch timeout", err)
	}
}

func TestWaitForReadinessRequiresExactGenerationBody(t *testing.T) {
	client := readinessClient(func(int) *http.Response {
		return readinessResponse(http.StatusOK, "candidate-token\n")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := waitForReadiness(ctx, client, "http://127.0.0.1:13133", time.Millisecond, "candidate-token")
	if err == nil || !strings.Contains(err.Error(), "response body did not match") {
		t.Fatalf("waitForReadiness() error = %v, want exact body mismatch", err)
	}
}

type readinessRoundTripper func(*http.Request) (*http.Response, error)

func (f readinessRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func readinessClient(response func(int) *http.Response) *http.Client {
	attempt := 0
	return &http.Client{Transport: readinessRoundTripper(func(*http.Request) (*http.Response, error) {
		current := attempt
		attempt++
		return response(current), nil
	})}
}

func readinessResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
