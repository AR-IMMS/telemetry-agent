package agenthealth

import (
	"strings"
	"testing"
	"time"
)

func TestReporterReturnsLatestEvaluatedSnapshot(t *testing.T) {
	reporter := NewReporter(RuntimeObservation{
		CollectorState:      CollectorStateStarting,
		DesiredGeneration:   4,
		ActivatedGeneration: 4,
		AppliedGeneration:   4,
	})

	initial := reporter.Snapshot()
	if initial.Status != StatusDegraded {
		t.Fatalf(
			"initial status = %q, want %q",
			initial.Status,
			StatusDegraded,
		)
	}

	reporter.Report(RuntimeObservation{
		CollectorState:      CollectorStateReady,
		DesiredGeneration:   5,
		ActivatedGeneration: 5,
		AppliedGeneration:   5,
	})

	got := reporter.Snapshot()

	if got.Status != StatusHealthy {
		t.Fatalf(
			"reported status = %q, want %q",
			got.Status,
			StatusHealthy,
		)
	}
	if got.AppliedGeneration != 5 {
		t.Fatalf(
			"reported applied generation = %d, want 5",
			got.AppliedGeneration,
		)
	}
}

func TestNilReporterSnapshotPreservesLegacyDegradedStatus(t *testing.T) {
	var reporter *Reporter

	snapshot := reporter.Snapshot()
	if snapshot.Status != StatusDegraded {
		t.Fatalf("nil reporter status=%q, want legacy degraded status", snapshot.Status)
	}
	if snapshot.Assessment.State != EvidenceUnknown {
		t.Fatalf("nil reporter assessment=%q, want unknown", snapshot.Assessment.State)
	}
}

func TestReportCopiesValidityDeadline(t *testing.T) {
	reporter := NewReporter(RuntimeObservation{})
	validUntil := time.Now().UTC().Add(time.Hour)
	observation := RuntimeObservation{
		CollectorState:        CollectorStateReady,
		DesiredGeneration:     2,
		ActivatedGeneration:   2,
		AppliedGeneration:     2,
		ConfigurationObserved: true,
		ObservedAt:            time.Now().UTC(),
		ValidUntil:            &validUntil,
	}
	reporter.Report(observation)
	validUntil = time.Now().UTC().Add(-time.Hour)

	collector := componentEvidence(t, reporter.Snapshot().Assessment, ComponentCollector)
	if collector.Freshness != FreshnessFresh {
		t.Fatalf("collector freshness=%q after caller mutation, want fresh copied deadline", collector.Freshness)
	}
}

func TestReporterIncludesOwnerEvidenceAndRefreshesAssessmentSummary(t *testing.T) {
	dependencyState := EvidenceHealthy
	reporter := NewReporterWithEvidenceSources(RuntimeObservation{},
		func(at time.Time) ComponentEvidence {
			return ComponentEvidence{Component: ComponentInventory, State: EvidenceHealthy, Freshness: FreshnessFresh, Completeness: CompletenessComplete, ObservedAt: timePointer(at), ReasonCode: "provider_snapshot_complete"}
		},
		func(at time.Time) ComponentEvidence {
			return ComponentEvidence{Component: ComponentDependencies, State: dependencyState, Freshness: FreshnessFresh, Completeness: CompletenessComplete, ObservedAt: timePointer(at), ReasonCode: "dependencies_checked"}
		},
	)

	snapshot := reporter.Snapshot()
	inventory := componentEvidence(t, snapshot.Assessment, ComponentInventory)
	if inventory.State != EvidenceHealthy || inventory.Completeness != CompletenessComplete {
		t.Fatalf("inventory evidence=%+v, want current complete owner evidence", inventory)
	}
	dependencies := componentEvidence(t, snapshot.Assessment, ComponentDependencies)
	if dependencies.State != EvidenceHealthy || dependencies.Freshness != FreshnessFresh {
		t.Fatalf("dependency evidence=%+v, want current owner evidence", dependencies)
	}

	dependencyState = EvidenceDegraded
	snapshot = reporter.Snapshot()
	if snapshot.Assessment.State != EvidenceDegraded {
		t.Fatalf("overall assessment=%q, want degraded when an owner reports a failure", snapshot.Assessment.State)
	}
}

func TestReporterBoundsAndRedactsOwnerEvidenceReasonCode(t *testing.T) {
	reporter := NewReporterWithEvidenceSources(RuntimeObservation{}, func(at time.Time) ComponentEvidence {
		return ComponentEvidence{
			Component: ComponentInventory, State: EvidenceDegraded,
			Freshness: FreshnessUnknown, Completeness: CompletenessPartial,
			ObservedAt: timePointer(at), ReasonCode: "token=secret-value\n" + strings.Repeat("x", 600),
		}
	})

	evidence := componentEvidence(t, reporter.Snapshot().Assessment, ComponentInventory)
	if strings.Contains(evidence.ReasonCode, "secret-value") || strings.ContainsAny(evidence.ReasonCode, "\r\n") || len(evidence.ReasonCode) > maxDiagnosticTextLength {
		t.Fatalf("owner reason code was not bounded and redacted: %q", evidence.ReasonCode)
	}
}
