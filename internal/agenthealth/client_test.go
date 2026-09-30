package agenthealth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchStatusDecodesLiveSnapshot(
	t *testing.T,
) {
	reporter := NewReporter(RuntimeObservation{
		CollectorState:      CollectorStateReady,
		DesiredGeneration:   5,
		ActivatedGeneration: 5,
		AppliedGeneration:   5,
	})

	server := httptest.NewServer(NewHTTPHandler(reporter))
	defer server.Close()

	snapshot, err := FetchStatus(
		context.Background(),
		&http.Client{Timeout: time.Second},
		server.URL+"/v1/status",
	)
	if err != nil {
		t.Fatalf("FetchStatus() error = %v", err)
	}

	if snapshot.Status != StatusHealthy {
		t.Fatalf(
			"status = %q, want %q",
			snapshot.Status,
			StatusHealthy,
		)
	}
	if snapshot.CollectorState != CollectorStateReady {
		t.Fatalf(
			"Collector state = %q, want %q",
			snapshot.CollectorState,
			CollectorStateReady,
		)
	}
	if snapshot.AppliedGeneration != 5 {
		t.Fatalf(
			"applied generation = %d, want 5",
			snapshot.AppliedGeneration,
		)
	}
}
