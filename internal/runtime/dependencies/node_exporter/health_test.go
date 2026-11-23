package nodeexporter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWaitForHealthReturnsWhenEndpointIsHealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := WaitForHealth(
		ctx,
		server.Client(),
		server.URL,
		10*time.Millisecond,
	)
	if err != nil {
		t.Fatalf("WaitForHealth() error = %v, want nil", err)
	}
}

func TestWaitForHealthRetriesUntilEndpointIsHealthy(t *testing.T) {
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			attempts++

			if attempts < 3 {
				writer.WriteHeader(http.StatusServiceUnavailable)
				return
			}

			writer.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := WaitForHealth(
		ctx,
		server.Client(),
		server.URL,
		10*time.Millisecond,
	)
	if err != nil {
		t.Fatalf("WaitForHealth() error = %v, want nil", err)
	}
	if attempts != 3 {
		t.Fatalf("health requests = %d, want 3", attempts)
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
		server.URL,
		10*time.Millisecond,
	)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf(
			"WaitForHealth() error = %v, want context deadline exceeded",
			err,
		)
	}
}
