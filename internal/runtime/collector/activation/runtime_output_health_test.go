package agentlifecycle

import (
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
)

func TestRuntimeWatcherReportsCollectorExporterNetworkFailure(t *testing.T) {
	reporter := agenthealth.NewReporter(agenthealth.RuntimeObservation{
		CollectorState:      agenthealth.CollectorStateReady,
		DesiredGeneration:   4,
		ActivatedGeneration: 4,
		AppliedGeneration:   4,
	})
	options := (runtimeWatcher{healthReporter: reporter}).launchOptions()
	if options.OnOutput == nil {
		t.Fatal("Collector output callback is nil, want health reporter adapter")
	}
	options.OnOutput(`2026-11-14T10:00:00Z error Exporting failed. Dropping data. {"kind":"exporter","data_type":"metrics","name":"otlp/default","error":"rpc unavailable: dial tcp 127.0.0.1:4317: connect: connection refused","dropped_items":3}`)

	snapshot := reporter.Snapshot()
	gateway := healthComponentEvidence(t, snapshot, agenthealth.ComponentGatewayConnectivity)
	if gateway.State != agenthealth.EvidenceDegraded || gateway.ReasonCode != "collector_exporter_network_failure_observed" {
		t.Fatalf("Gateway evidence=%+v, want exporter network failure", gateway)
	}
	delivery := healthComponentEvidence(t, snapshot, agenthealth.ComponentTelemetryDelivery)
	if delivery.State != agenthealth.EvidenceDegraded || delivery.ReasonCode != "collector_export_failure_observed" {
		t.Fatalf("delivery evidence=%+v, want failed export without receipt claim", delivery)
	}
	collector := healthComponentEvidence(t, snapshot, agenthealth.ComponentCollector)
	if collector.State != agenthealth.EvidenceHealthy {
		t.Fatalf("Collector evidence=%+v, want healthy process separate from exporter failure", collector)
	}
}

func healthComponentEvidence(t *testing.T, snapshot agenthealth.Snapshot, component agenthealth.Component) agenthealth.ComponentEvidence {
	t.Helper()
	for _, evidence := range snapshot.Assessment.Components {
		if evidence.Component == component {
			return evidence
		}
	}
	t.Fatalf("component %q not present in assessment", component)
	return agenthealth.ComponentEvidence{}
}
