package agenthealth

import (
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

// ObservationReporter records transient Collector runtime observations.
type ObservationReporter interface {
	Report(RuntimeObservation)
}

// Reporter stores the latest evaluated health snapshot for local runtime
// consumers such as CLI, HTTP, and future heartbeat adapters.
type Reporter struct {
	mu          sync.RWMutex
	observation RuntimeObservation
	snapshot    Snapshot
}

// NewReporter creates a reporter with an initial runtime observation.
func NewReporter(observation RuntimeObservation) *Reporter {
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = time.Now().UTC()
	}
	observation.LastError = sanitizeError(observation.LastError)
	return &Reporter{
		observation: observation,
		snapshot:    Evaluate(observation),
	}
}

// Report evaluates and publishes one new runtime observation.
func (r *Reporter) Report(observation RuntimeObservation) {
	if r == nil {
		return
	}
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

// Snapshot returns the latest published health snapshot.
func (r *Reporter) Snapshot() Snapshot {
	if r == nil {
		snapshot := EvaluateAt(RuntimeObservation{}, time.Now().UTC())
		for index := range snapshot.Assessment.Components {
			if snapshot.Assessment.Components[index].Component == ComponentAgentRuntime {
				snapshot.Assessment.Components[index] = unknownEvidence(ComponentAgentRuntime, "health_reporter_unavailable")
			}
		}
		snapshot.Assessment.State = EvidenceUnknown
		return snapshot
	}

	r.mu.RLock()
	observation := r.observation
	snapshot := EvaluateAt(observation, time.Now().UTC())
	snapshot.CollectorFailures = append([]CollectorFailure(nil), r.snapshot.CollectorFailures...)
	applyCollectorFailureEvidence(&snapshot)
	r.mu.RUnlock()

	return snapshot
}

func applyCollectorFailureEvidence(snapshot *Snapshot) {
	if snapshot == nil || len(snapshot.CollectorFailures) == 0 {
		return
	}
	for index := range snapshot.Assessment.Components {
		component := &snapshot.Assessment.Components[index]
		if component.Component != ComponentTelemetryCollection {
			continue
		}
		component.State = EvidenceDegraded
		component.Freshness = FreshnessUnknown
		component.Completeness = CompletenessPartial
		component.ReasonCode = "collector_scrape_failure_observed"
		if snapshot.Assessment.State != EvidenceDegraded {
			snapshot.Assessment.State = EvidenceDegraded
		}
		return
	}
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
	if signal != "metrics" || !strings.Contains(strings.ToLower(message), "dial tcp ") {
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
	endpoint := message[strings.Index(strings.ToLower(message), "dial tcp ")+len("dial tcp "):]
	end := strings.Index(endpoint, ": ")
	if end < 0 {
		return CollectorFailure{}, false
	}
	host, port, err := net.SplitHostPort(endpoint[:end])
	if err != nil {
		return CollectorFailure{}, false
	}
	if separator := strings.LastIndexByte(host, '@'); separator >= 0 {
		host = host[separator+1:]
	}
	if host == "" || strings.ContainsAny(host, "/?#@") {
		return CollectorFailure{}, false
	}
	endpointText := net.JoinHostPort(host, port)
	observedAt := time.Now().UTC()
	if fields := strings.Fields(line); len(fields) > 0 {
		if parsed, err := time.Parse(time.RFC3339Nano, fields[0]); err == nil {
			observedAt = parsed.UTC()
		}
	}
	return CollectorFailure{Receiver: boundDiagnosticText(sanitizeError(receiver)), Endpoint: boundDiagnosticText(endpointText), Code: "connection_failed", ObservedAt: observedAt.Format(time.RFC3339Nano)}, true
}
