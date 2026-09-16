package dependency

import (
	"context"
	"strings"
	"testing"
)

func TestManagedWindowsExporterInstallerStopsBeforeReconciliationWithoutAdmin(
	t *testing.T,
) {
	reconcileCalls := 0

	installer := managedWindowsExporterInstaller{
		administrator: func() (bool, error) {
			return false, nil
		},
		reconcile: func(context.Context) (InstallResult, error) {
			reconcileCalls++
			return InstallResult{}, nil
		},
	}

	_, err := installer.Install(context.Background())

	if err == nil || !strings.Contains(err.Error(), "Administrator") {
		t.Fatalf("Install() error = %v, want Administrator error", err)
	}
	if reconcileCalls != 0 {
		t.Fatalf("reconcile calls = %d, want 0", reconcileCalls)
	}
}
