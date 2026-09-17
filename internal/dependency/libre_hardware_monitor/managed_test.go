package librehardwaremonitor

import (
	"context"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

func TestManagedInstallerStopsBeforeReconciliationWithoutAdmin(
	t *testing.T,
) {
	reconcileCalls := 0

	installer := managedInstaller{
		administrator: func() (bool, error) {
			return false, nil
		},
		reconcile: func(
			ctx context.Context,
		) (dependency.InstallResult, error) {
			reconcileCalls++

			return dependency.InstallResult{}, nil
		},
	}

	_, err := installer.Install(context.Background())

	if err == nil {
		t.Fatal("managedInstaller.Install() error = nil, want admin error")
	}
	if !strings.Contains(err.Error(), "Administrator") {
		t.Fatalf(
			"managedInstaller.Install() error = %v, want Administrator error",
			err,
		)
	}
	if reconcileCalls != 0 {
		t.Fatalf(
			"reconcile calls = %d, want 0 before admin preflight",
			reconcileCalls,
		)
	}
}
