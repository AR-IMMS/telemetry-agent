package nodeexporter

import (
	"context"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

func TestNewDependencyInspectorMapsHealthyEnabledInstallation(
	t *testing.T,
) {
	options := DefaultOptions()

	inspector := newDependencyInspector(
		options,
		func(context.Context) (installationState, error) {
			return installationState{
				ServiceExists: true,
				Healthy:       true,
				UnitMatches:   true,
			}, nil
		},
		func(
			ctx context.Context,
			serviceName string,
		) (bool, error) {
			if serviceName != "ar-imms-node-exporter.service" {
				t.Fatalf(
					"service name = %q, want managed unit name",
					serviceName,
				)
			}

			return true, nil
		},
	)

	inspection, err := inspector(context.Background())
	if err != nil {
		t.Fatalf("inspector() error = %v", err)
	}

	want := dependency.Inspection{
		Enabled: true,
		Healthy: true,
		Drifted: false,
	}
	if inspection != want {
		t.Fatalf(
			"inspection = %+v, want %+v",
			inspection,
			want,
		)
	}
}
