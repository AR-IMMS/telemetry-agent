package agenthealth

import "testing"

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
