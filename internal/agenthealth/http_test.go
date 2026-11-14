package agenthealth

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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
	if snapshot.Assessment.State != EvidenceUnknown {
		t.Fatalf("assessment state = %q, want unknown while external evidence is absent", snapshot.Assessment.State)
	}
	if snapshot.Assessment.GeneratedAt.IsZero() {
		t.Fatal("assessment generatedAt is zero")
	}
	delivery := componentEvidence(t, snapshot.Assessment, ComponentTelemetryDelivery)
	if delivery.State != EvidenceUnknown || delivery.ReasonCode != "backend_receipt_not_measured" {
		t.Fatalf("delivery evidence=%+v, want explicit unknown", delivery)
	}
}

type healthRequestContextKey struct{}

type contextSnapshotTestProvider struct {
	requestContextObserved bool
}

func (p *contextSnapshotTestProvider) Snapshot() Snapshot {
	return Snapshot{}
}

func (p *contextSnapshotTestProvider) SnapshotContext(ctx context.Context) Snapshot {
	p.requestContextObserved = ctx.Value(healthRequestContextKey{}) == "health-request"
	return Snapshot{}
}

func TestStatusHandlerPassesRequestContextToProvider(t *testing.T) {
	provider := &contextSnapshotTestProvider{}
	request := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	request = request.WithContext(context.WithValue(request.Context(), healthRequestContextKey{}, "health-request"))
	response := httptest.NewRecorder()

	NewHTTPHandler(provider).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status response code=%d, want %d", response.Code, http.StatusOK)
	}
	if !provider.requestContextObserved {
		t.Fatal("health provider did not receive the request context")
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

func TestStatusHandlerRedactsSensitiveDiagnosticValues(t *testing.T) {
	reporter := NewReporter(RuntimeObservation{
		CollectorState: CollectorStateFailed,
		LastError:      "connect password=runtime-secret Bearer token-secret\n-----BEGIN PRIVATE KEY-----\nprivate-key-secret\n-----END PRIVATE KEY-----",
	})
	reporter.ReportCollectorOutput(`2026-11-12T10:00:00Z error Error scraping metrics {"otelcol.component.id":"redis/alternate_port","otelcol.signal":"metrics","error":"dial tcp endpoint-secret@127.0.0.1:16379: connect: connection refused"}`)

	request := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	response := httptest.NewRecorder()
	NewHTTPHandler(reporter).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status response code=%d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	for _, secret := range []string{"runtime-secret", "token-secret", "private-key-secret", "endpoint-secret"} {
		if strings.Contains(body, secret) {
			t.Fatalf("status JSON exposed %q: %s", secret, body)
		}
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
