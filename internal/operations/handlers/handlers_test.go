package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/operations"
)

type healthFake struct{ snapshot agenthealth.Snapshot }

func (f healthFake) SnapshotContext(context.Context) agenthealth.Snapshot { return f.snapshot }

type containerFake struct {
	value ContainerObservation
	calls int
}

func (f *containerFake) Inspect(_ context.Context, id string) (ContainerObservation, error) {
	f.calls++
	if id != f.value.ID {
		return ContainerObservation{}, errors.New("wrong id")
	}
	return f.value, nil
}

type dependencyFake struct{ calls int }

func (f *dependencyFake) Inspect(context.Context, string) (DependencyObservation, error) {
	f.calls++
	return DependencyObservation{Name: "otelcol-contrib", Availability: "enabled", Health: "healthy"}, nil
}

type containerRestartFake struct {
	observations []ContainerRestartObservation
	inspectIDs   []string
	restartIDs   []string
	restartErr   error
}

func (f *containerRestartFake) InspectForRestart(_ context.Context, id string) (ContainerRestartObservation, error) {
	f.inspectIDs = append(f.inspectIDs, id)
	if len(f.observations) == 0 {
		return ContainerRestartObservation{}, errors.New("no restart observation")
	}
	observation := f.observations[0]
	if len(f.observations) > 1 {
		f.observations = f.observations[1:]
	}
	return observation, nil
}

func (f *containerRestartFake) Restart(_ context.Context, id string) error {
	f.restartIDs = append(f.restartIDs, id)
	return f.restartErr
}

