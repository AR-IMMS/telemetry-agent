package dockeradapter

import (
	"context"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/operations/handlers"
	"github.com/moby/moby/api/types/container"
	moby "github.com/moby/moby/client"
)

type engineFake struct {
	inspection moby.ContainerInspectResult
	inspectID  string
	inspectErr error
	restartID  string
	restarts   int
	restartOpt moby.ContainerRestartOptions
	restartErr error
}

func (f *engineFake) ContainerInspect(_ context.Context, id string, _ moby.ContainerInspectOptions) (moby.ContainerInspectResult, error) {
	f.inspectID = id
	return f.inspection, f.inspectErr
}

func (f *engineFake) ContainerRestart(_ context.Context, id string, options moby.ContainerRestartOptions) (moby.ContainerRestartResult, error) {
	f.restartID, f.restartOpt = id, options
	f.restarts++
	return moby.ContainerRestartResult{}, f.restartErr
}

func TestRuntimeInspectsExactIDAndExposesOnlyRestartPolicyEvidence(t *testing.T) {
	containerID := strings.Repeat("a", 64)
	engine := &engineFake{inspection: moby.ContainerInspectResult{Container: container.InspectResponse{
		ID: containerID, RestartCount: 3, Config: &container.Config{Labels: map[string]string{
			handlers.ContainerRestartOptInLabel: "true", "secret.fixture": "must-not-escape",
		}}, State: &container.State{Status: "running"},
	}}}
	runtime := Runtime{client: engine}
	got, err := runtime.InspectForRestart(context.Background(), containerID)
	if err != nil {
		t.Fatal(err)
	}
	if engine.inspectID != containerID || got.ID != containerID || got.State != "running" || got.RestartCount != 3 || !got.RestartOptIn {
		t.Fatalf("inspect id=%q observation=%+v", engine.inspectID, got)
	}
}

func TestRuntimeRestartUsesExactIDAndDoesNotSetDestructiveImmediateTimeout(t *testing.T) {
	containerID := strings.Repeat("b", 64)
	engine := &engineFake{}
	runtime := Runtime{client: engine}
	if err := runtime.Restart(context.Background(), containerID); err != nil {
		t.Fatal(err)
	}
	if engine.restartID != containerID || engine.restarts != 1 || engine.restartOpt.Timeout != nil {
		t.Fatalf("restart id=%q calls=%d timeout=%v", engine.restartID, engine.restarts, engine.restartOpt.Timeout)
	}
}

func TestRuntimeRejectsNonLocalDockerEndpoints(t *testing.T) {
	for _, endpoint := range []string{"tcp://docker.example:2375", "unix://relative.sock", "npipe:////remote/pipe/docker_engine"} {
		if err := validateLocalEndpoint(endpoint, "windows"); err == nil {
			t.Errorf("validateLocalEndpoint(%q) accepted an unsafe or nonlocal endpoint", endpoint)
		}
	}
	if err := validateLocalEndpoint("unix:///var/run/docker.sock", "linux"); err != nil {
		t.Fatalf("valid Linux socket rejected: %v", err)
	}
	if err := validateLocalEndpoint("npipe:////./pipe/docker_engine", "windows"); err != nil {
		t.Fatalf("valid Windows pipe rejected: %v", err)
	}
}
