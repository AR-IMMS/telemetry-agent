package dependency

import (
	"bytes"
	"context"
	"errors"
	"testing"

	runtimeDependencies "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies"
)

func TestStatusPreservesLegacyPresentationAndManagedDisabledState(t *testing.T) {
	var stdout, stderr bytes.Buffer
	managed := map[string]struct{}{"node-exporter": {}}
	results := []runtimeDependencies.StatusResult{
		{
			Definition: runtimeDependencies.Definition{Name: "node-exporter", MetricsEndpoint: " http://127.0.0.1:9100/metrics "},
			Status:     runtimeDependencies.Status{Availability: runtimeDependencies.AvailabilityDisabled, Health: runtimeDependencies.HealthUnknown},
		},
		{
			Definition: runtimeDependencies.Definition{Name: "custom-exporter"},
			Status:     runtimeDependencies.Status{Availability: runtimeDependencies.AvailabilityEnabled, Health: runtimeDependencies.HealthHealthy, Drifted: true},
		},
		{
			Definition:      runtimeDependencies.Definition{Name: "failed-exporter"},
			InspectionError: errors.New("inspection unavailable"),
		},
	}

	exitCode := Status(context.Background(), &stdout, &stderr, func(context.Context) ([]runtimeDependencies.StatusResult, error) {
		return results, nil
	}, managed)

	if exitCode != 0 {
		t.Fatalf("Status() exit code = %d, want 0; stderr=%q", exitCode, stderr.String())
	}
	if got, want := stdout.String(), "Dependency status:\n- node-exporter: disabled (unknown) — metrics: http://127.0.0.1:9100/metrics\n- custom-exporter: enabled (healthy, drifted)\n- failed-exporter: unknown (inspection unavailable)\n"; got != want {
		t.Fatalf("Status() stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Status() stderr = %q, want empty", stderr.String())
	}
}

func TestStatusReportsUnmanagedDisabledDependencyAsNotInstalled(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := Status(context.Background(), &stdout, &stderr, func(context.Context) ([]runtimeDependencies.StatusResult, error) {
		return []runtimeDependencies.StatusResult{{
			Definition: runtimeDependencies.Definition{Name: "node-exporter"},
			Status:     runtimeDependencies.Status{Availability: runtimeDependencies.AvailabilityDisabled, Health: runtimeDependencies.HealthUnknown},
		}}, nil
	}, nil)

	if exitCode != 0 {
		t.Fatalf("Status() exit code = %d, want 0; stderr=%q", exitCode, stderr.String())
	}
	if got, want := stdout.String(), "Dependency status:\n- node-exporter: not installed\n"; got != want {
		t.Fatalf("Status() stdout = %q, want %q", got, want)
	}
}

func TestStatusReportsReaderErrorsAndMissingReader(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := Status(context.Background(), &stdout, &stderr, func(context.Context) ([]runtimeDependencies.StatusResult, error) {
		return nil, errors.New("runtime unavailable")
	}, nil)
	if exitCode != 1 {
		t.Fatalf("Status() reader-error exit code = %d, want 1", exitCode)
	}
	if got, want := stderr.String(), "dependency status: runtime unavailable\n"; got != want {
		t.Fatalf("Status() stderr = %q, want %q", got, want)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = Status(context.Background(), &stdout, &stderr, nil, nil)
	if exitCode != 1 {
		t.Fatalf("Status() missing-reader exit code = %d, want 1", exitCode)
	}
	if got, want := stderr.String(), "dependency status: lister is not configured\n"; got != want {
		t.Fatalf("Status() stderr = %q, want %q", got, want)
	}
}
