package dependency

import (
	"context"
	"testing"
)

func TestWindowsExporterReconcilerReusesHealthyExistingService(
	t *testing.T,
) {
	installCalls := 0

	reconciler := windowsExporterReconciler{
		inspect: func(
			context.Context,
		) (windowsExporterInstallationState, error) {
			return windowsExporterInstallationState{
				serviceExists:  true,
				serviceRunning: true,
				healthReady:    true,
			}, nil
		},
		install: func(context.Context) (InstallResult, error) {
			installCalls++
			return InstallResult{}, nil
		},
	}

	result, err := reconciler.Install(context.Background())
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if installCalls != 0 {
		t.Fatalf("fresh installer calls = %d, want 0", installCalls)
	}
	if result.Name != "windows-exporter" {
		t.Fatalf("result name = %q, want windows-exporter", result.Name)
	}
	if !result.Reused {
		t.Fatal("result Reused = false, want true")
	}
}

func TestWindowsExporterReconcilerRunsFreshInstallWhenServiceIsAbsent(
	t *testing.T,
) {
	installCalls := 0

	reconciler := windowsExporterReconciler{
		inspect: func(
			context.Context,
		) (windowsExporterInstallationState, error) {
			return windowsExporterInstallationState{}, nil
		},
		install: func(context.Context) (InstallResult, error) {
			installCalls++

			return InstallResult{
				Name: "windows-exporter",
			}, nil
		},
	}

	result, err := reconciler.Install(context.Background())
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if installCalls != 1 {
		t.Fatalf("fresh installer calls = %d, want 1", installCalls)
	}
	if result.Reused {
		t.Fatal("result Reused = true, want false")
	}
}

func TestWindowsExporterReconcilerDoesNotOverwriteUnhealthyService(
	t *testing.T,
) {
	installCalls := 0

	reconciler := windowsExporterReconciler{
		inspect: func(
			context.Context,
		) (windowsExporterInstallationState, error) {
			return windowsExporterInstallationState{
				serviceExists:  true,
				serviceRunning: true,
				healthReady:    false,
			}, nil
		},
		install: func(context.Context) (InstallResult, error) {
			installCalls++
			return InstallResult{}, nil
		},
	}

	_, err := reconciler.Install(context.Background())

	if err == nil {
		t.Fatal("Install() error = nil, want unhealthy-service failure")
	}
	if installCalls != 0 {
		t.Fatalf("fresh installer calls = %d, want 0", installCalls)
	}
}
