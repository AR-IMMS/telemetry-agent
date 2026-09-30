package agenthealth

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStatusHandlerReturnsLatestSnapshot(t *testing.T) {
	reporter := NewReporter(RuntimeObservation{
		CollectorState:      CollectorStateReady,
		DesiredGeneration:   6,
		ActivatedGeneration: 6,
		AppliedGeneration:   6,
	})

	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/status",
		nil,
	)
	response := httptest.NewRecorder()

	NewHTTPHandler(reporter).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"status response code = %d, want %d",
			response.Code,
			http.StatusOK,
		)
	}

	var snapshot Snapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode status response: %v", err)
	}

	if snapshot.Status != StatusHealthy {
		t.Fatalf(
			"status response = %q, want %q",
			snapshot.Status,
			StatusHealthy,
		)
	}
	if snapshot.AppliedGeneration != 6 {
		t.Fatalf(
			"applied generation = %d, want 6",
			snapshot.AppliedGeneration,
		)
	}
}

func TestStatusHandlerRejectsNonGetRequests(t *testing.T) {
	reporter := NewReporter(RuntimeObservation{})

	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/status",
		nil,
	)
	response := httptest.NewRecorder()

	NewHTTPHandler(reporter).ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf(
			"status response code = %d, want %d",
			response.Code,
			http.StatusMethodNotAllowed,
		)
	}
}

func TestServeStatusServesSnapshotAndStopsWithContext(
	t *testing.T,
) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	reporter := NewReporter(RuntimeObservation{
		CollectorState:      CollectorStateReady,
		DesiredGeneration:   4,
		ActivatedGeneration: 4,
		AppliedGeneration:   4,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)

	go func() {
		done <- ServeStatus(ctx, listener, reporter)
	}()

	client := &http.Client{Timeout: time.Second}

	response, err := client.Get(
		"http://" + listener.Addr().String() + "/v1/status",
	)
	if err != nil {
		t.Fatalf("GET /v1/status error = %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf(
			"GET /v1/status status = %d, want %d",
			response.StatusCode,
			http.StatusOK,
		)
	}

	var snapshot Snapshot

	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode status response: %v", err)
	}

	if snapshot.Status != StatusHealthy {
		t.Fatalf(
			"status = %q, want %q",
			snapshot.Status,
			StatusHealthy,
		)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ServeStatus() error = %v", err)
		}

	case <-time.After(time.Second):
		t.Fatal("ServeStatus() did not stop after context cancellation")
	}
}
