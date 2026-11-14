package agenthealth

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"sync"
	"time"
)

const maxCollectorFailures = 32

// CollectorOutputReporter accepts one complete Collector output line for safe diagnostic extraction.
type CollectorOutputReporter interface {
	ReportCollectorOutput(string)
}

// Provider exposes the latest local Agent health snapshot.
type Provider interface {
	Snapshot() Snapshot
}

// ContextProvider generates the local health snapshot with the request context.
type ContextProvider interface {
	// SnapshotContext creates a health snapshot using caller cancellation and request-scoped values.
	SnapshotContext(context.Context) Snapshot
}

// ObservationReporter records transient Collector runtime observations.
type ObservationReporter interface {
	Report(RuntimeObservation)
}

// EvidenceSource reads one owner-provided component observation at snapshot time.
// Sources must be read-only and bound their work; the context supports request-scoped cancellation.
type EvidenceSource func(context.Context, time.Time) ComponentEvidence

// Reporter stores the latest evaluated health snapshot for local runtime
// consumers such as CLI, HTTP, and future heartbeat adapters.
type Reporter struct {
	mu              sync.RWMutex
	observation     RuntimeObservation
	snapshot        Snapshot
	evidenceSources []EvidenceSource
}

// NewReporter creates a reporter with an initial runtime observation.
func NewReporter(observation RuntimeObservation) *Reporter {
	observation = cloneRuntimeObservation(observation)
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = time.Now().UTC()
	}
	observation.LastError = sanitizeError(observation.LastError)
	return &Reporter{
		observation: observation,
		snapshot:    Evaluate(observation),
	}
}

// NewReporterWithEvidenceSources creates a Reporter that adds read-only owner
// evidence to each snapshot without changing runtime observation ownership.
func NewReporterWithEvidenceSources(observation RuntimeObservation, sources ...EvidenceSource) *Reporter {
	reporter := NewReporter(observation)
	reporter.evidenceSources = append([]EvidenceSource(nil), sources...)
	return reporter
}

// Report evaluates and publishes one new runtime observation.
func (r *Reporter) Report(observation RuntimeObservation) {
	if r == nil {
		return
	}
	observation = cloneRuntimeObservation(observation)
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = time.Now().UTC()
	}
	observation.LastError = sanitizeError(observation.LastError)

	snapshot := Evaluate(observation)

	r.mu.Lock()
	r.observation = observation
	snapshot.CollectorFailures = append([]CollectorFailure(nil), r.snapshot.CollectorFailures...)
	r.snapshot = snapshot
	r.mu.Unlock()
}

func cloneRuntimeObservation(observation RuntimeObservation) RuntimeObservation {
	if observation.ValidUntil != nil {
		validUntil := *observation.ValidUntil
		observation.ValidUntil = &validUntil
	}
	return observation
}

// Snapshot returns the latest published health snapshot.
func (r *Reporter) Snapshot() Snapshot {
	return r.SnapshotContext(context.Background())
}

// SnapshotContext returns the current snapshot while propagating caller cancellation to evidence sources.
func (r *Reporter) SnapshotContext(ctx context.Context) Snapshot {
	if r == nil {
		snapshot := EvaluateAt(RuntimeObservation{}, time.Now().UTC())
		snapshot.Status = StatusDegraded
		for index := range snapshot.Assessment.Components {
			if snapshot.Assessment.Components[index].Component == ComponentAgentRuntime {
				snapshot.Assessment.Components[index] = unknownEvidence(ComponentAgentRuntime, "health_reporter_unavailable")
			}
		}
		snapshot.Assessment.State = EvidenceUnknown
		return snapshot
	}
	if ctx == nil {
		ctx = context.Background()
	}

	r.mu.RLock()
	observation := r.observation
	failures := append([]CollectorFailure(nil), r.snapshot.CollectorFailures...)
	sources := append([]EvidenceSource(nil), r.evidenceSources...)
	r.mu.RUnlock()

	now := time.Now().UTC()
	snapshot := EvaluateAt(observation, now)
	snapshot.CollectorFailures = failures
	applyCollectorFailureEvidence(&snapshot)
	for _, source := range sources {
		if source == nil {
			continue
		}
		upsertComponentEvidence(&snapshot.Assessment, source(ctx, now))
	}
	refreshAssessmentState(now, &snapshot.Assessment)

	return snapshot
}

func upsertComponentEvidence(assessment *Assessment, evidence ComponentEvidence) {
	if assessment == nil || evidence.Component == "" {
		return
	}
	evidence.ReasonCode = sanitizeError(evidence.ReasonCode)
	evidence.ObservedAt = timePointerFrom(evidence.ObservedAt)
	evidence.ValidUntil = timePointerFrom(evidence.ValidUntil)
	for index := range assessment.Components {
		if assessment.Components[index].Component == evidence.Component {
			assessment.Components[index] = evidence
			return
		}
	}
	assessment.Components = append(assessment.Components, evidence)
}

