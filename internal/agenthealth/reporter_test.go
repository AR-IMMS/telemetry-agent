package agenthealth

import (
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
