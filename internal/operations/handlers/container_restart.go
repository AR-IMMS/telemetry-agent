package handlers

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	operations "github.com/ar-imms/telemetry-agent/internal/operations/command"
)

const (
	// ContainerRestartOptInLabel is the container label required in addition to local Agent opt-in.
	ContainerRestartOptInLabel = "ar-imms.operations.container-restart.enabled"
	defaultRestartVerifyPoll   = 200 * time.Millisecond
)

// ContainerRestartObservation exposes only the runtime evidence needed to gate and verify a restart.
type ContainerRestartObservation struct {
	ID           string
	State        string
	Health       string
	RestartCount int
	StartedAt    string
	RestartOptIn bool
}

// ContainerRestartRuntime inspects one exact container and issues one restart request.
type ContainerRestartRuntime interface {
	InspectForRestart(context.Context, string) (ContainerRestartObservation, error)
	Restart(context.Context, string) error
}

// ContainerRestartParameters identifies the exact immutable Docker container ID.
type ContainerRestartParameters struct {
	ContainerID string `json:"container_id"`
}

// ContainerRestartResult reports restart evidence separately from the container's recovery state.
type ContainerRestartResult struct {
	RestartCountBefore int    `json:"restart_count_before"`
	RestartCountAfter  int    `json:"restart_count_after"`
	StartedAtBefore    string `json:"started_at_before"`
	StartedAtAfter     string `json:"started_at_after"`
	State              string `json:"state"`
	Health             string `json:"health,omitempty"`
	RecoveryStatus     string `json:"recovery_status"`
}

// ContainerRestart restarts one running, opted-in container only when the local feature gate is enabled.
type ContainerRestart struct {
	Runtime        ContainerRestartRuntime
	Enabled        bool
	VerifyInterval time.Duration
}

// Type returns the stable container restart operation name.
func (ContainerRestart) Type() operations.OperationType { return operations.OperationContainerRestart }

// Execute validates opt-in and exact identity, issues one restart, then verifies its runtime effect.
func (h ContainerRestart) Execute(ctx context.Context, request operations.RequestMetadata, parameters ContainerRestartParameters) (json.RawMessage, error) {
	if !h.Enabled {
		return nil, &operations.OperationError{Code: "RESTART_DISABLED", Message: "container restart is disabled by local Agent policy"}
	}
	if ctx == nil || h.Runtime == nil {
		return nil, &operations.OperationError{Code: "DOCKER_UNAVAILABLE", Message: "Docker restart is unavailable"}
	}
	if request.Target.Kind != "docker_container" || request.Target.ProviderID != "docker" ||
		!fullContainerID(parameters.ContainerID) || parameters.ContainerID != request.Target.ResourceID {
		return nil, &operations.OperationError{Code: "TARGET_IDENTITY_MISMATCH", Message: "request does not identify the exact Docker container"}
	}
	before, err := h.Runtime.InspectForRestart(ctx, parameters.ContainerID)
	if err != nil {
		return nil, &operations.OperationError{Code: "CONTAINER_INSPECTION_FAILED", Message: "Docker could not inspect the requested container"}
	}
	if before.ID != parameters.ContainerID {
		return nil, &operations.OperationError{Code: "STALE_TARGET", Message: "Docker returned a different container identity"}
	}
	if !before.RestartOptIn {
		return nil, &operations.OperationError{Code: "RESTART_NOT_OPTED_IN", Message: "container restart is not enabled on the target container"}
	}
	if before.State != "running" {
		return nil, &operations.OperationError{Code: "CONTAINER_NOT_RUNNING", Message: "only a currently running container can be restarted"}
	}
	if before.RestartCount < 0 {
		return nil, &operations.OperationError{Code: "INVALID_CONTAINER_STATE", Message: "Docker returned invalid restart evidence"}
	}
	startedAtBefore, err := time.Parse(time.RFC3339Nano, before.StartedAt)
	if err != nil {
		return nil, &operations.OperationError{Code: "INVALID_CONTAINER_STATE", Message: "Docker returned invalid container start time"}
	}
	if err := h.Runtime.Restart(ctx, parameters.ContainerID); err != nil {
		return nil, fmt.Errorf("%w: Docker restart request may have taken effect", operations.ErrOutcomeUnknown)
	}
	return h.verifyRestart(ctx, parameters.ContainerID, before, startedAtBefore)
}

func (h ContainerRestart) verifyRestart(ctx context.Context, containerID string, before ContainerRestartObservation, startedAtBefore time.Time) (json.RawMessage, error) {
	interval := h.VerifyInterval
	if interval <= 0 || interval > time.Second {
		interval = defaultRestartVerifyPoll
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: Docker restart could not be verified", operations.ErrOutcomeUnknown)
		case <-ticker.C:
		}
		observed, err := h.Runtime.InspectForRestart(ctx, containerID)
		if err != nil {
			continue
		}
		if observed.ID != containerID || observed.RestartCount < before.RestartCount {
			return nil, fmt.Errorf("%w: Docker returned inconsistent post-restart identity or counters", operations.ErrOutcomeUnknown)
		}
		startedAtAfter, startedAtErr := time.Parse(time.RFC3339Nano, observed.StartedAt)
		if startedAtErr != nil {
			continue
		}
		if observed.RestartCount <= before.RestartCount && !startedAtAfter.After(startedAtBefore) {
			continue
		}
		result := ContainerRestartResult{RestartCountBefore: before.RestartCount, RestartCountAfter: observed.RestartCount,
			StartedAtBefore: before.StartedAt, StartedAtAfter: observed.StartedAt,
			State: observed.State, Health: observed.Health, RecoveryStatus: restartRecoveryStatus(observed.State, observed.Health)}
		return encodeBounded(result)
	}
}

func restartRecoveryStatus(state, health string) string {
	if state != "running" {
		return "NOT_RUNNING"
	}
	switch strings.ToLower(health) {
	case "unhealthy":
		return "UNHEALTHY"
	case "starting":
		return "STARTING"
	default:
		return "RUNNING"
	}
}

func fullContainerID(id string) bool {
	if len(id) != 64 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
