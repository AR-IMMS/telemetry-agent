package agenthealth

import (
	"strings"
	"testing"
	"time"
)

func TestEvaluateAtKeepsCollectionAndDeliveryUnverifiedWithoutEvidence(t *testing.T) {
	now := time.Date(2026, time.November, 14, 6, 0, 0, 0, time.UTC)
	validUntil := now.Add(time.Minute)
	snapshot := EvaluateAt(RuntimeObservation{
		CollectorState:        CollectorStateReady,
		DesiredGeneration:     4,
		ActivatedGeneration:   4,
		AppliedGeneration:     4,
		ConfigurationObserved: true,
		ObservedAt:            now,
		ValidUntil:            &validUntil,
	}, now)

	collector := componentEvidence(t, snapshot.Assessment, ComponentCollector)
	if collector.State != EvidenceHealthy || collector.Freshness != FreshnessFresh || collector.Completeness != CompletenessComplete {
		t.Fatalf("collector evidence=%+v, want complete current healthy readiness", collector)
	}
	configuration := componentEvidence(t, snapshot.Assessment, ComponentConfiguration)
	if configuration.State != EvidenceHealthy || configuration.Completeness != CompletenessComplete {
		t.Fatalf("configuration evidence=%+v, want complete applied-generation evidence", configuration)
	}
	delivery := componentEvidence(t, snapshot.Assessment, ComponentTelemetryDelivery)
	if delivery.State != EvidenceUnknown || delivery.ReasonCode != "backend_receipt_not_measured" {
		t.Fatalf("delivery evidence=%+v, want unknown without sink receipt", delivery)
	}
	collection := componentEvidence(t, snapshot.Assessment, ComponentTelemetryCollection)
	if collection.State != EvidenceUnknown {
		t.Fatalf("collection evidence=%+v, want unknown without scrape result evidence", collection)
	}
	if snapshot.Assessment.State != EvidenceUnknown {
		t.Fatalf("overall assessment=%q, want unknown while dimensions lack evidence", snapshot.Assessment.State)
	}
}

func TestEvaluateAtMarksEvidenceStaleAtItsValidityBoundary(t *testing.T) {
	now := time.Date(2026, time.November, 14, 6, 0, 0, 0, time.UTC)
	validUntil := now
	snapshot := EvaluateAt(RuntimeObservation{
		CollectorState:        CollectorStateReady,
		DesiredGeneration:     4,
		ActivatedGeneration:   4,
		AppliedGeneration:     4,
		ConfigurationObserved: true,
		ObservedAt:            now.Add(-time.Minute),
		ValidUntil:            &validUntil,
	}, now)

	collector := componentEvidence(t, snapshot.Assessment, ComponentCollector)
	if collector.Freshness != FreshnessStale {
		t.Fatalf("collector freshness=%q, want stale at validUntil boundary", collector.Freshness)
	}
	if snapshot.Assessment.State != EvidenceDegraded {
		t.Fatalf("overall assessment=%q, want degraded for stale readiness evidence", snapshot.Assessment.State)
	}
}

func TestEvaluateAtDoesNotInventFreshnessWhenExpiryIsAbsent(t *testing.T) {
	now := time.Date(2026, time.November, 14, 6, 0, 0, 0, time.UTC)
	snapshot := EvaluateAt(RuntimeObservation{
		CollectorState:        CollectorStateReady,
		DesiredGeneration:     4,
		ActivatedGeneration:   4,
		AppliedGeneration:     4,
		ConfigurationObserved: true,
		ObservedAt:            now,
	}, now)

	collector := componentEvidence(t, snapshot.Assessment, ComponentCollector)
	if collector.Freshness != FreshnessUnknown {
		t.Fatalf("collector freshness=%q, want unknown without owner-provided validity", collector.Freshness)
	}
}

func TestEvaluateAtDoesNotMarkMissingObservationTimeStale(t *testing.T) {
	now := time.Date(2026, time.November, 14, 6, 0, 0, 0, time.UTC)
	validUntil := now.Add(-time.Minute)
	snapshot := EvaluateAt(RuntimeObservation{
		CollectorState:        CollectorStateReady,
		DesiredGeneration:     4,
		ActivatedGeneration:   4,
		AppliedGeneration:     4,
		ConfigurationObserved: true,
		ValidUntil:            &validUntil,
	}, now)

	collector := componentEvidence(t, snapshot.Assessment, ComponentCollector)
	if collector.Freshness != FreshnessUnknown {
		t.Fatalf("collector freshness=%q, want unknown when observation time is missing", collector.Freshness)
	}
}

