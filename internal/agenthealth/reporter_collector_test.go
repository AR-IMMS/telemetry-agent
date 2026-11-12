package agenthealth

import "testing"

func TestReporterRetainsOnlySanitizedCollectorNetworkFailures(t *testing.T) {
	reporter := NewReporter(RuntimeObservation{})
	reporter.ReportCollectorOutput(`2026-11-12T10:00:00Z error Error scraping metrics {"otelcol.component.id":"receiver_creator","name":"redis/alternate_port/receiver_creator{endpoint_id=abc}","otelcol.signal":"metrics","error":"dial tcp 127.0.0.1:16379: connect: connection refused"}`)
	reporter.ReportCollectorOutput(`2026-11-12T10:00:00Z info starting receiver {"name":"redis/alternate_port","endpoint":"127.0.0.1:16379"}`)
	reporter.Report(RuntimeObservation{CollectorState: CollectorStateReady})

	snapshot := reporter.Snapshot()
	if len(snapshot.CollectorFailures) != 1 {
		t.Fatalf("collector failures=%+v, want one sanitized network failure", snapshot.CollectorFailures)
	}
	failure := snapshot.CollectorFailures[0]
	if failure.Receiver != "redis/alternate_port" || failure.Endpoint != "127.0.0.1:16379" || failure.Code != "connection_failed" || failure.ObservedAt != "2026-11-12T10:00:00Z" {
		t.Fatalf("collector failure=%+v, want receiver, endpoint, and normalized code only", failure)
	}
}

func TestReporterBoundsCollectorFailures(t *testing.T) {
	reporter := NewReporter(RuntimeObservation{})
	for i := 0; i < maxCollectorFailures+3; i++ {
		reporter.ReportCollectorOutput(`error Error scraping metrics {"otelcol.component.id":"redis/endpoint","otelcol.signal":"metrics","error":"dial tcp 127.0.0.1:6379: connect: connection refused"}`)
	}
	if got := len(reporter.Snapshot().CollectorFailures); got != maxCollectorFailures {
		t.Fatalf("collector failures retained=%d, want bounded at %d", got, maxCollectorFailures)
	}
}
