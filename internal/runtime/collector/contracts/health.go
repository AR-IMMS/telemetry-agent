package contracts

import "time"

// CollectorState is the runtime-owned process phase reported to Health.
type CollectorState string

const (
	CollectorStateStarting CollectorState = "starting"
	CollectorStateReady    CollectorState = "ready"
	CollectorStateStopping CollectorState = "stopping"
	CollectorStateStopped  CollectorState = "stopped"
	CollectorStateFailed   CollectorState = "failed"
)

// RuntimeObservation contains transient Collector lifecycle and generation
// evidence. Health evaluates it; Runtime remains the source of these facts.
type RuntimeObservation struct {
	CollectorState        CollectorState
	DesiredGeneration     uint64
	ActivatedGeneration   uint64
	AppliedGeneration     uint64
	PendingTeardowns      int
	LastError             string
	ObservedAt            time.Time
	ValidUntil            *time.Time
	SupervisorConfigured  bool
	ConfigurationObserved bool
}

// ObservationReporter accepts evidence produced by the Runtime Collector owner.
type ObservationReporter interface {
	Report(RuntimeObservation)
}

// CollectorOutputReporter accepts one complete Collector output line for parsing
// and sanitized diagnostic retention by Health.
type CollectorOutputReporter interface {
	ReportCollectorOutput(string)
}
