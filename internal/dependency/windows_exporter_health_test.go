package dependency

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWaitForWindowsExporterHealthReturnsWhenEndpointIsHealthy(
	t *testing.T,
) {
	server := httptest.NewServer(
		http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			if request.URL.Path != "/health" {
				http.NotFound(writer, request)
				return
			}

			writer.WriteHeader(http.StatusOK)
		}),
	)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := waitForWindowsExporterHealth(
		ctx,
		server.Client(),
		server.URL+"/health",
		time.Millisecond,
	)
	if err != nil {
		t.Fatalf("waitForWindowsExporterHealth() error = %v", err)
	}
}

func TestWaitForWindowsExporterHealthRetriesUntilHealthy(
	t *testing.T,
) {
	attempts := 0

	server := httptest.NewServer(
		http.HandlerFunc(func(
			writer http.ResponseWriter,
			_ *http.Request,
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

	err := waitForWindowsExporterHealth(
		ctx,
		server.Client(),
		server.URL+"/health",
		time.Millisecond,
	)
	if err != nil {
		t.Fatalf("waitForWindowsExporterHealth() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("health attempts = %d, want 3", attempts)
	}
}

func TestWaitForWindowsExporterHealthReturnsContextDeadline(
	t *testing.T,
) {
	server := httptest.NewServer(
		http.HandlerFunc(func(
			writer http.ResponseWriter,
			_ *http.Request,
		) {
			writer.WriteHeader(http.StatusServiceUnavailable)
		}),
	)
	defer server.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		50*time.Millisecond,
	)
	defer cancel()

	err := waitForWindowsExporterHealth(
		ctx,
		server.Client(),
		server.URL+"/health",
		time.Millisecond,
	)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf(
			"waitForWindowsExporterHealth() error = %v, want context deadline exceeded",
			err,
		)
	}
}
