package nodeexporter

import (
	"context"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

func TestReconcilerReusesHealthyExistingService(t *testing.T) {
	reconciler := reconciler{
		inspect: func(
			ctx context.Context,
		) (installationState, error) {
			return installationState{
				ServiceExists: true,
				Healthy:       true,
			}, nil
		},
		install: func(
			ctx context.Context,
		) (dependency.InstallResult, error) {
			t.Fatal("fresh install must not run for a healthy service")
			return dependency.InstallResult{}, nil
		},
	}

	result, err := reconciler.Install(context.Background())
	if err != nil {
		t.Fatalf("reconciler.Install() error = %v", err)
	}
	if !result.Reused {
		t.Fatalf("reconciler.Install() reused = false, want true")
	}
}

func TestReconcilerRunsFreshInstallWhenServiceIsAbsent(t *testing.T) {
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
				Name: "node-exporter",
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
		t.Fatal("reconciler.Install() reused = true, want false")
	}
}

func TestReconcilerDoesNotOverwriteUnhealthyService(t *testing.T) {
	installCalls := 0

	reconciler := reconciler{
		inspect: func(
			ctx context.Context,
		) (installationState, error) {
			return installationState{
				ServiceExists: true,
				Healthy:       false,
			}, nil
		},
		install: func(
			ctx context.Context,
		) (dependency.InstallResult, error) {
			installCalls++
			return dependency.InstallResult{}, nil
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
		t.Fatalf(
			"fresh install calls = %d, want 0 for unhealthy existing service",
			installCalls,
		)
	}
}
