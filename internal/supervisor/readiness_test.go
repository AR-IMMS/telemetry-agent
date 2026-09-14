package supervisor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWaitForReadinessReturnsWhenEndpointIsHealthy(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			writer.WriteHeader(http.StatusOK)
		}),
	)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := waitForReadiness(
		ctx,
		server.Client(),
		server.URL,
		10*time.Millisecond,
	)
	if err != nil {
		t.Fatalf("waitForReadiness() error = %v", err)
	}
}

func TestWaitForReadinessRetriesUntilEndpointBecomesHealthy(t *testing.T) {
	attempts := 0

	server := httptest.NewServer(
		http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			attempts++

			if attempts < 3 {
				writer.WriteHeader(http.StatusServiceUnavailable)
				return
			}

			writer.WriteHeader(http.StatusOK)
		}),
	)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := waitForReadiness(
		ctx,
		server.Client(),
		server.URL,
		10*time.Millisecond,
	)
	if err != nil {
		t.Fatalf("waitForReadiness() error = %v", err)
	}

	if attempts != 3 {
		t.Fatalf("readiness attempts = %d, want 3", attempts)
	}
}
