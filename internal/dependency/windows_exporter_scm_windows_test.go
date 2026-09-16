//go:build windows

package dependency

import (
	"context"
	"testing"
	"time"
)

func TestInspectWindowsExporterInstallationReadsCurrentServiceState(
	t *testing.T,
) {
	elevated, err := isWindowsAdministrator()
	if err != nil {
		t.Fatalf("isWindowsAdministrator() error = %v", err)
	}
	if !elevated {
		t.Skip("requires an Administrator terminal to inspect Windows services")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err = inspectWindowsExporterInstallation(
		ctx,
		"http://127.0.0.1:9182/health",
	)
	if err != nil {
		t.Fatalf("inspectWindowsExporterInstallation() error = %v", err)
	}
}
