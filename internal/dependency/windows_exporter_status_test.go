package dependency

import (
	"context"
	"testing"
)

func TestNewWindowsExporterDependencyInspectorMapsHealthyManagedService(
	t *testing.T,
) {
	options := DefaultWindowsExporterOptions()

	inspector := newWindowsExporterDependencyInspector(
		options,
		func(
			ctx context.Context,
		) (windowsExporterInstallationState, error) {
			return windowsExporterInstallationState{
				serviceExists:  true,
				serviceEnabled: true,
				serviceRunning: true,
				healthReady:    true,
				startupMatches: true,
			}, nil
		},
		func(
			options WindowsExporterOptions,
		) (bool, error) {
			return true, nil
		},
	)

	got, err := inspector(context.Background())
	if err != nil {
		t.Fatalf("inspector() error = %v", err)
	}

	want := Inspection{
		Enabled: true,
		Healthy: true,
	}

	if got != want {
		t.Fatalf("inspection = %+v, want %+v", got, want)
	}
}

func TestNewWindowsExporterDependencyInspectorReportsDisabledService(
	t *testing.T,
) {
	configChecked := false

	inspector := newWindowsExporterDependencyInspector(
		DefaultWindowsExporterOptions(),
		func(
			ctx context.Context,
		) (windowsExporterInstallationState, error) {
			return windowsExporterInstallationState{
				serviceExists:  true,
				serviceEnabled: false,
			}, nil
		},
		func(
			options WindowsExporterOptions,
		) (bool, error) {
			configChecked = true

			return true, nil
		},
	)

	got, err := inspector(context.Background())
	if err != nil {
		t.Fatalf("inspector() error = %v", err)
	}

	if got != (Inspection{}) {
		t.Fatalf("inspection = %+v, want disabled inspection", got)
	}
	if configChecked {
		t.Fatal("configuration was inspected for a disabled service")
	}
}
