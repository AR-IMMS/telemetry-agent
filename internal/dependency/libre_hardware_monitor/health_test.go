package librehardwaremonitor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWaitForHealthReturnsWhenMetricsEndpointIsHealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/metrics" {
				t.Fatalf(
					"request path = %q, want /metrics",
					request.URL.Path,
				)
			}

			writer.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()

	err := WaitForHealth(
		context.Background(),
		server.Client(),
		server.URL+"/metrics",
		10*time.Millisecond,
	)
	if err != nil {
		t.Fatalf("WaitForHealth() error = %v", err)
	}
}

func TestWaitForHealthRetriesUntilMetricsEndpointIsHealthy(t *testing.T) {
	requests := 0

	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			requests++

			if requests == 1 {
				writer.WriteHeader(http.StatusServiceUnavailable)
				return
			}

			writer.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()

	err := WaitForHealth(
		context.Background(),
		server.Client(),
		server.URL+"/metrics",
		10*time.Millisecond,
	)
	if err != nil {
		t.Fatalf("WaitForHealth() error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestWaitForHealthReturnsContextDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusServiceUnavailable)
		},
	))
	defer server.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		50*time.Millisecond,
	)
	defer cancel()

	err := WaitForHealth(
		ctx,
		server.Client(),
		server.URL+"/metrics",
		10*time.Millisecond,
	)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf(
			"WaitForHealth() error = %v, want context deadline exceeded",
			err,
		)
	}
}