func TestEvaluateAtMarksTimestampOnlyRuntimeEvidencePartial(t *testing.T) {
	now := time.Date(2026, time.November, 14, 6, 0, 0, 0, time.UTC)
	snapshot := EvaluateAt(RuntimeObservation{ObservedAt: now}, now)

	collector := componentEvidence(t, snapshot.Assessment, ComponentCollector)
	if collector.Completeness != CompletenessPartial || collector.State != EvidenceUnknown {
		t.Fatalf("collector evidence=%+v, want unknown state and partial completeness without a state", collector)
	}
	configuration := componentEvidence(t, snapshot.Assessment, ComponentConfiguration)
	if configuration.Completeness != CompletenessPartial || configuration.State != EvidenceUnknown {
		t.Fatalf("configuration evidence=%+v, want unknown state and partial completeness without observed generations", configuration)
	}
}

func TestEvaluateAtDoesNotInferConfigurationObservationFromGenerationValues(t *testing.T) {
	now := time.Date(2026, time.November, 14, 6, 0, 0, 0, time.UTC)
	snapshot := EvaluateAt(RuntimeObservation{
		DesiredGeneration:   4,
		ActivatedGeneration: 4,
		AppliedGeneration:   4,
		ObservedAt:          now,
	}, now)

	configuration := componentEvidence(t, snapshot.Assessment, ComponentConfiguration)
	if configuration.State != EvidenceUnknown || configuration.Completeness != CompletenessPartial || configuration.ReasonCode != "configuration_observation_missing" {
		t.Fatalf("configuration evidence=%+v, want explicit unknown/partial without owner observation", configuration)
	}
}

func TestEvaluateAtTreatsZeroValidityTimeAsUnknown(t *testing.T) {
	now := time.Date(2026, time.November, 14, 6, 0, 0, 0, time.UTC)
	zero := time.Time{}
	snapshot := EvaluateAt(RuntimeObservation{
		CollectorState:        CollectorStateReady,
		DesiredGeneration:     4,
		ActivatedGeneration:   4,
		AppliedGeneration:     4,
		ConfigurationObserved: true,
		ObservedAt:            now.Add(-time.Minute),
		ValidUntil:            &zero,
	}, now)

	collector := componentEvidence(t, snapshot.Assessment, ComponentCollector)
	if collector.Freshness != FreshnessUnknown {
		t.Fatalf("collector freshness=%q, want unknown for zero validity timestamp", collector.Freshness)
	}
}

func TestEvaluateRedactsSensitiveLastErrorValues(t *testing.T) {
	snapshot := Evaluate(RuntimeObservation{
		CollectorState: CollectorStateFailed,
		LastError:      "connect https://agent:secret-value@example.invalid/path?token=token-value password=hunter2\nsecond forged line\n-----BEGIN RSA PRIVATE KEY-----\nprivate-key-material\n-----END RSA PRIVATE KEY-----",
	})

	for _, secret := range []string{"secret-value", "token-value", "hunter2", "private-key-material"} {
		if strings.Contains(snapshot.LastError, secret) {
			t.Fatalf("LastError exposed %q in %q", secret, snapshot.LastError)
		}
	}
	if len(snapshot.LastError) > maxDiagnosticTextLength {
		t.Fatalf("LastError length=%d, want <= %d", len(snapshot.LastError), maxDiagnosticTextLength)
	}
	if !strings.Contains(snapshot.LastError, "[REDACTED]") || !strings.Contains(snapshot.LastError, "[REDACTED PRIVATE KEY]") {
		t.Fatalf("LastError=%q, want redaction markers", snapshot.LastError)
	}
	if strings.ContainsAny(snapshot.LastError, "\r\n") {
		t.Fatalf("LastError contains a control-character line break: %q", snapshot.LastError)
	}
}

func TestEvaluateMarksRuntimeErrorAsDegradedEvidence(t *testing.T) {
	snapshot := Evaluate(RuntimeObservation{
		CollectorState:        CollectorStateReady,
		DesiredGeneration:     4,
		ActivatedGeneration:   4,
		AppliedGeneration:     4,
		ConfigurationObserved: true,
		LastError:             "runtime failed to persist activation result",
	})

	collector := componentEvidence(t, snapshot.Assessment, ComponentCollector)
	if collector.State != EvidenceDegraded || collector.ReasonCode != "runtime_error_observed" {
		t.Fatalf("collector evidence=%+v, want degraded runtime error", collector)
	}
}

func componentEvidence(t *testing.T, assessment Assessment, component Component) ComponentEvidence {
	t.Helper()
	for _, evidence := range assessment.Components {
		if evidence.Component == component {
			return evidence
		}
	}
	t.Fatalf("assessment lacks component %q", component)
	return ComponentEvidence{}
}
