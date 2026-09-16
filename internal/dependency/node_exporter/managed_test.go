package nodeexporter

import (
	"context"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

func TestManagedInstallerStopsBeforeReconciliationWithoutRoot(
	t *testing.T,
) {
	reconcileCalls := 0

	installer := managedInstaller{
		effectiveUserID: func() int {
			return 1000
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
		t.Fatal("Install() error = nil, want root privilege error")
	}
	if !strings.Contains(err.Error(), "root") {
		t.Fatalf("Install() error = %v, want root privilege error", err)
	}
	if reconcileCalls != 0 {
		t.Fatalf(
			"reconciliation calls = %d, want 0 without root",
			reconcileCalls,
		)
	}
}
