package agenthealth

import "testing"

func TestEvaluateReportsHealthyForReadyAppliedCollector(t *testing.T) {
	snapshot := Evaluate(RuntimeObservation{
		CollectorState:      CollectorStateReady,
		DesiredGeneration:   4,
		ActivatedGeneration: 4,
		AppliedGeneration:   4,
	})

	if snapshot.Status != StatusHealthy {
		t.Fatalf(
			"health status = %q, want %q",
			snapshot.Status,
			StatusHealthy,
		)
	}
}

func TestEvaluateReportsDegradedWhenCollectorIsNotReady(
	t *testing.T,
) {
	snapshot := Evaluate(RuntimeObservation{
		CollectorState:      CollectorStateStarting,
		DesiredGeneration:   4,
		ActivatedGeneration: 4,
		AppliedGeneration:   4,
	})

	if snapshot.Status != StatusDegraded {
		t.Fatalf(
			"health status = %q, want %q",
			snapshot.Status,
			StatusDegraded,
		)
	}
}

func TestEvaluateReportsDegradedWhenConfigurationIsNotApplied(
	t *testing.T,
) {
	snapshot := Evaluate(RuntimeObservation{
		CollectorState:      CollectorStateReady,
		DesiredGeneration:   5,
		ActivatedGeneration: 5,
		AppliedGeneration:   4,
		PendingTeardowns:    1,
	})

	if snapshot.Status != StatusDegraded {
		t.Fatalf(
			"health status = %q, want %q",
			snapshot.Status,
			StatusDegraded,
		)
	}
}
