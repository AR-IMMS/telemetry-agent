package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

func TestRunDependencyStatusWritesLifecycleResults(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	statePath := filepath.Join(t.TempDir(), "state.json")

	exitCode := runDependency(
		context.Background(),
		[]string{"status", "--state-path", statePath},
		&stdout,
		&stderr,
		dependencies{
			listDependencyStatuses: func(
				context.Context,
			) ([]dependency.StatusResult, error) {
				return []dependency.StatusResult{
					{
						Definition: dependency.Definition{
							Name:            "node-exporter",
							MetricsEndpoint: "http://127.0.0.1:9100/metrics",
						},
						Status: dependency.Status{
							Availability: dependency.AvailabilityEnabled,
							Health:       dependency.HealthHealthy,
						},
					},
					{
						Definition: dependency.Definition{
							Name: "windows-exporter",
						},
						Status: dependency.Status{
							Availability: dependency.AvailabilityUnknown,
							Health:       dependency.HealthUnknown,
						},
						InspectionError: errors.New("permission denied"),
					},
				}, nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf("runDependency(status) exit code = %d, want 0", exitCode)
	}

	want := "" +
		"Dependency status:\n" +
		"- node-exporter: enabled (healthy) — metrics: http://127.0.0.1:9100/metrics\n" +
		"- windows-exporter: unknown (permission denied)\n"

	if stdout.String() != want {
		t.Fatalf(
			"runDependency(status) stdout = %q, want %q",
			stdout.String(),
			want,
		)
	}
	if stderr.Len() != 0 {
		t.Fatalf(
			"runDependency(status) stderr = %q, want empty",
			stderr.String(),
		)
	}
}

func TestRunDependencyStatusWritesUnhealthyDriftedResult(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	statePath := filepath.Join(t.TempDir(), "state.json")

	exitCode := runDependency(
		context.Background(),
		[]string{"status", "--state-path", statePath},
		&stdout,
		&stderr,
		dependencies{
			listDependencyStatuses: func(
				context.Context,
			) ([]dependency.StatusResult, error) {
				return []dependency.StatusResult{
					{
						Definition: dependency.Definition{
							Name: "windows-exporter",
						},
						Status: dependency.Status{
							Availability: dependency.AvailabilityEnabled,
							Health:       dependency.HealthUnhealthy,
							Drifted:      true,
						},
					},
				}, nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf("runDependency(status) exit code = %d, want 0", exitCode)
	}

	want := "" +
		"Dependency status:\n" +
		"- windows-exporter: enabled (unhealthy, drifted)\n"

	if stdout.String() != want {
		t.Fatalf(
			"runDependency(status) stdout = %q, want %q",
			stdout.String(),
			want,
		)
	}
	if stderr.Len() != 0 {
		t.Fatalf(
			"runDependency(status) stderr = %q, want empty",
			stderr.String(),
		)
	}
}
