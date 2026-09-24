package librehardwaremonitor

import (
	"context"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

func TestNewDependencyInspectorMapsHealthyEnabledInstallation(
	t *testing.T,
) {
	inspector := newDependencyInspector(
		func(ctx context.Context) (installationState, error) {
			return installationState{
				TaskExists:      true,
				TaskEnabled:     true,
				Healthy:         true,
				TaskMatches:     true,
				ConfigMatches:   true,
				FirewallMatches: true,
			}, nil
		},
	)

	got, err := inspector(context.Background())
	if err != nil {
		t.Fatalf("inspector() error = %v", err)
	}

	want := dependency.Inspection{
		Enabled: true,
		Healthy: true,
	}

	if got != want {
		t.Fatalf("inspection = %+v, want %+v", got, want)
	}
}

func TestNewDependencyInspectorMapsDisabledTask(
	t *testing.T,
) {
	inspector := newDependencyInspector(
		func(ctx context.Context) (installationState, error) {
			return installationState{
				TaskExists:  true,
				TaskEnabled: false,
			}, nil
		},
	)

	got, err := inspector(context.Background())
	if err != nil {
		t.Fatalf("inspector() error = %v", err)
	}

	if got != (dependency.Inspection{}) {
		t.Fatalf("inspection = %+v, want disabled inspection", got)
	}
}

func TestNewDependencyInspectorMapsDriftedInstallation(
	t *testing.T,
) {
	inspector := newDependencyInspector(
		func(ctx context.Context) (installationState, error) {
			return installationState{
				TaskExists:      true,
				TaskEnabled:     true,
				Healthy:         true,
				TaskMatches:     false,
				ConfigMatches:   true,
				FirewallMatches: true,
			}, nil
		},
	)

	got, err := inspector(context.Background())
	if err != nil {
		t.Fatalf("inspector() error = %v", err)
	}

	want := dependency.Inspection{
		Enabled: true,
		Healthy: true,
		Drifted: true,
	}

	if got != want {
		t.Fatalf("inspection = %+v, want %+v", got, want)
	}
}
