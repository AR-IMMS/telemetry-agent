package dockeradapter

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/url"
	"runtime"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/operations/handlers"
	moby "github.com/moby/moby/client"
)

type engineClient interface {
	ContainerInspect(context.Context, string, moby.ContainerInspectOptions) (moby.ContainerInspectResult, error)
	ContainerRestart(context.Context, string, moby.ContainerRestartOptions) (moby.ContainerRestartResult, error)
}

// Runtime adapts one explicitly configured local Docker Engine to the Operations restart port.
type Runtime struct{ client engineClient }

// New constructs a Docker runtime client for the local Unix socket or Windows Engine pipe.
func New(endpoint string) (*Runtime, error) {
	if err := validateLocalEndpoint(endpoint, runtime.GOOS); err != nil {
		return nil, err
	}
	client, err := moby.New(moby.WithHost(endpoint))
	if err != nil {
		return nil, fmt.Errorf("configure local Docker Operations client")
	}
	return &Runtime{client: client}, nil
}

// InspectForRestart reads exact identity, state, restart count, health, and the opt-in label.
func (r *Runtime) InspectForRestart(ctx context.Context, containerID string) (handlers.ContainerRestartObservation, error) {
	if ctx == nil || r == nil || r.client == nil || !validContainerID(containerID) {
		return handlers.ContainerRestartObservation{}, fmt.Errorf("Docker restart inspection requires a client, context, and full container ID")
	}
	result, err := r.client.ContainerInspect(ctx, containerID, moby.ContainerInspectOptions{})
	if err != nil {
		return handlers.ContainerRestartObservation{}, err
	}
	item := result.Container
	if item.ID != containerID || item.State == nil || item.Config == nil || item.RestartCount < 0 || len(item.Config.Labels) > 256 {
		return handlers.ContainerRestartObservation{}, fmt.Errorf("Docker returned incomplete restart evidence")
	}
	state := strings.ToLower(string(item.State.Status))
	switch state {
	case "running", "created", "restarting", "removing", "paused", "exited", "dead":
	default:
		return handlers.ContainerRestartObservation{}, fmt.Errorf("Docker returned an unsupported container state")
	}
	health := ""
	if item.State.Health != nil {
		health = strings.ToLower(string(item.State.Health.Status))
	}
	if len(item.State.StartedAt) > 64 {
		return handlers.ContainerRestartObservation{}, fmt.Errorf("Docker returned an invalid container start time")
	}
	return handlers.ContainerRestartObservation{ID: item.ID, State: state, Health: health, RestartCount: item.RestartCount, StartedAt: item.State.StartedAt,
		RestartOptIn: item.Config.Labels[handlers.ContainerRestartOptInLabel] == "true"}, nil
}

// Restart issues exactly one request and leaves all retry and unknown-outcome handling to Operations.
func (r *Runtime) Restart(ctx context.Context, containerID string) error {
	if ctx == nil || r == nil || r.client == nil || !validContainerID(containerID) {
		return fmt.Errorf("Docker restart requires a client, context, and full container ID")
	}
	_, err := r.client.ContainerRestart(ctx, containerID, moby.ContainerRestartOptions{})
	if err != nil {
		return err
	}
	return nil
}

func validContainerID(id string) bool {
	if len(id) != 64 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func validateLocalEndpoint(endpoint, goos string) error {
	endpoint = strings.TrimSpace(endpoint)
	parsed, err := url.Parse(endpoint)
	if endpoint == "" || err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("invalid local Docker endpoint")
	}
	switch goos {
	case "linux":
		if parsed.Scheme != "unix" || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
			return fmt.Errorf("Linux Docker endpoint must be an absolute Unix socket")
		}
	case "windows":
		if parsed.Scheme != "npipe" || !strings.HasPrefix(strings.ToLower(parsed.Host+parsed.Path), "//./pipe/") {
			return fmt.Errorf("Windows Docker endpoint must use a local named pipe")
		}
	default:
		return fmt.Errorf("Docker Operations are unsupported on this platform")
	}
	return nil
}

var _ handlers.ContainerRestartRuntime = (*Runtime)(nil)
