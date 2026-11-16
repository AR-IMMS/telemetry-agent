package health

import (
	"time"

	runtimecontracts "github.com/ar-imms/telemetry-agent/internal/runtime/collector/contracts"
)

// Status is the compatibility classification for Collector readiness and configuration application.
type Status string

const (
	StatusHealthy  Status = "healthy"
	StatusDegraded Status = "degraded"
)

// CollectorState is the Runtime-owned Collector process phase used in Health snapshots.
type CollectorState = runtimecontracts.CollectorState

const (
	CollectorStateStarting = runtimecontracts.CollectorStateStarting
	CollectorStateReady    = runtimecontracts.CollectorStateReady
	CollectorStateStopping = runtimecontracts.CollectorStateStopping
	CollectorStateStopped  = runtimecontracts.CollectorStateStopped
	CollectorStateFailed   = runtimecontracts.CollectorStateFailed
)

// RuntimeObservation aliases Runtime-owned evidence consumed by Health.
type RuntimeObservation = runtimecontracts.RuntimeObservation

// Snapshot combines the compatible runtime status with independent health evidence.
type Snapshot struct {
	Status     Status     `json:"status"`
	Assessment Assessment `json:"assessment"`

	CollectorState      CollectorState     `json:"collectorState"`
	DesiredGeneration   uint64             `json:"desiredGeneration"`
	ActivatedGeneration uint64             `json:"activatedGeneration"`
	AppliedGeneration   uint64             `json:"appliedGeneration"`
	PendingTeardowns    int                `json:"pendingTeardowns"`
	LastError           string             `json:"lastError,omitempty"`
	CollectorFailures   []CollectorFailure `json:"collectorFailures,omitempty"`
}

// CollectorFailure is a bounded, sanitized Collector observation exposed by the local status API.
type CollectorFailure struct {
	Direction  CollectorFailureDirection `json:"direction,omitempty"`
	Receiver   string                    `json:"receiver"`
	Exporter   string                    `json:"exporter,omitempty"`
	Endpoint   string                    `json:"endpoint"`
	Code       string                    `json:"code"`
	ObservedAt string                    `json:"observedAt"`
}

// CollectorFailureDirection identifies the Collector component role that reported a network failure.
type CollectorFailureDirection string

const (
	// CollectorFailureReceiver identifies a source receiver scrape failure.
	CollectorFailureReceiver CollectorFailureDirection = "receiver"
	// CollectorFailureExporter identifies an exporter send failure.
	CollectorFailureExporter CollectorFailureDirection = "exporter"
)

// Evaluate derives local health from one runtime observation.
func Evaluate(observation RuntimeObservation) Snapshot {
	return EvaluateAt(observation, time.Now().UTC())
}

// EvaluateAt derives the compatible runtime snapshot and its evidence assessment at now.
func EvaluateAt(observation RuntimeObservation, now time.Time) Snapshot {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	snapshot := Snapshot{
		Status:              StatusDegraded,
		Assessment:          evaluateAssessment(now, observation),
		CollectorState:      observation.CollectorState,
		DesiredGeneration:   observation.DesiredGeneration,
		ActivatedGeneration: observation.ActivatedGeneration,
		AppliedGeneration:   observation.AppliedGeneration,
		PendingTeardowns:    observation.PendingTeardowns,
		LastError:           sanitizeError(observation.LastError),
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
