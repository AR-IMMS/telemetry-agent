package agenthealth

import "sync"

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
	r.mu.RUnlock()

	return snapshot
}
