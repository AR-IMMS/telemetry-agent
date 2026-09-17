package librehardwaremonitor

import (
	"context"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

func TestReconcilerReusesHealthyMatchingInstallation(t *testing.T) {
	reconciler := reconciler{
		inspect: func(
			ctx context.Context,
		) (installationState, error) {
			return installationState{
				TaskExists:      true,
				Healthy:         true,
				ConfigMatches:   true,
				FirewallMatches: true,
				TaskMatches:     true,
			}, nil
		},
		install: func(
			ctx context.Context,
		) (dependency.InstallResult, error) {
			t.Fatal("fresh install must not run for healthy matching LHM")
			return dependency.InstallResult{}, nil
		},
	}

	result, err := reconciler.Install(context.Background())
	if err != nil {
		t.Fatalf("reconciler.Install() error = %v", err)
	}
	if !result.Reused {
		t.Fatal("reconciler.Install() Reused = false, want true")
	}
}

func TestReconcilerRunsFreshInstallWhenTaskIsAbsent(t *testing.T) {
	installCalls := 0

	reconciler := reconciler{
		inspect: func(
			ctx context.Context,
		) (installationState, error) {
			return installationState{}, nil
		},
		install: func(
			ctx context.Context,
		) (dependency.InstallResult, error) {
			installCalls++

			return dependency.InstallResult{
				Name: "libre-hardware-monitor",
			}, nil
		},
	}

	result, err := reconciler.Install(context.Background())
	if err != nil {
		t.Fatalf("reconciler.Install() error = %v", err)
	}
	if installCalls != 1 {
		t.Fatalf("fresh install calls = %d, want 1", installCalls)
	}
	if result.Reused {
		t.Fatal("reconciler.Install() Reused = true, want false")
	}
}

func TestReconcilerUpdatesHealthyInstallationWithManagedDrift(t *testing.T) {
	updateCalls := 0

	reconciler := reconciler{
		inspect: func(
			ctx context.Context,
		) (installationState, error) {
			return installationState{
				TaskExists:      true,
				Healthy:         true,
				ConfigMatches:   false,
				FirewallMatches: true,
				TaskMatches:     true,
			}, nil
		},
		install: func(
			ctx context.Context,
		) (dependency.InstallResult, error) {
			t.Fatal("fresh install must not run for healthy LHM drift")
			return dependency.InstallResult{}, nil
		},
		update: func(ctx context.Context) error {
			updateCalls++
			return nil
		},
	}

	result, err := reconciler.Install(context.Background())
	if err != nil {
		t.Fatalf("reconciler.Install() error = %v", err)
	}
	if updateCalls != 1 {
		t.Fatalf("update calls = %d, want 1", updateCalls)
	}
	if !result.Reused {
		t.Fatal("reconciler.Install() Reused = false, want true")
	}
}

func TestReconcilerDoesNotOverwriteUnhealthyExistingTask(t *testing.T) {
	installCalls := 0
	updateCalls := 0

	reconciler := reconciler{
		inspect: func(
			ctx context.Context,
		) (installationState, error) {
			return installationState{
				TaskExists: true,
				Healthy:    false,
			}, nil
		},
		install: func(
			ctx context.Context,
		) (dependency.InstallResult, error) {
			installCalls++
			return dependency.InstallResult{}, nil
		},
		update: func(ctx context.Context) error {
			updateCalls++
			return nil
		},
	}

	_, err := reconciler.Install(context.Background())

	if err == nil {
		t.Fatal("reconciler.Install() error = nil, want unhealthy-state error")
	}
	if !strings.Contains(err.Error(), "unhealthy") {
		t.Fatalf(
			"reconciler.Install() error = %v, want unhealthy-state error",
			err,
		)
	}
	if installCalls != 0 {
		t.Fatalf("fresh install calls = %d, want 0", installCalls)
	}
	if updateCalls != 0 {
		t.Fatalf("update calls = %d, want 0", updateCalls)
	}
}