func applyCollectorFailureEvidence(snapshot *Snapshot) {
	if snapshot == nil || len(snapshot.CollectorFailures) == 0 {
		return
	}
	for index := range snapshot.Assessment.Components {
		component := &snapshot.Assessment.Components[index]
		switch component.Component {
		case ComponentMonitoring:
		case ComponentTelemetryCollection:
		default:
			continue
		}
		if hasCollectorFailure(snapshot.CollectorFailures, CollectorFailureReceiver) {
			component.State = EvidenceDegraded
			component.Freshness = FreshnessUnknown
			component.Completeness = CompletenessPartial
			if component.Component == ComponentMonitoring {
				component.ReasonCode = "monitoring_receiver_failure_observed"
			} else {
				component.ReasonCode = "collector_scrape_failure_observed"
			}
			snapshot.Assessment.State = EvidenceDegraded
		}
	}
	if hasCollectorFailure(snapshot.CollectorFailures, CollectorFailureExporter) {
		for index := range snapshot.Assessment.Components {
			component := &snapshot.Assessment.Components[index]
			switch component.Component {
			case ComponentGatewayConnectivity:
				component.State = EvidenceDegraded
				component.Freshness = FreshnessUnknown
				component.Completeness = CompletenessPartial
				component.ReasonCode = "collector_exporter_network_failure_observed"
			case ComponentTelemetryDelivery:
				component.State = EvidenceDegraded
				component.Freshness = FreshnessUnknown
				component.Completeness = CompletenessPartial
				component.ReasonCode = "collector_export_failure_observed"
			default:
				continue
			}
			snapshot.Assessment.State = EvidenceDegraded
		}
	}
}

func hasCollectorFailure(failures []CollectorFailure, direction CollectorFailureDirection) bool {
	for _, failure := range failures {
		if failure.Direction == direction || (direction == CollectorFailureReceiver && failure.Direction == "") {
			return true
		}
	}
	return false
}

// ReportCollectorOutput extracts bounded network scrape evidence without retaining raw Collector logs.
func (r *Reporter) ReportCollectorOutput(line string) {
	if r == nil {
		return
	}
	failure, ok := collectorNetworkFailure(line)
	if !ok {
		return
	}
	r.mu.Lock()
	r.snapshot.CollectorFailures = append(r.snapshot.CollectorFailures, failure)
	if len(r.snapshot.CollectorFailures) > maxCollectorFailures {
		r.snapshot.CollectorFailures = append([]CollectorFailure(nil), r.snapshot.CollectorFailures[len(r.snapshot.CollectorFailures)-maxCollectorFailures:]...)
	}
	r.mu.Unlock()
}

func collectorNetworkFailure(line string) (CollectorFailure, bool) {
	start := strings.IndexByte(line, '{')
	if start < 0 {
		return CollectorFailure{}, false
	}
	fields := make(map[string]any)
	if json.Unmarshal([]byte(line[start:]), &fields) != nil {
		return CollectorFailure{}, false
	}
	component, _ := fields["otelcol.component.id"].(string)
	signal, _ := fields["otelcol.signal"].(string)
	message, _ := fields["error"].(string)
	if message == "" || !strings.Contains(strings.ToLower(message), "dial tcp ") {
		return CollectorFailure{}, false
	}
	kind, _ := fields["kind"].(string)
	if kind == "" {
		kind, _ = fields["otelcol.component.kind"].(string)
	}
	if kind == "exporter" {
		name, _ := fields["name"].(string)
		if name == "" {
			name = component
		}
		if strings.TrimSpace(name) == "" {
			return CollectorFailure{}, false
		}
		endpoint, ok := collectorNetworkEndpoint(message)
		if !ok {
			return CollectorFailure{}, false
		}
		observedAt := collectorLogTimestamp(line)
		return CollectorFailure{
			Direction: CollectorFailureExporter, Exporter: boundDiagnosticText(sanitizeError(name)),
			Endpoint: boundDiagnosticText(endpoint), Code: "connection_failed",
			ObservedAt: observedAt.Format(time.RFC3339Nano),
		}, true
	}
	if signal != "metrics" {
		return CollectorFailure{}, false
	}
	receiver := component
	if component == "receiver_creator" {
		name, _ := fields["name"].(string)
		const marker = "/receiver_creator{"
		markerAt := strings.Index(name, marker)
		if markerAt <= 0 {
			return CollectorFailure{}, false
		}
		receiver = name[:markerAt]
	}
	if !strings.Contains(receiver, "/") {
		return CollectorFailure{}, false
	}
	endpoint, ok := collectorNetworkEndpoint(message)
	if !ok {
		return CollectorFailure{}, false
	}
	observedAt := collectorLogTimestamp(line)
	return CollectorFailure{
		Direction: CollectorFailureReceiver, Receiver: boundDiagnosticText(sanitizeError(receiver)),
		Endpoint: boundDiagnosticText(endpoint), Code: "connection_failed",
		ObservedAt: observedAt.Format(time.RFC3339Nano),
	}, true
}

func collectorNetworkEndpoint(message string) (string, bool) {
	start := strings.Index(strings.ToLower(message), "dial tcp ")
	if start < 0 {
		return "", false
	}
	endpoint := message[start+len("dial tcp "):]
	end := strings.Index(endpoint, ": ")
	if end < 0 {
		return "", false
	}
	host, port, err := net.SplitHostPort(endpoint[:end])
	if err != nil {
		return "", false
	}
	if separator := strings.LastIndexByte(host, '@'); separator >= 0 {
		host = host[separator+1:]
	}
	if host == "" || strings.ContainsAny(host, "/?#@") {
		return "", false
	}
	return net.JoinHostPort(host, port), true
}

func collectorLogTimestamp(line string) time.Time {
	if fields := strings.Fields(line); len(fields) > 0 {
		if parsed, err := time.Parse(time.RFC3339Nano, fields[0]); err == nil {
			return parsed.UTC()
		}
	}
	return time.Now().UTC()
}