func TestContainerInspectRequiresExactResourceIdentityAndReturnsAllowlist(t *testing.T) {
	inspector := &containerFake{value: ContainerObservation{ID: strings.Repeat("a", 64), Name: "redis", Image: "redis:7", State: "running", RestartCount: 1}}
	handler := ContainerInspect{Containers: inspector}
	request := operations.Request{OperationID: "op-1", OperationType: operations.OperationContainerInspect,
		Target:     operations.Target{Kind: "docker_container", ProviderID: "docker", ResourceID: inspector.value.ID},
		Parameters: json.RawMessage(`{"container_id":"` + inspector.value.ID + `"}`)}
	result, err := handler.Execute(context.Background(), operations.RequestMetadata{OperationID: request.OperationID, Target: request.Target}, ContainerInspectParameters{ContainerID: inspector.value.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(result) || inspector.calls != 1 || strings.Contains(string(result), "mount") || strings.Contains(string(result), "environment") {
		t.Fatalf("inspection result=%s calls=%d", result, inspector.calls)
	}
	_, err = handler.Execute(context.Background(), operations.RequestMetadata{OperationID: request.OperationID, Target: request.Target}, ContainerInspectParameters{ContainerID: "redis"})
	var operationError *operations.OperationError
	if !errors.As(err, &operationError) || operationError.Code != "TARGET_IDENTITY_MISMATCH" || inspector.calls != 1 {
		t.Fatalf("mismatched identity error=%v calls=%d", err, inspector.calls)
	}
}

func TestDependencyInspectIsLimitedToAgentManagedTarget(t *testing.T) {
	inspector := &dependencyFake{}
	handler := DependencyInspect{Dependencies: inspector}
	metadata := operations.RequestMetadata{Target: operations.Target{Kind: "managed_dependency", ProviderID: "agent", ResourceID: "otelcol-contrib"}}
	result, err := handler.Execute(context.Background(), metadata, DependencyInspectParameters{Name: "otelcol-contrib"})
	if err != nil || inspector.calls != 1 || !json.Valid(result) {
		t.Fatalf("result=%s calls=%d err=%v", result, inspector.calls, err)
	}
	metadata.Target.ProviderID = "host-service-manager"
	_, err = handler.Execute(context.Background(), metadata, DependencyInspectParameters{Name: "otelcol-contrib"})
	var operationError *operations.OperationError
	if !errors.As(err, &operationError) || operationError.Code != "UNSUPPORTED_TARGET" || inspector.calls != 1 {
		t.Fatalf("unsupported target error=%v calls=%d", err, inspector.calls)
	}
}

func TestDiagnosticsReusesBoundedHealthSnapshot(t *testing.T) {
	snapshot := agenthealth.Evaluate(agenthealth.RuntimeObservation{CollectorState: agenthealth.CollectorStateReady})
	handler := Diagnostics{Health: healthFake{snapshot: snapshot}}
	result, err := handler.Execute(context.Background(), operations.RequestMetadata{}, struct{}{})
	if err != nil || !json.Valid(result) || len(result) > maxHandlerResultBytes {
		t.Fatalf("diagnostics bytes=%d err=%v", len(result), err)
	}
}

func TestContainerRestartRequiresBothOptInsAndFreshExactRunningTarget(t *testing.T) {
	containerID := strings.Repeat("b", 64)
	metadata := operations.RequestMetadata{OperationID: "restart-1", Target: operations.Target{
		Kind: "docker_container", ProviderID: "docker", ResourceID: containerID}}
	for name, test := range map[string]struct {
		enabled bool
		initial ContainerRestartObservation
		wantErr string
	}{
		"local opt-in off":     {initial: ContainerRestartObservation{ID: containerID, State: "running", RestartOptIn: true}, wantErr: "RESTART_DISABLED"},
		"container opt-in off": {enabled: true, initial: ContainerRestartObservation{ID: containerID, State: "running"}, wantErr: "RESTART_NOT_OPTED_IN"},
		"stale identity":       {enabled: true, initial: ContainerRestartObservation{ID: strings.Repeat("c", 64), State: "running", RestartOptIn: true}, wantErr: "STALE_TARGET"},
		"not running":          {enabled: true, initial: ContainerRestartObservation{ID: containerID, State: "exited", RestartOptIn: true}, wantErr: "CONTAINER_NOT_RUNNING"},
	} {
		t.Run(name, func(t *testing.T) {
			runtime := &containerRestartFake{observations: []ContainerRestartObservation{test.initial}}
			handler := ContainerRestart{Runtime: runtime, Enabled: test.enabled}
			_, err := handler.Execute(context.Background(), metadata, ContainerRestartParameters{ContainerID: containerID})
			var operationError *operations.OperationError
			if !errors.As(err, &operationError) || operationError.Code != test.wantErr {
				t.Fatalf("error=%v want code %s", err, test.wantErr)
			}
			if len(runtime.restartIDs) != 0 || (test.wantErr == "RESTART_DISABLED" && len(runtime.inspectIDs) != 0) {
				t.Fatalf("unsafe Docker calls inspect=%v restart=%v", runtime.inspectIDs, runtime.restartIDs)
			}
		})
	}
}

func TestContainerRestartCallsOnceAndVerifiesRestartCountAndState(t *testing.T) {
	containerID := strings.Repeat("d", 64)
	runtime := &containerRestartFake{observations: []ContainerRestartObservation{
		{ID: containerID, State: "running", RestartCount: 4, StartedAt: "2026-01-01T00:00:00Z", RestartOptIn: true, Health: "healthy"},
		{ID: containerID, State: "running", RestartCount: 5, StartedAt: "2026-01-01T00:00:02Z", RestartOptIn: true, Health: "healthy"},
	}}
	handler := ContainerRestart{Runtime: runtime, Enabled: true, VerifyInterval: time.Millisecond}
	result, err := handler.Execute(context.Background(), operations.RequestMetadata{OperationID: "restart-2", Target: operations.Target{
		Kind: "docker_container", ProviderID: "docker", ResourceID: containerID}}, ContainerRestartParameters{ContainerID: containerID})
	if err != nil {
		t.Fatal(err)
	}
	var response ContainerRestartResult
	if err := json.Unmarshal(result, &response); err != nil {
		t.Fatal(err)
	}
	if len(runtime.restartIDs) != 1 || runtime.restartIDs[0] != containerID || response.RestartCountBefore != 4 ||
		response.RestartCountAfter != 5 || response.State != "running" || response.RecoveryStatus != "RUNNING" {
		t.Fatalf("restart calls=%v response=%+v", runtime.restartIDs, response)
	}
}

func TestContainerRestartReportsUnknownWithoutRetryWhenOutcomeCannotBeVerified(t *testing.T) {
	containerID := strings.Repeat("e", 64)
	runtime := &containerRestartFake{observations: []ContainerRestartObservation{
		{ID: containerID, State: "running", RestartCount: 2, StartedAt: "2026-01-01T00:00:00Z", RestartOptIn: true},
		{ID: containerID, State: "running", RestartCount: 2, StartedAt: "2026-01-01T00:00:00Z", RestartOptIn: true},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	handler := ContainerRestart{Runtime: runtime, Enabled: true, VerifyInterval: time.Millisecond}
	_, err := handler.Execute(ctx, operations.RequestMetadata{OperationID: "restart-unknown", Target: operations.Target{
		Kind: "docker_container", ProviderID: "docker", ResourceID: containerID}}, ContainerRestartParameters{ContainerID: containerID})
	if !errors.Is(err, operations.ErrOutcomeUnknown) {
		t.Fatalf("error=%v, want outcome unknown", err)
	}
	if len(runtime.restartIDs) != 1 {
		t.Fatalf("restart calls=%v, want one call", runtime.restartIDs)
	}
}

func TestContainerRestartCallErrorIsUnknownAndNeverRetried(t *testing.T) {
	containerID := strings.Repeat("f", 64)
	runtime := &containerRestartFake{observations: []ContainerRestartObservation{{ID: containerID, State: "running", StartedAt: "2026-01-01T00:00:00Z", RestartOptIn: true}}, restartErr: errors.New("connection lost after request")}
	handler := ContainerRestart{Runtime: runtime, Enabled: true}
	_, err := handler.Execute(context.Background(), operations.RequestMetadata{Target: operations.Target{
		Kind: "docker_container", ProviderID: "docker", ResourceID: containerID}}, ContainerRestartParameters{ContainerID: containerID})
	if !errors.Is(err, operations.ErrOutcomeUnknown) || len(runtime.restartIDs) != 1 {
		t.Fatalf("error=%v restart calls=%v", err, runtime.restartIDs)
	}
}

func TestContainerRestartVerifiesStartedAtWhenDockerRestartCountDoesNotChange(t *testing.T) {
	containerID := strings.Repeat("9", 64)
	runtime := &containerRestartFake{observations: []ContainerRestartObservation{
		{ID: containerID, State: "running", RestartCount: 0, StartedAt: "2026-01-01T00:00:00Z", RestartOptIn: true},
		{ID: containerID, State: "running", RestartCount: 0, StartedAt: "2026-01-01T00:00:02Z", RestartOptIn: true},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	handler := ContainerRestart{Runtime: runtime, Enabled: true, VerifyInterval: time.Millisecond}
	result, err := handler.Execute(ctx, operations.RequestMetadata{Target: operations.Target{
		Kind: "docker_container", ProviderID: "docker", ResourceID: containerID}}, ContainerRestartParameters{ContainerID: containerID})
	if err != nil {
		t.Fatalf("restart with newer Docker StartedAt was not verified: %v", err)
	}
	var response ContainerRestartResult
	if err := json.Unmarshal(result, &response); err != nil {
		t.Fatal(err)
	}
	if response.StartedAtBefore != "2026-01-01T00:00:00Z" || response.StartedAtAfter != "2026-01-01T00:00:02Z" || response.RestartCountAfter != response.RestartCountBefore {
		t.Fatalf("restart evidence=%+v", response)
	}
}
