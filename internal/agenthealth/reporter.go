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
	mu       sync.RWMutex
	snapshot Snapshot
}

// NewReporter creates a reporter with an initial runtime observation.
func NewReporter(observation RuntimeObservation) *Reporter {
	return &Reporter{
		snapshot: Evaluate(observation),
	}
}

// Report evaluates and publishes one new runtime observation.
func (r *Reporter) Report(observation RuntimeObservation) {
	if r == nil {
		return
	}

	snapshot := Evaluate(observation)

	r.mu.Lock()
	snapshot.CollectorFailures = append([]CollectorFailure(nil), r.snapshot.CollectorFailures...)
	r.snapshot = snapshot
	r.mu.Unlock()
}

// Snapshot returns the latest published health snapshot.
func (r *Reporter) Snapshot() Snapshot {
	if r == nil {
		return Snapshot{
			Status: StatusDegraded,
		}
	}

	r.mu.RLock()
	snapshot := r.snapshot
	snapshot.CollectorFailures = append([]CollectorFailure(nil), r.snapshot.CollectorFailures...)
	r.mu.RUnlock()

	return snapshot
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
	if end < 0 || netSplitHostPort(endpoint[:end]) == false {
		return CollectorFailure{}, false
	}
	observedAt := time.Now().UTC()
	if fields := strings.Fields(line); len(fields) > 0 {
		if parsed, err := time.Parse(time.RFC3339Nano, fields[0]); err == nil {
			observedAt = parsed.UTC()
		}
	}
	return CollectorFailure{Receiver: receiver, Endpoint: endpoint[:end], Code: "connection_failed", ObservedAt: observedAt.Format(time.RFC3339Nano)}, true
}

func netSplitHostPort(value string) bool {
	_, _, err := net.SplitHostPort(value)
	return err == nil
}
