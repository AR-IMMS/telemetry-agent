package agenthealth

import "strings"

// Status is the summarized local health classification of a running Agent.
type Status string

const (
	StatusHealthy  Status = "healthy"
	StatusDegraded Status = "degraded"
)

// CollectorState identifies the Collector phase observed by the Agent runtime.
type CollectorState string

const (
	CollectorStateStarting CollectorState = "starting"
	CollectorStateReady    CollectorState = "ready"
	CollectorStateStopping CollectorState = "stopping"
	CollectorStateStopped  CollectorState = "stopped"
	CollectorStateFailed   CollectorState = "failed"
)

// RuntimeObservation contains the runtime facts used to evaluate local Agent
// health. It is transient and must not be persisted as desired Agent state.
type RuntimeObservation struct {
	CollectorState      CollectorState
	DesiredGeneration   uint64
	ActivatedGeneration uint64
	AppliedGeneration   uint64
	PendingTeardowns    int
	LastError           string
}

// Snapshot is the evaluated local health view returned to future CLI, HTTP, and
// Control API adapters.
type Snapshot struct {
	Status Status `json:"status"`

	CollectorState      CollectorState `json:"collectorState"`
	DesiredGeneration   uint64         `json:"desiredGeneration"`
	ActivatedGeneration uint64         `json:"activatedGeneration"`
	AppliedGeneration   uint64         `json:"appliedGeneration"`
	PendingTeardowns    int            `json:"pendingTeardowns"`
	LastError           string         `json:"lastError,omitempty"`
}

// Evaluate derives local health from one runtime observation.
func Evaluate(observation RuntimeObservation) Snapshot {
	snapshot := Snapshot{
		Status:              StatusDegraded,
		CollectorState:      observation.CollectorState,
		DesiredGeneration:   observation.DesiredGeneration,
		ActivatedGeneration: observation.ActivatedGeneration,
		AppliedGeneration:   observation.AppliedGeneration,
		PendingTeardowns:    observation.PendingTeardowns,
		LastError:           strings.TrimSpace(observation.LastError),
	}

	if observation.CollectorState == CollectorStateReady &&
		observation.DesiredGeneration ==
			observation.ActivatedGeneration &&
		observation.ActivatedGeneration ==
			observation.AppliedGeneration &&
		observation.PendingTeardowns == 0 &&
		snapshot.LastError == "" {
		snapshot.Status = StatusHealthy
	}

	return snapshot
}
