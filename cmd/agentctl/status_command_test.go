package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
)

func TestRunStatusSeparatesRuntimeReadinessFromDelivery(t *testing.T) {
	deps := dependencies{fetchHealthStatus: func(context.Context, string) (agenthealth.Snapshot, error) {
		return agenthealth.Evaluate(agenthealth.RuntimeObservation{
			CollectorState:      agenthealth.CollectorStateReady,
			DesiredGeneration:   4,
			ActivatedGeneration: 4,
			AppliedGeneration:   4,
		}), nil
	}}
	var stdout, stderr bytes.Buffer

	code := runStatus(context.Background(), nil, &stdout, &stderr, deps)
	if code != 0 {
		t.Fatalf("runStatus() = %d, want 0; stderr=%s", code, stderr.String())
	}
	for _, want := range []string{
		"Agent health: healthy",
		"Collector state: ready",
		"Configuration generations: desired=4 activated=4 applied=4",
		"Pending dependency teardowns: 0",
		"Status scope: Collector/configuration runtime readiness (legacy projection)",
		"agent_runtime: healthy (freshness=fresh)",
		"Evidence assessment: unknown",
		"collector: healthy (freshness=unknown)",
		"telemetry_delivery: unknown (freshness=unknown)",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("status output %q lacks %q", stdout.String(), want)
		}
	}
	if strings.Contains(stdout.String(), "telemetry_delivery: healthy") {
		t.Fatalf("status inferred delivery success from readiness: %s", stdout.String())
	}
}

func TestRunDoctorExplainsUnknownAndStaleEvidence(t *testing.T) {
	now := time.Date(2026, time.November, 14, 6, 0, 0, 0, time.UTC)
	validUntil := now.Add(-time.Second)
	snapshot := agenthealth.EvaluateAt(agenthealth.RuntimeObservation{
		CollectorState:        agenthealth.CollectorStateReady,
		DesiredGeneration:     4,
		ActivatedGeneration:   4,
		AppliedGeneration:     4,
		ConfigurationObserved: true,
		ObservedAt:            now.Add(-time.Minute),
		ValidUntil:            &validUntil,
	}, now)
	deps := dependencies{fetchHealthStatus: func(context.Context, string) (agenthealth.Snapshot, error) { return snapshot, nil }}
	var stdout, stderr bytes.Buffer

	code := runDoctor(context.Background(), nil, &stdout, &stderr, deps)
	if code != 0 {
		t.Fatalf("runDoctor() = %d, want 0; stderr=%s", code, stderr.String())
	}
	for _, want := range []string{
		"Component evidence:",
		"collector: state=healthy freshness=stale completeness=complete",
		"core_connectivity: state=unknown freshness=unknown completeness=unknown reason=core_connectivity_evidence_unavailable",
		"telemetry_delivery: state=unknown freshness=unknown completeness=unknown reason=backend_receipt_not_measured",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("doctor output %q lacks %q", stdout.String(), want)
		}
	}
}

func TestRunDoctorDoesNotPrintEndpointCredentialWhenStatusIsUnavailable(t *testing.T) {
	deps := dependencies{fetchHealthStatus: func(context.Context, string) (agenthealth.Snapshot, error) {
		return agenthealth.Snapshot{}, context.DeadlineExceeded
	}}
	var stdout, stderr bytes.Buffer

	code := runDoctor(context.Background(), []string{"--endpoint", "http://user:secret-value@127.0.0.1:1/status"}, &stdout, &stderr, deps)
	if code != 1 {
		t.Fatalf("runDoctor() = %d, want 1; stderr=%s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "secret-value") {
		t.Fatalf("doctor error exposed endpoint credentials: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "(timeout)") {
		t.Fatalf("doctor error lacks safe timeout category: %s", stderr.String())
	}
}

func TestRunStatusReportsSafeConnectionFailureCategory(t *testing.T) {
	deps := dependencies{fetchHealthStatus: func(context.Context, string) (agenthealth.Snapshot, error) {
		return agenthealth.Snapshot{}, fmt.Errorf("request failed: %w", syscall.ECONNREFUSED)
	}}
	var stdout, stderr bytes.Buffer

	code := runStatus(context.Background(), nil, &stdout, &stderr, deps)
	if code != 1 {
		t.Fatalf("runStatus() = %d, want 1", code)
	}
	if want := "(connection_refused)"; !strings.Contains(stderr.String(), want) {
		t.Fatalf("status error=%q, want safe category %q", stderr.String(), want)
	}
}

func TestRunDispatchesDoctorCommandAndListsItInHelp(t *testing.T) {
	deps := dependencies{fetchHealthStatus: func(context.Context, string) (agenthealth.Snapshot, error) {
		return agenthealth.Evaluate(agenthealth.RuntimeObservation{}), nil
	}}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"doctor"}, &stdout, &stderr, deps); code != 0 {
		t.Fatalf("run(doctor) = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Component evidence:") {
		t.Fatalf("doctor dispatch output=%q, want detailed evidence", stdout.String())
	}

	var help bytes.Buffer
	writeRootHelp(&help)
	if !strings.Contains(help.String(), "doctor      Explain available health evidence") {
		t.Fatalf("root help does not list doctor: %s", help.String())
	}
}
